package board

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// dial 建一条到主板的 SSH 连接。
//
// 这段和 netcfg/ssh.go 几乎一样。模块之间不允许互相引用，共享逻辑要么下沉共享层、
// 要么各写一份；现在只有两个使用方，而 board 还要在这条连接上再叠一个 SFTP 客户端，
// 形态未必长期一致，所以先各写一份，出现第三个使用方再上提。
func dial(d Device, timeout time.Duration) (*ssh.Client, error) {
	host := strings.TrimSpace(d.Host)
	if host == "" {
		return nil, errors.New("请填写主板地址")
	}
	user := strings.TrimSpace(d.User)
	if user == "" {
		return nil, errors.New("请填写登录用户名")
	}
	port := d.Port
	if port == 0 {
		port = defaultPort
	}

	auth, err := authMethods(d)
	if err != nil {
		return nil, err
	}

	cfg := &ssh.ClientConfig{
		User: user,
		Auth: auth,
		// 内网嵌入式设备通常没有稳定的主机密钥，而且它的 IP 可能刚被 netcfg 改过。
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	client, err := dialWithin(addr, cfg, timeout)
	if err != nil {
		return nil, fmt.Errorf("连接 %s 失败: %w", addr, err)
	}
	return client, nil
}

func authMethods(d Device) ([]ssh.AuthMethod, error) {
	var auth []ssh.AuthMethod
	if path := strings.TrimSpace(d.KeyPath); path != "" {
		signer, err := loadSigner(path, d.Password)
		if err != nil {
			return nil, err
		}
		auth = append(auth, ssh.PublicKeys(signer))
	}
	auth = append(auth,
		ssh.Password(d.Password),
		ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
			answers := make([]string, len(questions))
			for i := range answers {
				answers[i] = d.Password
			}
			return answers, nil
		}),
	)
	return auth, nil
}

func loadSigner(path, passphrase string) (ssh.Signer, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取密钥失败: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(raw)
	if err == nil {
		return signer, nil
	}
	var missing *ssh.PassphraseMissingError
	if errors.As(err, &missing) || strings.Contains(err.Error(), "passphrase") {
		if passphrase == "" {
			return nil, errors.New("密钥受密码保护，请在密码框里填密钥口令")
		}
		signer, err = ssh.ParsePrivateKeyWithPassphrase(raw, []byte(passphrase))
		if err != nil {
			return nil, fmt.Errorf("密钥口令不对或密钥无法解析: %w", err)
		}
		return signer, nil
	}
	return nil, fmt.Errorf("解析密钥失败: %w", err)
}

// dialWithin 在 timeout 内走完 TCP 建连 + SSH 握手 + 认证，超时就放弃。
//
// 不能直接用 ssh.Dial：ClientConfig.Timeout 只约束 TCP 建连那一段，握手和认证不在其内。
// 对着一个接受 TCP 连接却不说 SSH 协议的地址，ssh.Dial 会在握手上一直等下去，
// 界面就永远停在「连接中…」。
//
// timeout 是这三步加起来的总预算，不是每步各给一份。
func dialWithin(addr string, cfg *ssh.ClientConfig, timeout time.Duration) (*ssh.Client, error) {
	deadline := time.Now().Add(timeout)

	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, err
	}
	if err := conn.SetDeadline(deadline); err != nil {
		conn.Close()
		return nil, err
	}

	c, chans, reqs, err := ssh.NewClientConn(conn, addr, cfg)
	if err != nil {
		conn.Close()
		return nil, err
	}

	// 握手完成后必须撤掉 deadline。它是设在连接上的，留着的话之后每条命令的读写
	// 都会在这个时间点上一起失败，而那时早已不是「连接超时」该管的事了。
	if err := conn.SetDeadline(time.Time{}); err != nil {
		conn.Close()
		return nil, err
	}
	return ssh.NewClient(c, chans, reqs), nil
}

// CommandResult 是一条指令跑完的结果。
//
// 退出码非 0 不算 Go 层面的错误，而是 Success 为 false：那是设备如实回答了
// 「这条命令失败了」，属于要显示给人看的正常结果，不是调用出了问题。
type CommandResult struct {
	Command string `json:"command"`
	Stdout  string `json:"stdout"`
	Stderr  string `json:"stderr"`
	Success bool   `json:"success"`
	// Error 是退出码非 0 时的简短说明，成功时为空。
	Error string `json:"error"`
}

// run 开一个 session 跑一条命令。SSH 协议里一条 session 只能跑一条命令，
// 但连接可以复用，所以这里只开 session、不重新建连。
//
// 超时到了就把 session 关掉——Run 会因此返回，goroutine 不会漏。不动整条连接：
// 一条命令跑挂不代表连接坏了，把连接拆了反而让后面每个按钮都要重连。
func run(c *ssh.Client, cmd string, timeout time.Duration) (CommandResult, error) {
	res := CommandResult{Command: cmd}

	sess, err := c.NewSession()
	if err != nil {
		return res, fmt.Errorf("打开会话失败: %w", err)
	}
	defer sess.Close()

	var stdout, stderr bytes.Buffer
	sess.Stdout = &stdout
	sess.Stderr = &stderr

	done := make(chan error, 1)
	go func() { done <- sess.Run(cmd) }()

	var runErr error
	select {
	case runErr = <-done:
	case <-time.After(timeout):
		sess.Close()
		<-done // 等那个 goroutine 退掉，别让它悬着写 buffer
		res.Stdout, res.Stderr = stdout.String(), stderr.String()
		return res, fmt.Errorf("命令超过 %s 还没结束，已放弃等待（设备上它可能还在跑）", timeout)
	}

	res.Stdout, res.Stderr = stdout.String(), stderr.String()

	if runErr == nil {
		res.Success = true
		return res, nil
	}
	// 退出码非 0：命令本身失败了，如实报回去而不是当调用错误。
	var exitErr *ssh.ExitError
	if errors.As(runErr, &exitErr) {
		res.Error = fmt.Sprintf("命令退出码 %d", exitErr.ExitStatus())
		return res, nil
	}
	// 走到这儿是传输层的问题：连接断了、session 开不出来。
	return res, runErr
}

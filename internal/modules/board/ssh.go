package board

import (
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

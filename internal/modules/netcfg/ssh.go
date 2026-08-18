package netcfg

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

func dial(d Device) (*ssh.Client, error) {
	if strings.TrimSpace(d.Host) == "" {
		return nil, errors.New("请填写设备地址")
	}
	if strings.TrimSpace(d.User) == "" {
		return nil, errors.New("请填写登录用户名")
	}
	port := d.Port
	if port == 0 {
		port = 22
	}
	timeout := time.Duration(loadSettings().ConnectTimeoutSeconds) * time.Second

	cfg := &ssh.ClientConfig{
		User: d.User,
		Auth: []ssh.AuthMethod{
			ssh.Password(d.Password),
			// dropbear 等轻量 SSH 服务常只开 keyboard-interactive。
			ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range answers {
					answers[i] = d.Password
				}
				return answers, nil
			}),
		},
		// 内网嵌入式设备通常没有稳定的主机密钥，且它的 IP 正要被本工具改掉。
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}

	client, err := dialWithin(net.JoinHostPort(d.Host, strconv.Itoa(port)), cfg, timeout)
	if err != nil {
		return nil, fmt.Errorf("连接 %s:%d 失败: %w", d.Host, port, err)
	}
	return client, nil
}

// dialWithin 在 timeout 内走完 TCP 建连 + SSH 握手 + 认证，超时就放弃。
//
// 不能直接用 ssh.Dial：ClientConfig.Timeout 只约束 TCP 建连那一段，握手和认证不在
// 其内。对着一个接受 TCP 连接却不说 SSH 协议的地址（透明代理、门户设备、端口被别的
// 服务占用），ssh.Dial 会在握手上一直等下去——实测过 40 秒还没返回，界面就一直卡在
// 「连接中…」。
//
// timeout 是这三步加起来的总预算，不是每步各给一份，这样配置里写 3 秒就真的是 3 秒。
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
	// 都会在这个时间点上一起失败，而那时早已不是"连接超时"该管的事了。
	if err := conn.SetDeadline(time.Time{}); err != nil {
		conn.Close()
		return nil, err
	}
	return ssh.NewClient(c, chans, reqs), nil
}

func run(c *ssh.Client, cmd string) (string, error) {
	sess, err := c.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()

	var stdout, stderr bytes.Buffer
	sess.Stdout = &stdout
	sess.Stderr = &stderr

	if err := sess.Run(cmd); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return stdout.String(), fmt.Errorf("设备执行命令失败: %s", msg)
		}
		return stdout.String(), fmt.Errorf("设备执行命令失败: %w", err)
	}
	return stdout.String(), nil
}

// quote 把字符串包成一个 sh 单引号参数。
func quote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// writeRemoteFile 通过 stdin 把内容写进设备文件。内容先归一成 LF：
// Windows 签出的脚本带 CRLF 时，/bin/sh\r 会让设备报 "not found"。
func writeRemoteFile(c *ssh.Client, path string, content []byte) error {
	content = bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))

	sess, err := c.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()

	sess.Stdin = bytes.NewReader(content)
	cmd := fmt.Sprintf("cat > %s && chmod +x %s", quote(path), quote(path))
	if out, err := sess.CombinedOutput(cmd); err != nil {
		return fmt.Errorf("设备执行命令失败: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

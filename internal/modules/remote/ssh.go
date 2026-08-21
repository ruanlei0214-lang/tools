package remote

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"embedtools/internal/module"
)

// rebootCommand 是下发给控制器的重启指令。按需求它不显示在界面上。
const rebootCommand = "reboot -f"

// RebootController 重启控制器。机器人不在未使能状态时拒绝——带着使能重启，
// 正在动的轴会失控。状态走 WS 订阅确认，重启指令走 SSH：远程接口文档里没有
// 重启，reboot 是系统命令，两条路各管一段。
func (s *Service) RebootController() error {
	st, err := s.GetRobotStatus()
	if err != nil {
		return fmt.Errorf("重启前确认机器人状态失败，已放弃重启：%w", err)
	}
	if err := checkRebootAllowed(st); err != nil {
		return err
	}

	// 地址跟共享配置走（顶栏改的就是它），共享里没有才退回本模块的出厂默认。
	host := module.LoadShared().Host
	if host == "" {
		host = s.snapshot().Device.Host
	}
	return rebootViaSSH(host, s.connectTimeout())
}

// rebootViaSSH 把重启指令交到设备上的 shell。成功标准是「指令送达」而不是
// 「命令跑完」：reboot -f 会立刻把 sshd 一起带走，等命令正常结束只会等到一个
// 连接断开的错。
func rebootViaSSH(host string, timeout time.Duration) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return errors.New("没有设备地址，无法重启")
	}

	shared := module.LoadShared()
	user := shared.User
	if user == "" {
		user = "root"
	}

	cfg := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(shared.Password),
			// dropbear 等轻量 SSH 服务常只开 keyboard-interactive。
			ssh.KeyboardInteractive(func(_, _ string, questions []string, _ []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range answers {
					answers[i] = shared.Password
				}
				return answers, nil
			}),
		},
		// 内网嵌入式设备没有稳定的主机密钥。
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}

	client, err := dialSSHWithin(net.JoinHostPort(host, strconv.Itoa(22)), cfg, timeout)
	if err != nil {
		return fmt.Errorf("SSH 连接 %s 失败：%w", host, err)
	}
	defer client.Close()

	sess, err := client.NewSession()
	if err != nil {
		return err
	}
	defer sess.Close()
	if err := sess.Start(rebootCommand); err != nil {
		return fmt.Errorf("下发重启指令失败：%w", err)
	}
	return nil
}

// dialSSHWithin 在 timeout 内走完 TCP 建连 + SSH 握手 + 认证，超时就放弃。
// 不能直接用 ssh.Dial：ClientConfig.Timeout 只约束 TCP 建连，握手和认证不在其内，
// 对着一个接受 TCP 却不说 SSH 的地址会一直等下去。与 netcfg 的 dialWithin 同源。
func dialSSHWithin(addr string, cfg *ssh.ClientConfig, timeout time.Duration) (*ssh.Client, error) {
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

	// 握手完成后必须撤掉 deadline：它是设在连接上的，留着的话之后的读写
	// 都会在这个时间点上一起失败。
	if err := conn.SetDeadline(time.Time{}); err != nil {
		conn.Close()
		return nil, err
	}
	return ssh.NewClient(c, chans, reqs), nil
}

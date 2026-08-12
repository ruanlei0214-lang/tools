package netcfg

import (
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// silentServer 起一个只接受 TCP 连接、一个字节都不发的监听，模拟"端口开着但不是
// SSH 服务"——透明代理、门户设备、被别的程序占用的 22 端口都长这样。
//
// 这正是 ssh.Dial 会无限期卡住的场景：TCP 建连成功，然后等对方发 SSH 版本号，
// 而 ClientConfig.Timeout 管不到这一段。
func silentServer(t *testing.T) string {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("起监听失败：%v", err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			// 收下就晾着，什么都不回。连接由 Cleanup 里关监听时一并释放。
			t.Cleanup(func() { conn.Close() })
		}
	}()
	return ln.Addr().String()
}

// TestDialWithinTimesOutOnSilentServer 是这个 bug 的回归测试：
// 之前对着这样的地址会一直等下去，界面永远停在「连接中…」。
//
// 用显式的短超时而不是配置里的值：测试不该因为有人把 connectTimeoutSeconds
// 调到 120 就跑上两分钟。
func TestDialWithinTimesOutOnSilentServer(t *testing.T) {
	addr := silentServer(t)
	const timeout = 300 * time.Millisecond

	cfg := &ssh.ClientConfig{
		User:            "root",
		Auth:            []ssh.AuthMethod{ssh.Password("")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}

	start := time.Now()
	client, err := dialWithin(addr, cfg, timeout)
	elapsed := time.Since(start)

	if err == nil {
		client.Close()
		t.Fatal("对着一个不说 SSH 的端口居然连成功了")
	}
	// 留一倍余量给调度抖动；关键是它必须回来，而不是卡死。
	if elapsed > 3*timeout {
		t.Errorf("耗时 %v，超过预算 %v 太多——握手那段没有被 deadline 约束住", elapsed, timeout)
	}
	t.Logf("耗时 %v，err = %v", elapsed.Round(time.Millisecond), err)
}

// 连不上的地址要在预算内返回，而不是靠操作系统的 TCP 重传自己放弃。
func TestDialWithinTimesOutOnUnroutableHost(t *testing.T) {
	const timeout = 300 * time.Millisecond
	cfg := &ssh.ClientConfig{
		User:            "root",
		Auth:            []ssh.AuthMethod{ssh.Password("")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         timeout,
	}

	start := time.Now()
	if _, err := dialWithin("192.0.2.1:22", cfg, timeout); err == nil {
		t.Fatal("保留地址 192.0.2.1 居然连上了")
	}
	if elapsed := time.Since(start); elapsed > 3*timeout {
		t.Errorf("耗时 %v，超过预算 %v 太多", elapsed, timeout)
	}
}

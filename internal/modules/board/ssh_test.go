package board

import (
	"net"
	"strconv"
	"strings"
	"testing"
	"time"
)

// silentServer 起一个只接受 TCP 连接、一个字节都不发的监听，模拟「端口开着但不是
// SSH 服务」——透明代理、门户设备、被别的程序占用的 22 端口都长这样。
//
// 这正是 ssh.Dial 会无限期卡住的场景：TCP 建连成功，然后等对方发 SSH 版本号，
// 而 ClientConfig.Timeout 管不到这一段。
func silentServer(t *testing.T) (string, int) {
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
			// 收下就晾着，什么都不回。
			t.Cleanup(func() { conn.Close() })
		}
	}()

	host, portStr, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatal(err)
	}
	return host, port
}

// 对着一个不说 SSH 协议的端口，dial 必须在预算内返回，而不是让界面停在「连接中…」。
// 用显式的短超时而不是配置里的值：测试不该因为有人把 connectTimeoutSeconds 调到 120
// 就跑上两分钟。
func TestDialTimesOutOnSilentServer(t *testing.T) {
	host, port := silentServer(t)
	const timeout = 300 * time.Millisecond

	start := time.Now()
	client, err := dial(Device{Host: host, Port: port, User: "root"}, timeout)
	elapsed := time.Since(start)

	if err == nil {
		client.Close()
		t.Fatal("对着一个不说 SSH 的端口居然连成功了")
	}
	// 留一倍余量给调度抖动；关键是它必须回来，而不是卡死。
	if elapsed > 3*timeout {
		t.Errorf("耗时 %v，超过预算 %v 太多——握手那段没有被 deadline 约束住", elapsed, timeout)
	}
}

// 连不上的地址要在预算内返回，而不是靠操作系统的 TCP 重传自己放弃。
func TestDialTimesOutOnUnroutableHost(t *testing.T) {
	const timeout = 300 * time.Millisecond

	start := time.Now()
	if _, err := dial(Device{Host: "192.0.2.1", Port: 22, User: "root"}, timeout); err == nil {
		t.Fatal("保留地址 192.0.2.1 居然连上了")
	}
	if elapsed := time.Since(start); elapsed > 3*timeout {
		t.Errorf("耗时 %v，超过预算 %v 太多", elapsed, timeout)
	}
}

// 地址和用户名为空要当场说清缺什么，而不是拿一个空地址去连然后报网络错误。
func TestDialRejectsMissingFields(t *testing.T) {
	cases := []struct {
		name string
		dev  Device
		want string
	}{
		{"没有地址", Device{User: "root"}, "地址"},
		{"没有用户名", Device{Host: "10.0.0.2"}, "用户名"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := dial(c.dev, time.Second)
			if err == nil {
				t.Fatal("应当报错")
			}
			if got := err.Error(); !strings.Contains(got, c.want) {
				t.Fatalf("err=%q，期望提到 %q", got, c.want)
			}
		})
	}
}

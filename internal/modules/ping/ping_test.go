package ping

import (
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestAppendDropsStaleGeneration(t *testing.T) {
	s := &Service{}
	s.gen = 2
	s.append(1, "旧代次的行")
	if len(s.lines) != 0 {
		t.Fatal("旧代次的行应被丢弃")
	}
	s.append(2, "当前代次的行")
	if len(s.lines) != 1 {
		t.Fatal("当前代次的行应入缓冲区")
	}
}

func TestRestartPingDoesNotGetClobbered(t *testing.T) {
	s := &Service{}
	// 连着开两次：第一次的 goroutine 收尾时不能清掉第二次的 running。
	if err := s.StartPing("127.0.0.1"); err != nil {
		t.Fatalf("StartPing 失败: %v", err)
	}
	if err := s.StartPing("127.0.0.1"); err != nil {
		t.Fatalf("第二次 StartPing 失败: %v", err)
	}
	s.StopPing()
	// 等 goroutine 收尾（loopback 的 echo 是即时的，200ms 足够）。
	time.Sleep(200 * time.Millisecond)
	log, err := s.ReadPing()
	if err != nil {
		t.Fatalf("ReadPing 失败: %v", err)
	}
	if log.Running {
		t.Fatal("停止后 Running 应为 false——旧 goroutine 的收尾可能清错了状态")
	}
}

func TestParseSegmentsShorthand(t *testing.T) {
	addrs, err := parseSegments("192.168.1")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(addrs) != 254 {
		t.Fatalf("192.168.1 应展开为 254 个地址，得到 %d", len(addrs))
	}
	if addrs[0].String() != "192.168.1.1" || addrs[len(addrs)-1].String() != "192.168.1.254" {
		t.Fatalf("应去掉网络号和广播号，得到 %s … %s", addrs[0], addrs[len(addrs)-1])
	}
}

func TestParseSegmentsMixedAndDedup(t *testing.T) {
	addrs, err := parseSegments("192.168.1.10, 192.168.1.10\n192.168.1.11；10.0.0.0/30")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	// 10.0.0.0/30 去掉一头一尾剩 2 个，加上去重后的 2 个单 IP。
	if len(addrs) != 4 {
		t.Fatalf("应得到 4 个地址，得到 %d: %v", len(addrs), addrs)
	}
}

func TestParseSegmentsKeepsSlash31And32(t *testing.T) {
	for _, cidr := range []string{"10.0.0.0/31", "10.0.0.1/32"} {
		addrs, err := parseSegments(cidr)
		if err != nil {
			t.Fatalf("%s 解析失败: %v", cidr, err)
		}
		want := 2
		if strings.HasSuffix(cidr, "/32") {
			want = 1
		}
		if len(addrs) != want {
			t.Fatalf("%s 应保留 %d 个地址，得到 %d", cidr, want, len(addrs))
		}
	}
}

func TestParseSegmentsRejectsGarbage(t *testing.T) {
	for _, input := range []string{"", "abc", "192.168.1.0/33", "192.168.1.x"} {
		if _, err := parseSegments(input); err == nil {
			t.Errorf("%q 应当报错", input)
		}
	}
}

func TestParseSegmentsCapsTotal(t *testing.T) {
	// /16 有 65534 个地址，远超上限。
	if _, err := parseSegments("10.0.0.0/16"); err == nil {
		t.Fatal("超过上限应当报错")
	}
}

func TestParseSegmentsRange(t *testing.T) {
	addrs, err := parseSegments("192.168.1.10-12")
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(addrs) != 3 || addrs[0].String() != "192.168.1.10" || addrs[2].String() != "192.168.1.12" {
		t.Fatalf("192.168.1.10-12 应展开为 .10 .11 .12，得到 %v", addrs)
	}
}

func TestParseSegmentsRejectsBadRange(t *testing.T) {
	for _, input := range []string{"192.168.1.100-10", "192.168.1.1-300", "192.168.1.1-"} {
		if _, err := parseSegments(input); err == nil {
			t.Errorf("%q 应当报错", input)
		}
	}
}

func TestFindConflictsDetectsIPConflict(t *testing.T) {
	alive := []pinged{{addr: netip.MustParseAddr("10.0.0.5")}}
	samples := []map[string]string{
		{"10.0.0.5": "AA-AA-AA-AA-AA-AA"},
		{"10.0.0.5": "BB-BB-BB-BB-BB-BB"},
		{"10.0.0.5": "BB-BB-BB-BB-BB-BB"},
	}
	cs := findConflicts(samples, alive)
	if len(cs) != 1 || cs[0].Kind != "ip" || cs[0].Addr != "10.0.0.5" {
		t.Fatalf("应报出 10.0.0.5 的 IP 冲突，得到 %+v", cs)
	}
	if len(cs[0].Peers) != 2 {
		t.Fatalf("应列出两个 MAC，得到 %v", cs[0].Peers)
	}
}

func TestFindConflictsDetectsMACConflict(t *testing.T) {
	alive := []pinged{
		{addr: netip.MustParseAddr("10.0.0.5")},
		{addr: netip.MustParseAddr("10.0.0.6")},
	}
	same := map[string]string{
		"10.0.0.5": "AA-AA-AA-AA-AA-AA",
		"10.0.0.6": "AA-AA-AA-AA-AA-AA",
	}
	cs := findConflicts([]map[string]string{same, same, same}, alive)
	if len(cs) != 1 || cs[0].Kind != "mac" || cs[0].Addr != "AA-AA-AA-AA-AA-AA" {
		t.Fatalf("应报出 MAC 冲突，得到 %+v", cs)
	}
	if len(cs[0].Peers) != 2 || cs[0].Peers[0] != "10.0.0.5" || cs[0].Peers[1] != "10.0.0.6" {
		t.Fatalf("应列出两个 IP，得到 %v", cs[0].Peers)
	}
}

func TestFindConflictsIgnoresUnrelatedEntries(t *testing.T) {
	alive := []pinged{{addr: netip.MustParseAddr("10.0.0.5")}}
	samples := []map[string]string{
		{"10.0.0.5": "AA-AA-AA-AA-AA-AA", "10.0.0.99": "11-11-11-11-11-11"},
		{"10.0.0.5": "AA-AA-AA-AA-AA-AA", "10.0.0.99": "22-22-22-22-22-22"},
		{"10.0.0.5": "AA-AA-AA-AA-AA-AA", "10.0.0.99": "22-22-22-22-22-22"},
	}
	// 10.0.0.99 不在本次扫描的在线列表里，它的 MAC 翻动与本次扫描无关。
	if cs := findConflicts(samples, alive); len(cs) != 0 {
		t.Fatalf("不应报冲突，得到 %+v", cs)
	}
}

func TestParseArp(t *testing.T) {
	// 中文 Windows 的 arp -a 输出。
	output := `
接口: 192.168.1.100 --- 0x6
  Internet 地址         物理地址              类型
  192.168.1.1           00-50-56-c0-00-08     动态
  192.168.1.136         3c-52-82-11-22-33     动态
  224.0.0.22            01-00-5e-00-00-16     静态
`
	macs := parseArp(output)
	if macs["192.168.1.1"] != "00-50-56-C0-00-08" {
		t.Errorf("192.168.1.1 的 MAC 解析错误: %q", macs["192.168.1.1"])
	}
	if macs["192.168.1.136"] != "3C-52-82-11-22-33" {
		t.Errorf("192.168.1.136 的 MAC 应统一为大写: %q", macs["192.168.1.136"])
	}
	if _, ok := macs["192.168.1.100"]; ok {
		t.Error("接口行不应被当成条目")
	}
}

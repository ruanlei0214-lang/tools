package netcfg

import "testing"

const addrOutput = `1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN qlen 1000
    link/loopback 00:00:00:00:00:00 brd 00:00:00:00:00:00
    inet 127.0.0.1/8 scope host lo
       valid_lft forever preferred_lft forever
2: eth0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc pfifo_fast state UP qlen 1000
    link/ether 00:0c:29:aa:bb:cc brd ff:ff:ff:ff:ff:ff
    inet 192.168.3.136/24 brd 192.168.3.255 scope global eth0
       valid_lft forever preferred_lft forever
3: eth1: <BROADCAST,MULTICAST> mtu 1500 qdisc noop state DOWN qlen 1000
    link/ether 00:0c:29:dd:ee:ff brd ff:ff:ff:ff:ff:ff
`

const routeOutput = `default via 192.168.3.1 dev eth0
192.168.3.0/24 dev eth0 scope link  src 192.168.3.136
`

func TestParseInterfaces(t *testing.T) {
	got := parseInterfaces(addrOutput, parseDefaultGateways(routeOutput))

	want := []Iface{
		{Name: "eth0", MAC: "00:0c:29:aa:bb:cc", Up: true, IP: "192.168.3.136", Mask: "255.255.255.0", Gateway: "192.168.3.1"},
		{Name: "eth1", MAC: "00:0c:29:dd:ee:ff", Up: false},
	}
	if len(got) != len(want) {
		t.Fatalf("网口数量 = %d, 期望 %d (%+v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("网口[%d] = %+v, 期望 %+v", i, got[i], want[i])
		}
	}
}

func TestParseInterfacesHandlesVethSuffix(t *testing.T) {
	out := "9: veth0@if8: <BROADCAST,UP> mtu 1500 qdisc noqueue\n"
	got := parseInterfaces(out, nil)
	if len(got) != 1 || got[0].Name != "veth0" {
		t.Fatalf("解析 veth0@if8 得到 %+v", got)
	}
}

func TestParseInterfacesMaster(t *testing.T) {
	out := `2: br0: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue state UP qlen 1000
    inet 192.168.1.136/24 brd 192.168.1.255 scope global br0
3: lan1: <BROADCAST,MULTICAST,UP,LOWER_UP> mtu 1500 qdisc noqueue master br0 state UP qlen 1000
4: wlan0: <BROADCAST,MULTICAST> mtu 1500 qdisc noop master br0 state DOWN qlen 1000
`
	got := parseInterfaces(out, nil)
	byName := make(map[string]Iface, len(got))
	for _, i := range got {
		byName[i.Name] = i
	}
	if byName["lan1"].Master != "br0" || byName["wlan0"].Master != "br0" {
		t.Errorf("桥成员的 Master 应当是 br0，实际 lan1=%q wlan0=%q", byName["lan1"].Master, byName["wlan0"].Master)
	}
	if byName["br0"].Master != "" {
		t.Errorf("桥自己的 Master 应当为空，实际 %q", byName["br0"].Master)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name       string
		cfg        Config
		wantPrefix int
		wantErr    bool
	}{
		{"正常配置", Config{Iface: "eth0", IP: "192.168.3.10", Mask: "255.255.255.0", Gateway: "192.168.3.1"}, 24, false},
		{"网关留空", Config{Iface: "eth0", IP: "192.168.3.10", Mask: "255.255.0.0"}, 16, false},
		{"网关跨网段", Config{Iface: "eth0", IP: "192.168.3.10", Mask: "255.255.255.0", Gateway: "10.0.0.1"}, 0, true},
		{"掩码不连续", Config{Iface: "eth0", IP: "192.168.3.10", Mask: "255.0.255.0"}, 0, true},
		{"IP 非法", Config{Iface: "eth0", IP: "192.168.3.999", Mask: "255.255.255.0"}, 0, true},
		{"网口名带分号", Config{Iface: "eth0; reboot", IP: "192.168.3.10", Mask: "255.255.255.0"}, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix, err := validate(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if err == nil && prefix != tt.wantPrefix {
				t.Errorf("prefix = %d, 期望 %d", prefix, tt.wantPrefix)
			}
		})
	}
}

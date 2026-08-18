package netcfg

import "testing"

// ifaceList 造一批系统网口，只关心名字和地址。
func ifaceList(names ...string) []Iface {
	out := make([]Iface, 0, len(names))
	for i, n := range names {
		out = append(out, Iface{
			Name: n,
			Up:   true,
			IP:   "192.168.1." + string(rune('1'+i)),
			Mask: "255.255.255.0",
			MAC:  "aa:bb:cc:dd:ee:0" + string(rune('0'+i)),
		})
	}
	return out
}

func portByName(ports []Port, name string) Port {
	for _, p := range ports {
		if p.Name == name {
			return p
		}
	}
	return Port{}
}

// 面板上永远是这五个口，顺序固定——现场是照着丝印找行的。wlan 没有丝印，另算。
func TestBuildPortsAlwaysFivePortsInPanelOrder(t *testing.T) {
	want := []string{"lan1", "lan2", "lan3", "lan4", "lan5"}

	for _, ifaces := range [][]Iface{
		ifaceList("br0", "lan3"),
		ifaceList("lan1", "lan3", "lan4"),
		nil, // 一个网口都没读到
	} {
		ports := buildPorts(ifaces)
		if len(ports) != len(want) {
			t.Fatalf("面板口数 = %d, 期望 %d", len(ports), len(want))
		}
		for i, w := range want {
			if ports[i].Name != w {
				t.Errorf("第 %d 行 = %q, 期望 %q", i, ports[i].Name, w)
			}
		}
	}
}

// 对应关系固定，不随有没有桥变化：lan1/lan2 是系统 lan1，lan4 是系统 lan3，
// lan5 是系统 lan4。有没有 br0 只影响显示谁的信息，不影响对应关系。
func TestBuildPortsFixedMapping(t *testing.T) {
	want := map[string]string{
		"lan1": "lan1",
		"lan2": "lan1",
		"lan3": "",
		"lan4": "lan3",
		"lan5": "lan4",
	}
	for name, ifaces := range map[string][]Iface{
		"无桥": ifaceList("lan1", "lan3", "lan4"),
		"有桥但成员不在桥里": ifaceList("br0", "lan1", "lan3", "lan4"),
	} {
		t.Run(name, func(t *testing.T) {
			for _, p := range buildPorts(ifaces) {
				if got := want[p.Name]; p.Iface != got {
					t.Errorf("面板 %s -> 系统 %q, 期望 %q", p.Name, p.Iface, got)
				}
			}
		})
	}
}

// 系统 lan1 进了 br0 时，面板 lan1/lan2 显示桥的信息，下发也落到桥上。
func TestBuildPortsBridgedPortsShowBridgeInfo(t *testing.T) {
	ports := buildPorts([]Iface{
		{Name: "br0", Up: true, IP: "10.0.0.5", Mask: "255.255.255.0", Gateway: "10.0.0.1", MAC: "aa:bb:cc:dd:ee:ff"},
		{Name: "lan1", Up: true, Master: "br0", MAC: "aa:bb:cc:00:00:01"},
		{Name: "lan3", Up: true, IP: "172.16.0.9", Mask: "255.255.255.0"},
	})

	for _, name := range []string{"lan1", "lan2"} {
		p := portByName(ports, name)
		if p.Iface != "br0" || p.IP != "10.0.0.5" || p.Gateway != "10.0.0.1" || p.MAC != "aa:bb:cc:dd:ee:ff" {
			t.Errorf("面板 %s 应当显示 br0 的信息，实际 %+v", name, p)
		}
	}
	// 同一个系统网口也只有 lan1 可改，lan2 只读。
	if p := portByName(ports, "lan1"); !p.Editable {
		t.Error("面板 lan1 应当可改")
	}
	if p := portByName(ports, "lan2"); p.Editable {
		t.Error("面板 lan2 应当只读")
	}
	// 不在桥里的口照常显示自己的信息。
	if p := portByName(ports, "lan4"); p.Iface != "lan3" || p.IP != "172.16.0.9" || p.Editable {
		t.Errorf("面板 lan4 应当显示系统 lan3 的信息且只读，实际 %+v", p)
	}
}

// 面板 lan3 不归本工具管：任何情况下都不带系统网口，也不带地址。
func TestBuildPortsLan3AlwaysBlank(t *testing.T) {
	for name, ifaces := range map[string][]Iface{
		"有 br0": ifaceList("br0", "lan3"),
		"无 br0": ifaceList("lan1", "lan3", "lan4"),
		// 就算系统里真有一个叫 lan3 的网口，面板 lan3 也不该显示它——
		// 系统 lan3 是面板 lan4 的。
		"系统里存在同名网口": ifaceList("lan3"),
	} {
		t.Run(name, func(t *testing.T) {
			if p := portByName(buildPorts(ifaces), "lan3"); p.Iface != "" || p.IP != "" || p.MAC != "" || p.Up {
				t.Errorf("面板 lan3 应当没有任何信息，实际 %+v", p)
			}
		})
	}
}

// 只有面板 lan1 能改地址，其余只读——包括和它是同一个系统网口的 lan2。
func TestBuildPortsEditable(t *testing.T) {
	want := map[string]bool{"lan1": true, "lan2": false, "lan3": false, "lan4": false, "lan5": false}
	for _, p := range buildPorts(ifaceList("lan1", "lan3", "lan4")) {
		if p.Editable != want[p.Name] {
			t.Errorf("面板 %s（系统 %q）可改 = %v, 期望 %v", p.Name, p.Iface, p.Editable, want[p.Name])
		}
	}
}

// 主网口不存在时，一个面板口都不能改——但表格照样是五行。
func TestBuildPortsNothingEditableWhenMainIfaceMissing(t *testing.T) {
	ports := buildPorts(ifaceList("lan3", "lan4"))

	if len(ports) != 5 {
		t.Fatalf("面板口数 = %d, 期望 5", len(ports))
	}
	for _, p := range ports {
		if p.Editable {
			t.Errorf("系统里没有 lan1，面板 %s 不该可改", p.Name)
		}
	}
}

// 映射表里写了、但设备上没有的系统网口，退化成占位行，而不是拿别的网口顶上。
func TestBuildPortsMissingIfaceBecomesBlank(t *testing.T) {
	ports := buildPorts(ifaceList("lan1"))

	if p := portByName(ports, "lan4"); p.Iface != "" || p.IP != "" {
		t.Errorf("系统里没有 lan3，面板 lan4 应当为空，实际 %+v", p)
	}
	if p := portByName(ports, "lan1"); p.Iface != "lan1" {
		t.Errorf("面板 lan1 应当落在系统 lan1，实际 %q", p.Iface)
	}
}

// 系统里有 wlan 才显示 wlan 行，没有就不占行。
func TestBuildPortsWlanOnlyWhenPresent(t *testing.T) {
	ports := buildPorts(ifaceList("lan1", "lan3", "lan4"))
	if p := portByName(ports, "wlan"); p.Name != "" {
		t.Errorf("系统里没有 wlan，不该出现 wlan 行，实际 %+v", p)
	}

	ports = buildPorts([]Iface{
		{Name: "lan1", Up: true, IP: "192.168.1.2", Mask: "255.255.255.0"},
		{Name: "wlan0", Up: true, IP: "192.168.6.1", Mask: "255.255.255.0", MAC: "aa:bb:cc:00:00:02"},
	})
	if len(ports) != 6 {
		t.Fatalf("有 wlan0 时应当是 6 行，实际 %d", len(ports))
	}
	p := ports[5]
	if p.Name != "wlan" || p.Iface != "wlan0" || p.IP != "192.168.6.1" || p.Editable {
		t.Errorf("wlan 行应当显示 wlan0 的信息且只读，实际 %+v", p)
	}
}

// wlan0 进了桥时，wlan 行也显示桥的信息。
func TestBuildPortsWlanInBridgeShowsBridgeInfo(t *testing.T) {
	ports := buildPorts([]Iface{
		{Name: "br0", Up: true, IP: "10.0.0.5", Mask: "255.255.255.0"},
		{Name: "lan1", Up: true, Master: "br0"},
		{Name: "wlan0", Up: true, Master: "br0"},
	})

	p := portByName(ports, "wlan")
	if p.Iface != "br0" || p.IP != "10.0.0.5" || p.Editable {
		t.Errorf("wlan0 在 br0 里，wlan 行应当显示 br0 的信息且只读，实际 %+v", p)
	}
}

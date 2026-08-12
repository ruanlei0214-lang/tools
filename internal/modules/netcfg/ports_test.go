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

// 面板上永远是这五个口，顺序固定——现场是照着丝印找行的。
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

// 有 br0 时，面板 lan1/lan2/lan5 都落在 br0 上，lan4 落在系统 lan3 上。
func TestBuildPortsWithBridge(t *testing.T) {
	ports := buildPorts(ifaceList("br0", "lan1", "lan3", "lan4"))

	want := map[string]string{
		"lan1": "br0",
		"lan2": "br0",
		"lan3": "",
		"lan4": "lan3",
		"lan5": "br0",
	}
	for _, p := range ports {
		if got := want[p.Name]; p.Iface != got {
			t.Errorf("面板 %s -> 系统 %q, 期望 %q", p.Name, p.Iface, got)
		}
	}
}

// 没有 br0 时换另一套：lan1/lan2 落在系统 lan1，lan5 落在系统 lan4。
func TestBuildPortsWithoutBridge(t *testing.T) {
	ports := buildPorts(ifaceList("lan1", "lan3", "lan4"))

	want := map[string]string{
		"lan1": "lan1",
		"lan2": "lan1",
		"lan3": "",
		"lan4": "lan3",
		"lan5": "lan4",
	}
	for _, p := range ports {
		if got := want[p.Name]; p.Iface != got {
			t.Errorf("面板 %s -> 系统 %q, 期望 %q", p.Name, p.Iface, got)
		}
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
			for _, p := range buildPorts(ifaces) {
				if p.Name != "lan3" {
					continue
				}
				if p.Iface != "" || p.IP != "" || p.MAC != "" || p.Up {
					t.Errorf("面板 lan3 应当没有任何信息，实际 %+v", p)
				}
			}
		})
	}
}

// 共用同一个系统网口的面板口，显示的地址必须一致——它们本来就是同一个网口。
func TestBuildPortsBridgedPortsShareAddress(t *testing.T) {
	ports := buildPorts([]Iface{
		{Name: "br0", Up: true, IP: "10.0.0.5", Mask: "255.255.255.0", Gateway: "10.0.0.1", MAC: "aa:bb:cc:dd:ee:ff"},
	})

	byName := make(map[string]Port, len(ports))
	for _, p := range ports {
		byName[p.Name] = p
	}
	for _, name := range []string{"lan1", "lan2", "lan5"} {
		p := byName[name]
		if p.IP != "10.0.0.5" || p.Gateway != "10.0.0.1" || p.MAC != "aa:bb:cc:dd:ee:ff" {
			t.Errorf("面板 %s 应当显示 br0 的地址，实际 %+v", name, p)
		}
	}
}

// 有 br0 时只有落在 br0 上的面板口能改；面板 lan4（系统 lan3）只读。
func TestBuildPortsEditableWithBridge(t *testing.T) {
	ports := buildPorts(ifaceList("br0", "lan1", "lan3", "lan4"))

	want := map[string]bool{"lan1": true, "lan2": true, "lan3": false, "lan4": false, "lan5": true}
	for _, p := range ports {
		if p.Editable != want[p.Name] {
			t.Errorf("面板 %s（系统 %q）可改 = %v, 期望 %v", p.Name, p.Iface, p.Editable, want[p.Name])
		}
	}
}

// 没有 br0 时只有落在系统 lan1 上的面板口能改；lan4、lan5 有信息但只读。
func TestBuildPortsEditableWithoutBridge(t *testing.T) {
	ports := buildPorts(ifaceList("lan1", "lan3", "lan4"))

	want := map[string]bool{"lan1": true, "lan2": true, "lan3": false, "lan4": false, "lan5": false}
	for _, p := range ports {
		if p.Editable != want[p.Name] {
			t.Errorf("面板 %s（系统 %q）可改 = %v, 期望 %v", p.Name, p.Iface, p.Editable, want[p.Name])
		}
	}
}

// 只读不等于没信息：面板 lan4 不能改，但地址照常显示。
func TestBuildPortsReadOnlyPortsStillShowInfo(t *testing.T) {
	ports := buildPorts([]Iface{
		{Name: "br0", Up: true, IP: "10.0.0.5", Mask: "255.255.255.0"},
		{Name: "lan3", Up: true, IP: "172.16.0.9", Mask: "255.255.0.0", MAC: "aa:bb:cc:00:11:22"},
	})

	for _, p := range ports {
		if p.Name != "lan4" {
			continue
		}
		if p.Editable {
			t.Error("面板 lan4 应当只读")
		}
		if p.Iface != "lan3" || p.IP != "172.16.0.9" || p.MAC != "aa:bb:cc:00:11:22" {
			t.Errorf("面板 lan4 只读但信息要照常显示，实际 %+v", p)
		}
	}
}

// 可改的那个系统网口不存在时，一个面板口都不能改——但表格照样是五行。
func TestBuildPortsNothingEditableWhenMainIfaceMissing(t *testing.T) {
	ports := buildPorts(ifaceList("lan3", "lan4"))

	if len(ports) != 5 {
		t.Fatalf("面板口数 = %d, 期望 5", len(ports))
	}
	for _, p := range ports {
		if p.Editable {
			t.Errorf("系统里没有 br0 也没有 lan1，面板 %s 不该可改", p.Name)
		}
	}
}

// 映射表里写了、但设备上没有的系统网口，退化成占位行，而不是拿别的网口顶上。
func TestBuildPortsMissingIfaceBecomesBlank(t *testing.T) {
	// 有 br0，但系统里没有 lan3 —— 面板 lan4 应当是空的。
	ports := buildPorts(ifaceList("br0"))

	for _, p := range ports {
		switch p.Name {
		case "lan4":
			if p.Iface != "" || p.IP != "" {
				t.Errorf("系统里没有 lan3，面板 lan4 应当为空，实际 %+v", p)
			}
		case "lan1", "lan2", "lan5":
			if p.Iface != bridgeIface {
				t.Errorf("面板 %s 应当落在 %s，实际 %q", p.Name, bridgeIface, p.Iface)
			}
		}
	}
}

package netcfg

import "strings"

// 机柜面板上印的网口名和系统里的网口名对不上，现场只认面板丝印，
// 所以表格按面板来展示，系统名只在下发配置时用。
//
// 这张对应关系是这台设备的物理接线决定的，没法从系统里推出来，只能写死。
// 换机型就要改这里。

// mainIface 是设备主网口的系统名。面板 lan1/lan2 都是它，也只有它允许改地址。
//
// 不复用配置里的 PersistIface：那个字段管的是"改完写不写持久化文件"，
// 和"面板口怎么对应系统口"是两件事。把它们绑在一起的话，谁为了关掉持久化把
// PersistIface 清空，会连带把面板映射也改掉。
const mainIface = "lan1"

// binding 是一个面板口到系统网口的对应。
type binding struct {
	port  string // 面板上印的名字
	iface string // 对应的系统网口；空表示这个口不归本工具管
}

// panelBindings 按面板从左到右的顺序给出对应关系，固定不变：
// 面板 lan1/lan2 都是系统 lan1，lan4 是系统 lan3，lan5 是系统 lan4。
//
// 面板 lan3 恒定不归本工具管，但仍然占一行：面板上有这个口，表格里凭空少一个
// 会让人以为读漏了。
var panelBindings = []binding{
	{"lan1", mainIface},
	{"lan2", mainIface},
	{"lan3", ""},
	{"lan4", "lan3"},
	{"lan5", "lan4"},
}

// Port 是机柜面板上的一个网口，地址信息取自它对应的系统网口；
// 那个网口进了桥时，取桥的信息。
type Port struct {
	// Name 是面板上印的名字，lan1..lan5；wlan 行没有丝印，是系统里有 wlan 才加的。
	Name string `json:"name"`
	// Iface 是实际生效的系统网口名，下发配置时用它。对应网口进了桥时是桥的名字。
	// 空表示这个口不归本工具管，界面上只做占位、不可选中。
	Iface string `json:"iface"`
	// Editable 为假表示这个口只读：信息照常展示，但不能在这里改地址。
	// 和 Iface 为空是两回事——那种是连信息都没有。
	Editable bool   `json:"editable"`
	MAC      string `json:"mac"`
	Up       bool   `json:"up"`
	IP       string `json:"ip"`
	Mask     string `json:"mask"`
	Gateway  string `json:"gateway"`
}

// buildPorts 把系统网口列表翻译成面板网口列表。
//
// 映射表里写了系统网口、但设备上并不存在时，这一行退化成占位行（Iface 置空）。
// 与其猜一个别的网口顶上，不如如实显示"这个口现在读不到"。
func buildPorts(ifaces []Iface) []Port {
	byName := make(map[string]Iface, len(ifaces))
	for _, i := range ifaces {
		byName[i.Name] = i
	}

	// resolve 返回一个系统网口实际要展示的网口：进了桥就显示桥的信息，
	// 地址挂在桥上，成员口自己是空的。
	resolve := func(name string) (Iface, bool) {
		src, ok := byName[name]
		if !ok {
			return Iface{}, false
		}
		if src.Master != "" {
			if br, ok := byName[src.Master]; ok {
				return br, true
			}
		}
		return src, true
	}

	fill := func(p *Port, src Iface) {
		p.Iface = src.Name
		p.MAC, p.Up = src.MAC, src.Up
		p.IP, p.Mask, p.Gateway = src.IP, src.Mask, src.Gateway
	}

	ports := make([]Port, 0, len(panelBindings)+1)
	for _, b := range panelBindings {
		p := Port{Name: b.port}
		if b.iface != "" {
			if src, ok := resolve(b.iface); ok {
				fill(&p, src)
				// 只有面板 lan1 能改地址；lan2 和它是同一个系统网口，能看不能改。
				p.Editable = b.port == "lan1"
			}
		}
		ports = append(ports, p)
	}

	// wlan 没有面板丝印，系统里有才显示，没有不占行。
	for _, i := range ifaces {
		if !strings.HasPrefix(i.Name, "wlan") {
			continue
		}
		p := Port{Name: "wlan"}
		if src, ok := resolve(i.Name); ok {
			fill(&p, src)
		}
		ports = append(ports, p)
		break
	}
	return ports
}

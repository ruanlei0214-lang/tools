package netcfg

// 机柜面板上印的网口名和系统里的网口名对不上，现场只认面板上那五个丝印，
// 所以表格按面板来展示，系统名只在下发配置时用。
//
// 这张对应关系是这台设备的物理接线决定的，没法从系统里推出来，只能写死。
// 换机型就要改这里。

// bridgeIface 和 mainIface 是设备主网口在两种形态下的名字：组了桥就是 br0，
// 没组桥就是系统 lan1。面板 lan1/lan2 落在它上面，也只有它允许改地址。
//
// 不复用配置里的 PersistIface：那个字段管的是"改完写不写持久化文件"，
// 和"面板口怎么对应系统口"是两件事。把它们绑在一起的话，谁为了关掉持久化把
// PersistIface 清空，会连带把面板映射也改掉。
const (
	bridgeIface = "br0"
	mainIface   = "lan1"
)

// editableIface 返回这台设备上唯一允许改地址的系统网口。
//
// 其余网口只读：能看不能改。这条规则和面板对应关系一样，是设备侧定的，
// 不做成配置。
func editableIface(hasBridge bool) string {
	if hasBridge {
		return bridgeIface
	}
	return mainIface
}

// binding 是一个面板口到系统网口的对应。
type binding struct {
	port  string // 面板上印的名字
	iface string // 对应的系统网口；空表示这个口不归本工具管
}

// bindings 按面板从左到右的顺序返回对应关系。
//
// 面板 lan3 恒定不归本工具管，但仍然占一行：面板上有这个口，表格里凭空少一个
// 会让人以为读漏了。
func bindings(hasBridge bool) []binding {
	if hasBridge {
		// 有桥时 lan1/lan2/lan5 是同一个桥上的三个物理口，共用 br0 的地址。
		return []binding{
			{"lan1", bridgeIface},
			{"lan2", bridgeIface},
			{"lan3", ""},
			{"lan4", "lan3"},
			{"lan5", bridgeIface},
		}
	}
	return []binding{
		{"lan1", mainIface},
		{"lan2", mainIface},
		{"lan3", ""},
		{"lan4", "lan3"},
		{"lan5", "lan4"},
	}
}

// Port 是机柜面板上的一个网口，地址信息取自它对应的系统网口。
type Port struct {
	// Name 是面板上印的名字，lan1..lan5。
	Name string `json:"name"`
	// Iface 是对应的系统网口名，下发配置时用它。空表示这个口不归本工具管，
	// 界面上只做占位、不可选中。
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
	_, hasBridge := byName[bridgeIface]
	editable := editableIface(hasBridge)

	ports := make([]Port, 0, 5)
	for _, b := range bindings(hasBridge) {
		p := Port{Name: b.port}
		if src, ok := byName[b.iface]; ok {
			p.Iface = src.Name
			p.Editable = src.Name == editable
			p.MAC, p.Up = src.MAC, src.Up
			p.IP, p.Mask, p.Gateway = src.IP, src.Mask, src.Gateway
		}
		ports = append(ports, p)
	}
	return ports
}

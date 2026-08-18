package netcfg

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

var (
	ifaceHeadRe = regexp.MustCompile(`^\d+:\s+([^:@\s]+)[:@].*<([^>]*)>`)
	masterRe    = regexp.MustCompile(`\bmaster\s+(\S+)`)
	macRe       = regexp.MustCompile(`link/ether\s+([0-9a-fA-F:]{17})`)
	inetRe      = regexp.MustCompile(`inet\s+(\d+\.\d+\.\d+\.\d+)/(\d+)`)
	ifaceNameRe = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,32}$`)
)

// parseInterfaces 解析 `ip addr show` 的输出，回环口不返回。
func parseInterfaces(addrOut string, gateways map[string]string) []Iface {
	var result []Iface
	var cur *Iface

	flush := func() {
		if cur != nil && cur.Name != "lo" {
			cur.Gateway = gateways[cur.Name]
			result = append(result, *cur)
		}
		cur = nil
	}

	for _, line := range strings.Split(addrOut, "\n") {
		if m := ifaceHeadRe.FindStringSubmatch(line); m != nil {
			flush()
			cur = &Iface{Name: m[1], Up: hasFlag(m[2], "UP")}
			// 桥成员的头行里有 "master br0"，地址挂在桥上而不是成员上。
			if mm := masterRe.FindStringSubmatch(line); mm != nil {
				cur.Master = mm[1]
			}
			continue
		}
		if cur == nil {
			continue
		}
		if m := macRe.FindStringSubmatch(line); m != nil {
			cur.MAC = m[1]
		}
		// 一个网口可能有多个地址，这里只展示第一个。
		if m := inetRe.FindStringSubmatch(line); m != nil && cur.IP == "" {
			prefix, err := strconv.Atoi(m[2])
			if err != nil {
				continue
			}
			cur.IP = m[1]
			cur.Mask = prefixToMask(prefix)
		}
	}
	flush()

	return result
}

// parseDefaultGateways 从 `ip route show` 中取出各网口的默认网关。
func parseDefaultGateways(routeOut string) map[string]string {
	gateways := make(map[string]string)
	for _, line := range strings.Split(routeOut, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "default" {
			continue
		}
		var via, dev string
		for i := 0; i < len(fields)-1; i++ {
			switch fields[i] {
			case "via":
				via = fields[i+1]
			case "dev":
				dev = fields[i+1]
			}
		}
		if via != "" && dev != "" {
			gateways[dev] = via
		}
	}
	return gateways
}

func hasFlag(flags, want string) bool {
	for _, f := range strings.Split(flags, ",") {
		if f == want {
			return true
		}
	}
	return false
}

// validate 校验待下发的配置，返回掩码对应的前缀长度。
func validate(cfg Config) (int, error) {
	if !ifaceNameRe.MatchString(cfg.Iface) {
		return 0, fmt.Errorf("网口名称不合法: %q", cfg.Iface)
	}
	ip, err := parseIPv4(cfg.IP, "IP 地址")
	if err != nil {
		return 0, err
	}
	prefix, err := maskToPrefix(cfg.Mask)
	if err != nil {
		return 0, err
	}
	if cfg.Gateway != "" {
		gw, err := parseIPv4(cfg.Gateway, "网关")
		if err != nil {
			return 0, err
		}
		subnet := net.IPNet{IP: ip.Mask(net.CIDRMask(prefix, 32)), Mask: net.CIDRMask(prefix, 32)}
		if !subnet.Contains(gw) {
			return 0, fmt.Errorf("网关 %s 不在 %s 网段内", cfg.Gateway, subnet.String())
		}
	}
	return prefix, nil
}

func parseIPv4(s, label string) (net.IP, error) {
	ip := net.ParseIP(strings.TrimSpace(s))
	if ip == nil || ip.To4() == nil {
		return nil, fmt.Errorf("%s格式错误: %q", label, s)
	}
	return ip.To4(), nil
}

func maskToPrefix(mask string) (int, error) {
	ip, err := parseIPv4(mask, "子网掩码")
	if err != nil {
		return 0, err
	}
	ones, bits := net.IPMask(ip).Size()
	if bits == 0 {
		return 0, fmt.Errorf("子网掩码必须是连续掩码: %q", mask)
	}
	return ones, nil
}

func prefixToMask(prefix int) string {
	if prefix < 0 || prefix > 32 {
		return ""
	}
	return net.IP(net.CIDRMask(prefix, 32)).String()
}

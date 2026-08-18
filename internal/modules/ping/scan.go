package ping

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
)

// ScanHost 是网段里一台在线的设备。Name / MAC 拿不到时留空：跨网段的 MAC
// 本来就拿不到（那是网关的），没有 PTR 记录的设备也没有名字。
type ScanHost struct {
	IP    string  `json:"ip"`
	Name  string  `json:"name"`
	MAC   string  `json:"mac"`
	RttMs float64 `json:"rttMs"`
}

// ScanResult 里 Total 是实际扫过的地址数，ElapsedMs 让界面能说出用时。
type ScanResult struct {
	Hosts     []ScanHost `json:"hosts"`
	Conflicts []Conflict `json:"conflicts"`
	Total     int        `json:"total"`
	ElapsedMs int64      `json:"elapsedMs"`
}

// Conflict 是扫描中发现的一处地址冲突。
// Kind 为 "ip"：同一个 IP 在扫描期间被多个 MAC 应答（Peers 是 MAC 列表）；
// Kind 为 "mac"：同一个 MAC 对应多个在线 IP（Peers 是 IP 列表）。
type Conflict struct {
	Kind  string   `json:"kind"`
	Addr  string   `json:"addr"`
	Peers []string `json:"peers"`
}

const (
	// scanConcurrency 是一波同时探测的地址数，/24 两波就完事。
	scanConcurrency = 128
	scanTimeout     = 500 * time.Millisecond
	// maxScanHosts 挡住 /16 这种一扫一分多钟的输入。
	maxScanHosts = 2048
	dnsTimeout   = 300 * time.Millisecond
)

// Scan 扫 input 里的网段并返回在线设备。input 用逗号、分号、空格或换行分隔多段，
// 每段可以是 CIDR（192.168.1.0/24）、前三段简写（192.168.1）、
// 末段区间（192.168.1.10-100）或单个 IP。
func (s *Service) Scan(input string) (ScanResult, error) {
	addrs, err := parseSegments(input)
	if err != nil {
		return ScanResult{}, err
	}
	start := time.Now()

	// ARP 表采三次：ping 前、ping 后、设备名反查后。同一个 IP 的 MAC 在采样间
	// 变了，就是有不止一台设备在应答它——这是没有管理员权限时抓 IP 冲突
	// 最便宜的办法（原始 ARP 报文要管理员/Npcap，不考虑）。
	samples := []map[string]string{arpTable()}
	alive := pingAll(addrs)
	samples = append(samples, arpTable())
	names := resolveNames(alive)
	samples = append(samples, arpTable())

	macs := samples[len(samples)-1]
	conflicts := findConflicts(samples, alive)

	hosts := make([]ScanHost, 0, len(alive))
	for _, h := range alive {
		ip := h.addr.String()
		hosts = append(hosts, ScanHost{
			IP:    ip,
			Name:  names[ip],
			MAC:   macs[ip],
			RttMs: h.rttMs,
		})
	}
	return ScanResult{
		Hosts:     hosts,
		Conflicts: conflicts,
		Total:     len(addrs),
		ElapsedMs: time.Since(start).Milliseconds(),
	}, nil
}

// findConflicts 汇总几份 ARP 采样里的冲突。只看本次扫到的在线地址，
// 表里别的条目跟这次扫描无关。
func findConflicts(samples []map[string]string, alive []pinged) []Conflict {
	ipMACs := map[string]map[string]bool{}
	for _, sample := range samples {
		for ip, mac := range sample {
			set := ipMACs[ip]
			if set == nil {
				set = map[string]bool{}
				ipMACs[ip] = set
			}
			set[mac] = true
		}
	}

	var out []Conflict
	// alive 已按 IP 排序，遍历它保证冲突列表也是有序的。
	last := samples[len(samples)-1]
	macIPs := map[string][]string{}
	for _, h := range alive {
		ip := h.addr.String()
		if set := ipMACs[ip]; len(set) > 1 {
			peers := make([]string, 0, len(set))
			for mac := range set {
				peers = append(peers, mac)
			}
			sort.Strings(peers)
			out = append(out, Conflict{Kind: "ip", Addr: ip, Peers: peers})
		}
		if mac := last[ip]; mac != "" {
			macIPs[mac] = append(macIPs[mac], ip)
		}
	}
	// 一个 MAC 应答多个在线 IP：可能是 MAC 仿冒，也可能是路由器代理 ARP
	// 或一台设备配了多个地址。报出来，由现场判断。
	macs := make([]string, 0, len(macIPs))
	for mac := range macIPs {
		macs = append(macs, mac)
	}
	sort.Strings(macs)
	for _, mac := range macs {
		if ips := macIPs[mac]; len(ips) > 1 {
			out = append(out, Conflict{Kind: "mac", Addr: mac, Peers: ips})
		}
	}
	return out
}

func parseSegments(input string) ([]netip.Addr, error) {
	seen := map[netip.Addr]bool{}
	var out []netip.Addr
	for _, tok := range strings.FieldsFunc(input, func(r rune) bool {
		return r == ',' || r == ';' || r == '，' || r == '；' || unicode.IsSpace(r)
	}) {
		addrs, err := expandSegment(tok)
		if err != nil {
			return nil, err
		}
		for _, a := range addrs {
			if !seen[a] {
				seen[a] = true
				out = append(out, a)
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("没有可扫的网段")
	}
	if len(out) > maxScanHosts {
		return nil, fmt.Errorf("一次最多扫 %d 个地址（现在有 %d 个），把网段拆小一点", maxScanHosts, len(out))
	}
	return out, nil
}

// rangeRe 匹配末段区间写法：192.168.1.10-100。
var rangeRe = regexp.MustCompile(`^(\d{1,3}(?:\.\d{1,3}){2}\.)(\d{1,3})-(\d{1,3})$`)

func expandSegment(tok string) ([]netip.Addr, error) {
	if m := rangeRe.FindStringSubmatch(tok); m != nil {
		return rangeAddrs(tok, m)
	}
	if strings.Contains(tok, "/") {
		p, err := netip.ParsePrefix(tok)
		if err != nil {
			return nil, fmt.Errorf("网段 %q 不是合法的 CIDR（形如 192.168.1.0/24）", tok)
		}
		return prefixAddrs(p)
	}
	if a, err := netip.ParseAddr(tok); err == nil {
		return []netip.Addr{a}, nil
	}
	// 前三段简写：192.168.1 → 192.168.1.0/24
	if p, err := netip.ParsePrefix(tok + ".0/24"); err == nil {
		return prefixAddrs(p)
	}
	return nil, fmt.Errorf("%q 看不懂：支持 192.168.1、192.168.1.0/24、192.168.1.10-100 或单个 IP", tok)
}

// rangeAddrs 展开 192.168.1.10-100 为 192.168.1.10 … 192.168.1.100。
// 区间不去网络号和广播号：用户点名要扫的范围，一个都不少。
func rangeAddrs(tok string, m []string) ([]netip.Addr, error) {
	start, err1 := netip.ParseAddr(m[1] + m[2])
	end, err2 := strconv.Atoi(m[3])
	if err1 != nil || err2 != nil || end > 255 {
		return nil, fmt.Errorf("区间 %q 不合法（形如 192.168.1.10-100）", tok)
	}
	if int(start.As4()[3]) > end {
		return nil, fmt.Errorf("区间 %q 起点比终点大", tok)
	}
	var out []netip.Addr
	for a := start; int(a.As4()[3]) <= end; a = a.Next() {
		out = append(out, a)
	}
	return out, nil
}

func prefixAddrs(p netip.Prefix) ([]netip.Addr, error) {
	if !p.Addr().Is4() {
		return nil, fmt.Errorf("只支持 IPv4 网段")
	}
	p = p.Masked()
	var out []netip.Addr
	for a := p.Addr(); p.Contains(a); a = a.Next() {
		out = append(out, a)
	}
	// /31、/32 没有网络号/广播号的概念，全保留；更宽的去掉一头一尾。
	if p.Bits() <= 30 && len(out) > 2 {
		out = out[1 : len(out)-1]
	}
	return out, nil
}

type pinged struct {
	addr  netip.Addr
	rttMs float64
}

func pingAll(addrs []netip.Addr) []pinged {
	var mu sync.Mutex
	var alive []pinged
	sem := make(chan struct{}, scanConcurrency)
	var wg sync.WaitGroup
	for _, a := range addrs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if rtt, ok := pingOnce(a.String()); ok {
				mu.Lock()
				alive = append(alive, pinged{addr: a, rttMs: rtt})
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	sort.Slice(alive, func(i, j int) bool { return alive[i].addr.Compare(alive[j].addr) < 0 })
	return alive
}

func pingOnce(ip string) (float64, bool) {
	rtt, ok := echo(net.ParseIP(ip), scanTimeout)
	if !ok {
		return 0, false
	}
	return ms(rtt), true
}

// arpLineRe 匹配 arp -a 的条目行。IP 和 MAC 的格式跟系统语言无关，
// 中文系统（「动态」）和英文系统（"dynamic"）都能匹配。
var arpLineRe = regexp.MustCompile(`^(\d{1,3}(?:\.\d{1,3}){3})\s+([0-9a-fA-F]{2}(?:-[0-9a-fA-F]{2}){5})\s`)

// arpTable 读本机 ARP 缓存。刚 ping 过的地址都在里面，这是拿 MAC 最便宜的办法；
// 跨网段的地址不会出现，拿不到就留空。
func arpTable() map[string]string {
	cmd := exec.Command("arp", "-a")
	hideConsole(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	return parseArp(string(out))
}

func parseArp(output string) map[string]string {
	macs := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		m := arpLineRe.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		macs[m[1]] = strings.ToUpper(m[2])
	}
	return macs
}

// resolveNames 对在线地址做反向 DNS。逐个查会串行累加超时，所以并发查、
// 每个带短超时；查不到就留空，不影响主结果。
func resolveNames(alive []pinged) map[string]string {
	names := make(map[string]string, len(alive))
	var mu sync.Mutex
	var wg sync.WaitGroup
	r := &net.Resolver{}
	for _, h := range alive {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), dnsTimeout)
			defer cancel()
			list, err := r.LookupAddr(ctx, ip)
			if err != nil || len(list) == 0 {
				return
			}
			mu.Lock()
			names[ip] = strings.TrimSuffix(list[0], ".")
			mu.Unlock()
		}(h.addr.String())
	}
	wg.Wait()
	return names
}

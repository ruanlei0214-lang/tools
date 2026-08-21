// Package ping 探测本机所在网络：长 ping 单个地址、扫网段里的在线设备。
//
// 和别的模块不同，它不连控制器——ping 的是本机网络里的任意地址，
// 用在拉线现场确认「这个地址通不通、网段里都有谁」。
package ping

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// Module 是 ping 模块的入口。
type Module struct {
	svc *Service
}

func New() *Module { return &Module{svc: &Service{}} }

func (m *Module) ID() string { return "ping" }

func (m *Module) Bindings() []any { return []any{m.svc} }

// Startup 收下 Wails 的上下文。扫描网段时往前端推「又发现一台」的事件要靠它。
func (m *Module) Startup(ctx context.Context) { m.svc.ctx = ctx }

// Service 暴露给前端。长 ping 在后台 goroutine 里跑，日志攒在缓冲区，
// 前端轮询 ReadPing 取走——和终端模块读输出的方式一样。
type Service struct {
	// ctx 是 Wails 的运行时上下文，扫描时的增量事件从它发。Startup 之前为 nil，
	// 那时事件直接不发，扫描本身不受影响。
	ctx context.Context

	mu      sync.Mutex
	cancel  context.CancelFunc
	running bool
	lines   []string
	// gen 是长 ping 的代次：StartPing 停掉旧 ping 再起新的时，旧 goroutine
	// 的收尾（写统计行、清 running）不能落到新一代头上，靠 gen 认出自己过期了。
	gen int
}

const (
	pingTimeout  = time.Second
	pingInterval = time.Second
	// maxPingLines 是缓冲区的上限。前端半秒取一次，正常永远到不了；
	// 这个上限防的是前端停在别的页签、日志一直攒的情况。
	maxPingLines = 500
)

// PingLog 是 ReadPing 一次取走的内容。Running 让前端知道长 ping 还在不在，
// 停了就把按钮换回来、停掉轮询。
type PingLog struct {
	Lines   []string `json:"lines"`
	Running bool     `json:"running"`
}

// StartPing 开始对 host 长 ping，每秒一个包，直到 StopPing。
// 已在跑的会被停掉再重新开始——界面上只有一个「开始」按钮，没有排队一说。
func (s *Service) StartPing(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("地址不能为空")
	}
	addr, err := net.ResolveIPAddr("ip4", host)
	if err != nil {
		return fmt.Errorf("无法解析 %q：%v", host, err)
	}

	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.running = true
	s.gen++
	gen := s.gen
	s.lines = []string{fmt.Sprintf("正在 ping %s（%s），每秒一个包，点「停止」结束", host, addr.IP)}
	s.mu.Unlock()

	go s.pingLoop(ctx, gen, addr.IP)
	return nil
}

// StopPing 停止长 ping。不在跑时调用不算错误。
func (s *Service) StopPing() {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// ReadPing 取走上次以来攒下的日志行。
func (s *Service) ReadPing() (PingLog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lines := s.lines
	s.lines = nil
	return PingLog{Lines: lines, Running: s.running}, nil
}

func (s *Service) pingLoop(ctx context.Context, gen int, ip net.IP) {
	sent, recv := 0, 0
	var sum, min, max time.Duration

	for {
		rtt, ok := echo(ip, pingTimeout)
		sent++
		if ok {
			recv++
			sum += rtt
			if recv == 1 || rtt < min {
				min = rtt
			}
			if rtt > max {
				max = rtt
			}
			s.append(gen, fmt.Sprintf("%s：通，%.1fms", ip, ms(rtt)))
		} else {
			s.append(gen, fmt.Sprintf("%s：超时", ip))
		}

		select {
		case <-ctx.Done():
			summary := fmt.Sprintf("已停止：发出 %d 个，收到 %d 个（丢 %d%%）",
				sent, recv, (sent-recv)*100/sent)
			if recv > 0 {
				summary += fmt.Sprintf("，时延 %.1f/%.1f/%.1fms（最小/平均/最大）",
					ms(min), ms(sum/time.Duration(recv)), ms(max))
			}
			s.append(gen, summary)
			s.mu.Lock()
			if s.gen == gen {
				s.running = false
				s.cancel = nil
			}
			s.mu.Unlock()
			return
		case <-time.After(pingInterval):
		}
	}
}

// append 把一行日志放进缓冲区。过期代次的行直接丢掉——那属于已被新一次
// 长 ping 顶替的旧任务，它的日志不该混进新日志里。
func (s *Service) append(gen int, line string) {
	s.mu.Lock()
	if s.gen == gen {
		s.lines = append(s.lines, line)
		if len(s.lines) > maxPingLines {
			s.lines = s.lines[len(s.lines)-maxPingLines:]
		}
	}
	s.mu.Unlock()
}

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }

// LocalIface 是本机一块网卡上的一个 IPv4 地址，给「扫描网段」做网卡选择：
// 选哪块网卡，就扫它所在的网段。
type LocalIface struct {
	Name    string `json:"name"`
	IP      string `json:"ip"`
	Segment string `json:"segment"`
}

// LocalIfaces 返回本机启用中的网卡及各自的 IPv4 网段（前三段）。
// 取 /24 而不是接口真实掩码：预填值宁小勿大，真要大网段让现场自己改。
func (s *Service) LocalIfaces() ([]LocalIface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []LocalIface
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipnet.IP.To4()
			if ip4 == nil {
				continue
			}
			item := LocalIface{
				Name:    iface.Name,
				IP:      ip4.String(),
				Segment: fmt.Sprintf("%d.%d.%d", ip4[0], ip4[1], ip4[2]),
			}
			key := item.Name + "|" + item.Segment
			if !seen[key] {
				seen[key] = true
				out = append(out, item)
			}
		}
	}
	return out, nil
}

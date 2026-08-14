// Package remote 通过控制器的远程模式接口做 IO 与寄存器操作。
//
// 报文格式照 doc/api_documentation/远程模式接口说明.md：请求与响应都是
// {"id","ty","db"} 三段式，失败时响应带 err。传输层是 WebSocket 而不是文档里写的
// 裸 TCP —— 现场这台控制器把远程接口挂在 WebSocket 上，文档没跟着更新。
package remote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	tyGetIO       = "IOManager/GetIOValue"
	tySetIO       = "IOManager/SetIOValue"
	tySetIOForced = "IOManager/SetIOForcedFlag"
	tyGetRegister = "RegisterManager/GetRegisterValue"
	tySetRegister = "RegisterManager/SetRegisterValue"
)

// unforceTimeout 是断开连接时把强制标志清掉的等待上限。
// 这时人已经在走了，不能按正常请求超时把断开拖成几十秒。
const unforceTimeout = time.Second

// verifyRetryDelay 是核对输入点位时第二次读回之前等的时间。实测这台控制器
// 一个请求来回 100ms 左右，等这么久足够跨过一个扫描周期。
const verifyRetryDelay = 250 * time.Millisecond

// Module 是 remote 模块的入口。
type Module struct {
	svc *Service
}

func New() *Module { return &Module{svc: newService()} }

func (m *Module) ID() string { return "remote" }

func (m *Module) Bindings() []any { return []any{m.svc} }

// Startup 收下 Wails 的上下文。导入导出要点系统文件对话框，那需要它。
func (m *Module) Startup(ctx context.Context) { m.svc.ctx = ctx }

// IOPoint 是一个 IO 点位。
type IOPoint struct {
	Type string `json:"type"`
	Port int    `json:"port"`
}

// IOValue 是一个点位的当前值。DI/DO 是 0 或 1，AI/AO 是模拟量。
type IOValue struct {
	Type  string  `json:"type"`
	Port  int     `json:"port"`
	Value float64 `json:"value"`
}

// RegisterValue 是一次寄存器读回的结果。
type RegisterValue struct {
	Address int    `json:"address"`
	Value   string `json:"value"`
}

// Status 是当前连接状态，界面靠它决定按钮能不能点。
type Status struct {
	Connected bool `json:"connected"`
	// Addr 是真正连上的那个完整地址（含路径）。路径可能是探出来的，
	// 显示出来才好照着钉回 config.json。
	Addr string `json:"addr"`
	// Error 是连接被动断开的原因。主动断开时为空。
	Error string `json:"error"`
}

// Service 暴露给前端。它持有一条到控制器的长连接，连接状态由 Connect /
// Disconnect 显式管理——IO 控制里「现在连没连上」是操作员必须能看见的信息，
// 藏在每次调用背后自动重连反而说不清刚才那一下到底发出去没有。
// 两把锁，各管一件事，且**永不嵌套**：
//
//	cfgMu 管配置（settings），s.mu 管连接（conn / lastEr / forced）。
//
// 配置能在运行中被界面改掉，所以读它也要加锁；连接的锁在整个请求期间都握着。
// 谁都不许在持有一把的时候去拿另一把——两条路径的顺序一旦相反就是死锁。
// 具体到代码里：Connect 先把超时取出来再去拿 s.mu，client() 持 s.mu 期间不碰配置，
// 保存配置的那几个方法只碰 cfgMu，一次连接都不动。
type Service struct {
	ctx context.Context

	cfgMu    sync.RWMutex
	settings Settings

	mu     sync.Mutex
	conn   *client
	lastEr string
	// forced 记下本会话里打开过强制的输入点位。断开时要逐路关掉：
	// 强制标志活在控制器上，socket 断了它不会自己掉，物理输入会一直被盖住。
	forced map[string]IOPoint
}

func newService() *Service {
	return &Service{settings: loadSettings()}
}

// Config 返回页面要用的全部配置：连接默认值与标签页定义。
func (s *Service) Config() Settings { return s.snapshot() }

// snapshot 取一份当前配置。返回的是浅拷贝，Tabs 那个切片是共用的——
// 保存时一律整份换掉 s.settings 而不是原地改里面的元素，所以拿着旧快照的调用方是安全的。
func (s *Service) snapshot() Settings {
	s.cfgMu.RLock()
	defer s.cfgMu.RUnlock()
	return s.settings
}

// SaveDevice 保存连接参数。校验不过就不写盘，界面上那份也不动。
//
// 保存不碰连接：地址改了之后是否重连由操作员按「连接」决定。
// 自动断了重连的话，正在观察某一路信号的人会莫名其妙丢一次连接。
func (s *Service) SaveDevice(in DeviceSettings) (Settings, error) {
	normalized, err := validateDevice(in)
	if err != nil {
		return Settings{}, err
	}
	raw, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return Settings{}, err
	}
	if err := writeStore(deviceFileName, raw); err != nil {
		return Settings{}, err
	}
	return s.reload(), nil
}

// SavePanel 保存一整个点位面板。前端每次改动都送整份过来，增、删、改共用这一条路径——
// 三个方法各写一遍校验和落盘只会让它们慢慢长歪。
func (s *Service) SavePanel(tab Tab) (Settings, error) {
	normalized, src, err := validatePanel(tab)
	if err != nil {
		return Settings{}, err
	}
	raw, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return Settings{}, err
	}
	if err := writeStore(src.file, raw); err != nil {
		return Settings{}, err
	}
	return s.reload(), nil
}

// ResetDevice 把连接参数退回出厂默认。
func (s *Service) ResetDevice() (Settings, error) {
	if err := removeStore(deviceFileName); err != nil {
		return Settings{}, err
	}
	return s.reload(), nil
}

// ResetPanel 把一个点位面板退回出厂默认。
func (s *Service) ResetPanel(kind string) (Settings, error) {
	src, ok := panelSourceByKind(kind)
	if !ok {
		return Settings{}, fmt.Errorf("不认识的标签页类型 %q，只支持 %s、%s", kind, kindIO, kindRegister)
	}
	if err := removeStore(src.file); err != nil {
		return Settings{}, err
	}
	return s.reload(), nil
}

// PanelFileResult 是一次导入的结果。取消选文件时 Canceled 为真，配置没动。
type PanelFileResult struct {
	Settings Settings `json:"settings"`
	Path     string   `json:"path"`
	Canceled bool     `json:"canceled"`
}

// ExportPanel 把当前这一页点位存成用户选的 JSON 文件。取消时返回空路径。
// 写出的就是界面上正在用的那份，和 exe 旁边现场配置同一格式。
func (s *Service) ExportPanel(kind string) (string, error) {
	if s.ctx == nil {
		return "", errors.New("界面还没准备好，稍后再试")
	}
	raw, src, err := s.panelBytes(kind)
	if err != nil {
		return "", err
	}
	path, err := runtime.SaveFileDialog(s.ctx, runtime.SaveDialogOptions{
		Title:           "导出" + panelNoun(src),
		DefaultFilename: src.file,
		Filters:         []runtime.FileFilter{{DisplayName: "JSON", Pattern: "*.json"}},
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return "", fmt.Errorf("写入 %s 失败：%w", path, err)
	}
	return path, nil
}

// ImportPanel 用用户选的 JSON 替换当前这一页点位。取消选文件时配置不动。
// 校验和保存走同一条路径：过不了的文件不会写盘。
func (s *Service) ImportPanel(kind string) (PanelFileResult, error) {
	if s.ctx == nil {
		return PanelFileResult{}, errors.New("界面还没准备好，稍后再试")
	}
	src, ok := panelSourceByKind(kind)
	if !ok {
		return PanelFileResult{}, fmt.Errorf("不认识的标签页类型 %q，只支持 %s、%s", kind, kindIO, kindRegister)
	}
	path, err := runtime.OpenFileDialog(s.ctx, runtime.OpenDialogOptions{
		Title:   "导入" + panelNoun(src),
		Filters: []runtime.FileFilter{{DisplayName: "JSON", Pattern: "*.json"}},
	})
	if err != nil {
		return PanelFileResult{}, err
	}
	if path == "" {
		return PanelFileResult{Settings: s.snapshot(), Canceled: true}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return PanelFileResult{}, fmt.Errorf("读取 %s 失败：%w", path, err)
	}
	cfg, err := s.applyImportedPanel(kind, raw, filepath.Base(path))
	if err != nil {
		return PanelFileResult{}, err
	}
	return PanelFileResult{Settings: cfg, Path: path}, nil
}

func panelNoun(src panelSource) string {
	if src.kind == kindIO {
		return "IO 点位"
	}
	return "寄存器点位"
}

func tabByKind(s Settings, kind string) *Tab {
	for i := range s.Tabs {
		if s.Tabs[i].Kind == kind {
			return &s.Tabs[i]
		}
	}
	return nil
}

// panelBytes 把当前内存里这一页编成 JSON，给导出用。
func (s *Service) panelBytes(kind string) ([]byte, panelSource, error) {
	src, ok := panelSourceByKind(kind)
	if !ok {
		return nil, panelSource{}, fmt.Errorf("不认识的标签页类型 %q，只支持 %s、%s", kind, kindIO, kindRegister)
	}
	tab := tabByKind(s.snapshot(), src.kind)
	if tab == nil {
		return nil, src, fmt.Errorf("当前没有 %s 这一页", src.kind)
	}
	raw, err := json.MarshalIndent(tab, "", "  ")
	if err != nil {
		return nil, src, err
	}
	return raw, src, nil
}

// applyImportedPanel 解析、校验、落盘、立刻重载。对话框之外的测试走这条，
// 避免在单元测试里弹系统选文件框。
func (s *Service) applyImportedPanel(kind string, raw []byte, label string) (Settings, error) {
	src, ok := panelSourceByKind(kind)
	if !ok {
		return Settings{}, fmt.Errorf("不认识的标签页类型 %q，只支持 %s、%s", kind, kindIO, kindRegister)
	}
	var peek struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &peek); err == nil {
		if k := strings.ToLower(strings.TrimSpace(peek.Kind)); k != "" && k != src.kind {
			return Settings{}, fmt.Errorf("%s 是 %s 配置，不能导入到这一页", label, k)
		}
	}
	tab, err := buildPanel(raw, label, src)
	if err != nil {
		return Settings{}, err
	}
	out, err := json.MarshalIndent(tab, "", "  ")
	if err != nil {
		return Settings{}, err
	}
	if err := writeStore(src.file, out); err != nil {
		return Settings{}, err
	}
	return s.reload(), nil
}

// reload 落盘之后整份重新加载，而不是只把改动那一块塞进内存。
// 图的是内存里那份和盘上那份不会分叉：保存后界面看到的就是下次开机会看到的。
func (s *Service) reload() Settings {
	next := loadSettings()
	s.cfgMu.Lock()
	s.settings = next
	s.cfgMu.Unlock()
	return next
}

// Status 报告当前连接状态，顺便把已经断掉的连接清理干净。
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statusLocked()
}

func (s *Service) statusLocked() Status {
	if s.conn == nil {
		return Status{Error: s.lastEr}
	}
	if !s.conn.alive() {
		s.lastEr = s.conn.closedErr().Error()
		s.conn = nil
		s.forced = nil
		return Status{Error: s.lastEr}
	}
	return Status{Connected: true, Addr: s.conn.addr}
}

// Connect 建立到控制器的长连接，重复调用会先断开旧的。
func (s *Service) Connect(d Device) (Status, error) {
	// 超时在拿 s.mu 之前取出来：持有 s.mu 时不许再去拿 cfgMu，见 Service 上的说明。
	timeout := s.connectTimeout()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.releaseLocked()
	s.lastEr = ""

	c, err := dial(d.Host, d.Port, d.Path, timeout)
	if err != nil {
		s.lastEr = err.Error()
		return Status{Error: s.lastEr}, err
	}
	s.conn = c
	return Status{Connected: true, Addr: c.addr}, nil
}

// Disconnect 主动断开。切走页面或换设备时调用，别把 socket 挂在那儿。
func (s *Service) Disconnect() Status {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.releaseLocked()
	s.lastEr = ""
	return Status{}
}

// releaseLocked 先把本会话打开的强制标志清掉，再关连接。
func (s *Service) releaseLocked() {
	s.unforceAllLocked()
	if s.conn != nil {
		s.conn.Close()
		s.conn = nil
	}
}

// GetIO 批量读点位。界面上的一屏状态一次读完，别一个点位一个请求。
func (s *Service) GetIO(points []IOPoint) ([]IOValue, error) {
	if len(points) == 0 {
		return []IOValue{}, nil
	}
	for _, p := range points {
		if err := checkPoint(p); err != nil {
			return nil, err
		}
	}

	c, err := s.client()
	if err != nil {
		return nil, err
	}
	raw, err := c.call(tyGetIO, points, s.requestTimeout())
	if err != nil {
		return nil, err
	}
	return parseIOValues(raw)
}

// SetIO 写一个点位。输入点位（DI/AI）写完会读回来核对一次，见 verifyInput。
func (s *Service) SetIO(point IOPoint, value float64) error {
	if err := checkPoint(point); err != nil {
		return err
	}
	c, err := s.client()
	if err != nil {
		return err
	}
	if err := s.write(c, point, value); err != nil {
		return err
	}
	return s.verifyInput(c, point, value)
}

// SetIOForced 打开或关闭某一路输入的强制标志。
//
// 这台控制器上，直接 SetIOValue 写 DI 会被接受但值立刻被现场扫描盖掉。
// 要改 DI，得先发 IOManager/SetIOForcedFlag（value 1 打开、0 关掉），再 SetIOValue。
// 界面上一键强制会对配置里每一路 DI 逐个调用这个方法。
func (s *Service) SetIOForced(point IOPoint, forced bool) error {
	if err := checkPoint(point); err != nil {
		return err
	}
	if !isInputType(point.Type) {
		return fmt.Errorf("%s%d 不是输入点位，不用强制", point.Type, point.Port)
	}

	c, err := s.client()
	if err != nil {
		return err
	}

	v := 0
	if forced {
		v = 1
	}
	db := map[string]any{"type": point.Type, "port": point.Port, "value": v}
	if _, err := c.call(tySetIOForced, db, s.requestTimeout()); err != nil {
		return err
	}

	s.mu.Lock()
	s.trackForcedLocked(point, forced)
	s.mu.Unlock()
	return nil
}

// SetIOForcedAll 对一批 DI 逐路发 SetIOForcedFlag。界面上一键强制走这里，
// 控制器没有「全部 DI」这种批量指令，只能一路一路发。
func (s *Service) SetIOForcedAll(points []IOPoint, forced bool) error {
	di := make([]IOPoint, 0, len(points))
	for _, p := range points {
		if err := checkPoint(p); err != nil {
			return err
		}
		if p.Type != "DI" {
			return fmt.Errorf("%s%d 不是 DI，一键强制只针对 DI", p.Type, p.Port)
		}
		di = append(di, p)
	}
	if len(di) == 0 {
		return errors.New("没有要强制的 DI")
	}

	var failed []string
	for _, p := range di {
		if err := s.SetIOForced(p, forced); err != nil {
			failed = append(failed, fmt.Sprintf("DI%d：%v", p.Port, err))
		}
	}
	if len(failed) > 0 {
		action := "打开"
		if !forced {
			action = "关闭"
		}
		return fmt.Errorf("%s强制未全部完成：%s", action, strings.Join(failed, "；"))
	}
	return nil
}

func (s *Service) trackForcedLocked(point IOPoint, forced bool) {
	if s.forced == nil {
		s.forced = map[string]IOPoint{}
	}
	key := pointKey(point)
	if forced {
		s.forced[key] = point
		return
	}
	delete(s.forced, key)
}

func (s *Service) unforceAllLocked() {
	if s.conn == nil || len(s.forced) == 0 {
		s.forced = nil
		return
	}
	if !s.conn.alive() {
		s.forced = nil
		return
	}
	for _, p := range s.forced {
		db := map[string]any{"type": p.Type, "port": p.Port, "value": 0}
		_, _ = s.conn.call(tySetIOForced, db, unforceTimeout)
	}
	s.forced = nil
}

func pointKey(p IOPoint) string {
	return p.Type + ":" + strconv.Itoa(p.Port)
}

// PulseIO 写 value，等 pulseMs 毫秒，再写 offValue。
//
// 等待放后端而不是前端：前端等待期间用户可以切标签页甚至切模块，组件一销毁
// 定时器就没了，点位会一直停在按下的那个值上。
func (s *Service) PulseIO(point IOPoint, value, offValue float64, pulseMs int) error {
	if err := checkPoint(point); err != nil {
		return err
	}
	if pulseMs <= 0 {
		pulseMs = defaultPulseMs
	}
	if pulseMs > maxPulseMs {
		pulseMs = maxPulseMs
	}

	c, err := s.client()
	if err != nil {
		return err
	}
	if err := s.write(c, point, value); err != nil {
		return err
	}
	// 输入点位先确认这一下真的落下去了。没落下去就不必再等那几百毫秒——
	// 等的是一个并不存在的脉冲。照样把 offValue 写回去，别留下半个动作。
	if err := s.verifyInput(c, point, value); err != nil {
		_ = s.write(c, point, offValue)
		return err
	}
	time.Sleep(time.Duration(pulseMs) * time.Millisecond)
	if err := s.write(c, point, offValue); err != nil {
		return fmt.Errorf("%s%d 已置为 %g，但恢复失败：%v", point.Type, point.Port, value, err)
	}
	return nil
}

// ToggleIO 先读回当前值再写反的那个，返回写进去的值。
//
// 不靠界面记上一次点了什么：点位也可能被程序或别的上位机改掉，
// 本地记的状态一旦和现场对不上，按钮就会连点两下才动一次。
func (s *Service) ToggleIO(point IOPoint, onValue, offValue float64) (float64, error) {
	if err := checkPoint(point); err != nil {
		return 0, err
	}
	c, err := s.client()
	if err != nil {
		return 0, err
	}

	current, err := s.readOne(c, point)
	if err != nil {
		return 0, err
	}

	next := onValue
	if current == onValue {
		next = offValue
	}
	if err := s.write(c, point, next); err != nil {
		return 0, err
	}
	if err := s.verifyInput(c, point, next); err != nil {
		return 0, err
	}
	return next, nil
}

// verifyInput 写完输入点位后读回来核对一次。输出点位不做这一步：那是正常写得动的东西，
// 每次写都多一个来回只是白等。
//
// 输入点位需要这一步，因为没开强制时「控制器答应了」和「值真的变了」是两件事：
// 直接 SetIOValue 写 DI 一律回成功，但值一动不动。不核对的话界面会显示一条绿色的
// 「已写入 1」，而现场那一路根本没动。
func (s *Service) verifyInput(c *client, point IOPoint, want float64) error {
	if !isInputType(point.Type) {
		return nil
	}

	// 第一次读回不对不立刻下结论：控制器可能要等一个扫描周期才把强制值刷进来。
	// 只有再等一下还是老值才说它没生效——这个额外的等待只在出问题时才付。
	got, err := s.readOne(c, point)
	if err == nil && got != want {
		time.Sleep(verifyRetryDelay)
		got, err = s.readOne(c, point)
	}
	switch {
	case err != nil:
		// 读不回来只说明这一次没能确认。写入本身是被应答过的，不该因此报成写失败；
		// 连接真的坏了的话，下一次操作会如实报出来。
		return nil
	case got != want:
		return fmt.Errorf("%s%d 已下发 %g，但读回仍是 %g：这一路没有真的被改动"+
			"（控制器接受了写入却没有生效，现场信号或扫描周期会把强制值盖掉）",
			point.Type, point.Port, want, got)
	default:
		return nil
	}
}

// readOne 读一个点位的当前值。
func (s *Service) readOne(c *client, point IOPoint) (float64, error) {
	raw, err := c.call(tyGetIO, []IOPoint{point}, s.requestTimeout())
	if err != nil {
		return 0, err
	}
	rows, err := parseIOValues(raw)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("控制器没有返回 %s%d 的当前值", point.Type, point.Port)
	}
	return rows[0].Value, nil
}

// GetRegisters 批量读寄存器。
func (s *Service) GetRegisters(addresses []int) ([]RegisterValue, error) {
	if len(addresses) == 0 {
		return nil, errors.New("至少提供一个寄存器地址")
	}
	for _, a := range addresses {
		if a < 0 {
			return nil, fmt.Errorf("寄存器地址不能为负数：%d", a)
		}
	}

	c, err := s.client()
	if err != nil {
		return nil, err
	}
	raw, err := c.call(tyGetRegister, addresses, s.requestTimeout())
	if err != nil {
		return nil, err
	}
	return parseRegisterValues(raw)
}

// SetRegister 写一个寄存器。value 按 true/false、整数、小数依次尝试，
// 都不像就当字符串下发。
func (s *Service) SetRegister(address int, value string) error {
	if address < 0 {
		return fmt.Errorf("寄存器地址不能为负数：%d", address)
	}
	parsed, err := parseWriteValue(value)
	if err != nil {
		return err
	}

	c, err := s.client()
	if err != nil {
		return err
	}
	return s.writeRegister(c, address, parsed)
}

// PulseRegister 写 value，等 pulseMs 毫秒，再写 offValue。等待放后端，理由同 PulseIO。
func (s *Service) PulseRegister(address int, value, offValue float64, pulseMs int) error {
	if address < 0 {
		return fmt.Errorf("寄存器地址不能为负数：%d", address)
	}
	if pulseMs <= 0 {
		pulseMs = defaultPulseMs
	}
	if pulseMs > maxPulseMs {
		pulseMs = maxPulseMs
	}

	c, err := s.client()
	if err != nil {
		return err
	}
	if err := s.writeRegister(c, address, value); err != nil {
		return err
	}
	time.Sleep(time.Duration(pulseMs) * time.Millisecond)
	if err := s.writeRegister(c, address, offValue); err != nil {
		return fmt.Errorf("地址 %d 已置为 %g，但恢复失败：%v", address, value, err)
	}
	return nil
}

// ToggleRegister 先读回当前值再写反的那个，返回写进去的值。理由同 ToggleIO。
func (s *Service) ToggleRegister(address int, onValue, offValue float64) (float64, error) {
	if address < 0 {
		return 0, fmt.Errorf("寄存器地址不能为负数：%d", address)
	}
	c, err := s.client()
	if err != nil {
		return 0, err
	}

	current, err := s.readRegister(c, address)
	if err != nil {
		return 0, err
	}

	next := onValue
	if current == onValue {
		next = offValue
	}
	if err := s.writeRegister(c, address, next); err != nil {
		return 0, err
	}
	return next, nil
}

func (s *Service) readRegister(c *client, address int) (float64, error) {
	raw, err := c.call(tyGetRegister, []int{address}, s.requestTimeout())
	if err != nil {
		return 0, err
	}
	rows, err := parseRegisterValues(raw)
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, fmt.Errorf("控制器没有返回地址 %d 的当前值", address)
	}
	return parseRegisterNumber(rows[0].Value)
}

func (s *Service) writeRegister(c *client, address int, value any) error {
	_, err := c.call(tySetRegister, map[string]any{"address": address, "value": value}, s.requestTimeout())
	return err
}

func (s *Service) write(c *client, point IOPoint, value float64) error {
	db := map[string]any{"type": point.Type, "port": point.Port, "value": value}
	_, err := c.call(tySetIO, db, s.requestTimeout())
	return describeWriteError(point, err)
}

// describeWriteError 给控制器的「invalid xx port」补一句话。
//
// 这个错最容易撞上也最难猜：GetIOValue 对不存在的端口照样回 0 且不报错，所以一个端口号
// 配错的点位在界面上是个正常的 OFF，只有点下去才冒出来。实测这台控制器认的范围写在下面，
// 换机型会变，所以措辞是「实测这台」而不是「规格是」。
func describeWriteError(point IOPoint, err error) error {
	if err == nil || !strings.Contains(err.Error(), "invalid") {
		return err
	}
	return fmt.Errorf("%w —— 控制器不认 %s%d 这个端口号"+
		"（实测这台认 DI 0-24、DO 0-17、AI/AO 0-3）。读取时不存在的端口也会回 0，"+
		"所以它在界面上看着是个正常的 OFF", err, point.Type, point.Port)
}

// client 取当前连接，顺带把已经断掉的那条清掉，让下一次 Status 如实报未连接。
func (s *Service) client() (*client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn == nil {
		if s.lastEr != "" {
			return nil, fmt.Errorf("尚未连接控制器（%s）", s.lastEr)
		}
		return nil, errors.New("尚未连接控制器")
	}
	if !s.conn.alive() {
		err := s.conn.closedErr()
		s.lastEr = err.Error()
		s.conn = nil
		s.forced = nil
		return nil, err
	}
	return s.conn, nil
}

// connectTimeout / requestTimeout 每次都从快照里取：界面改过超时之后，
// 下一次请求就该按新值等，不用重启程序。
func (s *Service) connectTimeout() time.Duration {
	return time.Duration(s.snapshot().ConnectTimeoutSeconds) * time.Second
}

func (s *Service) requestTimeout() time.Duration {
	return time.Duration(s.snapshot().RequestTimeoutSeconds) * time.Second
}

func isInputType(t string) bool {
	return t == "DI" || t == "AI"
}

// checkPoint 只校验类型和端口。DI/AI 的写入本身不在这儿拦；
// 要改输入得先 SetIOForced 打开那一路的强制标志，见 SetIOForced。
func checkPoint(p IOPoint) error {
	if !validIOType(p.Type) {
		return fmt.Errorf("IO 类型 %q 不认识，只支持 DI、DO、AI、AO", p.Type)
	}
	if p.Port < 0 {
		return fmt.Errorf("IO 端口不能为负数：%d", p.Port)
	}
	return nil
}

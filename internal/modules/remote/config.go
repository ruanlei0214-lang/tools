package remote

import (
	"bytes"
	_ "embed"
	"embedtools/internal/module"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// 这三份是**出厂默认**：exe 旁边对应的现场配置不存在时用它们播种。
// 现场在界面上改出来的配置落在 exe 同目录（见 store.go），所以改这三份只影响
// 「一份现场配置都没有」的干净机器，改完仍要重新构建。
//
// 单独放一个 config/ 目录，是为了让现场一眼看出这个模块有哪些东西是配置：
// 模块目录里 .go 文件越堆越多之后，配置文件夹在中间并不显眼。
// 连接参数和按钮清单分开：改 IO 点位不用翻寄存器，改寄存器也不用碰 IO。
//
//go:embed config/config.json
var configJSON []byte

//go:embed config/io.json
var ioJSON []byte

//go:embed config/register.json
var registerJSON []byte

const (
	defaultPort           = 9001
	defaultConnectTimeout = 3
	maxConnectTimeout     = 60
	defaultRequestTimeout = 8
	maxRequestTimeout     = 120
	defaultRefreshMs      = 1000
	minRefreshMs          = 200
	maxRefreshMs          = 60000
	defaultPulseMs        = 300
	maxPulseMs            = 10000
)

// Settings 是整个页面的配置：连上谁、超时多久、有哪些标签页。
type Settings struct {
	Device Device `json:"device"`
	// ConnectTimeoutSeconds 只管建连，不管建连之后每次请求的等待。
	ConnectTimeoutSeconds int `json:"connectTimeoutSeconds"`
	// RequestTimeoutSeconds 是单次请求等响应的上限。控制器不回包时靠它兜底，
	// 否则按钮会一直转圈。
	RequestTimeoutSeconds int `json:"requestTimeoutSeconds"`
	// RefreshIntervalMs 是 IO / 寄存器标签页自动刷新的间隔。调快了看得实时，
	// 代价是控制器每隔这么久就要应付一次读全部点位；现场按需要在配置里改。
	RefreshIntervalMs int `json:"refreshIntervalMs"`
	// Tabs 决定页面里有几个标签页、分别是什么。顺序即显示顺序。
	Tabs []Tab `json:"tabs"`
	// TabVisibility 决定三个标签页各自显不显示，键是 kind（io / register / command）。
	// 没写的键按显示处理——现场配置只需要列出要藏的那个。指令页不是点位面板，
	// 没有自己的文件，显不显示只由这里决定。
	TabVisibility map[string]bool `json:"tabVisibility"`
	// ConfigDir 是现场配置所在的目录，也就是 exe 所在目录。
	ConfigDir string `json:"configDir"`
	// Warning 非空表示某份配置不可用，当前这些值来自能读出来的文件或内置兜底。
	Warning string `json:"warning"`
}

// Device 是控制器远程模式的 WebSocket 地址，最终拼成 ws://host:port/path。
//
// Host 通常来自共享配置 toolbox-config.json（全系列工具共用一个地址）；
// 界面上不给 host 输入框，要改地址去共享配置或别的模块改。Port 和 Path 仍归本模块：
// 端口是接一次定死的东西，路径只在配置里改。
// 留空按 "/" 处理；连不上时会自动试几个常见路径，见 client.go 的 probePaths。
type Device struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	Path string `json:"path"`
}

// Tab 是一个标签页。Kind 决定前端拿什么界面来渲染它，
// Groups 在 kind 为 io 或 register 时都要用：按钮从这里长出来。
type Tab struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Kind        string  `json:"kind"`
	Description string  `json:"description"`
	Groups      []Group `json:"groups,omitempty"`
}

// Group 是一组点位，Title 是这组的小标题。输入和输出各分一组最清楚。
type Group struct {
	Title  string  `json:"title"`
	Points []Point `json:"points"`
}

// Point 是界面上的一个按钮。IO 和寄存器共用这一份定义，所以两个标签页长得一样。
//
// IO：Type 是 DI/DO/AI/AO，Port 是端口号。输出可切换，DI 要先一键强制。
// 寄存器：Type 是 BOOL/INT/FLOAT，Port 是寄存器地址（文档里 GetRegisterValue 的 address）。
type Point struct {
	Label string `json:"label"`
	Type  string `json:"type"`
	Port  int    `json:"port"`
	// OnValue / OffValue 是开关量切换时写的两个值，省略（或写成一样）时按 1 / 0。
	OnValue  float64 `json:"onValue"`
	OffValue float64 `json:"offValue"`
	// Value 是 INT / FLOAT / AO 的默认可填值，界面预填、下发都用它。
	// 开关量不用这个字段：它们在两个值之间翻转，走 OnValue / OffValue。
	// INT 必须是整数；FLOAT 按字符串原样下发，控制器可能认数字也可能认别的文本。
	Value string `json:"value"`
	// PulseMs 大于 0 时这个点位多一个「点动」按钮：写 OnValue，等这么久，再写 OffValue。
	// 只对开关量有意义。指令类信号（开门、夹紧）用得上，同时它照样能被 ON/OFF 切换。
	PulseMs int `json:"pulseMs"`
	// Danger 让这个点位显示成红色，给急停一类的动作用。
	Danger bool   `json:"danger"`
	Hint   string `json:"hint"`
}

// builtinSettings 是 config.json 不可用时的兜底。配置坏了只影响界面内容，
// 没道理连带把连接和读写一起废掉——现场至少还能用寄存器标签页。
func builtinSettings() Settings {
	// 兜底只放文档示例地址 10000。IO 点位号猜不出来，凭空给一组只会误导现场。
	// 指令页不依赖任何配置，兜底里也带上。
	return Settings{
		Device:                Device{Host: "192.168.1.136", Port: defaultPort, Path: "/"},
		ConnectTimeoutSeconds: defaultConnectTimeout,
		RequestTimeoutSeconds: defaultRequestTimeout,
		RefreshIntervalMs:     defaultRefreshMs,
		Tabs: []Tab{
			{
				ID:    "register",
				Title: "寄存器",
				Kind:  kindRegister,
				Groups: []Group{{
					Title:  "寄存器",
					Points: []Point{{Type: "BOOL", Port: 10000}},
				}},
			},
			commandTab(),
		},
	}
}

// panelSource 是一个点位面板的两个来源：现场配置文件与编译进产物的出厂默认。
type panelSource struct {
	kind      string
	defaultID string
	file      string // exe 旁边的现场配置
	builtin   []byte // 出厂默认
	builtinAs string // 出厂默认那份的文件名，只用在告警里
}

func panelSources() []panelSource {
	return []panelSource{
		{kindIO, "io", ioFileName, ioJSON, "io.json"},
		{kindRegister, "register", registerFileName, registerJSON, "register.json"},
	}
}

// loadSettings 每一部分都是「现场配置优先，没有或坏掉就退回出厂默认」。
//
// 三部分各自独立：IO 点位表里少个逗号不该把连接参数和寄存器一起废掉。
// 全程只读不写——坏文件里可能还有能人工救回来的点位，自动重写等于把它们抹掉。
func loadSettings() Settings {
	var warns []string

	s, warn := loadDevicePart()
	if warn != "" {
		warns = append(warns, warn)
	}
	// 目录取不到（定位不了 exe）时留空，界面就不显示这一行，其余功能照常。
	if dir, err := configDir(); err == nil {
		s.ConfigDir = dir
	}

	taken := map[string]bool{}
	for _, src := range panelSources() {
		tab, warn := loadPanelPart(src)
		if warn != "" {
			warns = append(warns, warn)
		}
		if tab == nil {
			continue
		}
		// 两份文件都能自己写 id，撞上了就把后一个按回默认值并说一声。
		// 不静悄悄改：id 是界面上选中哪一页的凭据，被人改过又被工具改回去最难查。
		if taken[tab.ID] {
			warns = append(warns, fmt.Sprintf("%s 的 id %q 和前一个标签页重复，已按 %q 处理",
				src.file, tab.ID, src.defaultID))
			tab.ID = src.defaultID
		}
		taken[tab.ID] = true
		s.Tabs = append(s.Tabs, *tab)
	}

	// 显隐开关最后统一过一遍：被藏起来的面板照常读盘校验（坏了照样告警），
	// 只是不进标签页列表——藏起来的那一页出了配置错不该变成无声无息。
	filtered := s.Tabs[:0]
	for _, tab := range s.Tabs {
		if tabVisible(s.TabVisibility, tab.Kind) {
			filtered = append(filtered, tab)
		}
	}
	s.Tabs = filtered
	if tabVisible(s.TabVisibility, kindCommand) {
		s.Tabs = append(s.Tabs, commandTab())
	}

	s.Warning = strings.Join(warns, "；")
	// 共享配置的地址优先：三个模块连的是同一台控制器，地址只该在一个地方改。
	// 共享配置里没有地址时，才用本模块自己那份（出厂默认或 remote-config.json 里的）。
	if host := module.LoadShared().Host; host != "" {
		s.Device.Host = host
	}
	return s
}

// loadDevicePart 读连接参数。现场那份盖在出厂默认上逐键合并（见 mergeRoot）；
// 现场那份坏掉时整份退回出厂默认，出厂默认也坏掉时退回内置兜底——
// 内置兜底有测试盯着，走到最后这一支基本只剩「有人改坏了 config/config.json 又没跑测试」。
func loadDevicePart() (Settings, string) {
	base, warn := builtinDevicePart("")

	raw, path, err := readStore(deviceFileName)
	if err == nil {
		merged, perr := mergeRoot(base, raw)
		if perr == nil {
			return merged, warn
		}
		w := fmt.Sprintf("%s 不可用（%v），连接参数已整份退回出厂默认。"+
			"这个文件没有被改动，可以打开它人工修好。", path, perr)
		return base, joinWarn(warn, w)
	}
	if errors.Is(err, errNoOverride) {
		return base, warn
	}
	return base, joinWarn(warn, err.Error())
}

// mergeRoot 把现场配置盖在出厂默认上：json.Unmarshal 进已填好出厂值的结构体，
// 只有文件里写了的键会覆盖，没写的键透出出厂值。tabVisibility 这类 map 也是
// 逐键合并——现场只写要藏的那页，其余页跟着出厂走。
//
// 要逐键合并而不是整份替换：新版本给出厂配置加新键时（tabVisibility 就是这么来的），
// 老机器上的现场文件里没有这个键，整份替换会把它永远挡住，改出厂默认也透不出来。
//
// 点位面板（io.json / register.json）不走合并：一份面板是一个整体文档，
// 半新半旧的点位拼在一起只会更难查。
func mergeRoot(base Settings, raw []byte) (Settings, error) {
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	if err := json.Unmarshal(raw, &base); err != nil {
		return Settings{}, describeParseError(raw, err)
	}
	if err := applyDeviceDefaults(&base); err != nil {
		return Settings{}, err
	}
	// 连接配置里不放按钮，标签页由两份点位文件和指令页决定。
	base.Tabs = nil
	base.Warning = ""
	return base, nil
}

func joinWarn(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + "；" + b
}

// builtinDevicePart 取出厂默认的连接参数。Tabs 一律清空——标签页由两份点位文件决定，
// 这里带一份出去会和它们叠成两个同名标签页。
func builtinDevicePart(warn string) (Settings, string) {
	s, err := parseRoot(configJSON)
	if err != nil {
		s = builtinSettings()
		s.Tabs = nil
		if warn == "" {
			warn = fmt.Sprintf("出厂默认 config.json 不可用，连接参数是内置默认值：%v", err)
		}
		return s, warn
	}
	return s, warn
}

// loadPanelPart 读一个点位面板。返回 nil 表示连出厂默认都不可用，那一页干脆不显示——
// 显示一个空的标签页比没有这一页更让人以为「点位真的没了」。
func loadPanelPart(src panelSource) (*Tab, string) {
	raw, path, err := readStore(src.file)
	if err == nil {
		tab, perr := buildPanel(raw, path, src)
		if perr == nil {
			return tab, ""
		}
		return builtinPanelPart(src,
			fmt.Sprintf("%v，已退回出厂默认。这个文件没有被改动，可以打开它人工修好。", perr))
	}
	if errors.Is(err, errNoOverride) {
		return builtinPanelPart(src, "")
	}
	return builtinPanelPart(src, err.Error())
}

func builtinPanelPart(src panelSource, warn string) (*Tab, string) {
	tab, err := buildPanel(src.builtin, src.builtinAs, src)
	if err != nil {
		if warn == "" {
			return nil, fmt.Sprintf("出厂默认 %s 不可用：%v", src.builtinAs, err)
		}
		return nil, fmt.Sprintf("%s；出厂默认也不可用：%v", warn, err)
	}
	return tab, warn
}

// buildPanel 是解析加校验的那一步，加载和保存都走它——两套规则迟早分叉，
// 那时候「能存进去但打不开」的配置就出现了。
func buildPanel(raw []byte, label string, src panelSource) (*Tab, error) {
	tab, err := parsePanel(raw, label, src.kind, src.defaultID)
	if err != nil {
		return nil, err
	}
	// seen 每次新建：这里只校验这一份自己，跨文件的 id 撞车由调用方处理。
	if err := normalizeTab(&tab, map[string]bool{}); err != nil {
		return nil, fmt.Errorf("%s：%v", label, err)
	}
	return &tab, nil
}

func panelSourceByKind(kind string) (panelSource, bool) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	for _, src := range panelSources() {
		if src.kind == kind {
			return src, true
		}
	}
	return panelSource{}, false
}

// validatePanel 是界面保存点位时的校验入口，和加载走同一个 normalizeTab。
// 保存一套规则、加载另一套的话，迟早会存进去一份自己打不开的配置。
func validatePanel(tab Tab) (Tab, panelSource, error) {
	src, ok := panelSourceByKind(tab.Kind)
	if !ok {
		return Tab{}, panelSource{}, fmt.Errorf("不认识的标签页类型 %q，只支持 %s",
			tab.Kind, supportedKinds())
	}
	tab.Kind = src.kind
	if strings.TrimSpace(tab.ID) == "" {
		tab.ID = src.defaultID
	}
	if err := normalizeTab(&tab, map[string]bool{}); err != nil {
		return Tab{}, panelSource{}, err
	}
	return tab, src, nil
}

const (
	kindIO       = "io"
	kindRegister = "register"
	// kindCommand 是「指令」标签页：不放点位，前端按 kind 渲染成固定界面（重启控制器）。
	kindCommand = "command"
)

// commandTab 合成指令标签页。它不是点位面板，没有自己的配置文件，
// 显不显示只由 remote-config.json 的 tabVisibility 决定。
func commandTab() Tab {
	return Tab{ID: "command", Title: "指令", Kind: kindCommand}
}

// tabVisible 报告一个标签页显不显示。配置里没写的键按显示处理，
// 键名大小写不敏感——现场手写配置时 IO 和 io 不该是两种结果。
func tabVisible(vis map[string]bool, kind string) bool {
	for k, v := range vis {
		if strings.EqualFold(strings.TrimSpace(k), kind) {
			return v
		}
	}
	return true
}

func supportedKinds() string {
	return kindIO + "、" + kindRegister
}

// parseSettings 拦的都是「界面会当场坏掉」的错：标签页没有、类型拼错、
// 往只读点位上挂写按钮。地址和端口这类现场要改的值只做范围检查。
// 测试仍用这一份「连接 + 标签页」的合成 JSON；真正加载走 parseRoot + parsePanel。
func parseSettings(raw []byte) (Settings, error) {
	s, err := unmarshalSettings(raw)
	if err != nil {
		return Settings{}, err
	}
	if err := applyDeviceDefaults(&s); err != nil {
		return Settings{}, err
	}
	if len(s.Tabs) == 0 {
		return Settings{}, fmt.Errorf("tabs 为空，页面会没有任何内容")
	}

	seen := make(map[string]bool, len(s.Tabs))
	for i := range s.Tabs {
		if err := normalizeTab(&s.Tabs[i], seen); err != nil {
			return Settings{}, err
		}
	}

	s.Warning = ""
	return s, nil
}

func parseRoot(raw []byte) (Settings, error) {
	s, err := unmarshalSettings(raw)
	if err != nil {
		return Settings{}, err
	}
	if err := applyDeviceDefaults(&s); err != nil {
		return Settings{}, err
	}
	// 连接配置里不再放按钮。IO / 寄存器用各自的文件。
	s.Tabs = nil
	s.Warning = ""
	return s, nil
}

func parsePanel(raw []byte, filename, kind, defaultID string) (Tab, error) {
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	var t Tab
	if err := json.Unmarshal(raw, &t); err != nil {
		return Tab{}, fmt.Errorf("%s：%v", filename, describeParseError(raw, err))
	}
	t.Kind = kind
	if strings.TrimSpace(t.ID) == "" {
		t.ID = defaultID
	}
	return t, nil
}

func unmarshalSettings(raw []byte) (Settings, error) {
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	var s Settings
	if err := json.Unmarshal(raw, &s); err != nil {
		return Settings{}, describeParseError(raw, err)
	}
	return s, nil
}

func applyDeviceDefaults(s *Settings) error {
	if s.Device.Port == 0 {
		s.Device.Port = defaultPort
	}
	if s.Device.Port < 1 || s.Device.Port > 65535 {
		return fmt.Errorf("端口 %d 不在 1-65535 之间", s.Device.Port)
	}
	s.Device.Path = normalizePath(s.Device.Path)
	if s.ConnectTimeoutSeconds == 0 {
		s.ConnectTimeoutSeconds = defaultConnectTimeout
	}
	if s.ConnectTimeoutSeconds < 1 || s.ConnectTimeoutSeconds > maxConnectTimeout {
		return fmt.Errorf(
			"connectTimeoutSeconds %d 不在 1-%d 秒之间", s.ConnectTimeoutSeconds, maxConnectTimeout)
	}
	if s.RequestTimeoutSeconds == 0 {
		s.RequestTimeoutSeconds = defaultRequestTimeout
	}
	if s.RequestTimeoutSeconds < 1 || s.RequestTimeoutSeconds > maxRequestTimeout {
		return fmt.Errorf(
			"requestTimeoutSeconds %d 不在 1-%d 秒之间", s.RequestTimeoutSeconds, maxRequestTimeout)
	}
	if s.RefreshIntervalMs == 0 {
		s.RefreshIntervalMs = defaultRefreshMs
	}
	if s.RefreshIntervalMs < minRefreshMs || s.RefreshIntervalMs > maxRefreshMs {
		return fmt.Errorf(
			"refreshIntervalMs %d 不在 %d-%d 毫秒之间", s.RefreshIntervalMs, minRefreshMs, maxRefreshMs)
	}
	return nil
}

func normalizeTab(t *Tab, seen map[string]bool) error {
	t.ID = strings.TrimSpace(t.ID)
	if t.ID == "" {
		return fmt.Errorf("每个标签页都要有 id")
	}
	if seen[t.ID] {
		return fmt.Errorf("标签页 id %q 重复", t.ID)
	}
	seen[t.ID] = true

	if t.Title = strings.TrimSpace(t.Title); t.Title == "" {
		t.Title = t.ID
	}
	t.Kind = strings.ToLower(strings.TrimSpace(t.Kind))
	switch t.Kind {
	case kindIO, kindRegister:
	case kindCommand:
		// 指令页没有点位，到这就够了。
		return nil
	default:
		return fmt.Errorf("标签页 %q 的 kind %q 不认识，只支持 %s、%s", t.ID, t.Kind, supportedKinds(), kindCommand)
	}

	if len(t.Groups) == 0 {
		return fmt.Errorf("标签页 %q 里没有任何点位组", t.ID)
	}
	for gi := range t.Groups {
		g := &t.Groups[gi]
		g.Title = strings.TrimSpace(g.Title)
		if len(g.Points) == 0 {
			return fmt.Errorf("标签页 %q 的第 %d 组一个点位都没有", t.ID, gi+1)
		}
		for pi := range g.Points {
			if err := normalizePoint(&g.Points[pi], t.ID, t.Kind); err != nil {
				return err
			}
		}
	}
	return nil
}

func normalizePoint(p *Point, tabID, kind string) error {
	p.Type = strings.ToUpper(strings.TrimSpace(p.Type))
	switch kind {
	case kindRegister:
		if !validRegisterType(p.Type) {
			return fmt.Errorf("标签页 %q 里点位 %q 的类型 %q 不认识，只支持 BOOL、INT、FLOAT",
				tabID, p.Label, p.Type)
		}
	default:
		if !validIOType(p.Type) {
			return fmt.Errorf("标签页 %q 里点位 %q 的类型 %q 不认识，只支持 DI、DO、AI、AO",
				tabID, p.Label, p.Type)
		}
	}
	if p.Port < 0 {
		return fmt.Errorf("标签页 %q 里点位 %q 的端口不能为负数", tabID, p.Label)
	}
	if p.Label = strings.TrimSpace(p.Label); p.Label == "" {
		p.Label = fmt.Sprintf("%s%d", p.Type, p.Port)
	}

	p.Value = strings.TrimSpace(p.Value)
	if digitalType(p.Type) {
		// 两个值一样就没法切换了。省略 onValue / offValue 时两个都是 0，正好落进这一支。
		if p.OnValue == p.OffValue {
			p.OnValue, p.OffValue = 1, 0
		}
		p.Value = ""
		if p.PulseMs != 0 && (p.PulseMs < 20 || p.PulseMs > maxPulseMs) {
			return fmt.Errorf("点位 %q 的 pulseMs %d 不在 20-%d 之间", p.Label, p.PulseMs, maxPulseMs)
		}
		return nil
	}

	// INT / FLOAT / AO / AI 不走 ON/OFF，点动也没有意义。
	p.OnValue, p.OffValue, p.PulseMs = 0, 0, 0
	if p.Value != "" {
		if err := validatePointValue(p.Type, p.Value); err != nil {
			return fmt.Errorf("点位 %q 的值 %q %v", p.Label, p.Value, err)
		}
	}
	return nil
}

func digitalType(t string) bool {
	return t == "DI" || t == "DO" || t == "BOOL"
}

func validatePointValue(typ, value string) error {
	switch typ {
	case "INT":
		if _, err := strconv.ParseInt(value, 10, 64); err != nil {
			return fmt.Errorf("不是整数")
		}
	case "AO", "AI":
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return fmt.Errorf("不是数字")
		}
	}
	return nil
}

// describeParseError 给 JSON 报错补上行号。encoding/json 只说「invalid character '{'」，
// 一份几十行的按钮配置里少个逗号，光靠这句话得一行行数过去。
func describeParseError(raw []byte, err error) error {
	var se *json.SyntaxError
	if errors.As(err, &se) {
		line, col := lineCol(raw, se.Offset)
		return fmt.Errorf("第 %d 行第 %d 列语法错误：%v（多半是逗号多了或少了）", line, col, se)
	}
	var te *json.UnmarshalTypeError
	if errors.As(err, &te) {
		line, col := lineCol(raw, te.Offset)
		return fmt.Errorf("第 %d 行第 %d 列字段 %q 类型不对，期望 %s", line, col, te.Field, te.Type)
	}
	return err
}

func lineCol(raw []byte, offset int64) (int, int) {
	if offset < 0 {
		offset = 0
	}
	if offset > int64(len(raw)) {
		offset = int64(len(raw))
	}
	line, col := 1, 1
	for _, b := range raw[:offset] {
		if b == '\n' {
			line, col = line+1, 1
			continue
		}
		col++
	}
	return line, col
}

func validIOType(t string) bool {
	switch t {
	case "DI", "DO", "AI", "AO":
		return true
	}
	return false
}

func validRegisterType(t string) bool {
	switch t {
	case "BOOL", "INT", "FLOAT":
		return true
	}
	return false
}

package remote

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// 打进产物的那几份配置必须是好的，否则界面一开就是兜底加一条告警。
func TestEmbeddedConfigIsValid(t *testing.T) {
	if _, err := parseRoot(configJSON); err != nil {
		t.Fatalf("config.json 解析失败：%v", err)
	}

	// 指到临时目录：开发机上真有现场配置的话，这个测试就在验那份而不是出厂默认。
	useTempConfigDir(t)

	s := loadSettings()
	if s.Warning != "" {
		t.Fatalf("内置配置不可用：%s", s.Warning)
	}
	if len(s.Tabs) == 0 {
		t.Fatal("加载后没有标签页")
	}
	var points int
	var regs int
	for _, tab := range s.Tabs {
		n := 0
		for _, g := range tab.Groups {
			n += len(g.Points)
		}
		switch tab.Kind {
		case kindIO:
			points += n
		case kindRegister:
			regs += n
		}
	}
	if points == 0 {
		t.Fatal("io.json 里没有任何点位")
	}
	if regs == 0 {
		t.Fatal("register.json 里没有任何点位")
	}
}

func TestParseRootIgnoresTabs(t *testing.T) {
	raw := []byte(`{
		"device": {"host": "10.0.0.2"},
		"tabs": [{"id": "io", "kind": "io", "groups": [{"points": [{"type": "DO", "port": 0}]}]}]
	}`)
	s, err := parseRoot(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Tabs) != 0 {
		t.Fatalf("连接配置不该带上按钮：%d 个标签页", len(s.Tabs))
	}
	if s.Device.Port != defaultPort {
		t.Fatalf("port=%d", s.Device.Port)
	}
}

func TestParsePanelFillsDefaults(t *testing.T) {
	raw := []byte(`{"groups":[{"points":[{"type":"bool","port":10000}]}]}`)
	tab, err := parsePanel(raw, "register.json", kindRegister, "register")
	if err != nil {
		t.Fatal(err)
	}
	if err := normalizeTab(&tab, map[string]bool{}); err != nil {
		t.Fatal(err)
	}
	if tab.ID != "register" || tab.Kind != kindRegister || tab.Title != "register" {
		t.Fatalf("tab=%+v", tab)
	}
	p := tab.Groups[0].Points[0]
	if p.Type != "BOOL" || p.Label != "BOOL10000" {
		t.Fatalf("point=%+v", p)
	}
}

func TestParseSettingsFillsDefaults(t *testing.T) {
	raw := []byte(`{
		"device": {"host": "10.0.0.2"},
		"tabs": [
			{"id": "io", "kind": "io", "groups": [
				{"points": [{"label": "点动", "type": "do", "port": 1, "pulseMs": 500}]}
			]}
		]
	}`)

	s, err := parseSettings(raw)
	if err != nil {
		t.Fatal(err)
	}
	if s.Device.Port != defaultPort {
		t.Fatalf("port=%d", s.Device.Port)
	}
	if s.Device.Path != "/" {
		t.Fatalf("path=%q", s.Device.Path)
	}
	if s.ConnectTimeoutSeconds != defaultConnectTimeout || s.RequestTimeoutSeconds != defaultRequestTimeout {
		t.Fatalf("timeouts=%d/%d", s.ConnectTimeoutSeconds, s.RequestTimeoutSeconds)
	}
	if s.RefreshIntervalMs != defaultRefreshMs {
		t.Fatalf("refreshIntervalMs=%d", s.RefreshIntervalMs)
	}

	tab := s.Tabs[0]
	if tab.Title != "io" {
		t.Fatalf("title 缺省应当退回 id，实际 %q", tab.Title)
	}
	p := tab.Groups[0].Points[0]
	if p.Type != "DO" {
		t.Fatalf("类型没有归一成大写：%q", p.Type)
	}
	if p.OnValue != 1 || p.OffValue != 0 {
		t.Fatalf("切换值缺省应当是 1/0，实际 %g/%g", p.OnValue, p.OffValue)
	}
}

// 输入点位和输出点位配法完全一样，界面上也长一样，只是默认不能改。
func TestParseSettingsAllowsInputPoints(t *testing.T) {
	raw := []byte(`{"tabs":[{"id":"io","kind":"io","groups":[{"points":[
		{"label":"安全OK","type":"di","port":3}
	]}]}]}`)

	s, err := parseSettings(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Tabs[0].Groups[0].Points[0].Type; got != "DI" {
		t.Fatalf("type=%q", got)
	}
}

func TestParseSettingsPointLabelFallsBackToPointName(t *testing.T) {
	raw := []byte(`{"tabs":[{"id":"io","kind":"io","groups":[{"points":[{"type":"di","port":4}]}]}]}`)
	s, err := parseSettings(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Tabs[0].Groups[0].Points[0].Label; got != "DI4" {
		t.Fatalf("label=%q", got)
	}
}

// 反相点位（按下写 0）要保留原样，不能被「缺省补 1/0」冲掉。
func TestParseSettingsKeepsInvertedValues(t *testing.T) {
	raw := []byte(`{"tabs":[{"id":"io","kind":"io","groups":[{"points":[
		{"label":"反相","type":"do","port":1,"onValue":0,"offValue":1}
	]}]}]}`)

	s, err := parseSettings(raw)
	if err != nil {
		t.Fatal(err)
	}
	p := s.Tabs[0].Groups[0].Points[0]
	if p.OnValue != 0 || p.OffValue != 1 {
		t.Fatalf("%g/%g", p.OnValue, p.OffValue)
	}
}

func TestParseSettingsRegisterPoints(t *testing.T) {
	raw := []byte(`{"tabs":[{"id":"r","kind":"register","groups":[{"points":[
		{"type":"bool","port":10000,"pulseMs":500},
		{"label":"计数","type":"int","port":20000}
	]}]}]}`)

	s, err := parseSettings(raw)
	if err != nil {
		t.Fatal(err)
	}
	p0 := s.Tabs[0].Groups[0].Points[0]
	if p0.Type != "BOOL" || p0.Label != "BOOL10000" || p0.OnValue != 1 {
		t.Fatalf("BOOL 缺省不对：%+v", p0)
	}
	p1 := s.Tabs[0].Groups[0].Points[1]
	if p1.Type != "INT" || p1.Label != "计数" {
		t.Fatalf("INT 不对：%+v", p1)
	}
	if p1.OnValue != 0 || p1.OffValue != 0 || p1.PulseMs != 0 {
		t.Fatalf("INT 不该带 ON/OFF/点动：%+v", p1)
	}
}

func TestNormalizePointValueTypes(t *testing.T) {
	ok := Point{Type: "FLOAT", Port: 1, Value: "ready"}
	if err := normalizePoint(&ok, "r", kindRegister); err != nil {
		t.Fatal(err)
	}
	if ok.Value != "ready" || ok.OnValue != 0 {
		t.Fatalf("FLOAT 应当原样保留字符串：%+v", ok)
	}

	num := Point{Type: "INT", Port: 2, Value: "42"}
	if err := normalizePoint(&num, "r", kindRegister); err != nil {
		t.Fatal(err)
	}
	if num.Value != "42" {
		t.Fatalf("INT 应当保留整数：%+v", num)
	}

	bad := Point{Type: "INT", Port: 3, Value: "1.5"}
	if err := normalizePoint(&bad, "r", kindRegister); err == nil {
		t.Fatal("INT 填小数应当被拒")
	}
}

func TestParseSettingsRejects(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "没有标签页",
			raw:  `{"tabs": []}`,
			want: "tabs 为空",
		},
		{
			name: "标签页 id 重复",
			raw:  `{"tabs": [{"id":"io","kind":"register","groups":[{"points":[{"type":"BOOL","port":1}]}]},{"id":"io","kind":"register","groups":[{"points":[{"type":"BOOL","port":2}]}]}]}`,
			want: "重复",
		},
		{
			name: "kind 不认识",
			raw:  `{"tabs": [{"id":"x","kind":"magic"}]}`,
			want: "kind",
		},
		{
			name: "点位类型拼错",
			raw:  `{"tabs": [{"id":"io","kind":"io","groups":[{"points":[{"label":"x","type":"XX","port":0}]}]}]}`,
			want: "不认识",
		},
		{
			name: "点位端口是负数",
			raw:  `{"tabs": [{"id":"io","kind":"io","groups":[{"points":[{"label":"x","type":"DO","port":-1}]}]}]}`,
			want: "端口不能为负数",
		},
		{
			name: "pulseMs 超范围",
			raw:  `{"tabs": [{"id":"io","kind":"io","groups":[{"points":[{"label":"x","type":"DO","port":0,"pulseMs":99999}]}]}]}`,
			want: "pulseMs",
		},
		{
			name: "点位组是空的",
			raw:  `{"tabs": [{"id":"io","kind":"io","groups":[{"title":"DO","points":[{"label":"x","type":"DO","port":0}]},{}]}]}`,
			want: "第 2 组一个点位都没有",
		},
		{
			name: "寄存器标签页空空如也",
			raw:  `{"tabs": [{"id":"r","kind":"register"}]}`,
			want: "没有任何点位组",
		},
		{
			name: "寄存器类型拼错",
			raw:  `{"tabs": [{"id":"r","kind":"register","groups":[{"points":[{"label":"x","type":"DO","port":0}]}]}]}`,
			want: "BOOL、INT、FLOAT",
		},
		{
			name: "端口越界",
			raw:  `{"device":{"port":70000},"tabs":[{"id":"r","kind":"register"}]}`,
			want: "65535",
		},
		{
			name: "请求超时越界",
			raw:  `{"requestTimeoutSeconds":9999,"tabs":[{"id":"r","kind":"register"}]}`,
			want: "requestTimeoutSeconds",
		},
		{
			name: "刷新间隔太短",
			raw:  `{"refreshIntervalMs":50,"tabs":[{"id":"r","kind":"register"}]}`,
			want: "refreshIntervalMs",
		},
		{
			name: "刷新间隔太长",
			raw:  `{"refreshIntervalMs":600000,"tabs":[{"id":"r","kind":"register"}]}`,
			want: "refreshIntervalMs",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseSettings([]byte(c.raw))
			if err == nil {
				t.Fatal("应当报错")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err=%v，期望包含 %q", err, c.want)
			}
		})
	}
}

// 少个逗号是改这份配置时最容易犯的错，报错必须直接指到行上。
func TestParseSettingsReportsLineNumber(t *testing.T) {
	raw := []byte(`{
  "tabs": [
    {"id": "a", "kind": "register"}
    {"id": "b", "kind": "register"}
  ]
}`)

	_, err := parseSettings(raw)
	if err == nil {
		t.Fatal("应当报错")
	}
	if !strings.Contains(err.Error(), "第 4 行") {
		t.Fatalf("err=%v，期望指出第 4 行", err)
	}
	if !strings.Contains(err.Error(), "逗号") {
		t.Fatalf("err=%v，期望提示逗号", err)
	}
}

func findTab(s Settings, kind string) *Tab {
	for i := range s.Tabs {
		if s.Tabs[i].Kind == kind {
			return &s.Tabs[i]
		}
	}
	return nil
}

// 现场配置的存在感就体现在这里：盘上有那一份，出厂默认就退到后面去。
func TestLoadSettingsPrefersStoredConfig(t *testing.T) {
	useTempConfigDir(t)

	if err := writeStore(deviceFileName, []byte(`{"device":{"host":"10.1.2.3","port":8080}}`)); err != nil {
		t.Fatal(err)
	}
	if err := writeStore(ioFileName, []byte(
		`{"title":"现场IO","groups":[{"title":"输出","points":[{"label":"夹紧","type":"DO","port":7}]}]}`)); err != nil {
		t.Fatal(err)
	}

	s := loadSettings()
	if s.Warning != "" {
		t.Fatalf("不该有告警：%s", s.Warning)
	}
	if s.Device.Host != "10.1.2.3" || s.Device.Port != 8080 {
		t.Fatalf("连接参数没用现场那份：%+v", s.Device)
	}
	io := findTab(s, kindIO)
	if io == nil || io.Title != "现场IO" || io.Groups[0].Points[0].Label != "夹紧" {
		t.Fatalf("IO 没用现场那份：%+v", io)
	}
	// 寄存器那份没写过，仍该是出厂默认，且照样在。
	if findTab(s, kindRegister) == nil {
		t.Fatal("寄存器标签页不见了")
	}
	if s.ConfigDir == "" {
		t.Fatal("配置目录要带给界面显示")
	}
}

// 一份坏掉只许影响它自己：IO 里少个逗号不能把连接参数和寄存器一起废掉。
func TestLoadSettingsIsolatesBadFile(t *testing.T) {
	useTempConfigDir(t)

	if err := writeStore(deviceFileName, []byte(`{"device":{"host":"10.1.2.3"}}`)); err != nil {
		t.Fatal(err)
	}
	if err := writeStore(ioFileName, []byte(`{"groups":[`)); err != nil {
		t.Fatal(err)
	}
	if err := writeStore(registerFileName, []byte(
		`{"groups":[{"points":[{"label":"节拍","type":"INT","port":20001}]}]}`)); err != nil {
		t.Fatal(err)
	}

	s := loadSettings()
	if !strings.Contains(s.Warning, ioFileName) {
		t.Fatalf("告警要指出是哪个文件：%q", s.Warning)
	}
	if s.Device.Host != "10.1.2.3" {
		t.Fatalf("连接参数被牵连了：%+v", s.Device)
	}
	reg := findTab(s, kindRegister)
	if reg == nil || reg.Groups[0].Points[0].Label != "节拍" {
		t.Fatalf("寄存器被牵连了：%+v", reg)
	}
	// 坏掉那一页退回出厂默认，不是消失。
	if findTab(s, kindIO) == nil {
		t.Fatal("IO 标签页应当退回出厂默认")
	}
}

// 坏文件里可能还有能人工救回来的点位，加载路径绝不能顺手把它重写掉。
func TestLoadSettingsNeverRewritesBadFile(t *testing.T) {
	useTempConfigDir(t)

	broken := []byte(`{"groups":[{"points":[{"type":"DO","port":`)
	if err := writeStore(ioFileName, broken); err != nil {
		t.Fatal(err)
	}
	loadSettings()

	raw, _, err := readStore(ioFileName)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != string(broken) {
		t.Fatalf("坏文件被改动了：%q", raw)
	}
}

// 越界的值存进过盘也不能让界面开不起来，退回默认加一条告警就够了。
func TestLoadSettingsRejectsOutOfRangeStoredDevice(t *testing.T) {
	useTempConfigDir(t)

	if err := writeStore(deviceFileName, []byte(`{"refreshIntervalMs":50}`)); err != nil {
		t.Fatal(err)
	}
	s := loadSettings()
	if !strings.Contains(s.Warning, "refreshIntervalMs") {
		t.Fatalf("告警要说明原因：%q", s.Warning)
	}
	if s.RefreshIntervalMs < minRefreshMs {
		t.Fatalf("没退回可用的间隔：%d", s.RefreshIntervalMs)
	}
}

// 两份现场配置各写了同一个 id 时不能顶掉彼此，界面得还是两页。
func TestLoadSettingsHandlesDuplicateTabID(t *testing.T) {
	useTempConfigDir(t)

	panel := `{"id":"same","groups":[{"points":[{"type":"%s","port":1}]}]}`
	if err := writeStore(ioFileName, []byte(fmt.Sprintf(panel, "DO"))); err != nil {
		t.Fatal(err)
	}
	if err := writeStore(registerFileName, []byte(fmt.Sprintf(panel, "INT"))); err != nil {
		t.Fatal(err)
	}

	s := loadSettings()
	if len(s.Tabs) != 2 {
		t.Fatalf("应当还是两页，实际 %d", len(s.Tabs))
	}
	if s.Tabs[0].ID == s.Tabs[1].ID {
		t.Fatalf("id 还是撞着的：%q", s.Tabs[0].ID)
	}
	if !strings.Contains(s.Warning, "重复") {
		t.Fatalf("改过 id 要说一声：%q", s.Warning)
	}
}

func TestValidateDeviceRangesMatchLoading(t *testing.T) {
	if _, err := validateDevice(DeviceSettings{
		Device: Device{Host: "10.0.0.1", Port: 70000}}); err == nil {
		t.Fatal("端口越界应当被拒")
	}
	if _, err := validateDevice(DeviceSettings{
		Device: Device{Host: "10.0.0.1"}, RequestTimeoutSeconds: 9999}); err == nil {
		t.Fatal("请求超时越界应当被拒")
	}

	// 缺省值该被补齐，界面上留空不等于 0。
	got, err := validateDevice(DeviceSettings{Device: Device{Host: "10.0.0.1"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Device.Port != defaultPort || got.Device.Path != "/" ||
		got.RefreshIntervalMs != defaultRefreshMs {
		t.Fatalf("缺省没补齐：%+v", got)
	}
}

func TestValidatePanelUsesLoadingRules(t *testing.T) {
	bad := Tab{Kind: kindRegister, Groups: []Group{{Points: []Point{{Type: "DO", Port: 1}}}}}
	if _, _, err := validatePanel(bad); err == nil {
		t.Fatal("寄存器页里的 DO 应当被拒")
	}
	if _, _, err := validatePanel(Tab{Kind: "magic"}); err == nil {
		t.Fatal("不认识的 kind 应当被拒")
	}

	good := Tab{Kind: kindIO, Groups: []Group{{Points: []Point{{Type: "do", Port: 2}}}}}
	tab, src, err := validatePanel(good)
	if err != nil {
		t.Fatal(err)
	}
	if tab.ID != "io" || src.file != ioFileName {
		t.Fatalf("tab=%+v file=%q", tab, src.file)
	}
	if tab.Groups[0].Points[0].Label != "DO2" || tab.Groups[0].Points[0].OnValue != 1 {
		t.Fatalf("保存路径没跑归一化：%+v", tab.Groups[0].Points[0])
	}
}

// 配置坏了只该影响界面内容，剩下的功能得照常能用，所以兜底本身也必须过校验。
func TestBuiltinSettingsAreValid(t *testing.T) {
	if _, err := parseSettings([]byte(`{"tabs":`)); err == nil {
		t.Fatal("坏 JSON 应当报错")
	}

	fallback := builtinSettings()
	raw, err := json.Marshal(fallback)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseSettings(raw); err != nil {
		t.Fatalf("兜底配置过不了自己的校验：%v", err)
	}
}

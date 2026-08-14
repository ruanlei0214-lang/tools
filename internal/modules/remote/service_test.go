package remote

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeIO 是假控制器里的点位状态，读写都落在它上面。
type fakeIO struct {
	mu        sync.Mutex
	values    map[string]float64
	writes    []IOValue
	forced    map[string]bool
	flags     []IOValue
	flagReq   []map[string]any
	regs      map[int]float64
	regWrites []struct {
		Address int
		Value   float64
	}
}

func (f *fakeIO) key(t string, port int) string { return fmt.Sprintf("%s:%d", t, port) }

// record 记下客户端要求写什么，apply 才真的改值。分开是为了能装出真机那种
// 「收下、答应、但值不变」——那种情况下这次写入照样发生过，得留在历史里。
func (f *fakeIO) record(t string, port int, v float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes = append(f.writes, IOValue{Type: t, Port: port, Value: v})
}

func (f *fakeIO) apply(t string, port int, v float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[f.key(t, port)] = v
}

func (f *fakeIO) get(t string, port int) float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.values[f.key(t, port)]
}

func (f *fakeIO) history() []IOValue {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]IOValue(nil), f.writes...)
}

func (f *fakeIO) setForced(t string, port int, on bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.forced == nil {
		f.forced = map[string]bool{}
	}
	f.forced[f.key(t, port)] = on
	v := 0.0
	if on {
		v = 1
	}
	f.flags = append(f.flags, IOValue{Type: t, Port: port, Value: v})
}

func (f *fakeIO) isForced(t string, port int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.forced[f.key(t, port)]
}

func (f *fakeIO) flagHistory() []IOValue {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]IOValue(nil), f.flags...)
}

func (f *fakeIO) recordFlagReq(db map[string]any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.flagReq = append(f.flagReq, db)
}

func (f *fakeIO) lastFlagReq() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.flagReq) == 0 {
		return nil
	}
	return f.flagReq[len(f.flagReq)-1]
}

func (f *fakeIO) getReg(addr int) float64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.regs[addr]
}

func (f *fakeIO) setReg(addr int, v float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.regs == nil {
		f.regs = map[int]float64{}
	}
	f.regs[addr] = v
	f.regWrites = append(f.regWrites, struct {
		Address int
		Value   float64
	}{addr, v})
}

func (f *fakeIO) regHistory() []struct {
	Address int
	Value   float64
} {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]struct {
		Address int
		Value   float64
	}(nil), f.regWrites...)
}

// newTestService 起一个假控制器并连上，返回可直接调用的 Service。
func newTestService(t *testing.T) (*Service, *fakeIO) {
	return newTestServiceWith(t, false)
}

// newTestServiceWith 的 dropInputWrites 照的是真机行为：那台控制器对 DI 的写入
// 一律回成功，但值一动不动。这是「界面显示已写入、现场没反应」的来源，
// 假控制器得能装出这个样子，否则这条路永远测不到。
func newTestServiceWith(t *testing.T, dropInputWrites bool) (*Service, *fakeIO) {
	t.Helper()

	io := &fakeIO{values: map[string]float64{}}
	host, port := startServer(t, "/", func(send sendFunc, req envelope) {
		switch req.Ty {
		case tySetIOForced:
			var db map[string]any
			_ = json.Unmarshal(req.DB, &db)
			io.recordFlagReq(db)
			t, _ := db["type"].(string)
			port, _ := db["port"].(float64)
			v, _ := db["value"].(float64)
			io.setForced(t, int(port), v != 0)
			send(map[string]any{"id": req.ID, "ty": req.Ty})
		case tySetIO:
			var db IOValue
			_ = json.Unmarshal(req.DB, &db)
			io.record(db.Type, db.Port, db.Value)
			// 没开强制的输入：真机收下、答应、值不变。开了强制才真的改。
			if dropInputWrites && isInputType(db.Type) && !io.isForced(db.Type, db.Port) {
				send(map[string]any{"id": req.ID, "ty": req.Ty})
				break
			}
			io.apply(db.Type, db.Port, db.Value)
			send(map[string]any{"id": req.ID, "ty": req.Ty})
		case tyGetIO:
			var points []IOPoint
			_ = json.Unmarshal(req.DB, &points)
			rows := make([]IOValue, 0, len(points))
			for _, p := range points {
				rows = append(rows, IOValue{Type: p.Type, Port: p.Port, Value: io.get(p.Type, p.Port)})
			}
			send(map[string]any{"id": req.ID, "ty": req.Ty, "db": rows})
		case tyGetRegister:
			var addrs []int
			_ = json.Unmarshal(req.DB, &addrs)
			rows := make([]map[string]any, 0, len(addrs))
			for _, a := range addrs {
				rows = append(rows, map[string]any{"address": a, "value": io.getReg(a)})
			}
			send(map[string]any{"id": req.ID, "ty": req.Ty, "db": rows})
		case tySetRegister:
			var db struct {
				Address int     `json:"address"`
				Value   float64 `json:"value"`
			}
			_ = json.Unmarshal(req.DB, &db)
			io.setReg(db.Address, db.Value)
			send(map[string]any{"id": req.ID, "ty": req.Ty})
		default:
			send(map[string]any{"id": req.ID, "ty": req.Ty, "err": "1/unsupported"})
		}
	})

	s := &Service{settings: builtinSettings()}
	if _, err := s.Connect(Device{Host: host, Port: port, Path: "/"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Disconnect() })
	return s, io
}

// 控制器答应了写入却没改值时，不许报成功。
//
// 这是实测出来的真机行为（DI 写入一律回成功、值不变），而在修好之前界面会显示
// 一条绿色的「已写入 1」——现场按了没反应又找不到错误信息，这种失败最难查。
func TestServiceReportsIgnoredInputWrite(t *testing.T) {
	s, io := newTestServiceWith(t, true)

	err := s.SetIO(IOPoint{Type: "DI", Port: 3}, 1)
	if err == nil {
		t.Fatal("控制器没有真的改值，不该报成功")
	}
	for _, want := range []string{"DI3", "读回"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("错误里应当提到 %q，实际是：%v", want, err)
		}
	}
	if io.get("DI", 3) != 0 {
		t.Fatal("假控制器本该丢掉这次写入")
	}
}

// 输出点位不做读回核对：那是正常写得动的东西，每次写都多一个来回只是白等。
func TestServiceDoesNotVerifyOutputs(t *testing.T) {
	s, io := newTestServiceWith(t, true)

	if err := s.SetIO(IOPoint{Type: "DO", Port: 2}, 1); err != nil {
		t.Fatalf("输出点位不该被核对拦住：%v", err)
	}
	if io.get("DO", 2) != 1 {
		t.Fatal("DO 应当被写进去")
	}
}

// 点动一路写不动的输入：不必等完那几百毫秒（等的是一个并不存在的脉冲），
// 但 offValue 照样要写回去，别留下半个动作。
func TestServicePulseOnIgnoredInputFailsFast(t *testing.T) {
	s, io := newTestServiceWith(t, true)

	start := time.Now()
	err := s.PulseIO(IOPoint{Type: "DI", Port: 4}, 1, 0, 3000)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("这一路根本没被改动，点动不该报成功")
	}
	if elapsed > 2*time.Second {
		t.Errorf("等了 %v，本该确认写不动之后立刻返回", elapsed)
	}
	writes := io.history()
	if len(writes) != 2 || writes[0].Value != 1 || writes[1].Value != 0 {
		t.Fatalf("应当先写 1 再写回 0，实际 writes=%+v", writes)
	}
}

// 翻转同样要核对：读回没变就是没变。
func TestServiceToggleReportsIgnoredInputWrite(t *testing.T) {
	s, _ := newTestServiceWith(t, true)

	if _, err := s.ToggleIO(IOPoint{Type: "DI", Port: 5}, 1, 0); err == nil {
		t.Fatal("控制器没有真的改值，翻转不该报成功")
	}
}

// 端口号超出控制器认的范围时，报错要带上「读的时候不报错」这句话——
// GetIOValue 对不存在的端口照样回 0，配错的点位在界面上是个正常的 OFF。
func TestDescribeWriteErrorExplainsInvalidPort(t *testing.T) {
	raw := fmt.Errorf("控制器返回错误：1000/Failed to set IO value: invalid DI port.")
	err := describeWriteError(IOPoint{Type: "DI", Port: 30}, raw)

	for _, want := range []string{"DI30", "回 0"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("错误里应当提到 %q，实际是：%v", want, err)
		}
	}
	if !errors.Is(err, raw) {
		t.Error("控制器的原话要留在错误链里")
	}
	// 别的错误不该被加料。
	plain := errors.New("等待响应超时")
	if got := describeWriteError(IOPoint{Type: "DO", Port: 1}, plain); got != plain {
		t.Errorf("不相关的错误被改写了：%v", got)
	}
	if describeWriteError(IOPoint{Type: "DO", Port: 1}, nil) != nil {
		t.Error("没有错误的时候不该造一个出来")
	}
}

func TestServiceRejectsCallsBeforeConnect(t *testing.T) {
	s := &Service{settings: builtinSettings()}

	if st := s.Status(); st.Connected {
		t.Fatal("还没连就报已连接")
	}
	if err := s.SetIO(IOPoint{Type: "DO", Port: 1}, 1); err == nil ||
		!strings.Contains(err.Error(), "尚未连接") {
		t.Fatalf("err=%v", err)
	}
}

func TestServiceSetAndGetIO(t *testing.T) {
	s, io := newTestService(t)

	if st := s.Status(); !st.Connected || st.Addr == "" {
		t.Fatalf("status=%+v", st)
	}
	if err := s.SetIO(IOPoint{Type: "DO", Port: 2}, 1); err != nil {
		t.Fatal(err)
	}
	if got := io.get("DO", 2); got != 1 {
		t.Fatalf("控制器侧值为 %v", got)
	}

	rows, err := s.GetIO([]IOPoint{{Type: "DO", Port: 2}, {Type: "DI", Port: 0}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Value != 1 || rows[1].Value != 0 {
		t.Fatalf("rows=%+v", rows)
	}
}

// 输入点位要先开这一路的强制标志，再 SetIOValue。假控制器按真机行为：
// 没开强制的 DI 写入一律回成功、值不变；开了之后才改得动。
func TestServiceCanForceInput(t *testing.T) {
	s, io := newTestServiceWith(t, true)

	if err := s.SetIOForced(IOPoint{Type: "DI", Port: 3}, true); err != nil {
		t.Fatal(err)
	}
	req := io.lastFlagReq()
	if req == nil {
		t.Fatal("没有发出 SetIOForcedFlag")
	}
	if req["type"] != "DI" {
		t.Errorf("type=%v，应当是 DI", req["type"])
	}
	if req["port"] != float64(3) {
		t.Errorf("port=%v，应当是 3", req["port"])
	}
	if req["value"] != float64(1) {
		t.Errorf("value=%v，打开强制应当发 1", req["value"])
	}

	if err := s.SetIO(IOPoint{Type: "DI", Port: 3}, 1); err != nil {
		t.Fatal(err)
	}
	if io.get("DI", 3) != 1 {
		t.Fatal("开了强制之后 DI 应当被写入")
	}

	rows, err := s.GetIO([]IOPoint{{Type: "DI", Port: 3}})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Value != 1 {
		t.Fatalf("rows=%+v", rows)
	}
}

func TestServiceRejectsForceOnOutput(t *testing.T) {
	s, _ := newTestService(t)

	if err := s.SetIOForced(IOPoint{Type: "DO", Port: 1}, true); err == nil ||
		!strings.Contains(err.Error(), "不是输入") {
		t.Fatalf("err=%v", err)
	}
}

// 关掉某一路的强制之后，再写就又不生效了。
func TestServiceUnforceStopsInputWrite(t *testing.T) {
	s, io := newTestServiceWith(t, true)
	p := IOPoint{Type: "DI", Port: 3}

	if err := s.SetIOForced(p, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetIO(p, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.SetIOForced(p, false); err != nil {
		t.Fatal(err)
	}
	if io.lastFlagReq()["value"] != float64(0) {
		t.Errorf("关掉强制应当发 value=0，实际 %v", io.lastFlagReq()["value"])
	}
	if err := s.SetIO(p, 0); err == nil {
		t.Fatal("强制已经关掉，写入不该报成功")
	}
}

// 断开时要把本会话打开的强制标志清掉，否则物理输入会一直被盖住。
func TestServiceDisconnectUnforces(t *testing.T) {
	s, io := newTestServiceWith(t, true)

	if err := s.SetIOForced(IOPoint{Type: "DI", Port: 3}, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetIOForced(IOPoint{Type: "DI", Port: 4}, true); err != nil {
		t.Fatal(err)
	}
	if st := s.Disconnect(); st.Connected {
		t.Fatalf("status=%+v", st)
	}

	flags := io.flagHistory()
	if len(flags) < 4 {
		t.Fatalf("应当先开两路再关两路，实际 flags=%+v", flags)
	}
	off := 0
	for _, f := range flags {
		if f.Value == 0 {
			off++
		}
	}
	if off < 2 {
		t.Fatalf("断开时应当把打开的强制关掉，flags=%+v", flags)
	}
	if io.isForced("DI", 3) || io.isForced("DI", 4) {
		t.Fatal("断开后强制标志还亮着")
	}
}

func TestServiceSetIOForcedAll(t *testing.T) {
	s, io := newTestServiceWith(t, true)
	pts := []IOPoint{{Type: "DI", Port: 3}, {Type: "DI", Port: 4}}

	if err := s.SetIOForcedAll(pts, true); err != nil {
		t.Fatal(err)
	}
	if !io.isForced("DI", 3) || !io.isForced("DI", 4) {
		t.Fatal("两路 DI 都应当被强制")
	}
	if err := s.SetIO(IOPoint{Type: "DI", Port: 3}, 1); err != nil {
		t.Fatal(err)
	}

	if err := s.SetIOForcedAll(pts, false); err != nil {
		t.Fatal(err)
	}
	if io.isForced("DI", 3) || io.isForced("DI", 4) {
		t.Fatal("应当全部关掉")
	}
}

func TestServiceSetIOForcedAllRejectsNonDI(t *testing.T) {
	s, _ := newTestService(t)
	if err := s.SetIOForcedAll([]IOPoint{{Type: "DO", Port: 1}}, true); err == nil ||
		!strings.Contains(err.Error(), "不是 DI") {
		t.Fatalf("err=%v", err)
	}
	if err := s.SetIOForcedAll(nil, true); err == nil {
		t.Fatal("空列表应当报错")
	}
}

func TestServiceRejectsUnknownIOType(t *testing.T) {
	s, _ := newTestService(t)

	if err := s.SetIO(IOPoint{Type: "XX", Port: 0}, 1); err == nil ||
		!strings.Contains(err.Error(), "不认识") {
		t.Fatalf("err=%v", err)
	}
	if err := s.SetIO(IOPoint{Type: "DO", Port: -1}, 1); err == nil {
		t.Fatal("负数端口应当报错")
	}
}

// toggle 读的是控制器的当前值，不是界面记的上一次点击。
func TestServiceToggleReadsCurrentValue(t *testing.T) {
	s, io := newTestService(t)

	got, err := s.ToggleIO(IOPoint{Type: "DO", Port: 5}, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1 || io.get("DO", 5) != 1 {
		t.Fatalf("第一次 toggle 应当置 1，got=%v", got)
	}

	got, err = s.ToggleIO(IOPoint{Type: "DO", Port: 5}, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 || io.get("DO", 5) != 0 {
		t.Fatalf("第二次 toggle 应当回 0，got=%v", got)
	}

	// 现场把点位改了，toggle 要按现场的值走。
	io.apply("DO", 5, 1)
	got, err = s.ToggleIO(IOPoint{Type: "DO", Port: 5}, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 {
		t.Fatalf("点位已被外部置 1，toggle 应当写 0，got=%v", got)
	}
}

func TestServicePulseRestoresValue(t *testing.T) {
	s, io := newTestService(t)

	start := time.Now()
	if err := s.PulseIO(IOPoint{Type: "DO", Port: 1}, 1, 0, 50); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < 40*time.Millisecond {
		t.Fatalf("脉冲没有等够时间：%s", elapsed)
	}

	writes := io.history()
	if len(writes) != 2 || writes[0].Value != 1 || writes[1].Value != 0 {
		t.Fatalf("writes=%+v", writes)
	}
	if io.get("DO", 1) != 0 {
		t.Fatal("脉冲结束后点位没有恢复")
	}
}

func TestServiceDisconnectStopsCalls(t *testing.T) {
	s, _ := newTestService(t)

	if st := s.Disconnect(); st.Connected {
		t.Fatalf("status=%+v", st)
	}
	if err := s.SetIO(IOPoint{Type: "DO", Port: 1}, 1); err == nil {
		t.Fatal("断开后仍然写成功")
	}
}

func TestServiceConfigIsUsable(t *testing.T) {
	useTempConfigDir(t)

	s := newService()
	cfg := s.Config()

	if cfg.Warning != "" {
		t.Fatalf("内置配置不可用：%s", cfg.Warning)
	}
	if len(cfg.Tabs) == 0 {
		t.Fatal("配置里没有标签页")
	}
}

// 保存点位之后 Config() 立刻是新的：这就是「改完立即生效」的全部含义，
// 不用重启程序，也不用重新构建。
func TestServiceSavePanelTakesEffect(t *testing.T) {
	useTempConfigDir(t)
	s := newService()

	saved, err := s.SavePanel(Tab{
		Kind:   kindIO,
		Title:  "现场IO",
		Groups: []Group{{Title: "输出", Points: []Point{{Label: "夹紧", Type: "do", Port: 7}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, cfg := range []Settings{saved, s.Config()} {
		io := findTab(cfg, kindIO)
		if io == nil || io.Title != "现场IO" || io.Groups[0].Points[0].Label != "夹紧" {
			t.Fatalf("保存后没有立即生效：%+v", io)
		}
		// 返回的是归一化之后那份：前端拿它直接替换本地清单，不该还带着没归一的值。
		if io.Groups[0].Points[0].Type != "DO" || io.Groups[0].Points[0].OnValue != 1 {
			t.Fatalf("返回的不是归一化后的点位：%+v", io.Groups[0].Points[0])
		}
		// 只保存了 IO，寄存器那一页得原样留着。
		if findTab(cfg, kindRegister) == nil {
			t.Fatal("寄存器标签页被牵连了")
		}
	}

	// 重新起一个 Service 读盘，验证真的落下去了而不只是改了内存。
	if io := findTab(newService().Config(), kindIO); io == nil || io.Title != "现场IO" {
		t.Fatalf("没有落盘：%+v", io)
	}
}

func TestServiceSavePanelRejectsBadPointsWithoutWriting(t *testing.T) {
	useTempConfigDir(t)
	s := newService()
	before := s.Config()

	_, err := s.SavePanel(Tab{
		Kind:   kindRegister,
		Groups: []Group{{Points: []Point{{Label: "错的", Type: "DO", Port: 1}}}},
	})
	if err == nil {
		t.Fatal("寄存器页里的 DO 应当被拒")
	}
	if _, _, rerr := readStore(registerFileName); !errors.Is(rerr, errNoOverride) {
		t.Fatal("被拒的保存不该落盘")
	}
	if got := findTab(s.Config(), kindRegister); got == nil ||
		got.Title != findTab(before, kindRegister).Title {
		t.Fatal("被拒的保存不该动内存里那份")
	}
}

func TestServiceSaveDeviceTakesEffect(t *testing.T) {
	useTempConfigDir(t)
	s := newService()

	saved, err := s.SaveDevice(DeviceSettings{
		Device:                Device{Host: "10.9.8.7", Port: 9100, Path: "ws"},
		ConnectTimeoutSeconds: 9,
		RequestTimeoutSeconds: 11,
		RefreshIntervalMs:     2000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if saved.Device.Host != "10.9.8.7" || saved.Device.Path != "/ws" {
		t.Fatalf("device=%+v", saved.Device)
	}
	// 超时立刻按新值算，不用重启。
	if s.requestTimeout() != 11*time.Second || s.connectTimeout() != 9*time.Second {
		t.Fatalf("超时没跟着变：%v/%v", s.connectTimeout(), s.requestTimeout())
	}
	if s.Config().RefreshIntervalMs != 2000 {
		t.Fatalf("刷新间隔没跟着变：%d", s.Config().RefreshIntervalMs)
	}
	// 标签页不该被连接参数的保存牵连掉。
	if len(s.Config().Tabs) != len(saved.Tabs) || len(saved.Tabs) == 0 {
		t.Fatalf("标签页被牵连了：%d", len(saved.Tabs))
	}
}

func TestServiceSaveDeviceRejectsOutOfRangeWithoutWriting(t *testing.T) {
	useTempConfigDir(t)
	s := newService()

	for _, in := range []DeviceSettings{
		{Device: Device{Host: "10.0.0.1", Port: 70000}},
		{Device: Device{Host: "10.0.0.1"}, RequestTimeoutSeconds: 9999},
		{Device: Device{Host: "10.0.0.1"}, RefreshIntervalMs: 50},
	} {
		if _, err := s.SaveDevice(in); err == nil {
			t.Fatalf("越界的值应当被拒：%+v", in)
		}
	}
	if _, _, err := readStore(deviceFileName); !errors.Is(err, errNoOverride) {
		t.Fatal("被拒的保存不该落盘")
	}
}

// 改配置不碰连接：正在盯着某一路信号的人不该因为别人改了个点位名而丢一次连接。
func TestServiceSaveKeepsConnection(t *testing.T) {
	useTempConfigDir(t)
	s, io := newTestService(t)
	addr := s.Status().Addr

	if _, err := s.SavePanel(Tab{
		Kind:   kindIO,
		Groups: []Group{{Points: []Point{{Type: "DO", Port: 3}}}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveDevice(DeviceSettings{Device: Device{Host: "10.9.8.7"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResetPanel(kindIO); err != nil {
		t.Fatal(err)
	}

	st := s.Status()
	if !st.Connected || st.Addr != addr {
		t.Fatalf("连接被动过了：%+v", st)
	}
	// 保存配置不该顺手向控制器发点什么。
	if len(io.history()) != 0 || len(io.regHistory()) != 0 || len(io.flagHistory()) != 0 {
		t.Fatal("保存配置期间向控制器发了请求")
	}
	// 连着的还是原来那台，写得动。
	if err := s.SetIO(IOPoint{Type: "DO", Port: 3}, 1); err != nil {
		t.Fatal(err)
	}
}

func TestServiceResetGoesBackToFactoryDefaults(t *testing.T) {
	useTempConfigDir(t)
	s := newService()
	factory := s.Config()

	if _, err := s.SavePanel(Tab{
		Kind:   kindIO,
		Title:  "改过的",
		Groups: []Group{{Points: []Point{{Type: "DO", Port: 9}}}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveDevice(DeviceSettings{Device: Device{Host: "10.9.8.7"}}); err != nil {
		t.Fatal(err)
	}

	back, err := s.ResetPanel(kindIO)
	if err != nil {
		t.Fatal(err)
	}
	if findTab(back, kindIO).Title != findTab(factory, kindIO).Title {
		t.Fatalf("IO 没退回出厂默认：%+v", findTab(back, kindIO))
	}
	// 只恢复了 IO，连接参数那份该留着。
	if back.Device.Host != "10.9.8.7" {
		t.Fatalf("连接参数被一起恢复了：%+v", back.Device)
	}

	back, err = s.ResetDevice()
	if err != nil {
		t.Fatal(err)
	}
	if back.Device.Host != factory.Device.Host {
		t.Fatalf("连接参数没退回出厂默认：%+v", back.Device)
	}
	// 连点两次「恢复默认」不该报错。
	if _, err := s.ResetDevice(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResetPanel("magic"); err == nil {
		t.Fatal("不认识的类型应当报错")
	}
}

func TestServiceGetAndToggleRegister(t *testing.T) {
	s, io := newTestService(t)

	rows, err := s.GetRegisters([]int{10000, 20000})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Address != 10000 || rows[0].Value != "0" {
		t.Fatalf("rows=%+v", rows)
	}

	got, err := s.ToggleRegister(10000, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1 || io.getReg(10000) != 1 {
		t.Fatalf("第一次 toggle 应当置 1，got=%v", got)
	}

	got, err = s.ToggleRegister(10000, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != 0 || io.getReg(10000) != 0 {
		t.Fatalf("再 toggle 应当回到 0，got=%v", got)
	}
}

func TestServicePulseRegister(t *testing.T) {
	s, io := newTestService(t)

	if err := s.PulseRegister(10000, 1, 0, 20); err != nil {
		t.Fatal(err)
	}
	hist := io.regHistory()
	if len(hist) != 2 || hist[0].Value != 1 || hist[1].Value != 0 {
		t.Fatalf("应当先写 1 再写回 0，实际 %+v", hist)
	}
	if io.getReg(10000) != 0 {
		t.Fatal("点动结束后应当停在 offValue")
	}
}

func TestServiceRejectsNegativeRegisterAddress(t *testing.T) {
	s, _ := newTestService(t)
	if _, err := s.GetRegisters([]int{-1}); err == nil {
		t.Fatal("负数地址应当报错")
	}
	if _, err := s.ToggleRegister(-1, 1, 0); err == nil {
		t.Fatal("负数地址应当报错")
	}
	if err := s.PulseRegister(-1, 1, 0, 20); err == nil {
		t.Fatal("负数地址应当报错")
	}
}

func TestServiceApplyImportedPanelTakesEffect(t *testing.T) {
	useTempConfigDir(t)
	s := newService()

	raw := []byte(`{
		"id": "io",
		"title": "导入的IO",
		"groups": [{"title": "输出", "points": [{"label": "夹紧", "type": "do", "port": 7}]}]
	}`)
	saved, err := s.applyImportedPanel(kindIO, raw, "import-io.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, cfg := range []Settings{saved, s.Config()} {
		io := findTab(cfg, kindIO)
		if io == nil || io.Title != "导入的IO" || io.Groups[0].Points[0].Label != "夹紧" {
			t.Fatalf("导入后没有立即生效：%+v", io)
		}
		if io.Groups[0].Points[0].Type != "DO" || io.Groups[0].Points[0].OnValue != 1 {
			t.Fatalf("导入后不是归一化后的点位：%+v", io.Groups[0].Points[0])
		}
		if findTab(cfg, kindRegister) == nil {
			t.Fatal("寄存器标签页被牵连了")
		}
	}
	if io := findTab(newService().Config(), kindIO); io == nil || io.Title != "导入的IO" {
		t.Fatalf("没有落盘：%+v", io)
	}
}

func TestServiceApplyImportedPanelRejectsBadPointsWithoutWriting(t *testing.T) {
	useTempConfigDir(t)
	s := newService()
	before := s.Config()

	_, err := s.applyImportedPanel(kindRegister, []byte(`{
		"groups": [{"title": "错的", "points": [{"label": "错的", "type": "DO", "port": 1}]}]
	}`), "bad.json")
	if err == nil {
		t.Fatal("寄存器页里的 DO 应当被拒")
	}
	if _, _, rerr := readStore(registerFileName); !errors.Is(rerr, errNoOverride) {
		t.Fatalf("坏文件不该落盘：%v", rerr)
	}
	if findTab(s.Config(), kindRegister).Title != findTab(before, kindRegister).Title {
		t.Fatal("拒收之后内存里的寄存器页不该变")
	}
}

func TestServiceApplyImportedPanelRejectsWrongKind(t *testing.T) {
	useTempConfigDir(t)
	s := newService()

	_, err := s.applyImportedPanel(kindIO, []byte(`{
		"kind": "register",
		"groups": [{"title": "焊接", "points": [{"label": "就绪", "type": "BOOL", "port": 10000}]}]
	}`), "register.json")
	if err == nil {
		t.Fatal("把寄存器配置导进 IO 页应当被拒")
	}
	if !strings.Contains(err.Error(), "register") {
		t.Fatalf("错误应当点明文件类型：%v", err)
	}
	if _, _, rerr := readStore(ioFileName); !errors.Is(rerr, errNoOverride) {
		t.Fatalf("错页导入不该写 IO 文件：%v", rerr)
	}
}

func TestServicePanelBytesExportsCurrent(t *testing.T) {
	useTempConfigDir(t)
	s := newService()
	if _, err := s.SavePanel(Tab{
		Kind:   kindIO,
		Title:  "现场IO",
		Groups: []Group{{Title: "输出", Points: []Point{{Label: "夹紧", Type: "DO", Port: 7}}}},
	}); err != nil {
		t.Fatal(err)
	}

	raw, src, err := s.panelBytes(kindIO)
	if err != nil {
		t.Fatal(err)
	}
	if src.file != ioFileName {
		t.Fatalf("默认文件名应当是现场那份：%s", src.file)
	}
	tab, err := buildPanel(raw, "export.json", src)
	if err != nil {
		t.Fatal(err)
	}
	if tab.Title != "现场IO" || tab.Groups[0].Points[0].Label != "夹紧" {
		t.Fatalf("导出的不是当前这一页：%+v", tab)
	}
}

func TestServiceExportImportNeedStartup(t *testing.T) {
	s := newService()
	if _, err := s.ExportPanel(kindIO); err == nil {
		t.Fatal("没 Startup 不该弹出导出框")
	}
	if _, err := s.ImportPanel(kindIO); err == nil {
		t.Fatal("没 Startup 不该弹出导入框")
	}
}

func TestServiceSaveFlowTakesEffect(t *testing.T) {
	useTempConfigDir(t)
	s := newService()

	saved, err := s.SavePanel(Tab{
		Kind:  kindIOFlow,
		Title: "现场流程",
		Steps: []FlowStep{{Label: "开门", Type: "do", Port: 3, Action: "pulse", PulseMs: 200}},
	})
	if err != nil {
		t.Fatal(err)
	}
	flow := findTab(saved, kindIOFlow)
	if flow == nil || flow.Title != "现场流程" || len(flow.Steps) != 1 || flow.Steps[0].Action != "pulse" {
		t.Fatalf("保存后没有立即生效：%+v", flow)
	}
	if findTab(saved, kindIO) == nil {
		t.Fatal("IO 标签页被牵连了")
	}
	if flow := findTab(newService().Config(), kindIOFlow); flow == nil || flow.Title != "现场流程" {
		t.Fatalf("没有落盘：%+v", flow)
	}
}

func TestServiceRunFlowStepPulsesDO(t *testing.T) {
	useTempConfigDir(t)
	s, io := newTestService(t)
	if _, err := s.SavePanel(Tab{
		Kind:  kindIOFlow,
		Steps: []FlowStep{{Label: "开门", Type: "DO", Port: 3, Action: "on"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.RunFlowStep(0); err != nil {
		t.Fatal(err)
	}
	if io.get("DO", 3) != 1 {
		t.Fatal("第 1 步应当把 DO3 写成 ON")
	}
	if err := s.RunFlowStep(1); err == nil {
		t.Fatal("没有第 2 步")
	}
}

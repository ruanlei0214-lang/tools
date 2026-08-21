package remote

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// 订阅后控制器推的第一帧就该被收进来；之后的查询用缓存，不该再发订阅。
func TestGetRobotStatusSubscribesOnce(t *testing.T) {
	useTempConfigDir(t)

	var subs atomic.Int32
	host, port := startServer(t, "/", func(send sendFunc, req envelope) {
		if req.Ty == tyRobotStatus {
			subs.Add(1)
			// 订阅报文带 id 或 db 的话真机会直接沉默（实测），这里把它们钉死。
			if req.ID != nil {
				t.Error("订阅请求不该带 id")
			}
			if req.DB != nil {
				t.Error("订阅请求不该带 db")
			}
			send(map[string]any{"ty": tyRobotStatus, "db": map[string]any{
				"mode": 2, "state": 0, "stateName": "未使能",
			}})
		}
	})

	s := newService()
	if _, err := s.Connect(Device{Host: host, Port: port, Path: "/"}); err != nil {
		t.Fatal(err)
	}
	defer s.Disconnect()

	st, err := s.GetRobotStatus()
	if err != nil {
		t.Fatal(err)
	}
	if st.State != 0 || st.Mode != 2 {
		t.Fatalf("st=%+v", st)
	}
	// 展示用的名字由后端补齐：推送里带的 stateName 原样保留，模式名按表翻。
	if st.StateName != "未使能" || st.ModeName != "远程" {
		t.Fatalf("名字没补对：%+v", st)
	}

	if _, err := s.GetRobotStatus(); err != nil {
		t.Fatal(err)
	}
	if n := subs.Load(); n != 1 {
		t.Fatalf("订阅发了 %d 次，应当只有 1 次", n)
	}
}

// 状态变化时控制器会再推一帧，缓存要跟着换。
func TestGetRobotStatusTracksLaterPushes(t *testing.T) {
	useTempConfigDir(t)

	var subs atomic.Int32
	host, port := startServer(t, "/", func(send sendFunc, req envelope) {
		if req.Ty == tyRobotStatus {
			// 订阅时先推一帧「未使能」，紧跟着补一帧状态变化。
			subs.Add(1)
			send(map[string]any{"ty": tyRobotStatus, "db": map[string]any{"state": 0}})
			send(map[string]any{"ty": tyRobotStatus, "db": map[string]any{"state": 1, "stateName": "使能中"}})
		}
	})

	s := newService()
	if _, err := s.Connect(Device{Host: host, Port: port, Path: "/"}); err != nil {
		t.Fatal(err)
	}
	defer s.Disconnect()

	// 首帧可能是 0 也可能已经是 1（两帧紧挨着到），等到 1 出现再断言缓存。
	deadline := time.Now().Add(2 * time.Second)
	for {
		st, err := s.GetRobotStatus()
		if err != nil {
			t.Fatal(err)
		}
		if st.State == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("缓存没跟上后来的推送：%+v", st)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if n := subs.Load(); n != 1 {
		t.Fatalf("订阅发了 %d 次，应当只有 1 次", n)
	}
}

// 控制器沉默（不支持订阅）时要报错，不能一直等。
func TestGetRobotStatusTimesOut(t *testing.T) {
	useTempConfigDir(t)

	host, port := startServer(t, "/", func(send sendFunc, req envelope) {})

	s := newService()
	s.cfgMu.Lock()
	s.settings.RequestTimeoutSeconds = 1
	s.cfgMu.Unlock()

	if _, err := s.Connect(Device{Host: host, Port: port, Path: "/"}); err != nil {
		t.Fatal(err)
	}
	defer s.Disconnect()

	if _, err := s.GetRobotStatus(); err == nil || !strings.Contains(err.Error(), "超时") {
		t.Fatalf("err=%v，期望超时", err)
	}
}

// 未连接时查询要如实报未连接，而不是凭空等推送。
func TestGetRobotStatusNeedsConnection(t *testing.T) {
	useTempConfigDir(t)
	s := newService()
	if _, err := s.GetRobotStatus(); err == nil || !strings.Contains(err.Error(), "尚未连接") {
		t.Fatalf("err=%v", err)
	}
}

// 推送里没带 stateName 时按文档的表补齐；带了的原样保留。
func TestRobotStatusNormalizeFillsNames(t *testing.T) {
	st := RobotStatus{State: 3, Mode: 0}
	st.normalize()
	if st.StateName != "点动中" || st.ModeName != "手动" {
		t.Fatalf("补齐不对：%+v", st)
	}

	st = RobotStatus{State: 1, StateName: "使能中"}
	st.normalize()
	if st.StateName != "使能中" {
		t.Fatalf("推送里带的名字不该被盖掉：%+v", st)
	}
}

func TestCheckRebootAllowed(t *testing.T) {
	// state 为 0（文档的未使能）放行。
	if err := checkRebootAllowed(RobotStatus{State: 0}); err != nil {
		t.Fatalf("未使能应当放行：%v", err)
	}
	// 控制器自己报「未使能」时 state 不是 0 也放行——文档的状态表和真机对不上。
	if err := checkRebootAllowed(RobotStatus{State: 7, StateName: "未使能"}); err != nil {
		t.Fatalf("stateName 是未使能应当放行：%v", err)
	}
	for _, state := range []int{1, 2, 3, 4, 5} {
		err := checkRebootAllowed(RobotStatus{State: state})
		if err == nil || !strings.Contains(err.Error(), "不允许重启") {
			t.Fatalf("state=%d 应当被拒：%v", state, err)
		}
	}
	// 推送里带了 stateName 就用它，别拿本地翻的表盖住控制器的说法。
	err := checkRebootAllowed(RobotStatus{State: 2, StateName: "已使能"})
	if err == nil || !strings.Contains(err.Error(), "已使能") {
		t.Fatalf("err=%v，期望带上状态名", err)
	}
}

// 使能状态下重启必须在 SSH 之前就被拦下。
func TestRebootControllerRefusesWhenEnabled(t *testing.T) {
	useTempConfigDir(t)

	host, port := startServer(t, "/", func(send sendFunc, req envelope) {
		if req.Ty == tyRobotStatus {
			send(map[string]any{"ty": tyRobotStatus, "db": map[string]any{"state": 1, "stateName": "使能中"}})
		}
	})

	s := newService()
	if _, err := s.Connect(Device{Host: host, Port: port, Path: "/"}); err != nil {
		t.Fatal(err)
	}
	defer s.Disconnect()

	err := s.RebootController()
	if err == nil || !strings.Contains(err.Error(), "不允许重启") {
		t.Fatalf("err=%v，期望在状态检查处被拦下", err)
	}
}

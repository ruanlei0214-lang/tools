package remote

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// tyRobotStatus 是接口文档「主题订阅/推送接口说明」里的机器人状态主题。
// 订阅后控制器在开始订阅时推一帧，之后状态每变一次推一帧。
const tyRobotStatus = "publish/RobotStatus"

// stateDisabled 是文档里「未使能」的状态值。重启控制器只在这个状态下放行。
const stateDisabled = 0

// RobotStatus 是 publish/RobotStatus 推送里本模块关心的字段。推送里还有很多
// 别的字段（倍率、工具号之类），用不上就不进结构体。
type RobotStatus struct {
	// Mode：0=手动 1=自动 2=远程。
	Mode int `json:"mode"`
	// State：文档说是 0=未使能 1=使能中 2=空闲 3=点动中 4=RunTo 5=拖动中，
	// 但实测这台控制器（S5-90-ECO-V2）state=2 时自带的 stateName 是「已使能」——
	// 文档的表和真机对不上。所以展示一律用控制器自己给的 stateName，
	// 重启放行也认 stateName，见 checkRebootAllowed。
	State int `json:"state"`
	// StateName 是控制器自己给的状态名；推送里没带时由后端按文档的表补齐，
	// 前端直接用，不再各翻一份表。
	StateName string `json:"stateName"`
	// ModeName 推送里永远没有，一律由后端补上。
	ModeName     string `json:"modeName"`
	IsSimulation bool   `json:"isSimulation"`
}

// normalize 补齐展示用的名字，缓存在 tracker 里的都是补过的。
func (st *RobotStatus) normalize() {
	if st.StateName == "" {
		st.StateName = stateName(st.State)
	}
	st.ModeName = modeName(st.Mode)
}

func stateName(state int) string {
	switch state {
	case 0:
		return "未使能"
	case 1:
		return "使能中"
	case 2:
		return "空闲"
	case 3:
		return "点动中"
	case 4:
		return "RunTo"
	case 5:
		return "拖动中"
	}
	return fmt.Sprintf("状态 %d", state)
}

func modeName(mode int) string {
	switch mode {
	case 0:
		return "手动"
	case 1:
		return "自动"
	case 2:
		return "远程"
	}
	return fmt.Sprintf("模式 %d", mode)
}

// checkRebootAllowed 只在未使能时放行。带着使能重启控制器，正在动的轴会失控。
//
// 放行看两处：state 为 0（文档的未使能），或控制器自己报的 stateName 是「未使能」。
// 单看 state 不够——文档的状态表和真机对不上（这台 state=2 叫「已使能」），
// 万一另一台固件的未使能不是 0，按文档表拦就把能重启的也拦死了；反过来
// stateName 说「未使能」时 state 不是 0，也该放行。
func checkRebootAllowed(st RobotStatus) error {
	st.normalize()
	if st.State == stateDisabled || st.StateName == "未使能" {
		return nil
	}
	return fmt.Errorf("机器人当前处于「%s」，不允许重启：请先把机器人打到未使能，再重启控制器", st.StateName)
}

// robotTracker 跟着一条连接走：缓存最新一帧 RobotStatus，ready 在首帧到达时关闭。
// 推送只在「开始订阅」和「状态变化」时到达，所以缓存着的永远是最新状态，
// 之后每次查询不用再等控制器。
//
// onPush 只被 client 的读协程调用，天然串行；锁是为了和 GetRobotStatus 的读者隔开。
type robotTracker struct {
	mu         sync.Mutex
	st         *RobotStatus
	ready      chan struct{}
	subscribed bool
}

func newRobotTracker() *robotTracker {
	return &robotTracker{ready: make(chan struct{})}
}

func (t *robotTracker) onPush(ty string, db json.RawMessage) {
	if ty != tyRobotStatus {
		return
	}
	var st RobotStatus
	if json.Unmarshal(db, &st) != nil {
		return
	}
	st.normalize()
	t.mu.Lock()
	first := t.st == nil
	t.st = &st
	t.mu.Unlock()
	if first {
		close(t.ready)
	}
}

// GetRobotStatus 返回控制器推送的机器人状态。第一次调用发订阅并等首帧推送；
// 之后订阅还活着，直接回缓存里最新的一帧，不再让界面等一个来回。
func (s *Service) GetRobotStatus() (RobotStatus, error) {
	c, tr, err := s.clientAndTracker()
	if err != nil {
		return RobotStatus{}, err
	}

	tr.mu.Lock()
	st := tr.st
	subscribed := tr.subscribed
	tr.subscribed = true
	tr.mu.Unlock()

	if !subscribed {
		if err := c.subscribe(tyRobotStatus); err != nil {
			tr.mu.Lock()
			tr.subscribed = false
			tr.mu.Unlock()
			return RobotStatus{}, err
		}
	}
	if st != nil {
		return *st, nil
	}

	select {
	case <-tr.ready:
		tr.mu.Lock()
		defer tr.mu.Unlock()
		return *tr.st, nil
	case <-c.done:
		return RobotStatus{}, c.closedErr()
	case <-time.After(s.requestTimeout()):
		return RobotStatus{}, errors.New("等待控制器推送机器人状态超时：这台控制器可能不支持状态订阅")
	}
}

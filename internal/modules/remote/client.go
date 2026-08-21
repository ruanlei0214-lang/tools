package remote

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// pingInterval 是空闲时的保活间隔。自动刷新关掉之后这条连接可能长时间没有流量，
// 中间隔着的反向代理或控制器自己的空闲超时会把它悄悄掐掉，等下次点按钮才发现。
const pingInterval = 30 * time.Second

// writeTimeout 是 send 的写超时。call 的写超时由调用方给，send 没有响应可等，
// 写本身不该无限期堵着。
const writeTimeout = 5 * time.Second

// probePaths 是配置里的路径连不上时依次再试的候选。
//
// 接口文档只写了 TCP 那一套，没说 WebSocket 挂在哪个路径上。与其让现场对着
// 「握手失败」猜，不如把常见的几个试一遍——试出来哪个能用会显示在界面上，
// 照着钉回 config.json 的 path 就不用每次再探。
var probePaths = []string{"/", "/ws", "/websocket", "/api"}

// client 是一条到控制器的 WebSocket 长连接。
//
// 用长连接而不是每次调用现连：IO 按钮是连着点的，一次点击一次握手既慢又让
// 控制器侧不断新建会话。写请求靠 writeMu 串行（gorilla 不允许并发写），响应由一个
// 读协程按 id 分发回各自的等待者——控制器可以主动推送，也可能乱序回包，只认自己那条。
type client struct {
	conn *websocket.Conn
	addr string

	writeMu sync.Mutex

	mu      sync.Mutex
	pending map[string]chan envelope
	// onPush 处理「没人等的包」：订阅推送（publish/...）走这里。建连后由 Service 挂上，
	// 读写都在 mu 保护下——readLoop 从连接建立起就在跑，直接写字段会撞上它。
	onPush func(ty string, db json.RawMessage)

	seq       atomic.Uint64
	closeOnce sync.Once
	closeErr  atomic.Value // error
	done      chan struct{}
}

type envelope struct {
	ID  any             `json:"id"`
	Ty  string          `json:"ty"`
	DB  json.RawMessage `json:"db"`
	Err string          `json:"err"`
}

func dial(host string, port int, path string, timeout time.Duration) (*client, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return nil, errors.New("请填写控制器 IP")
	}
	if port <= 0 {
		port = defaultPort
	}
	if port > 65535 {
		return nil, fmt.Errorf("端口无效：%d", port)
	}

	hostPort := net.JoinHostPort(host, strconv.Itoa(port))
	dialer := &websocket.Dialer{HandshakeTimeout: timeout}

	tried := candidatePaths(path)
	var firstErr error
	for _, p := range tried {
		u := url.URL{Scheme: "ws", Host: hostPort, Path: p}
		conn, resp, err := dialer.Dial(u.String(), nil)
		if err == nil {
			return newClient(conn, u.String()), nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		if firstErr == nil {
			firstErr = dialError(u.String(), err, resp)
		}
		// 只有握手被 HTTP 层拒了才值得换个路径再试。主机不可达、端口没人听、
		// 超时——换路径也是一样的结果，别让用户多等三遍。
		if !errors.Is(err, websocket.ErrBadHandshake) {
			return nil, firstErr
		}
	}
	if len(tried) > 1 {
		return nil, fmt.Errorf("%v（另外还试过 %s，都被拒绝）",
			firstErr, strings.Join(tried[1:], "、"))
	}
	return nil, firstErr
}

// candidatePaths 把配置里的路径排在最前，后面接上候选，去掉重复的。
func candidatePaths(configured string) []string {
	out := []string{normalizePath(configured)}
	for _, p := range probePaths {
		if p != out[0] {
			out = append(out, p)
		}
	}
	return out
}

func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		return "/" + p
	}
	return p
}

// dialError 优先报 HTTP 状态码。gorilla 在握手被拒时只给一句
// 「bad handshake」，404 和 401 得看状态码才分得清。
func dialError(u string, err error, resp *http.Response) error {
	if resp != nil {
		return fmt.Errorf("连接 %s 失败：%s", u, resp.Status)
	}
	return fmt.Errorf("连接 %s 失败：%v", u, err)
}

func newClient(conn *websocket.Conn, addr string) *client {
	c := &client{
		conn:    conn,
		addr:    addr,
		pending: make(map[string]chan envelope),
		done:    make(chan struct{}),
	}
	go c.readLoop()
	go c.pingLoop()
	return c
}

// readLoop 是这条连接上唯一的读者。WebSocket 自带消息边界，一帧就是一条完整的
// JSON，不用再自己拆包。
func (c *client) readLoop() {
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			c.closeWith(fmt.Errorf("与控制器的连接已断开：%v", err))
			return
		}

		var resp envelope
		if err := json.Unmarshal(data, &resp); err != nil {
			// 不是 JSON 就不是这套协议的东西，丢掉继续读。
			continue
		}

		id := normalizeID(resp.ID)
		c.mu.Lock()
		ch, ok := c.pending[id]
		if ok {
			delete(c.pending, id)
		}
		push := c.onPush
		c.mu.Unlock()

		if ok {
			ch <- resp
		} else if push != nil && resp.Ty != "" {
			// 没人等的包是控制器的主动推送（订阅的状态、报警之类），交给 onPush。
			push(resp.Ty, resp.DB)
		}
	}
}

// setOnPush 挂上推送处理。在持锁读它的 readLoop 面前，直接赋值不是安全的。
func (c *client) setOnPush(fn func(ty string, db json.RawMessage)) {
	c.mu.Lock()
	c.onPush = fn
	c.mu.Unlock()
}

// pingLoop 定期发控制帧保活。WriteControl 可以和普通写并发，不用抢 writeMu。
func (c *client) pingLoop() {
	t := time.NewTicker(pingInterval)
	defer t.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-t.C:
			err := c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second))
			if err != nil {
				c.closeWith(fmt.Errorf("与控制器的连接已断开：%v", err))
				return
			}
		}
	}
}

func (c *client) alive() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

func (c *client) closeWith(err error) {
	c.closeOnce.Do(func() {
		if err != nil {
			c.closeErr.Store(err)
		}
		close(c.done)
		// 不走 1 秒的关闭握手：对端不回 Close 时 WriteControl 会把「断开」卡住。
		// pending 的 call 已经能从 done 退出来，直接拆 TCP 即可。
		_ = c.conn.SetWriteDeadline(time.Now())
		_ = c.conn.Close()
	})
}

func (c *client) Close() {
	c.closeWith(errors.New("连接已关闭"))
}

func (c *client) closedErr() error {
	if v := c.closeErr.Load(); v != nil {
		return v.(error)
	}
	return errors.New("连接已关闭")
}

func (c *client) nextID() string {
	return fmt.Sprintf("t%d-%d", time.Now().UnixNano(), c.seq.Add(1))
}

// writeFrame 串行写一帧。gorilla 不允许并发写，所以抢 writeMu。
// 写失败说明连接已经不可用（写到一半断掉，帧边界就乱了），顺手把整条连接判死。
func (c *client) writeFrame(payload []byte, timeout time.Duration) error {
	c.writeMu.Lock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(timeout))
	err := c.conn.WriteMessage(websocket.TextMessage, payload)
	c.writeMu.Unlock()
	if err != nil {
		c.closeWith(fmt.Errorf("发送失败：%v", err))
		return fmt.Errorf("发送请求失败：%v", err)
	}
	return nil
}

// call 发一条请求并等自己那条响应。一次调用只发一帧，失败不重发。
//
// 不重发是有意的：这些请求全是 IO 动作，重发等于让现场的气缸多动一次。发送失败时
// 连接本身已经不可用了，所以顺手关掉整条连接，让界面如实退回未连接。
//
// 响应超时是另一码事：只结束这次等待，不动连接——控制器慢一次不代表连接坏了，
// 把连接拆了反而让后面每个按钮都要重连。
func (c *client) call(ty string, db any, timeout time.Duration) (json.RawMessage, error) {
	if !c.alive() {
		return nil, c.closedErr()
	}

	id := c.nextID()
	payload, err := json.Marshal(map[string]any{"id": id, "ty": ty, "db": db})
	if err != nil {
		return nil, fmt.Errorf("编码请求失败：%v", err)
	}

	ch := make(chan envelope, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()

	if err := c.writeFrame(payload, timeout); err != nil {
		return nil, err
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case resp := <-ch:
		if resp.Err != "" {
			return nil, fmt.Errorf("控制器返回错误：%s", resp.Err)
		}
		return resp.DB, nil
	case <-c.done:
		return nil, c.closedErr()
	case <-timer.C:
		return nil, fmt.Errorf("等待 %s 响应超时（%s）", ty, timeout)
	}
}

// subscribe 发一帧订阅请求。订阅报文只能有 ty：实测这台控制器对带 id、
// 带 "db":null 的订阅都直接沉默——不报错也不推数据，订阅方只能干等超时。
// 文档里的订阅示例同样只有 ty。订阅也没有响应可等，真正的数据由推送送回来，见 setOnPush。
func (c *client) subscribe(ty string) error {
	if !c.alive() {
		return c.closedErr()
	}
	payload, err := json.Marshal(map[string]any{"ty": ty})
	if err != nil {
		return fmt.Errorf("编码请求失败：%v", err)
	}
	return c.writeFrame(payload, writeTimeout)
}

// normalizeID 把响应里的 id 还原成我们发出去的那个字符串。请求 id 一律是字符串，
// 但文档说 id 可以是数字，encoding/json 会解成 float64，直接 Sprint 会变成 1e+06。
func normalizeID(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case nil:
		return ""
	default:
		return fmt.Sprint(t)
	}
}

func parseIOValues(raw json.RawMessage) ([]IOValue, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, errors.New("控制器未返回 IO 数据")
	}
	var rows []IOValue
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("解析 IO 数据失败：%v", err)
	}
	return rows, nil
}

func parseRegisterValues(raw json.RawMessage) ([]RegisterValue, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, errors.New("控制器未返回寄存器数据")
	}

	var rows []struct {
		Address int             `json:"address"`
		Value   json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("解析寄存器数据失败：%v", err)
	}
	out := make([]RegisterValue, 0, len(rows))
	for _, row := range rows {
		out = append(out, RegisterValue{Address: row.Address, Value: stringifyJSON(row.Value)})
	}
	return out, nil
}

// stringifyJSON 把寄存器值压成字符串。寄存器可以是 bool / 整数 / 浮点，
// 三种类型各在绑定层开一个字段，前端每次都要判空，不如统一成展示用的文本。
func stringifyJSON(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return strings.TrimSpace(string(raw))
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'g', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprint(t)
		}
		return string(b)
	}
}

func parseWriteValue(s string) (any, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("请填写要写入的值")
	}
	switch strings.ToLower(s) {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i, nil
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f, nil
	}
	return s, nil
}

// parseRegisterNumber 把 GetRegisterValue 读回来的文本还原成数字，给翻转比较用。
// 控制器可能回 1、1.0、true，都当成同一个值。
func parseRegisterNumber(s string) (float64, error) {
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "true":
		return 1, nil
	case "false":
		return 0, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("寄存器值 %q 不是数字", s)
	}
	return v, nil
}

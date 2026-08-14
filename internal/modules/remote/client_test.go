package remote

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// sendFunc 把一个值当作一帧文本发给客户端。
type sendFunc func(v any)

// startServer 起一个假控制器，对每条请求调用 handle。handle 不回包就等于控制器沉默。
// 只在 wsPath 上接受握手，其余路径返回 404，用来验证路径探测。
func startServer(t *testing.T, wsPath string, handle func(send sendFunc, req envelope)) (string, int) {
	t.Helper()

	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != wsPath {
			http.NotFound(w, r)
			return
		}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		var mu sync.Mutex
		send := func(v any) {
			b, err := json.Marshal(v)
			if err != nil {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			_ = conn.WriteMessage(websocket.TextMessage, b)
		}

		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var req envelope
			if json.Unmarshal(data, &req) != nil {
				continue
			}
			handle(send, req)
		}
	}))
	t.Cleanup(srv.Close)

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	return u.Hostname(), port
}

func dialTest(t *testing.T, host string, port int) *client {
	t.Helper()
	c, err := dial(host, port, "/", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	return c
}

func TestCallRoundTrip(t *testing.T) {
	host, port := startServer(t, "/", func(send sendFunc, req envelope) {
		send(map[string]any{
			"id": req.ID,
			"ty": req.Ty,
			"db": []map[string]any{{"type": "DO", "port": 3, "value": 1}},
		})
	})

	c := dialTest(t, host, port)
	raw, err := c.call(tyGetIO, []IOPoint{{Type: "DO", Port: 3}}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := parseIOValues(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Port != 3 || rows[0].Value != 1 {
		t.Fatalf("rows=%+v", rows)
	}
}

func TestCallReportsControllerError(t *testing.T) {
	host, port := startServer(t, "/", func(send sendFunc, req envelope) {
		send(map[string]any{"id": req.ID, "ty": req.Ty, "err": "1/bad port"})
	})

	c := dialTest(t, host, port)
	if _, err := c.call(tySetIO, nil, time.Second); err == nil || !strings.Contains(err.Error(), "bad port") {
		t.Fatalf("err=%v", err)
	}
}

// 控制器会主动推状态，那些包没人等；它们不能顶掉真正的响应。
func TestCallIgnoresUnsolicitedPush(t *testing.T) {
	host, port := startServer(t, "/", func(send sendFunc, req envelope) {
		send(map[string]any{"id": "push-1", "ty": "System/notify", "db": []any{}})
		send(map[string]any{"id": req.ID, "ty": req.Ty, "db": []map[string]any{{"address": 10000, "value": 7}}})
	})

	c := dialTest(t, host, port)
	raw, err := c.call(tyGetRegister, []int{10000}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := parseRegisterValues(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Value != "7" {
		t.Fatalf("rows=%+v", rows)
	}
}

func TestCallTimesOutButKeepsConnection(t *testing.T) {
	host, port := startServer(t, "/", func(send sendFunc, req envelope) {
		if req.Ty == tyGetIO {
			return // 装作没听见
		}
		send(map[string]any{"id": req.ID, "ty": req.Ty})
	})

	c := dialTest(t, host, port)
	if _, err := c.call(tyGetIO, nil, 150*time.Millisecond); err == nil || !strings.Contains(err.Error(), "超时") {
		t.Fatalf("err=%v", err)
	}
	if !c.alive() {
		t.Fatal("一次超时不该把连接判死")
	}
	if _, err := c.call(tySetIO, nil, time.Second); err != nil {
		t.Fatalf("超时后仍应能继续用同一条连接：%v", err)
	}
}

func TestCallAfterPeerClose(t *testing.T) {
	host, port := startServer(t, "/", func(send sendFunc, req envelope) {})

	c := dialTest(t, host, port)
	// 底层连接断掉，读协程应当立刻察觉并把整条 client 判死。
	c.conn.Close()

	deadline := time.Now().Add(2 * time.Second)
	for c.alive() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if c.alive() {
		t.Fatal("连接断了却还报活着")
	}
	if _, err := c.call(tyGetIO, nil, time.Second); err == nil {
		t.Fatal("断开后仍然返回成功")
	}
}

// 一次调用只发一帧，绝不重发。这些请求都是 IO 动作，重发等于让现场的气缸多动一次。
func TestCallSendsExactlyOnce(t *testing.T) {
	t.Run("控制器回错误", func(t *testing.T) {
		var got atomic.Int32
		host, port := startServer(t, "/", func(send sendFunc, req envelope) {
			got.Add(1)
			send(map[string]any{"id": req.ID, "ty": req.Ty, "err": "1/rejected"})
		})

		c := dialTest(t, host, port)
		if _, err := c.call(tySetIO, nil, time.Second); err == nil {
			t.Fatal("应当报错")
		}
		if n := got.Load(); n != 1 {
			t.Fatalf("控制器收到 %d 帧，应当只有 1 帧", n)
		}
	})

	t.Run("控制器不回包", func(t *testing.T) {
		var got atomic.Int32
		host, port := startServer(t, "/", func(send sendFunc, req envelope) {
			got.Add(1)
		})

		c := dialTest(t, host, port)
		if _, err := c.call(tySetIO, nil, 150*time.Millisecond); err == nil {
			t.Fatal("应当超时")
		}
		time.Sleep(100 * time.Millisecond)
		if n := got.Load(); n != 1 {
			t.Fatalf("控制器收到 %d 帧，超时后不该补发", n)
		}
	})
}

// 发送失败时连接已经不可用，直接判死；后续调用不许悄悄重连再发一次。
func TestCallDoesNotResendAfterWriteFailure(t *testing.T) {
	host, port := startServer(t, "/", func(send sendFunc, req envelope) {
		send(map[string]any{"id": req.ID, "ty": req.Ty})
	})

	c := dialTest(t, host, port)
	c.conn.Close()

	if _, err := c.call(tySetIO, nil, time.Second); err == nil {
		t.Fatal("底层连接已关，应当报错")
	}
	if c.alive() {
		t.Fatal("发送失败后这条连接应当被判死")
	}
	if _, err := c.call(tySetIO, nil, time.Second); err == nil {
		t.Fatal("死掉的连接上不该还能发出请求")
	}
}

// 配置里的路径不对时，自动把常见路径试一遍，并且如实报出真正连上的那个地址。
func TestDialProbesPaths(t *testing.T) {
	host, port := startServer(t, "/ws", func(send sendFunc, req envelope) {})

	c, err := dial(host, port, "/", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if !strings.HasSuffix(c.addr, "/ws") {
		t.Fatalf("addr=%q，期望以 /ws 结尾", c.addr)
	}
}

func TestDialUsesConfiguredPathFirst(t *testing.T) {
	host, port := startServer(t, "/api", func(send sendFunc, req envelope) {})

	c, err := dial(host, port, "api", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if !strings.HasSuffix(c.addr, "/api") {
		t.Fatalf("addr=%q", c.addr)
	}
}

// 一个路径都不认时，报错要说清楚试过哪些，别只留一句「握手失败」。
func TestDialReportsAllTriedPaths(t *testing.T) {
	host, port := startServer(t, "/nope", func(send sendFunc, req envelope) {})

	_, err := dial(host, port, "/", time.Second)
	if err == nil {
		t.Fatal("应当连不上")
	}
	for _, want := range []string{"404", "/ws", "/websocket", "/api"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err=%v，期望包含 %q", err, want)
		}
	}
}

// 主机根本连不上的时候别把候选路径挨个试一遍，那只是把失败时间拖长三倍。
func TestDialDoesNotProbeWhenHostUnreachable(t *testing.T) {
	start := time.Now()
	_, err := dial("127.0.0.1", 1, "/", 300*time.Millisecond)
	if err == nil {
		t.Fatal("应当连不上")
	}
	if elapsed := time.Since(start); elapsed > 900*time.Millisecond {
		t.Fatalf("耗时 %s，看着是把候选路径都试了一遍", elapsed)
	}
}

func TestNormalizePath(t *testing.T) {
	cases := map[string]string{
		"":     "/",
		"  ":   "/",
		"/":    "/",
		"ws":   "/ws",
		"/ws":  "/ws",
		" /a ": "/a",
	}
	for in, want := range cases {
		if got := normalizePath(in); got != want {
			t.Fatalf("normalizePath(%q)=%q want %q", in, got, want)
		}
	}
}

func TestNormalizeID(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{"t1", "t1"},
		{float64(1000000), "1000000"},
		{float64(1.5), "1.5"},
		{nil, ""},
	}
	for _, c := range cases {
		if got := normalizeID(c.in); got != c.want {
			t.Fatalf("normalizeID(%v)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestParseWriteValue(t *testing.T) {
	cases := []struct {
		in   string
		want any
	}{
		{"true", true},
		{"FALSE", false},
		{"42", int64(42)},
		{"3.5", 3.5},
		{"  hello ", "hello"},
	}
	for _, c := range cases {
		got, err := parseWriteValue(c.in)
		if err != nil {
			t.Fatalf("parseWriteValue(%q): %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("parseWriteValue(%q)=%#v want %#v", c.in, got, c.want)
		}
	}
	if _, err := parseWriteValue("  "); err == nil {
		t.Fatal("空值应当报错")
	}
}

func TestStringifyJSON(t *testing.T) {
	cases := map[string]string{
		`0`:       "0",
		`1.5`:     "1.5",
		`true`:    "true",
		`"abc"`:   "abc",
		`null`:    "",
		`[1,2]`:   "[1,2]",
		`1000000`: "1000000",
	}
	for in, want := range cases {
		if got := stringifyJSON(json.RawMessage(in)); got != want {
			t.Fatalf("stringifyJSON(%s)=%q want %q", in, got, want)
		}
	}
}

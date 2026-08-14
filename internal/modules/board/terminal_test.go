package board

import (
	"strings"
	"testing"
)

func TestTerminalDrainConsumesOutput(t *testing.T) {
	terminal := &terminalSession{}
	terminal.appendOutput([]byte("hello"))
	terminal.appendOutput([]byte(" world"))

	if got := terminal.drain(); got != "hello world" {
		t.Fatalf("drain=%q", got)
	}
	if got := terminal.drain(); got != "" {
		t.Fatalf("读过的输出不应再次返回：%q", got)
	}
}

func TestTerminalOutputIsBounded(t *testing.T) {
	terminal := &terminalSession{}
	terminal.appendOutput([]byte(strings.Repeat("a", maxTerminalBuffer)))
	terminal.appendOutput([]byte("new-output"))

	got := terminal.drain()
	if len(got) >= maxTerminalBuffer {
		t.Fatalf("缓冲区没有截断：len=%d", len(got))
	}
	if !strings.Contains(got, "较早的终端输出已省略") || !strings.HasSuffix(got, "new-output") {
		t.Fatalf("截断提示或最新输出丢失")
	}
}

func TestDrainHoldsIncompleteUTF8(t *testing.T) {
	terminal := &terminalSession{}
	// “目录”的 UTF-8 是 E7 9B AE E5 BD 95，中间切开不应先吐出替换符。
	han := []byte("目录")
	terminal.appendOutput(han[:2])
	if got := terminal.drain(); got != "" {
		t.Fatalf("半个汉字不该先交出：%q", got)
	}
	terminal.appendOutput(han[2:])
	if got := terminal.drain(); got != "目录" {
		t.Fatalf("拼完应得完整汉字，得到 %q", got)
	}
}

func TestClosedTerminalRejectsInput(t *testing.T) {
	terminal := &terminalSession{closed: true}
	if err := terminal.write("echo test\n"); err == nil {
		t.Fatal("已关闭终端不应接受输入")
	}
}

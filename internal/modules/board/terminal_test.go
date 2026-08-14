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

func TestClosedTerminalRejectsInput(t *testing.T) {
	terminal := &terminalSession{closed: true}
	if err := terminal.write("echo test\n"); err == nil {
		t.Fatal("已关闭终端不应接受输入")
	}
}

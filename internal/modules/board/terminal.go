package board

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"
	"unicode/utf8"

	"golang.org/x/crypto/ssh"
)

const maxTerminalBuffer = 1024 * 1024

// terminalSession 是一条持久 SSH PTY。命令按钮和手工输入都写进同一个 shell，
// stdout/stderr 合并进缓冲区，前端短轮询取走后显示。
type terminalSession struct {
	session *ssh.Session
	stdin   io.WriteCloser

	writeMu sync.Mutex
	outMu    sync.Mutex
	output   bytes.Buffer
	leftover []byte

	stateMu sync.Mutex
	closed  bool
}

func newTerminalSession(client *ssh.Client) (*terminalSession, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("打开终端会话失败: %w", err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		return nil, fmt.Errorf("打开终端输入失败: %w", err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		return nil, fmt.Errorf("打开终端输出失败: %w", err)
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		session.Close()
		return nil, fmt.Errorf("打开终端错误输出失败: %w", err)
	}

	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm", 30, 120, modes); err != nil {
		session.Close()
		return nil, fmt.Errorf("申请终端 PTY 失败: %w", err)
	}
	if err := session.Shell(); err != nil {
		session.Close()
		return nil, fmt.Errorf("启动远端 shell 失败: %w", err)
	}

	t := &terminalSession{session: session, stdin: stdin}
	go t.copyOutput(stdout)
	go t.copyOutput(stderr)
	go func() {
		err := session.Wait()
		t.stateMu.Lock()
		t.closed = true
		t.stateMu.Unlock()
		if err != nil {
			t.appendOutput(fmt.Appendf(nil, "\r\n[终端已关闭：%v]\r\n", err))
		} else {
			t.appendOutput([]byte("\r\n[终端已关闭]\r\n"))
		}
	}()
	return t, nil
}

func (t *terminalSession) copyOutput(src io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			t.appendOutput(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

func (t *terminalSession) appendOutput(p []byte) {
	t.outMu.Lock()
	defer t.outMu.Unlock()

	if t.output.Len()+len(p) > maxTerminalBuffer {
		current := t.output.Bytes()
		keepFrom := len(current) - maxTerminalBuffer/2
		if keepFrom < 0 {
			keepFrom = 0
		}
		keep := append([]byte(nil), current[keepFrom:]...)
		t.output.Reset()
		t.output.WriteString("[较早的终端输出已省略]\r\n")
		t.output.Write(keep)
	}
	t.output.Write(p)
}

func (t *terminalSession) write(text string) error {
	if text == "" {
		return nil
	}
	t.stateMu.Lock()
	closed := t.closed
	t.stateMu.Unlock()
	if closed {
		return errors.New("终端已经关闭")
	}

	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	if _, err := io.WriteString(t.stdin, text); err != nil {
		return fmt.Errorf("写入终端失败: %w", err)
	}
	return nil
}

func (t *terminalSession) drain() string {
	t.outMu.Lock()
	defer t.outMu.Unlock()
	data := make([]byte, 0, len(t.leftover)+t.output.Len())
	data = append(data, t.leftover...)
	data = append(data, t.output.Bytes()...)
	t.output.Reset()
	complete, rest := splitCompleteUTF8(data)
	t.leftover = rest
	return string(complete)
}

// splitCompleteUTF8 只交出完整的 UTF-8。半个汉字卡在两次 drain 中间时，
// 直接 String() 会变成 U+FFFD，界面上就是 Tab 补全旁边那个问号方块。
func splitCompleteUTF8(data []byte) (complete, rest []byte) {
	write := 0
	i := 0
	for i < len(data) {
		r, size := utf8.DecodeRune(data[i:])
		if r == utf8.RuneError && size == 1 {
			if utf8Prefix(data[i:]) {
				return append([]byte(nil), data[:write]...), append([]byte(nil), data[i:]...)
			}
			i++
			continue
		}
		if write != i {
			copy(data[write:write+size], data[i:i+size])
		}
		write += size
		i += size
	}
	return data[:write], nil
}

func utf8Prefix(p []byte) bool {
	if len(p) == 0 || len(p) >= 4 {
		return false
	}
	need := 0
	switch {
	case p[0]&0xE0 == 0xC0:
		need = 2
	case p[0]&0xF0 == 0xE0:
		need = 3
	case p[0]&0xF8 == 0xF0:
		need = 4
	default:
		return false
	}
	return len(p) < need
}

func (t *terminalSession) alive() bool {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	return !t.closed
}

func (t *terminalSession) close() {
	t.stateMu.Lock()
	if t.closed {
		t.stateMu.Unlock()
		return
	}
	t.closed = true
	t.stateMu.Unlock()
	_ = t.stdin.Close()
	_ = t.session.Close()
}

package board

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"

	"golang.org/x/crypto/ssh"
)

const maxTerminalBuffer = 1024 * 1024

// terminalSession 是一条持久 SSH PTY。命令按钮和手工输入都写进同一个 shell，
// stdout/stderr 合并进缓冲区，前端短轮询取走后显示。
type terminalSession struct {
	session *ssh.Session
	stdin   io.WriteCloser

	writeMu sync.Mutex
	outMu   sync.Mutex
	output  bytes.Buffer

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
	out := t.output.String()
	t.output.Reset()
	return out
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

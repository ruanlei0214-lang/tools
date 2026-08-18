//go:build windows

package ping

import (
	"os/exec"
	"syscall"
)

// hideConsole 防止 GUI 程序拉起 arp.exe 时闪一个黑色控制台窗口。
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}

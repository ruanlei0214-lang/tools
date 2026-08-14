package netcfg

import (
	"embedtools/internal/module"
	"os"
	"path/filepath"
	"testing"
)

// isolateState 把状态文件挪到临时目录，避免测试读写开发机上真正的那份。
func isolateState(t *testing.T) {
	t.Helper()
	t.Cleanup(module.UseTempDataDir(t.TempDir()))
}

func TestRememberAndLoadHost(t *testing.T) {
	isolateState(t)

	if got := loadLastHost(); got != "" {
		t.Fatalf("还没记过任何地址，却读到 %q", got)
	}

	rememberHost("192.168.3.136")
	if got := loadLastHost(); got != "192.168.3.136" {
		t.Errorf("loadLastHost() = %q, 期望 192.168.3.136", got)
	}

	rememberHost("10.0.0.9")
	if got := loadLastHost(); got != "10.0.0.9" {
		t.Errorf("覆盖后 loadLastHost() = %q, 期望 10.0.0.9", got)
	}
}

// 非法地址不该被记下来，否则下次打开会用一个连不上的值覆盖掉出厂地址。
func TestRememberHostRejectsInvalid(t *testing.T) {
	isolateState(t)
	rememberHost("192.168.3.136")

	for _, bad := range []string{"", "not-an-ip", "192.168.3.999"} {
		rememberHost(bad)
		if got := loadLastHost(); got != "192.168.3.136" {
			t.Errorf("记入 %q 后地址变成了 %q，应当保持 192.168.3.136", bad, got)
		}
	}
}

// 状态文件损坏时退回空值，让页面用 config.json 的出厂地址，而不是报错。
func TestLoadLastHostIgnoresBrokenFile(t *testing.T) {
	isolateState(t)

	path, err := stateFile()
	if err != nil {
		t.Fatalf("stateFile() 失败：%v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	if err := os.WriteFile(path, []byte("{坏掉的 json"), 0o644); err != nil {
		t.Fatalf("写文件失败：%v", err)
	}

	if got := loadLastHost(); got != "" {
		t.Errorf("文件损坏时应返回空字符串，实际 %q", got)
	}
}

// Defaults 在没有记录时给出厂地址，有记录时给记住的那个。
func TestDefaultsPrefersRememberedHost(t *testing.T) {
	isolateState(t)
	svc := &Service{}

	factory := svc.Defaults().Device.Host
	if factory == "" {
		t.Fatal("出厂地址不该为空")
	}

	rememberHost("10.1.2.3")
	if got := svc.Defaults().Device.Host; got != "10.1.2.3" {
		t.Errorf("Defaults() 的地址 = %q, 期望 10.1.2.3", got)
	}
}

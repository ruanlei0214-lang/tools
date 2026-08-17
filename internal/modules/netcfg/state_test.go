package netcfg

import (
	"embedtools/internal/module"
	"os"
	"path/filepath"
	"testing"
)

// isolateState 把共享配置挪到临时目录，避免测试读写开发机上真正的那份。
func isolateState(t *testing.T) {
	t.Helper()
	t.Cleanup(module.UseTempDataDir(t.TempDir()))
}

func TestRememberAndLoadHost(t *testing.T) {
	isolateState(t)

	if got := module.LoadShared().Host; got != "" {
		t.Fatalf("还没记过任何地址，却读到 %q", got)
	}

	rememberHost("192.168.3.136")
	if got := module.LoadShared().Host; got != "192.168.3.136" {
		t.Errorf("LoadShared().Host = %q, 期望 192.168.3.136", got)
	}

	rememberHost("10.0.0.9")
	if got := module.LoadShared().Host; got != "10.0.0.9" {
		t.Errorf("覆盖后 LoadShared().Host = %q, 期望 10.0.0.9", got)
	}
}

// 空地址不该被记下来，否则下次打开会用一个连不上的值覆盖掉出厂地址。
func TestRememberHostRejectsEmpty(t *testing.T) {
	isolateState(t)
	rememberHost("192.168.3.136")

	rememberHost("")
	if got := module.LoadShared().Host; got != "192.168.3.136" {
		t.Errorf("记入空地址后变成了 %q，应当保持 192.168.3.136", got)
	}
}

// 记地址只动 Host 字段：凭据是顶栏凭据弹层管的，netcfg 不能顺手抹掉。
func TestRememberHostKeepsCredentials(t *testing.T) {
	isolateState(t)
	if err := module.SaveShared(module.Shared{Host: "192.168.3.136", User: "root", Password: "pw"}); err != nil {
		t.Fatal(err)
	}

	rememberHost("10.0.0.9")
	got := module.LoadShared()
	if got.Host != "10.0.0.9" || got.User != "root" || got.Password != "pw" {
		t.Errorf("记地址把凭据动了：%+v", got)
	}
}

// 共享配置损坏时退回出厂默认，而不是报错。
func TestDefaultsIgnoresBrokenSharedFile(t *testing.T) {
	isolateState(t)

	path, err := module.SharedPath()
	if err != nil {
		t.Fatalf("SharedPath() 失败：%v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	if err := os.WriteFile(path, []byte("{坏掉的 json"), 0o644); err != nil {
		t.Fatalf("写坏文件失败：%v", err)
	}

	svc := &Service{}
	if got := svc.Defaults().Device.Host; got == "" {
		t.Error("共享配置损坏时应退回出厂地址，实际为空")
	}
}

// Defaults 在没有记录时给出厂地址，有记录时给共享配置里的地址和凭据。
func TestDefaultsPrefersShared(t *testing.T) {
	isolateState(t)
	svc := &Service{}

	factory := svc.Defaults().Device
	if factory.Host == "" {
		t.Fatal("出厂地址不该为空")
	}

	if err := module.SaveShared(module.Shared{Host: "10.1.2.3", User: "admin", Password: "pw"}); err != nil {
		t.Fatal(err)
	}
	got := svc.Defaults().Device
	if got.Host != "10.1.2.3" {
		t.Errorf("Defaults() 的地址 = %q, 期望 10.1.2.3", got.Host)
	}
	if got.User != "admin" || got.Password != "pw" {
		t.Errorf("Defaults() 的凭据 = %q/%q, 期望 admin/pw", got.User, got.Password)
	}
}

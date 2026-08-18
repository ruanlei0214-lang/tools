package module

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSharedRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(UseTempDataDir(dir))

	if got := LoadShared(); got.Host != "" {
		t.Fatalf("还没保存过，却读到 %+v", got)
	}

	want := Shared{Host: "192.168.3.136", User: "root", Password: "pw", KeyPath: `C:\keys\id`}
	if err := SaveShared(want); err != nil {
		t.Fatal(err)
	}
	if got := LoadShared(); got != want {
		t.Fatalf("LoadShared() = %+v, 期望 %+v", got, want)
	}

	// 覆盖保存。
	next := Shared{Host: "10.0.0.9", User: "admin"}
	if err := SaveShared(next); err != nil {
		t.Fatal(err)
	}
	if got := LoadShared(); got != next {
		t.Fatalf("覆盖后 LoadShared() = %+v, 期望 %+v", got, next)
	}
}

func TestSaveHostKeepsCredentials(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(UseTempDataDir(dir))

	if err := SaveShared(Shared{Host: "192.168.3.136", User: "root", Password: "pw", KeyPath: `C:\keys\id`}); err != nil {
		t.Fatal(err)
	}
	if err := SaveHost("10.0.0.9"); err != nil {
		t.Fatal(err)
	}
	got := LoadShared()
	want := Shared{Host: "10.0.0.9", User: "root", Password: "pw", KeyPath: `C:\keys\id`}
	if got != want {
		t.Fatalf("SaveHost 后 = %+v, 期望 %+v", got, want)
	}
}

func TestSaveHostRejectsEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(UseTempDataDir(dir))

	if err := SaveShared(Shared{Host: "192.168.3.136", User: "root"}); err != nil {
		t.Fatal(err)
	}
	if err := SaveHost("  "); err == nil {
		t.Fatal("空地址应当被拒")
	}
	if got := LoadShared().Host; got != "192.168.3.136" {
		t.Fatalf("被拒后 host = %q, 不该改", got)
	}
}

func TestSaveSharedRejectsEmptyHost(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(UseTempDataDir(dir))

	if err := SaveShared(Shared{User: "root"}); err == nil {
		t.Fatal("空地址应当被拒")
	}
	if _, err := os.Stat(filepath.Join(dir, SharedConfigName)); !os.IsNotExist(err) {
		t.Fatal("被拒的保存不该落盘")
	}
}

func TestLoadSharedIgnoresBrokenFile(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(UseTempDataDir(dir))

	path, err := SharedPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{坏掉的 json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LoadShared(); got.Host != "" {
		t.Fatalf("文件损坏时应返回空，实际 %+v", got)
	}
}

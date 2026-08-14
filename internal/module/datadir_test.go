package module

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDataDirIsExeDir(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}

	dir, err := DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Dir(exe) {
		t.Fatalf("DataDir()=%q，应当是 exe 所在目录 %q", dir, filepath.Dir(exe))
	}
}

func TestUseTempDataDir(t *testing.T) {
	want := t.TempDir()
	restore := UseTempDataDir(want)
	t.Cleanup(restore)

	got, err := DataDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("DataDir()=%q，应当是临时目录 %q", got, want)
	}
}

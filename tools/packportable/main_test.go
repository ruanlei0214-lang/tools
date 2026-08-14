package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPackAssemblesFolderAndKeepsExistingConfig(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "wails.json"), []byte(`{"outputfilename":"C2toolsV9.9.9"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "build", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "C2toolsV9.9.9.exe"), []byte("exe"), 0o644); err != nil {
		t.Fatal(err)
	}

	remoteDir := filepath.Join(root, "internal", "modules", "remote", "config")
	if err := os.MkdirAll(remoteDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"config.json", "io.json", "register.json"} {
		if err := os.WriteFile(filepath.Join(remoteDir, name), []byte(`{"factory":"`+name+`"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	boardDir := filepath.Join(root, "internal", "modules", "board", "config")
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(boardDir, "commands.json"), []byte(`[{"factory":"commands.json"}]`), 0o644); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(bin, "C2toolsV9.9.9")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "remote-io.json"), []byte(`{"keep":true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := pack(root); err != nil {
		t.Fatal(err)
	}

	if !fileExists(filepath.Join(dest, "C2toolsV9.9.9.exe")) {
		t.Fatal("exe 没进绿色版目录")
	}
	if fileExists(filepath.Join(bin, "C2toolsV9.9.9.exe")) {
		t.Fatal("build/bin 根下不该再留一份 exe")
	}
	if got, _ := os.ReadFile(filepath.Join(dest, "remote-io.json")); string(got) != `{"keep":true}` {
		t.Fatalf("已有配置被覆盖了：%s", got)
	}
	if got, _ := os.ReadFile(filepath.Join(dest, "remote-config.json")); string(got) != `{"factory":"config.json"}` {
		t.Fatalf("缺出厂连接配置：%s", got)
	}
	if got, _ := os.ReadFile(filepath.Join(dest, "board-commands.json")); string(got) != `[{"factory":"commands.json"}]` {
		t.Fatalf("缺出厂指令清单：%s", got)
	}
	if st, err := os.Stat(filepath.Join(dest, "webview2")); err != nil || !st.IsDir() {
		t.Fatal("应当建好 webview2 目录")
	}
}

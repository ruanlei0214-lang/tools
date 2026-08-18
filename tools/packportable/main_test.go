package main

import (
	"encoding/json"
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
	if err := os.WriteFile(filepath.Join(boardDir, "config.json"), []byte(`{"device":{"host":"192.168.1.136","user":"root"}}`), 0o644); err != nil {
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
	if got, _ := os.ReadFile(filepath.Join(dest, "toolbox-config.json")); string(got) != `{"device":{"host":"192.168.1.136","user":"root"}}` {
		t.Fatalf("缺共享配置：%s", got)
	}
	if st, err := os.Stat(filepath.Join(dest, "webview2")); err != nil || !st.IsDir() {
		t.Fatal("应当建好 webview2 目录")
	}
}

func TestWriteBackCopiesChangedConfig(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "wails.json"), []byte(`{"outputfilename":"C2toolsV9.9.9"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	remoteDir := filepath.Join(root, "internal", "modules", "remote", "config")
	if err := os.MkdirAll(remoteDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(remoteDir, "io.json"), []byte(`{"factory":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(remoteDir, "config.json"), []byte(`{"factory":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	boardDir := filepath.Join(root, "internal", "modules", "board", "config")
	if err := os.MkdirAll(boardDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(boardDir, "config.json"), []byte(`{"device":{"host":"192.168.1.136","user":"root"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "build", "bin", "C2toolsV9.9.9")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "remote-io.json"), []byte(`{"field":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "remote-config.json"), []byte(`{"factory":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "toolbox-config.json"), []byte(`{"host":"10.9.8.7","user":"root"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := writeBack(root); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(remoteDir, "io.json")); string(got) != `{"field":true}` {
		t.Fatalf("改过的 IO 应当写回源码：%s", got)
	}
	if got, _ := os.ReadFile(filepath.Join(remoteDir, "config.json")); string(got) != `{"factory":true}` {
		t.Fatalf("没改的不该动：%s", got)
	}
	// 共享配置的 host 应当写回 board 的出厂默认。
	boardRaw, _ := os.ReadFile(filepath.Join(boardDir, "config.json"))
	var boardCfg map[string]any
	if err := json.Unmarshal(boardRaw, &boardCfg); err != nil {
		t.Fatal(err)
	}
	dev := boardCfg["device"].(map[string]any)
	if dev["host"] != "10.9.8.7" {
		t.Fatalf("共享配置的 host 没写回 board：%v", dev["host"])
	}
}

func TestWriteBackRejectsBrokenJSON(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "wails.json"), []byte(`{"outputfilename":"C2toolsV9.9.9"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	remoteDir := filepath.Join(root, "internal", "modules", "remote", "config")
	if err := os.MkdirAll(remoteDir, 0o755); err != nil {
		t.Fatal(err)
	}
	const factory = `{"factory":true}`
	if err := os.WriteFile(filepath.Join(remoteDir, "io.json"), []byte(factory), 0o644); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "build", "bin", "C2toolsV9.9.9")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "remote-io.json"), []byte(`{"半份`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := writeBack(root); err == nil {
		t.Fatal("坏 JSON 应当被拒")
	}
	if got, _ := os.ReadFile(filepath.Join(remoteDir, "io.json")); string(got) != factory {
		t.Fatalf("源码不该被坏文件覆盖：%s", got)
	}
}

// Command packportable 把 wails build 刚打出来的 exe 收成绿色版目录。
//
// 必须在仓库根目录跑（或传 -root）。它做三件事：
//  1. 把 build/bin/<名字>.exe 挪进 build/bin/<名字>/
//  2. 把 remote / board 的出厂配置拷进去（盘上已有的不覆盖，免得重建冲掉现场改过的）
//  3. 建好 webview2 目录，下次启动 WebView2 缓存落在这里，不用再去 %APPDATA%
//
// netcfg 记住的地址是用出来才有的，出厂没有可拷的，不在这里造空文件。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	root := flag.String("root", ".", "仓库根目录")
	flag.Parse()
	if err := pack(*root); err != nil {
		fmt.Fprintln(os.Stderr, "packportable:", err)
		os.Exit(1)
	}
}

func pack(root string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	name, err := outputName(root)
	if err != nil {
		return err
	}

	bin := filepath.Join(root, "build", "bin")
	srcExe := filepath.Join(bin, name+".exe")
	if !fileExists(srcExe) {
		return fmt.Errorf("找不到 %s，先跑 wails build", srcExe)
	}

	dest := filepath.Join(bin, name)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}

	destExe := filepath.Join(dest, name+".exe")
	if err := replaceFile(srcExe, destExe); err != nil {
		return fmt.Errorf("放入 exe：%w", err)
	}

	seeds := []struct{ src, dst string }{
		{filepath.Join(root, "internal", "modules", "remote", "config", "config.json"), "remote-config.json"},
		{filepath.Join(root, "internal", "modules", "remote", "config", "io.json"), "remote-io.json"},
		{filepath.Join(root, "internal", "modules", "remote", "config", "register.json"), "remote-register.json"},
		{filepath.Join(root, "internal", "modules", "board", "config", "commands.json"), "board-commands.json"},
	}
	for _, s := range seeds {
		dst := filepath.Join(dest, s.dst)
		if fileExists(dst) {
			fmt.Printf("保留已有 %s\n", s.dst)
			continue
		}
		if err := copyFile(s.src, dst); err != nil {
			return fmt.Errorf("拷 %s：%w", s.dst, err)
		}
		fmt.Printf("写入 %s\n", s.dst)
	}

	if err := os.MkdirAll(filepath.Join(dest, "webview2"), 0o755); err != nil {
		return err
	}

	fmt.Printf("绿色版：%s\n", dest)
	return nil
}

func outputName(root string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(root, "wails.json"))
	if err != nil {
		return "", fmt.Errorf("读 wails.json：%w", err)
	}
	var cfg struct {
		OutputFilename string `json:"outputfilename"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil || cfg.OutputFilename == "" {
		return "", fmt.Errorf("wails.json 里没有 outputfilename")
	}
	return cfg.OutputFilename, nil
}

// replaceFile 把 src 挪到 dst。同盘用改名，跨盘就拷完再删源。
func replaceFile(src, dst string) error {
	if src == dst {
		return nil
	}
	_ = os.Remove(dst)
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	if err := copyFile(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

// Command packportable 把 wails build 刚打出来的 exe 收成绿色版目录。
//
// 必须在仓库根目录跑（或传 -root）。它做四件事：
//  1. 把 build/bin/<名字>.exe 挪进 build/bin/<名字>/
//  2. 把 remote / board 的出厂配置拷进去（盘上已有的不覆盖，免得重建冲掉现场改过的）
//  3. 把 netcfg 的 config 目录整份拷进去（热插拔脚本等文件现场改盘上的即可，不用重建）
//  4. 建好 webview2 目录，下次启动 WebView2 缓存落在这里，不用再去 %APPDATA%
//
// netcfg 记住的地址是用出来才有的，出厂没有可拷的，不在这里造空文件。
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	root := flag.String("root", ".", "仓库根目录")
	writeback := flag.Bool("writeback", false, "把绿色版目录里的配置写回源码出厂文件")
	flag.Parse()
	var err error
	if *writeback {
		err = writeBack(*root)
	} else {
		err = pack(*root)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "packportable:", err)
		os.Exit(1)
	}
}

type seed struct {
	src string // 仓库里的出厂文件
	dst string // 绿色版目录里的文件名
}

func factorySeeds(root string) []seed {
	return []seed{
		{filepath.Join(root, "internal", "modules", "remote", "config", "config.json"), "remote-config.json"},
		{filepath.Join(root, "internal", "modules", "remote", "config", "io.json"), "remote-io.json"},
		{filepath.Join(root, "internal", "modules", "remote", "config", "register.json"), "remote-register.json"},
		{filepath.Join(root, "internal", "modules", "board", "config", "commands.json"), "board-commands.json"},
	}
}

// seedSharedConfig 生成绿色版的 toolbox-config.json。host/user/password 来自
// board 的出厂默认，但不能直拷那份文件：board 的是嵌套的 {"device":{...}}，
// 共享配置是平的 {"host":...}——直拷会让 LoadShared 读不出 host，顶栏地址空白。
func seedSharedConfig(root, dst string) error {
	if fileExists(dst) {
		fmt.Printf("保留已有 %s\n", filepath.Base(dst))
		return nil
	}
	src := filepath.Join(root, "internal", "modules", "board", "config", "config.json")
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	var boardCfg struct {
		Device struct {
			Host     string `json:"host"`
			User     string `json:"user"`
			Password string `json:"password"`
			KeyPath  string `json:"keyPath"`
		} `json:"device"`
	}
	if err := json.Unmarshal(bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf")), &boardCfg); err != nil {
		return fmt.Errorf("解析 %s：%w", src, err)
	}
	shared := struct {
		Host     string `json:"host"`
		User     string `json:"user"`
		Password string `json:"password"`
		KeyPath  string `json:"keyPath"`
	}{boardCfg.Device.Host, boardCfg.Device.User, boardCfg.Device.Password, boardCfg.Device.KeyPath}
	out, err := json.MarshalIndent(shared, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(dst, out, 0o644); err != nil {
		return err
	}
	fmt.Printf("写入 %s\n", filepath.Base(dst))
	return nil
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

	for _, s := range factorySeeds(root) {
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

	// 共享配置：绿色版第一次打开时三个模块都读这份，不用各自再填一遍。
	if err := seedSharedConfig(root, filepath.Join(dest, "toolbox-config.json")); err != nil {
		return fmt.Errorf("写共享配置：%w", err)
	}

	// netcfg 的 config 目录整份拷进绿色版。热插拔规则和脚本由 netcfg 按
	// 「盘上优先」读取，现场要改直接改这里，不用重新构建。和出厂配置一样，
	// 盘上已有的不覆盖。
	configSrc := filepath.Join(root, "internal", "modules", "netcfg", "config")
	if err := copyDirSkipExisting(configSrc, filepath.Join(dest, "config")); err != nil {
		return fmt.Errorf("拷 config 目录：%w", err)
	}

	if err := os.MkdirAll(filepath.Join(dest, "webview2"), 0o755); err != nil {
		return err
	}

	fmt.Printf("绿色版：%s\n", dest)
	return nil
}

// writebackSeeds 比 factorySeeds 多一份共享配置：pack 时 toolbox-config.json 由
// seedSharedConfig 转换生成（不是直拷，所以不在 factorySeeds 里），但回写时要把
// 它的 host 写回 board 出厂默认。
func writebackSeeds(root string) []seed {
	return append(factorySeeds(root), seed{
		filepath.Join(root, "internal", "modules", "board", "config", "config.json"), "toolbox-config.json",
	})
}

// writeBack 把绿色版目录里改过的配置拷回源码出厂文件。
// 现场调好点位或指令之后，下次构建要带着走，就走这一步。
// 绿色版里没有的、和源码一样的，都跳过；坏 JSON 不写，免得把出厂文件毁了。
// toolbox-config.json 是共享配置，回写时只取 host 写回 board 的出厂默认，
// 不整份覆盖——remote 的端口、路径不该被共享配置冲掉。
func writeBack(root string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	name, err := outputName(root)
	if err != nil {
		return err
	}
	dir := filepath.Join(root, "build", "bin", name)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return fmt.Errorf("找不到绿色版目录 %s，先构建", dir)
	}

	changed := 0
	for _, s := range writebackSeeds(root) {
		from := filepath.Join(dir, s.dst)
		if !fileExists(from) {
			fmt.Printf("跳过 %s（绿色版里没有）\n", s.dst)
			continue
		}
		raw, err := os.ReadFile(from)
		if err != nil {
			return fmt.Errorf("读 %s：%w", from, err)
		}
		if !json.Valid(bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))) {
			return fmt.Errorf("%s 不是合法 JSON，源码没有改动", from)
		}

		// 共享配置只回写 host 到 board 的出厂默认，不整份覆盖。
		if s.dst == "toolbox-config.json" {
			var shared struct {
				Host string `json:"host"`
			}
			if err := json.Unmarshal(raw, &shared); err != nil {
				return fmt.Errorf("解析 %s：%w", from, err)
			}
			if shared.Host == "" {
				fmt.Printf("跳过 %s（没有 host）\n", s.dst)
				continue
			}
			// 读 board 出厂默认，只改 host。
			boardRaw, err := os.ReadFile(s.src)
			if err != nil {
				return fmt.Errorf("读 %s：%w", s.src, err)
			}
			var boardCfg map[string]any
			if err := json.Unmarshal(boardRaw, &boardCfg); err != nil {
				return fmt.Errorf("解析 %s：%w", s.src, err)
			}
			dev, ok := boardCfg["device"].(map[string]any)
			if !ok {
				dev = map[string]any{}
				boardCfg["device"] = dev
			}
			if dev["host"] == shared.Host {
				fmt.Printf("未改 %s\n", s.dst)
				continue
			}
			dev["host"] = shared.Host
			out, err := json.MarshalIndent(boardCfg, "", "  ")
			if err != nil {
				return err
			}
			if err := os.WriteFile(s.src, out, 0o644); err != nil {
				return fmt.Errorf("回写 %s：%w", s.dst, err)
			}
			rel, _ := filepath.Rel(root, s.src)
			fmt.Printf("回写 %s → %s（仅 host）\n", s.dst, rel)
			changed++
			continue
		}

		if old, err := os.ReadFile(s.src); err == nil && bytes.Equal(old, raw) {
			fmt.Printf("未改 %s\n", s.dst)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(s.src), 0o755); err != nil {
			return err
		}
		if err := copyFile(from, s.src); err != nil {
			return fmt.Errorf("回写 %s：%w", s.dst, err)
		}
		rel, err := filepath.Rel(root, s.src)
		if err != nil {
			rel = s.src
		}
		fmt.Printf("回写 %s → %s\n", s.dst, rel)
		changed++
	}
	if changed == 0 {
		fmt.Println("没有需要回写的改动")
	}
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

// copyDirSkipExisting 把 src 目录下的文件平铺拷到 dst，盘上已有的跳过。
// config 目录只有一层文件，不做递归。
func copyDirSkipExisting(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		to := filepath.Join(dst, e.Name())
		if fileExists(to) {
			fmt.Printf("保留已有 config/%s\n", e.Name())
			continue
		}
		if err := copyFile(filepath.Join(src, e.Name()), to); err != nil {
			return err
		}
		fmt.Printf("写入 config/%s\n", e.Name())
	}
	return nil
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

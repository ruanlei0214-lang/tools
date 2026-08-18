package module

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// 全系列工具共用一台控制器。地址和 SSH 凭据只在这一份里改，
// remote（WebSocket）、netcfg、board 三个模块启动时都从这里取。
//
// 存在 exe 同目录，不进 %APPDATA%。现场整夹拷走时这份配置跟着走。
// 文件名不带任何模块前缀：它是工具箱级的，不是哪个模块的。
const SharedConfigName = "toolbox-config.json"

// Shared 是三个模块共用的连接参数。
type Shared struct {
	Host     string `json:"host"`
	User     string `json:"user"`
	Password string `json:"password"`
	KeyPath  string `json:"keyPath"`
}

// errNoShared 表示还没有共享配置，该用各模块自己的出厂默认。
var errNoShared = errors.New("没有共享配置")

// SharedPath 返回共享配置的完整路径，给界面显示「配置存在本机」。
func SharedPath() (string, error) {
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, SharedConfigName), nil
}

// LoadShared 读共享配置。没有或坏掉都返回空 Shared：
// 没有是正常状态（干净机器第一次打开），坏掉时各模块退回自己的出厂默认，
// 不在这里造告警——告警由各模块按自己的方式报。
func LoadShared() Shared {
	path, err := SharedPath()
	if err != nil {
		return Shared{}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Shared{}
	}
	s, err := parseShared(raw)
	if err != nil {
		return Shared{}
	}
	return s
}

// SaveHost 只改共享配置里的地址，用户名和密码原样留下。
// 顶栏改 IP 走这条，避免把凭据冲成空。
func SaveHost(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return errors.New("请填写设备地址")
	}
	s := LoadShared()
	s.Host = host
	return SaveShared(s)
}

// SaveShared 校验并整份写回。先写 .tmp 再改名，避免写一半挂掉留下半份 JSON。
func SaveShared(s Shared) error {
	s.Host = strings.TrimSpace(s.Host)
	s.User = strings.TrimSpace(s.User)
	if s.Host == "" {
		return errors.New("请填写设备地址")
	}
	if strings.ContainsAny(s.Host, " \t\r\n") {
		return fmt.Errorf("设备地址 %q 里有空白字符", s.Host)
	}

	path, err := SharedPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("无法创建 %s：%w", filepath.Dir(path), err)
	}

	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("写入 %s 失败：%w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("写入 %s 失败：%w", path, err)
	}
	return nil
}

// parseShared 只拦「填了等于没填」的错。地址格式不在这里卡死：
// 现场可能填主机名，也可能填 IPv6，parseIP 认不全。
func parseShared(raw []byte) (Shared, error) {
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	var s Shared
	if err := json.Unmarshal(raw, &s); err != nil {
		return Shared{}, err
	}
	s.Host = strings.TrimSpace(s.Host)
	s.User = strings.TrimSpace(s.User)
	return s, nil
}

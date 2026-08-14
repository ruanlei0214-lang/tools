package netcfg

import (
	"embedtools/internal/module"
	"encoding/json"
	"log"
	"net"
	"os"
	"path/filepath"
)

// 记住上次连通的设备地址，下次打开直接用它，而不是回到 config.json 里的出厂地址。
// 存在 exe 旁边，不进 %APPDATA%。现场整夹拷走时这份记录跟着走。
const stateFileName = "netcfg-state.json"

type state struct {
	Host string `json:"host"`
}

func stateFile() (string, error) {
	dir, err := module.DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, stateFileName), nil
}

// loadLastHost 返回上次连通的地址，没有记录或读取失败时返回空字符串。
// 这条路径出问题只会退回默认地址，不值得打扰用户。
func loadLastHost() string {
	path, err := stateFile()
	if err != nil {
		return ""
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var s state
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	if net.ParseIP(s.Host) == nil {
		return ""
	}
	return s.Host
}

// rememberHost 记下一个已经确认连得通的地址。写失败不影响本次操作，
// 只是下次打开会退回默认地址，所以记日志而不是往上抛。
//
// 地址没变时也照写：省掉那次写要先把旧值读回来解析一遍，而读加解析比写 30 字节更贵。
func rememberHost(host string) {
	if net.ParseIP(host) == nil {
		return
	}
	path, err := stateFile()
	if err != nil {
		log.Printf("netcfg: 无法定位状态文件：%v", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Printf("netcfg: 无法创建 %s：%v", filepath.Dir(path), err)
		return
	}
	raw, err := json.Marshal(state{Host: host})
	if err != nil {
		log.Printf("netcfg: 序列化状态失败：%v", err)
		return
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		log.Printf("netcfg: 写入 %s 失败：%v", path, err)
	}
}

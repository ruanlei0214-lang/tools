package netcfg

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
)

// config.json 编译进产物，改完它需要重新构建才生效。
//
// 单独放一个 config/ 目录，是为了让现场一眼看出这个模块有哪些东西是配置：
// 模块目录里 .go 文件越堆越多之后，一个 config.json 夹在中间并不显眼。
//
//go:embed config/config.json
var configJSON []byte

// defaultConnectTimeout 是 connectTimeoutSeconds 省略时用的值，也是兜底配置里的值。
// 两处共用一个常量，避免出现 0 —— ssh.ClientConfig.Timeout 为 0 表示永不超时，
// 那会让界面卡在「连接中…」再也回不来。
const defaultConnectTimeout = 8

// maxConnectTimeout 拦住离谱的值。连接期间按钮是禁用的且没有取消入口，
// 填个 3600 等于把页面冻住一小时。
const maxConnectTimeout = 120

// Settings 是页面的默认值、连接超时与一键恢复的目标路径。
type Settings struct {
	Device      Device `json:"device"`
	Mask        string `json:"mask"`
	Gateway     string `json:"gateway"`
	RestoreFile string `json:"restoreFile"`
	// ConnectTimeoutSeconds 是 SSH 建连（含认证握手）的等待上限，不限制建连之后
	// 命令执行的时间。设备关机或地址填错时，要等满这么久才会报失败。
	ConnectTimeoutSeconds int `json:"connectTimeoutSeconds"`
	// PersistIface 是「改完要写进 RestoreFile 持久化」的那个网口，通常是 br0。
	// 改其他网口只改运行时地址，重启就没了。
	//
	// 留空表示任何网口都不写文件：validate 不接受空网口名，所以空值永远匹配不上，
	// 不需要额外的开关判断。
	PersistIface string `json:"persistIface"`
	// Warning 非空表示 config.json 不可用，当前这些值来自内置兜底。
	Warning string `json:"warning"`
}

// builtinSettings 是 config.json 不可用时的兜底。配置坏了只影响初始值，
// 没道理连带把 SSH 那套功能一起废掉。
func builtinSettings() Settings {
	return Settings{
		Device:                Device{Host: "192.168.1.100", Port: 22, User: "root"},
		Mask:                  "255.255.255.0",
		RestoreFile:           "/opt/runtime/pi",
		ConnectTimeoutSeconds: defaultConnectTimeout,
		PersistIface:          "br0",
	}
}

func loadSettings() Settings {
	s, err := parseSettings(configJSON)
	if err != nil {
		fallback := builtinSettings()
		fallback.Warning = fmt.Sprintf("config.json 不可用，当前是内置默认值：%v", err)
		return fallback
	}
	return s
}

// parseSettings 只校验会造成实际危害的三项：端口范围、连接超时范围，以及恢复路径
// 必须是绝对路径（它要拼进 rm -f）。地址、用户名、掩码这些本来就是给用户改的初始值，
// 下发前的 validate 会拦，这里再拦一遍等于逼现场先改对配置才能开页面。
func parseSettings(raw []byte) (Settings, error) {
	// Windows 上的编辑器常写出带 BOM 的 UTF-8，encoding/json 不认。
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))

	var s Settings
	if err := json.Unmarshal(raw, &s); err != nil {
		return Settings{}, err
	}

	if s.Device.Port == 0 {
		s.Device.Port = 22
	}
	if s.Device.Port < 1 || s.Device.Port > 65535 {
		return Settings{}, fmt.Errorf("端口 %d 不在 1-65535 之间", s.Device.Port)
	}
	if s.ConnectTimeoutSeconds == 0 {
		s.ConnectTimeoutSeconds = defaultConnectTimeout
	}
	if s.ConnectTimeoutSeconds < 1 || s.ConnectTimeoutSeconds > maxConnectTimeout {
		return Settings{}, fmt.Errorf(
			"connectTimeoutSeconds %d 不在 1-%d 秒之间", s.ConnectTimeoutSeconds, maxConnectTimeout)
	}
	if !strings.HasPrefix(s.RestoreFile, "/") || strings.Trim(s.RestoreFile, "/") == "" {
		return Settings{}, fmt.Errorf("restoreFile 必须是绝对路径，当前是 %q", s.RestoreFile)
	}

	// 配置自带的 warning 字段没有意义，别让它伪装成加载失败。
	s.Warning = ""
	return s, nil
}

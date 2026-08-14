package board

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

//go:embed config/commands.json
var commandsJSON []byte

const (
	defaultPort = 22
	// defaultConnectTimeout 与 maxConnectTimeout：连接期间按钮是禁用的且没有取消入口，
	// 填个 3600 等于把页面冻住一小时。
	defaultConnectTimeout = 8
	maxConnectTimeout     = 120
	// defaultCommandTimeout 是单条指令的上限。设备上跑挂的命令不能让界面一直转圈，
	// 但重启服务这类操作确实要几秒到几十秒，所以给得比连接超时宽。
	defaultCommandTimeout = 30
	maxCommandTimeout     = 600
	defaultRemotePath     = "/opt"
)

// Settings 是页面的默认值与两个超时。
type Settings struct {
	Device                Device `json:"device"`
	ConnectTimeoutSeconds int    `json:"connectTimeoutSeconds"`
	CommandTimeoutSeconds int    `json:"commandTimeoutSeconds"`
	// DefaultPath 是文件标签页打开时填在路径框里的远端目录。
	DefaultPath string `json:"defaultPath"`
	// Warning 非空表示 config.json 不可用，当前这些值来自内置兜底。
	Warning string `json:"warning"`
}

// Device 是主板的 SSH 连接参数。
type Device struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	// KeyPath 是本机私钥路径。选了就先用密钥登录，密码仍可作为密钥口令或备用认证。
	KeyPath string `json:"keyPath"`
}

// builtinSettings 是 config.json 不可用时的兜底。配置坏了只该影响默认值，
// 没道理连带把连接和文件操作一起废掉——现场手填地址照样能干活。
func builtinSettings() Settings {
	return Settings{
		Device:                Device{Host: "192.168.1.136", Port: defaultPort, User: "root"},
		ConnectTimeoutSeconds: defaultConnectTimeout,
		CommandTimeoutSeconds: defaultCommandTimeout,
		DefaultPath:           defaultRemotePath,
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

// parseSettings 只做缺省填充与范围检查。地址、用户名、密码这类现场要改的值一概不拦：
// 空密码是这台设备的真实状态，把它当配置错误挡下来就没法用了。
func parseSettings(raw []byte) (Settings, error) {
	// Windows 上的编辑器常写出带 BOM 的 UTF-8，encoding/json 不认。
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))

	var s Settings
	if err := json.Unmarshal(raw, &s); err != nil {
		return Settings{}, err
	}

	s.Device.Host = strings.TrimSpace(s.Device.Host)
	s.Device.User = strings.TrimSpace(s.Device.User)
	if s.Device.Port == 0 {
		s.Device.Port = defaultPort
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
	if s.CommandTimeoutSeconds == 0 {
		s.CommandTimeoutSeconds = defaultCommandTimeout
	}
	if s.CommandTimeoutSeconds < 1 || s.CommandTimeoutSeconds > maxCommandTimeout {
		return Settings{}, fmt.Errorf(
			"commandTimeoutSeconds %d 不在 1-%d 秒之间", s.CommandTimeoutSeconds, maxCommandTimeout)
	}
	if s.DefaultPath = strings.TrimSpace(s.DefaultPath); s.DefaultPath == "" {
		s.DefaultPath = defaultRemotePath
	}

	// 配置自带的 warning 字段没有意义，别让它伪装成加载失败。
	s.Warning = ""
	return s, nil
}

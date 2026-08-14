package board

import (
	"encoding/json"
	"strings"
	"testing"
)

// 守住随包发布的 config.json：go:embed 只保证文件存在、不看内容，
// 配置写坏本来要到运行时才由兜底接住，那时问题已经进了产物。
func TestEmbeddedConfigIsValid(t *testing.T) {
	s, err := parseSettings(configJSON)
	if err != nil {
		t.Fatalf("config.json 不可用：%v", err)
	}
	if s.Device.Host == "" || s.Device.User == "" {
		t.Errorf("配置里的地址与用户名不该为空：%+v", s.Device)
	}
	if s.DefaultPath == "" {
		t.Error("defaultPath 不该为空，否则文件标签页打开时没有起点")
	}
}

func TestParseSettingsFillsDefaults(t *testing.T) {
	s, err := parseSettings([]byte(`{"device": {"host": "10.0.0.2", "user": "root"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Device.Port != defaultPort {
		t.Errorf("port=%d，期望 %d", s.Device.Port, defaultPort)
	}
	if s.ConnectTimeoutSeconds != defaultConnectTimeout {
		t.Errorf("connectTimeout=%d", s.ConnectTimeoutSeconds)
	}
	if s.CommandTimeoutSeconds != defaultCommandTimeout {
		t.Errorf("commandTimeout=%d", s.CommandTimeoutSeconds)
	}
	if s.DefaultPath != defaultRemotePath {
		t.Errorf("defaultPath=%q，期望 %q", s.DefaultPath, defaultRemotePath)
	}
}

// 空密码是这台设备的真实状态，不能被当成配置错误挡下来。
func TestParseSettingsAcceptsEmptyPassword(t *testing.T) {
	s, err := parseSettings([]byte(`{"device": {"host": "10.0.0.2", "user": "root", "password": ""}}`))
	if err != nil {
		t.Fatalf("空密码被拒了：%v", err)
	}
	if s.Device.Password != "" {
		t.Errorf("密码不该被填上什么东西：%q", s.Device.Password)
	}
}

func TestParseSettingsRejects(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"端口越界", `{"device":{"port":70000}}`, "65535"},
		{"连接超时越界", `{"connectTimeoutSeconds":9999}`, "connectTimeoutSeconds"},
		{"指令超时越界", `{"commandTimeoutSeconds":9999}`, "commandTimeoutSeconds"},
		{"不是 JSON", `{`, "unexpected end"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseSettings([]byte(c.raw))
			if err == nil {
				t.Fatal("应当报错")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err=%v，期望包含 %q", err, c.want)
			}
		})
	}
}

// 配置坏了只该影响默认值，剩下的功能得照常能用，所以兜底本身也必须过校验。
func TestBuiltinSettingsAreValid(t *testing.T) {
	raw, err := json.Marshal(builtinSettings())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parseSettings(raw); err != nil {
		t.Fatalf("兜底配置过不了自己的校验：%v", err)
	}
}

// 配置读不出来时要带着告警退回兜底，而不是让页面打不开。
func TestLoadSettingsNeverFails(t *testing.T) {
	s := loadSettings()
	if s.Device.Host == "" {
		t.Error("无论如何都该有一个默认地址")
	}
}

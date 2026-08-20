package netcfg

import (
	"strings"
	"testing"
)

// TestEmbeddedConfig 守住随包发布的 config.json：go:embed 只保证文件存在、不看内容，
// 配置写坏本来要到运行时才由兜底接住，那时问题已经进了产物。
func TestEmbeddedConfig(t *testing.T) {
	s, err := parseSettings(configJSON)
	if err != nil {
		t.Fatalf("config.json 不可用：%v", err)
	}
	if s.Device.Host == "" || s.Device.User == "" {
		t.Errorf("config.json 里的设备地址与用户名不该为空：%+v", s.Device)
	}
	if s.Mask == "" {
		t.Error("config.json 里的默认掩码不该为空")
	}
	// 0 会被 ssh.ClientConfig 当成「永不超时」，界面会永远停在「连接中…」。
	if s.ConnectTimeoutSeconds < 1 {
		t.Errorf("连接超时必须为正数，当前 %d", s.ConnectTimeoutSeconds)
	}
}

// TestLoadSettingsAlwaysUsable 保证配置这条路不会把页面搞成不可用状态。
func TestLoadSettingsAlwaysUsable(t *testing.T) {
	s := loadSettings()
	if s.Warning != "" {
		t.Fatalf("随包配置应当可用，却退回了兜底：%s", s.Warning)
	}
	if s.RestoreFile == "" {
		t.Error("恢复路径不该为空")
	}
	if s.ConnectTimeoutSeconds < 1 {
		t.Errorf("连接超时必须为正数，当前 %d", s.ConnectTimeoutSeconds)
	}
}

// TestParseSettingsTimeout 单独测超时：它省略时要落到默认值，越界时要拒绝。
// 拒绝很重要——0 表示永不超时，负数和超大值都会让「连接中…」下不来。
func TestParseSettingsTimeout(t *testing.T) {
	const base = `{"device":{"user":"root"},"restoreFile":"/opt/runtime/pi"`

	tests := []struct {
		name    string
		raw     string
		want    int
		wantErr bool
	}{
		{"省略按默认值", base + `}`, defaultConnectTimeout, false},
		{"显式 0 也按默认值", base + `,"connectTimeoutSeconds":0}`, defaultConnectTimeout, false},
		{"正常取值", base + `,"connectTimeoutSeconds":30}`, 30, false},
		{"下界", base + `,"connectTimeoutSeconds":1}`, 1, false},
		{"上界", base + `,"connectTimeoutSeconds":120}`, maxConnectTimeout, false},
		{"负数", base + `,"connectTimeoutSeconds":-1}`, 0, true},
		{"超上界", base + `,"connectTimeoutSeconds":121}`, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSettings([]byte(tt.raw))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if err == nil && got.ConnectTimeoutSeconds != tt.want {
				t.Errorf("超时 = %d, 期望 %d", got.ConnectTimeoutSeconds, tt.want)
			}
		})
	}
}

// 配置坏掉时走的是内置兜底，它同样不能给出 0。
func TestBuiltinSettingsHasTimeout(t *testing.T) {
	if got := builtinSettings().ConnectTimeoutSeconds; got != defaultConnectTimeout {
		t.Errorf("兜底超时 = %d, 期望 %d", got, defaultConnectTimeout)
	}
}

func TestParseSettings(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{"正常配置", `{"device":{"host":"10.0.0.2","port":22,"user":"root"},"mask":"255.255.255.0","restoreFile":"/opt/runtime/pi"}`, false},
		{"省略端口按 22", `{"device":{"host":"10.0.0.2","user":"root"},"mask":"255.255.255.0","restoreFile":"/opt/runtime/pi"}`, false},
		{"带 BOM", "\xef\xbb\xbf" + `{"device":{"host":"10.0.0.2","user":"root"},"restoreFile":"/opt/runtime/pi"}`, false},
		{"坏 JSON", `{"device":`, true},
		{"端口越界", `{"device":{"port":70000},"restoreFile":"/opt/runtime/pi"}`, true},
		{"恢复路径为空", `{"restoreFile":""}`, true},
		{"恢复路径是相对路径", `{"restoreFile":"opt/runtime/pi"}`, true},
		{"恢复路径只有斜杠", `{"restoreFile":"/"}`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSettings([]byte(tt.raw))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tt.wantErr)
			}
			if err == nil && got.Device.Port != 22 {
				t.Errorf("端口 = %d, 期望 22", got.Device.Port)
			}
		})
	}
}

// lan3/lan4 与桥无关，必须在任何 exit 1 之前配完，失败也不能中断另一个口。
func TestSetBridgeAlwaysConfiguresLan34(t *testing.T) {
	s := string(setBridgeScript)
	lan3 := strings.Index(s, `set_fixed lan3`)
	lan4 := strings.Index(s, `set_fixed lan4`)
	exit := strings.Index(s, "exit 1")
	if lan3 < 0 || lan4 < 0 {
		t.Fatal("lan3/lan4 必须通过 set_fixed 配置")
	}
	if exit < 0 || lan3 > exit || lan4 > exit {
		t.Fatal("lan3/lan4 必须在任何 exit 1 之前配置，br0 失败也不能把它们跳过")
	}
	if strings.Contains(s, "DEF_GW") {
		t.Fatal("网关非法时不回退默认值，DEF_GW 是死代码")
	}
}

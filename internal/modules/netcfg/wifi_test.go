package netcfg

import (
	"strings"
	"testing"
)

func TestParseWifiAp(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		ssid string
		pass string
		ch   int
		band string
	}{
		{"两行没有信道", "codroidRobot\ncodroid123", "codroidRobot", "codroid123", 0, band5G},
		{"CRLF 两行", "codroidRobot\r\ncodroid123", "codroidRobot", "codroid123", 0, band5G},
		{"三行信道", "codroidRobot\ncodroid123\n149\n", "codroidRobot", "codroid123", 149, band5G},
		{"第三行不是数字", "a\nb\nxx\n", "a", "b", 0, band5G},
		{"行尾空白", "ssid \n pass \n 36 \n", "ssid", "pass", 36, band5G},
		{"四行 2.4G", "a\nb\n6\n2.4G\n", "a", "b", 6, band24G},
		{"频段小写也认", "a\nb\n6\n2.4g\n", "a", "b", 6, band24G},
		{"频段乱写回落 5G", "a\nb\n149\nxx\n", "a", "b", 149, band5G},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseWifiAp(tt.raw)
			if got.ssid != tt.ssid || got.password != tt.pass || got.channel != tt.ch || got.band != tt.band {
				t.Fatalf("got %+v, want ssid=%q pass=%q ch=%d band=%q", got, tt.ssid, tt.pass, tt.ch, tt.band)
			}
		})
	}
}

func TestValidateChannel(t *testing.T) {
	for _, ch := range []int{36, 149, 165} {
		if err := validateChannel(band5G, ch); err != nil {
			t.Fatal(err)
		}
	}
	if err := validateChannel(band5G, 1); err == nil {
		t.Fatal("5G 下 2.4G 信道必须拒绝")
	}
	if err := validateChannel(band5G, 52); err == nil {
		t.Fatal("DFS 信道必须拒绝")
	}
	if got := validateChannel(band5G, 52); got == nil || !strings.Contains(got.Error(), "DFS") {
		t.Fatalf("DFS 信道的报错必须点名 DFS，实际 %v", got)
	}
	for _, ch := range []int{64, 100, 140, 144} {
		if err := validateChannel(band5G, ch); err == nil {
			t.Fatalf("DFS 信道 %d 必须拒绝", ch)
		}
	}
	if err := validateChannel(band5G, 0); err == nil {
		t.Fatal("0 必须拒绝")
	}
	for _, ch := range []int{1, 6, 13} {
		if err := validateChannel(band24G, ch); err != nil {
			t.Fatal(err)
		}
	}
	if err := validateChannel(band24G, 14); err == nil {
		t.Fatal("2.4G 信道不能超过 13")
	}
	if err := validateChannel(band24G, 149); err == nil {
		t.Fatal("2.4G 下 5G 信道必须拒绝")
	}
}

func TestValidateBand(t *testing.T) {
	if err := validateBand(band5G); err != nil {
		t.Fatal(err)
	}
	if err := validateBand(band24G); err != nil {
		t.Fatal(err)
	}
	if err := validateBand("6G"); err == nil {
		t.Fatal("不支持的频段必须拒绝")
	}
}

func TestWifiApWriteScript(t *testing.T) {
	got := wifiApWriteScript(wifiApPath, wifiApFile{
		ssid: "codroidRobot", password: "codroid123", channel: 149, band: band5G,
	})
	want := "mkdir -p '/opt' && printf '%s\\n%s\\n%d\\n%s\\n' 'codroidRobot' 'codroid123' 149 '5G' > '/opt/wifiAp'"
	if got != want {
		t.Fatalf("实际: %s\n期望: %s", got, want)
	}
}

// 出厂默认值必须和 setWifi.sh 文件缺失时的兜底一致，否则自动建出来的文件
// 和脚本自己脑补的配置对不上。
func TestDefaultWifiApMatchesScript(t *testing.T) {
	if defaultWifiAp.ssid != "760K" || defaultWifiAp.password != "codroid123" {
		t.Fatalf("默认 SSID/密码要和 setWifi.sh 兜底一致，实际 %+v", defaultWifiAp)
	}
	if defaultWifiAp.band != band5G || defaultWifiAp.channel != 149 {
		t.Fatalf("默认频段/信道要和 setWifi.sh 兜底一致，实际 %+v", defaultWifiAp)
	}
	if err := validateChannel(defaultWifiAp.band, defaultWifiAp.channel); err != nil {
		t.Fatalf("默认信道必须合法：%v", err)
	}
}

func TestWifiRestartCmdStaysInOpt(t *testing.T) {
	if !strings.Contains(wifiRestartCmd, "cd /opt && /bin/sh ./setWifi.sh") {
		t.Fatal("重启必须在 /opt 下跑 setWifi.sh，脚本里 wifiAp 是相对路径")
	}
	if !strings.Contains(wifiRestartCmd, "rmmod aic8800_fdrv") || !strings.Contains(wifiRestartCmd, "rmmod aic_load_fw") {
		t.Fatal("驱动重载兜底必须保留")
	}
	if !strings.Contains(wifiRestartCmd, "if ! ifconfig -a") {
		t.Fatal("wlan0 还在时应跳过卸驱动，那是整段重启里最贵的一步")
	}
	if !strings.Contains(wifiRestartCmd, "brctl delif br0 wlan0") {
		t.Fatal("现行脚本把 wlan0 挂进 br0，重启必须先从桥上摘掉")
	}
	if !strings.Contains(wifiRestartCmd, "nohup") {
		t.Fatal("重启必须 nohup：前台跑会把走 br0 的 SSH 一起杀掉")
	}
	if strings.Contains(wifiRestartCmd, "192.168.6.1") {
		t.Fatal("现行脚本不再给 wlan0 配 192.168.6.1")
	}
}

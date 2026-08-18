package netcfg

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"golang.org/x/crypto/ssh"
)

const defaultWifiApFile = "/opt/runtime/wifiAp"
const bootWifiApFile = "/opt/wifiAp"

const (
	band5G   = "5G"
	band24G  = "2.4G"
	bandHint = "5G 或 2.4G"
)

const (
	wifi5GChannelHint  = "36、40、44、48、149、153、157、161、165"
	wifi24GChannelHint = "1-13"
)

// WifiAp 给界面看。密码不往前端传。
type WifiAp struct {
	SSID    string `json:"ssid"`
	Channel int    `json:"channel"`
	Band    string `json:"band"`
}

type wifiApFile struct {
	ssid     string
	password string
	channel  int
	band     string
}

func normalizeBand(s string) string {
	if strings.EqualFold(strings.TrimSpace(s), band24G) {
		return band24G
	}
	return band5G
}

func parseWifiAp(raw string) wifiApFile {
	lines := strings.Split(strings.ReplaceAll(raw, "\r", ""), "\n")
	for len(lines) < 4 {
		lines = append(lines, "")
	}
	ch, _ := strconv.Atoi(strings.TrimSpace(lines[2]))
	return wifiApFile{
		ssid:     strings.TrimSpace(lines[0]),
		password: strings.TrimSpace(lines[1]),
		channel:  ch,
		band:     normalizeBand(lines[3]),
	}
}

func validateBand(band string) error {
	if band == band5G || band == band24G {
		return nil
	}
	return fmt.Errorf("频段只能是 %s，当前是 %q", bandHint, band)
}

func validateChannel(band string, ch int) error {
	if band == band24G {
		if ch >= 1 && ch <= 13 {
			return nil
		}
		return fmt.Errorf("2.4G 信道只能是 %s，当前是 %d", wifi24GChannelHint, ch)
	}
	switch ch {
	case 36, 40, 44, 48, 149, 153, 157, 161, 165:
		return nil
	}
	return fmt.Errorf("5G 信道只能是 %s，当前是 %d", wifi5GChannelHint, ch)
}

// 信道和新频段对不上时 hostapd 起不来，切频段时顺手拉回合法值。
func defaultChannel(band string) int {
	if band == band24G {
		return 6
	}
	return 149
}

func wifiApWriteScript(p string, f wifiApFile) string {
	return fmt.Sprintf("mkdir -p %s && printf '%%s\\n%%s\\n%%d\\n%%s\\n' %s %s %d %s > %s",
		quote(path.Dir(p)), quote(f.ssid), quote(f.password), f.channel, quote(f.band), quote(p))
}

func wifiApPaths(primary string) []string {
	if primary == "" || primary == bootWifiApFile {
		return []string{bootWifiApFile}
	}
	return []string{primary, bootWifiApFile}
}

func readWifiApFile(client *ssh.Client, p string) (wifiApFile, bool, error) {
	if _, err := run(client, "test -f "+quote(p)); err != nil {
		return wifiApFile{}, false, nil
	}
	out, err := run(client, "cat "+quote(p))
	if err != nil {
		return wifiApFile{}, false, fmt.Errorf("读取 %s 失败: %w", p, err)
	}
	return parseWifiAp(out), true, nil
}

func loadWifiAp(client *ssh.Client, primary string) (wifiApFile, bool, error) {
	for _, p := range wifiApPaths(primary) {
		file, found, err := readWifiApFile(client, p)
		if err != nil || found {
			return file, found, err
		}
	}
	return wifiApFile{}, false, nil
}

func (s *Service) GetWifiAp(d Device) (WifiAp, error) {
	client, err := dial(d)
	if err != nil {
		return WifiAp{}, err
	}
	defer client.Close()

	file, _, err := loadWifiAp(client, loadSettings().WifiApFile)
	if err != nil {
		return WifiAp{}, err
	}
	rememberHost(d.Host)
	return WifiAp{SSID: file.ssid, Channel: file.channel, Band: file.band}, nil
}

// writeWifiAp 把同一份配置写进 runtime 和开机目录，setWifi.sh 读的是后者。
func writeWifiAp(client *ssh.Client, primary string, f wifiApFile) error {
	for _, p := range wifiApPaths(primary) {
		if _, err := run(client, wifiApWriteScript(p, f)); err != nil {
			return fmt.Errorf("写入 %s 失败: %w", p, err)
		}
	}
	return nil
}

func (s *Service) loadWifiApChecked(client *ssh.Client) (wifiApFile, string, error) {
	primary := loadSettings().WifiApFile
	file, found, err := loadWifiAp(client, primary)
	if err != nil {
		return wifiApFile{}, primary, err
	}
	if !found || file.ssid == "" {
		return wifiApFile{}, primary, fmt.Errorf("设备上没有可用的 wifiAp（%s 或 %s）", primary, bootWifiApFile)
	}
	return file, primary, nil
}

// ApplyWifi 写入频段和信道，然后后台整段重启 WiFi。channel 为 0 表示保持当前信道；
// 当前信道和新频段对不上时拉回默认值——hostapd 对不上就起不来。
//
// 不做「只重载 hostapd」的轻量路径：切频段要重跑 fcu760k_ap.sh 重建配置，
// 两条路径并存只会让「什么时候该按哪个」变成现场要判断的事。一个按钮，一律重启。
func (s *Service) ApplyWifi(d Device, band string, channel int) (string, error) {
	if err := validateBand(band); err != nil {
		return "", err
	}
	client, err := dial(d)
	if err != nil {
		return "", err
	}
	defer client.Close()

	file, primary, err := s.loadWifiApChecked(client)
	if err != nil {
		return "", err
	}
	if channel != 0 {
		if err := validateChannel(band, channel); err != nil {
			return "", err
		}
		file.channel = channel
	} else if validateChannel(band, file.channel) != nil {
		file.channel = defaultChannel(band)
	}
	file.band = band
	if err := writeWifiAp(client, primary, file); err != nil {
		return "", err
	}
	if _, err := run(client, wifiRestartCmd); err != nil {
		return "", fmt.Errorf("配置已写入，但重启 WiFi 失败: %w", err)
	}
	rememberHost(d.Host)
	return fmt.Sprintf("WiFi 正在后台重启（%s 信道 %d），有线桥会保持。约 10 秒后再刷新。", band, file.channel), nil
}

// SSH 走 br0，前台跑 setWifi.sh 会把会话杀掉。整段丢进 nohup。
//
// 快路径：wlan0 还在就不卸驱动——rmmod+insmod 后 wlan0 要 7 秒才重新出现，
// 是整段重启里最贵的一步，而改频段/信道只需要重建 hostapd 配置，用不着动驱动。
// 快路径没把 hostapd 拉起来时，再完整重载驱动兜底一次（驱动状态坏了的情况）。
const wifiRestartCmd = "nohup sh -c '" +
	"sleep 1; " +
	"killall hostapd udhcpd wpa_supplicant udhcpc 2>/dev/null; " +
	"brctl delif br0 wlan0 2>/dev/null; " +
	"ifconfig wlan0 down 2>/dev/null; " +
	"if ! ifconfig -a 2>/dev/null | grep -q wlan0; then " +
	"rmmod aic8800_fdrv 2>/dev/null; rmmod aic_load_fw 2>/dev/null; sleep 1; fi; " +
	"cd /opt && /bin/sh ./setWifi.sh; " +
	"if ! hostapd_cli -p /var/run/hostapd status 2>/dev/null | grep -q state=ENABLED; then " +
	"killall hostapd udhcpd 2>/dev/null; " +
	"brctl delif br0 wlan0 2>/dev/null; " +
	"ifconfig wlan0 down 2>/dev/null; " +
	"rmmod aic8800_fdrv 2>/dev/null; rmmod aic_load_fw 2>/dev/null; sleep 1; " +
	"cd /opt && /bin/sh ./setWifi.sh; fi; " +
	"ip link set br0 up 2>/dev/null; " +
	"if ! ip addr show br0 2>/dev/null | grep -q \"inet \"; then " +
	"IP=$(sed -n 1p /opt/runtime/ip | tr -d \"\\r\"); " +
	"MASK=$(sed -n 2p /opt/runtime/ip | tr -d \"\\r\"); " +
	"ifconfig br0 \"$IP\" netmask \"$MASK\" up; fi; " +
	"if ! hostapd_cli -p /var/run/hostapd status 2>/dev/null | grep -q state=ENABLED; then " +
	"brctl delif br0 wlan0 2>/dev/null; fi" +
	"' >/tmp/wifi-restart.log 2>&1 &"


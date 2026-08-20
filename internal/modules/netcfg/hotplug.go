package netcfg

import (
	"fmt"
	pathpkg "path"
)

// USB WiFi 热插拔开关在设备上的落点。开：udev 规则里多两条 wlan0 add/remove
// 触发 /opt/shell/fcu760k_hotplug.sh 恢复 AP；关：规则只剩弹存储形态那一条。
const (
	udevRulesPath     = "/lib/udev/rules.d/99-ax900-eject-usbdev.rules"
	hotplugScriptPath = "/opt/shell/fcu760k_hotplug.sh"

	rulesOnName  = "99-ax900-eject-usbdev.rules-hot-plug"
	rulesOffName = "99-ax900-eject-usbdev.rules"
	hotplugName  = "fcu760k_hotplug.sh"
)

// GetWifiHotplug 读设备上热插拔的当前状态：规则文件里带 fcu760k_hotplug 就是开。
// 规则文件不存在视为关——没装规则和老版本规则是同一个效果。
func (s *Service) GetWifiHotplug(d Device) (bool, error) {
	client, err := dial(d)
	if err != nil {
		return false, err
	}
	defer client.Close()

	out, err := run(client, "grep -q fcu760k_hotplug "+quote(udevRulesPath)+" 2>/dev/null && echo 1 || echo 0")
	if err != nil {
		return false, err
	}
	rememberHost(d.Host)
	return out == "1\n" || out == "1", nil
}

// SetWifiHotplug 开启或关闭 USB WiFi 热插拔，重启控制器后生效。
//
// 根文件系统平时挂载为只读，先 remount 成可写再下文件。开启写规则（热插拔版）
// 和恢复脚本两个文件；关闭只把规则盖回出厂版——恢复脚本留着无害，规则不指向它
// 就不会跑。
func (s *Service) SetWifiHotplug(d Device, enable bool) error {
	client, err := dial(d)
	if err != nil {
		return err
	}
	defer client.Close()

	if _, err := run(client, "mount -o remount,rw /"); err != nil {
		return fmt.Errorf("重新挂载根目录为可写失败: %w", err)
	}

	rules := configFile(rulesOffName, udevRulesOff)
	if enable {
		rules = configFile(rulesOnName, udevRulesOn)
	}
	if err := writeRemoteFile(client, udevRulesPath, rules); err != nil {
		return fmt.Errorf("写入 %s 失败: %w", udevRulesPath, err)
	}

	if enable {
		if _, err := run(client, "mkdir -p "+quote(pathpkg.Dir(hotplugScriptPath))); err != nil {
			return err
		}
		if err := writeRemoteFile(client, hotplugScriptPath, configFile(hotplugName, hotplugScript)); err != nil {
			return fmt.Errorf("写入 %s 失败: %w", hotplugScriptPath, err)
		}
	}

	rememberHost(d.Host)
	return nil
}

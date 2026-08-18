// wifiprobe 排查「WiFi 重启后拔掉有线，192.168.1.136 无法登录」：
// 把桥、网口、hostapd、日志的实时状态拉回来。
// 带 -push 时把本地脚本推到设备（CRLF 归一成 LF）。
package main

import (
	"bytes"
	"fmt"
	"os"
	"time"

	"golang.org/x/crypto/ssh"
)

var cmds = []string{
	"brctl show",
	"ip addr show br0; ip addr show wlan0",
	"for i in br0 lan1 wlan0; do echo \"$i operstate=$(cat /sys/class/net/$i/operstate 2>/dev/null) carrier=$(cat /sys/class/net/$i/carrier 2>/dev/null)\"; done",
	"hostapd_cli -p /var/run/hostapd status 2>/dev/null | head -8",
	"pidof udhcpd || echo 'udhcpd 未运行'",
	"pidof hostapd || echo 'hostapd 未运行'",
	"cat /var/run/lan-dhcp-watchdog.pid 2>/dev/null || echo '看门狗未启动'",
}

func main() {
	args := os.Args[1:]
	push := false
	if len(args) > 0 && args[0] == "-push" {
		push = true
		args = args[1:]
	}
	host := "192.168.1.136"
	if len(args) > 0 {
		host = args[0]
	}
	cfg := &ssh.ClientConfig{
		User:            "root",
		Auth:            []ssh.AuthMethod{ssh.Password("")},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         3 * time.Second,
	}
	client, err := ssh.Dial("tcp", host+":22", cfg)
	if err != nil {
		fmt.Println("dial:", err)
		os.Exit(1)
	}
	defer client.Close()

	if push {
		data, err := os.ReadFile("internal/modules/netcfg/config/setWifi.sh")
		if err != nil {
			fmt.Println("read:", err)
			os.Exit(1)
		}
		data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
		s, err := client.NewSession()
		if err != nil {
			fmt.Println("session:", err)
			os.Exit(1)
		}
		s.Stdin = bytes.NewReader(data)
		out, err := s.CombinedOutput("cat > /opt/setWifi.sh && echo pushed $(wc -l < /opt/setWifi.sh) lines")
		fmt.Print(string(out))
		if err != nil {
			fmt.Println("push:", err)
			os.Exit(1)
		}
		s.Close()
		fmt.Println("setWifi.sh 已推到 /opt，下次「应用并重启」或重启控制器后生效")
		return
	}

	for _, c := range cmds {
		fmt.Printf("\n===== %s =====\n", c)
		s, err := client.NewSession()
		if err != nil {
			fmt.Println("session:", err)
			continue
		}
		out, err := s.CombinedOutput(c)
		if err != nil {
			fmt.Println("(exit)", err)
		}
		fmt.Print(string(out))
		s.Close()
	}
}

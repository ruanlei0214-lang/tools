package netcfg

import "testing"

// persistScript 生成的是要在设备上跑的 shell 命令，写错了要到真机才暴露，
// 而那时地址已经改了、连接已经断了，很难查。这里把命令文本本身钉住。
//
// 持久化是三个文件：ip 一行、mask 一行、gateway 一行，和 setBridge.sh 的读取方对应。
func TestPersistScript(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		path string
		want string
	}{
		{
			name: "三项齐全",
			cfg:  Config{IP: "192.168.1.50", Mask: "255.255.255.0", Gateway: "192.168.1.1"},
			path: "/opt/runtime/ip",
			want: `printf '%s\n' '192.168.1.50' > '/opt/runtime/ip'; printf '%s\n' '255.255.255.0' > '/opt/runtime/mask'; printf '%s\n' '192.168.1.1' > '/opt/runtime/gateway'`,
		},
		{
			// 没有默认路由是合法状态，空网关写成空文件，脚本读到空行就不配路由。
			name: "网关为空写空文件",
			cfg:  Config{IP: "10.0.0.2", Mask: "255.255.0.0", Gateway: ""},
			path: "/opt/runtime/ip",
			want: `printf '%s\n' '10.0.0.2' > '/opt/runtime/ip'; printf '%s\n' '255.255.0.0' > '/opt/runtime/mask'; printf '%s\n' '' > '/opt/runtime/gateway'`,
		},
		{
			// 路径由配置提供，出现单引号时不能把命令截断。
			name: "路径里的单引号被转义",
			cfg:  Config{IP: "10.0.0.2", Mask: "255.255.255.0"},
			path: "/opt/it's/ip",
			want: `printf '%s\n' '10.0.0.2' > '/opt/it'\''s/ip'; printf '%s\n' '255.255.255.0' > '/opt/it'\''s/mask'; printf '%s\n' '' > '/opt/it'\''s/gateway'`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := persistScript(tt.cfg, tt.path); got != tt.want {
				t.Errorf("生成的命令不对\n实际: %s\n期望: %s", got, tt.want)
			}
		})
	}
}

// 只有配置指定的那个网口才写文件。写错网口会把别的网口地址塞进持久化文件，
// 重启后控制器就用错地址起来了。
func TestPersistIfaceMatching(t *testing.T) {
	s := loadSettings()
	if s.PersistIface == "" {
		t.Fatal("随包配置应当指定 persistIface")
	}

	if s.PersistIface != "br0" {
		t.Errorf("persistIface = %q，随包配置预期是 br0", s.PersistIface)
	}

	// 空 PersistIface 靠「匹配不上」来关闭持久化，前提是网口名不可能为空。
	if _, err := validate(Config{Iface: "", IP: "10.0.0.2", Mask: "255.255.255.0"}); err == nil {
		t.Error("空网口名必须被 validate 拒绝，否则空 persistIface 会意外触发写文件")
	}
}


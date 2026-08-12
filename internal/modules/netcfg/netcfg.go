// Package netcfg 通过 SSH 远程查看和修改嵌入式 Linux 设备的网络配置。
package netcfg

import (
	"fmt"
	"strings"
)

// Module 是 netcfg 模块的入口。
type Module struct {
	svc *Service
}

func New() *Module {
	return &Module{svc: &Service{}}
}

func (m *Module) ID() string { return "netcfg" }

func (m *Module) Bindings() []any { return []any{m.svc} }

// Device 是一台目标设备的 SSH 连接参数。
type Device struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
}

// Iface 是设备上一个网口的当前状态。
type Iface struct {
	Name    string `json:"name"`
	MAC     string `json:"mac"`
	Up      bool   `json:"up"`
	IP      string `json:"ip"`
	Mask    string `json:"mask"`
	Gateway string `json:"gateway"`
}

// Config 是要下发给某个网口的新配置，Gateway 可以留空表示不改默认路由。
type Config struct {
	Iface   string `json:"iface"`
	IP      string `json:"ip"`
	Mask    string `json:"mask"`
	Gateway string `json:"gateway"`
}

// Service 暴露给前端调用。每次调用都是一次独立的短连接，
// 避免设备改完 IP 后本地还握着一个已失效的连接。
type Service struct{}

// TestConnection 验证连接参数是否可用。
//
// 跑一条命令而不是 dial 成功就算数：dial 能暴露网络不通和认证失败，但暴露不了
// 「认证过了却开不出 session」——dropbear 这类轻量服务上这是真实存在的情况。
// 命令的输出不返回，界面上没有地方显示它。
func (s *Service) TestConnection(d Device) error {
	client, err := dial(d)
	if err != nil {
		return err
	}
	defer client.Close()

	if _, err := run(client, "uname -a"); err != nil {
		return err
	}
	rememberHost(d.Host)
	return nil
}

// ListPorts 读取设备网口，并按机柜面板的口名重新组织后返回。
// 现场只认面板丝印，系统里的网口名不往界面上放。
func (s *Service) ListPorts(d Device) ([]Port, error) {
	client, err := dial(d)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	addrOut, err := run(client, "ip addr show")
	if err != nil {
		return nil, err
	}
	routeOut, err := run(client, "ip route show")
	if err != nil {
		return nil, err
	}
	rememberHost(d.Host)
	return buildPorts(parseInterfaces(addrOut, parseDefaultGateways(routeOut))), nil
}

// ApplyConfig 把新地址下发到设备。
//
// 改地址会切断当前这条 SSH 连接，所以命令被放进后台脱离会话执行：
// 连接中断属于预期结果，不当作失败。成功返回后需要用新地址重新连接。
//
// 改的如果是配置里的 PersistIface（通常是 br0），先把地址写进 RestoreFile 做持久化，
// 重启后仍然生效；改其他网口只改运行时地址。
func (s *Service) ApplyConfig(d Device, cfg Config) error {
	prefix, err := validate(cfg)
	if err != nil {
		return err
	}

	client, err := dial(d)
	if err != nil {
		return err
	}
	defer client.Close()

	// 写文件放在改地址之前，而且是前台执行。两个原因：写文件不会断连，失败能当场报给
	// 用户；而地址一改这条连接就没了，之后再发生什么都看不见。写失败就不改地址了——
	// 用户要的是「改完能留住」，只把运行时地址改掉、持久化却悄悄失败，是更难查的状态。
	settings := loadSettings()
	if cfg.Iface == settings.PersistIface {
		if _, err := run(client, persistScript(cfg, settings.RestoreFile)); err != nil {
			return fmt.Errorf("写入 %s 失败，地址未改动: %w", settings.RestoreFile, err)
		}
	}

	script := applyScript(cfg, prefix)
	if _, err := run(client, fmt.Sprintf("nohup sh -c %s >/dev/null 2>&1 &", quote(script))); err != nil {
		return err
	}
	// 设备接下来会在新地址上，记的是新地址而不是这次连的那个。
	rememberHost(cfg.IP)
	return nil
}

// Defaults 返回页面的默认值，来自编译进产物的 config.json。
// 设备地址优先用上次连通过的那个，没有记录才回到配置里的出厂地址。
func (s *Service) Defaults() Settings {
	settings := loadSettings()
	if host := loadLastHost(); host != "" {
		settings.Device.Host = host
	}
	return settings
}

// RestoreNetwork 删除设备上的网络持久化文件。只删文件，重启由现场人工执行。
// 删哪个文件由 config.json 决定。rm -f 让文件本来就不存在时也算成功。
func (s *Service) RestoreNetwork(d Device) error {
	client, err := dial(d)
	if err != nil {
		return err
	}
	defer client.Close()

	_, err = run(client, fmt.Sprintf("rm -f %s", quote(loadSettings().RestoreFile)))
	return err
}

// persistScript 把地址写进设备的持久化文件，三行依次是 IP、子网掩码、默认网关。
//
// 网关为空时留一个空行而不是只写两行：行号和字段的对应关系是这个文件格式的全部，
// 少一行会让读取方把空网关误当成掩码。
//
// 用 printf 不用 echo：echo 对反斜杠的处理各家 shell 不一致，busybox 尤其。
func persistScript(cfg Config, path string) string {
	return fmt.Sprintf("printf '%%s\\n%%s\\n%%s\\n' %s %s %s > %s",
		quote(cfg.IP), quote(cfg.Mask), quote(cfg.Gateway), quote(path))
}

// applyScript 先 sleep 让 SSH 有机会把命令投递完，再改地址。
func applyScript(cfg Config, prefix int) string {
	cmds := []string{
		"sleep 1",
		fmt.Sprintf("ip addr flush dev %s", cfg.Iface),
		fmt.Sprintf("ip addr add %s/%d dev %s", cfg.IP, prefix, cfg.Iface),
		fmt.Sprintf("ip link set %s up", cfg.Iface),
	}
	if cfg.Gateway != "" {
		cmds = append(cmds,
			"ip route del default 2>/dev/null",
			fmt.Sprintf("ip route add default via %s dev %s", cfg.Gateway, cfg.Iface),
		)
	}
	return strings.Join(cmds, "; ")
}

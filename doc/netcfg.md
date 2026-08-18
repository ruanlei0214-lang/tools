# 网络配置（netcfg）

当前版本 **V1.0.21**，声明在 `frontend/src/modules/netcfg/module.ts`。

## 做什么

通过 SSH 登录嵌入式 Linux 设备，查看网口现状并修改 IP、子网掩码、默认网关。
面向的场景是设备出厂 IP 与现场网段不符，需要在不接串口的情况下远程改地址。

另外提供「一键恢复网络」：删除机器人控制器持久化网络参数的文件（默认
`/opt/runtime/pi`，见下面的「默认配置」），并把 `/opt/setBridge.sh`、`/opt/setWifi.sh`
替换成随包的出厂版本，让控制器网络回到默认状态。面向的场景是那套持久化参数写错了、
启动脚本被现场改坏了，或者设备被搬到别的网段，现场原本只能登录设备手工删文件再重启。

这个功能**只做删除和脚本替换**，不碰设备上的任何其他东西，也不代替人工重启——删完只
弹窗提示需要重启机器人控制器。

## 界面操作

页面收成三块：**设备 → 网口 → 改地址**。地址在顶栏，本页不再重复。

1. **设备**：上一行是刷新、一键恢复和状态；连上之后出现「WiFi 设置」区：WiFi 名、
   密码（密码框，输入不可见）、频段、信道、应用并重启。没刷新过不摆后面这些，免误点。
   「应用并重启」把 WiFi 名、密码（`wifiAp` 第 1、2 行）、频段（第 4 行）和信道
   （第 3 行）写进配置，然后后台整段重启 WiFi。
   `wifiAp` 固定在 `/opt/wifiAp`（`setWifi.sh` 在 `/opt` 下按相对路径读它），不去其他
   目录找；文件不存在时自动按出厂默认值创建（SSID `760K`、密码 `codroid123`、5G 信道 149）。
   整段重启丢进 `nohup` 立刻返回——SSH 走 br0，前台跑会被桥抖动杀掉。
   驱动（AIC8800）只在 `wlan0` 不存在时才卸了重载：实测重载后 `wlan0` 要 7 秒才重新
   出现，是整段里最贵的一步，而改频段/信道只需重建 hostapd 配置。快路径约 7 秒；
   快路径没把 hostapd 拉起来时自动完整重载驱动兜底一次。
   信道留空表示保持现状；和新频段对不上时拉回默认值（5G→149、2.4G→6）。
   信道范围按频段约束并显示在控件下方：5G 只允许非 DFS 信道（36/40/44/48/149/153/157/161/165），
   2.4G 允许 1-13。DFS 信道（52-64、100-144）被单独点名：输入时提示条变红说明
   「雷达避让会让热点长时间不可用」，后端报错同样点名 DFS，而不是只说「不在列表里」。
   「一键恢复网络」删除 `restoreFile`，替换 `/opt/setBridge.sh`、`/opt/setWifi.sh`
   为随包出厂版本，弹窗提醒重启控制器，工具不代劳。
   本模块是独立短连接，点「刷新网络配置」前不用先点顶栏「连接」。

2. **网口**：刷新成功会把网口读回来。表头是口名、IP、掩码、网关。
   五个面板口（`lan1`~`lan5`）竖排；系统里有 `wlan` 网口时多出一行 `wlan`，没有就不占行。
   现场对着机柜找口，只认面板丝印，系统网口名不出现。

   **只有面板 `lan1` 能改**（可点），其余行只读、点不动。面板 `lan3` 恒定为空：
   不由本工具管理，但保留占位，免得少一个口让人以为读漏了。
   对应关系见下面的「面板网口对应关系」。

   刷新失败时列表会被清空。留着上一次的网口会让人以为那是当前设备的状态。

3. **修改地址**：点选可改的网口后出现。改 IP、掩码、网关（网关留空表示不动默认路由），
   点「下发配置」→「确认下发」。填错网段等于把设备从网络上摘掉，所以要二次确认。

   选中的口若和别的口共用同一系统网口（桥接形态下 `lan1`/`lan2`/`lan5`），会写明
   「改一个另外几个一起变」。

   改的如果落在 `persistIface`（默认 `br0`）上，会写进 `restoreFile` 做持久化，
   **重启后仍然生效**；否则只改运行时。下发成功的提示里会写明有没有持久化。

4. 下发成功后，连接地址会自动切换到新 IP，再点「刷新网络配置」确认即可。

## 面板网口对应关系

机柜面板上印的网口名和系统里的网口名对不上，而现场只认面板丝印，所以界面按面板展示，
系统网口名只在下发配置时用，界面上不出现。

对应关系由设备的物理接线决定，没法从系统里推出来，写死在 `ports.go`，**固定不变**：

| 面板口 | 系统网口 |
| --- | --- |
| `lan1` | `lan1`（可改） |
| `lan2` | `lan1`（只读） |
| `lan3` | 不归本工具管 |
| `lan4` | `lan3`（只读） |
| `lan5` | `lan4`（只读） |
| `wlan` | 系统里有 `wlan*` 网口才显示这一行（只读），没有不占行 |

**桥接不再改变对应关系，只改变显示谁的信息。** 对应的系统网口进了桥（`ip addr` 头行里
有 `master br0`）时，这一行显示桥的地址、掩码、网关——地址挂在桥上，成员口自己是空的。
比如系统 `lan1` 在 `br0` 里，面板 `lan1`/`lan2` 显示 `br0` 的信息，下发也落到 `br0`；
`wlan0` 在 `br0` 里时 `wlan` 行同样显示 `br0` 的信息。

几点后果：

- **面板永远是五行**，顺序固定，跟设备上读到几个网口无关。现场是照着丝印找行的。
  `wlan` 行是唯一的变数：有就显示，没有就不占行。
- **多个面板口落在同一个系统网口上时，它们显示同样的地址**，因为它们本来就是同一个网口
  （`lan1`/`lan2` 都是系统 `lan1`；进桥后一起显示桥的信息）。改其中一个就是改全部，
  界面上会提示。
- **对应表里写了、但设备上不存在的系统网口**，那一行退化成空的占位行。与其猜一个别的
  网口顶上去，不如如实显示「这个口现在读不到」。

换机型要改这张表。没有做成配置项：这是接线决定的固定事实，不是部署时需要调的参数，
做成配置只会让「配错了导致改到别的口上」变成一种可能。

## 只有主网口能改地址

**设备主网口**固定是系统 `lan1`（`mainIface` 常量）。只有面板 `lan1` 能在这里改地址，
其余行只读——信息照常展示，但点不动，也不会打开修改表单。`lan2` 和 `lan1` 是同一个
系统网口，同样只读：改地址的入口只留一个，免得现场在两行之间犹豫。`lan1` 进了桥时，
下发目标是桥（配置写到桥上，和 `persistIface` 的 `br0` 正好对上），可改性不变。

标题旁会把当前可改的口列出来，不用对着上面那张表推。

「只读」和「空占位」在列表上都是整行置灰、点不动，区别只在内容：只读行有完整信息，
空占位行全是「—」。代码里对应 `Port.Editable` 与 `Port.Iface == ""`。

这条规则同样由设备侧决定，写死在 `ports.go`，不做成配置。

## 默认配置

页面打开时各输入框的初始值、连接超时，以及一键恢复要删除的路径，都来自
`internal/modules/netcfg/config/config.json`：

```json
{
  "device": {
    "host": "192.168.1.100",
    "port": 22,
    "user": "root",
    "password": ""
  },
  "mask": "255.255.255.0",
  "gateway": "",
  "connectTimeoutSeconds": 3,
  "restoreFile": "/opt/runtime/pi"
}
```

| 字段 | 作用 |
| --- | --- |
| `device.host` | 共享配置里没有地址时的出厂值；改地址在顶栏，写入 `toolbox-config.json` |
| `device.port` / `user` / `password` | SSH 连接用的端口、账号、密码的出厂值；共享配置里有凭据时凭据优先，界面上不显示、改不了；`port` 写 0 或省略按 22 处理 |
| `mask` | 地址表单的默认子网掩码，也用于选中网口时读不到掩码的兜底 |
| `gateway` | 地址表单的默认网关，留空表示不预填 |
| `connectTimeoutSeconds` | SSH 建连的等待上限，写 0 或省略按 3 秒处理，允许 1–120 |
| `persistIface` | 改完要写进 `restoreFile` 持久化的那个网口，默认 `br0`；留空表示任何网口都不持久化 |
| `restoreFile` | 持久化文件的路径，也是一键恢复要删除的文件，必须是绝对路径 |

WiFi 配置文件 `wifiAp` 固定在设备 `/opt/wifiAp`，不可配置；不存在时由工具按出厂默认值自动创建。

### 连接超时

`connectTimeoutSeconds` 管的是**建连加认证握手**这一段，不限制连上之后命令跑多久。
设备关机、地址填错、被防火墙丢包时，要等满这么久才会报失败——这个值直接决定了填错
地址后要干等多长时间。

默认 3 秒：局域网内正常设备建连远快于此，3 秒没连上按失败处理，填错地址不用干等。
现场链路差、设备启动慢可以往上调。

这个值是 **TCP 建连 + SSH 握手 + 认证三步加起来的总预算**，不是每步各给一份，
所以写 3 秒最多就等 3 秒。

<details>
<summary>为什么不能只靠 ssh.ClientConfig.Timeout</summary>

`ssh.ClientConfig.Timeout` 只约束 `net.DialTimeout` 那一段，握手和认证不在其内。
只要对方**接受 TCP 连接却不说 SSH 协议**——透明代理、门户设备、22 端口被别的服务
占用——`ssh.Dial` 就会在等对方发版本号这一步一直挂着。实测对着一个这样的地址等了
40 秒还没返回，界面一直停在「连接中…」。

所以 `dialWithin` 自己拆开做：`net.DialTimeout` 建连，`conn.SetDeadline` 罩住握手，
握手成功后再把 deadline 撤掉——deadline 是设在连接上的，留着的话之后每条命令的读写
都会在那个时间点上一起失败。

`ssh_test.go` 里用一个"只接受连接、一个字节都不发"的本地监听把这个场景钉死了。
</details>

两个边界值会被拒绝，配置退回内置默认并在界面上给出提示：

- **0 或负数**——`ssh.ClientConfig.Timeout` 把 0 当作「永不超时」，界面会永远停在
  「连接中…」再也回不来。所以 0 被当成「没填」，落到默认 3 秒；负数直接判非法。
- **大于 120**——连接期间按钮是禁用的，也没有取消入口，填个 3600 等于把页面冻住一小时。

这份配置用 `go:embed` **编译进 exe**，所以改完要重新构建才生效：

```powershell
.\build.ps1              # 或 go run ./tools/pickbuild
```

改配置只需要动这一个文件——前端不再自带任何默认值字面量。结果提示一律从简：
成功只说「已恢复出厂网络配置，请重启控制器」这种一句话，细节（删了哪个文件、
换了什么地址）不往提示里写，需要排查时看本文档或设备。

### 上次用过的地址

设备地址有个例外：如果之前连通过某个地址，下次打开页面用的是**那个地址**，而不是
配置里的出厂地址。设备改完 IP 之后不用每次重新输入。

记录存在共享配置 `toolbox-config.json`（exe 同目录），remote 和 board 也读这份——
三个模块连的是同一台控制器，地址只该在一个地方改。只有确认连得通
才会记下来——「刷新网络配置」成功（连接与读网口两步各记一次），以及「下发配置」成功（这时
记的是下发的新地址，因为设备接下来在那个地址上）。连不上的地址不会被记住，所以填错一次
不会污染下次打开的初始值。记的时候只动 `host` 一个字段，凭据在 `toolbox-config.json` 里。

想回到配置里的出厂地址，删掉这个文件即可。整夹拷走会一起带走。

这条路径上的任何失败（目录建不出来、文件损坏、写不进去）都不往界面上报，只写日志：
它不影响任何实际功能，弹一条横幅纯属打扰。

输入框的 placeholder 是"该填成什么样"的例子，不是默认值，仍然写在组件里，不进配置。
界面文案（标题、按钮名、提示语）同样不进配置。

**地址和凭据不在本页显示。** 它们都收进共享配置，要改去 exe
同目录的 `toolbox-config.json`。连接失败时的错误消息里带着 `host:port`，排查时还能看到实际用的端口。

## 后端接口

`internal/modules/netcfg` 的 `Service`，前端从 `wailsjs/go/netcfg/Service` 导入。

| 方法 | 签名 | 说明 |
| --- | --- | --- |
| `TestConnection` | `(Device) => Promise<void>` | 验证连接是否可用，不返回设备信息 |
| `ListPorts` | `(Device) => Promise<Port[]>` | 读取网口，按机柜面板口名返回，恒为五个，标出哪些可改 |
| `ApplyConfig` | `(Device, Config) => Promise<void>` | 下发新地址 |
| `GetWifiAp` | `(Device) => Promise<WifiAp>` | 读 wifiAp 的 WiFi 名、密码、信道与频段，预填进编辑框 |
| `ApplyWifi` | `(Device, string, string, string, number) => Promise<string>` | 写 WiFi 名、密码、频段与信道（信道 0 表示保持现状，与新频段不符时拉回默认值），然后 `nohup` 后台整段重启 WiFi，立刻返回 |
| `RestoreNetwork` | `(Device) => Promise<void>` | 删除 `restoreFile`，并替换 `/opt/setBridge.sh`、`/opt/setWifi.sh` 为随包出厂版本 |
| `Defaults` | `() => Promise<Settings>` | 返回页面默认值；地址和 SSH 凭据优先用共享配置 `toolbox-config.json` |

```go
type Device struct { Host string; Port int; User string; Password string }
type Config struct { Iface, IP, Mask, Gateway string }

// Iface 是系统网口，只在模块内部流转，不出现在前端接口上。
type Iface struct { Name, MAC string; Up bool; IP, Mask, Gateway string }

// Port 是机柜面板上的网口，Name 是面板丝印（lan1..lan5）。
// Iface 为空表示这个口不归本工具管，界面上只占位；Editable 为假表示只读，
// 有信息但不能改地址。下发配置时 Config.Iface 填的是这里的 Iface，不是 Name。
type Port struct {
	Name, Iface string
	Editable    bool
	MAC         string
	Up          bool
	IP, Mask, Gateway string
}

// Warning 非空表示 config.json 不可用，这些值来自内置兜底。
type Settings struct {
	Device                Device
	Mask, Gateway         string
	ConnectTimeoutSeconds int
	RestoreFile, Warning  string
}
```

`Port` 传 0 时按 22 处理。所有方法都是独立短连接，用完即关——设备改完 IP 后，
本地握着的旧连接已经失效，复用它只会带来困惑。

## 实现要点

**改地址不会掐断自己。** `ip addr flush` 一执行，当前 SSH 连接立刻断开，
命令后半段就没机会跑完了。所以下发的命令被包成后台任务脱离会话执行：

```sh
nohup sh -c 'sleep 1; ip addr flush dev eth0; ip addr add 192.168.1.10/24 dev eth0; \
             ip link set eth0 up; ip route del default 2>/dev/null; \
             ip route add default via 192.168.1.1 dev eth0' >/dev/null 2>&1 &
```

开头的 `sleep 1` 是留给 SSH 把命令投递完的余量。

**持久化写在改地址之前，而且是前台执行。** 改 `persistIface` 时，先往 `restoreFile`
写三行，成功之后才发改地址的后台命令：

```sh
printf '%s\n%s\n%s\n' '192.168.1.50' '255.255.255.0' '192.168.1.1' > '/opt/runtime/ip'
```

顺序不能反。写文件不断连，失败能当场报给用户；而地址一改这条连接就没了，之后再发生
什么都看不见。所以写失败时**不改地址**直接返回错误——用户要的是"改完能留住"，只把
运行时地址改掉、持久化却悄悄失败，是个更难查的状态：现场看着地址生效了，重启才发现
回到了旧地址。

网关为空时仍然写一个空行。行号与字段的对应关系是这个文件格式的全部，少一行会让读取
方把空网关当成掩码。用 `printf` 而不是 `echo`，因为 `echo` 对反斜杠的处理各家 shell
不一致，busybox 尤其。

**兼容 busybox。** 读取用 `ip addr show` 和 `ip route show` 的原始输出做文本解析，
没用 `-j`（JSON）或 `-o`（单行）这些 busybox 上不一定有的选项。解析同时覆盖
iproute2 和 busybox 两种格式，veth 那种 `eth0@if3` 的命名也能正确取出网口名。

**认证方式给了两种。** 先试 `password`，再试 `keyboard-interactive`，因为 dropbear
这类轻量 SSH 服务常常只开后者。连接超时取自配置（见上面的「连接超时」）。

**下发前做校验，不合法的配置根本不出门：**

- 网口名必须匹配 `^[A-Za-z0-9_.:-]{1,32}$`——这同时挡住了命令注入，
  因为网口名是唯一会被拼进 shell 命令的自由文本
- IP 和网关必须是合法 IPv4
- 掩码必须是连续掩码，`255.0.255.0` 这种会被拒
- 网关必须与新 IP 同网段，否则 `ip route add default via` 会失败

**恢复网络只删文件、替换脚本，工具不代做重启。** 动作是 `rm -f /opt/runtime/pi`，
再把随包的 `setBridge.sh` / `setWifi.sh` 写到 `/opt`（stdin 写入，先归一成 LF——
Windows 签出的 CRLF 会让设备报 `/bin/sh\r: not found`）。重启由现场人工执行。
这条边界是刻意划的：重启机器人控制器影响面大得多，什么时候能重启只有现场知道，
工具替用户按下去不合适。所以模块里没有 `RebootDevice` 这类方法——不留这个口子，
就不会有人顺手调它。

**删除用 `rm -f`。** `-f` 让文件不存在时退出码仍是 0，直接满足「本来就没有也算成功」，
不用额外判断 stderr。路径来自配置，会过一遍 `quote`，与下发配置的写法保持一致。

**配置坏了不阻断启动，只退回内置默认值并提示。** `Defaults` 不返回 error，而是在
`Settings.Warning` 里带回原因。走 error 通道的话前端只能弹一条错误横幅、表单还是空的，
页面基本没法用；而配置坏了并不影响 SSH 那套功能，没理由连带废掉。告警在界面上单独占
一条常驻横幅，不和操作结果那条抢位置——否则点一次「刷新网络配置」就把它冲掉了。

**加载时只校验会造成实际危害的两项。** 端口范围，以及 `restoreFile` 必须是绝对路径
（它要拼进 `rm -f`，空值或相对路径都可能删到意料之外的东西）。地址、用户名、掩码这些
本来就是给用户改的初始值，下发前的 `validate` 会拦，加载时再拦一遍等于逼现场先把配置
改对才能打开页面。

**随包配置的合法性由单测守住。** `go:embed` 只保证文件存在、不看内容，配置写坏本来要到
运行时才由兜底接住，那时问题已经进了产物。`config_test.go` 直接解析嵌入的那份配置并断言
通过校验，把这个错误挪到 `go test` 阶段。

**弹窗只剩标题和一个「知道了」。** 这里没有要用户做的决定——重启这件事工具做不到，
弹窗的唯一作用是别让人删完就忘了还要重启，标题一句话就说完了。banner 同样只给
一句「已恢复出厂网络配置，请重启控制器」，不罗列删了哪个文件。

**弹窗是模块自带的，没有进全局样式。** 项目没有 UI 组件库，所以在 `NetcfgView.vue` 里
写了一个 scoped 的遮罩加卡片，约二十行 CSS。没放进全局 `style.css`，是为了不让它变成
跨模块的共享控件。也没用 `window.confirm`：它在 WebView2 里的外观和应用风格脱节，
而且会阻塞渲染。

## 已知限制

**面板对应关系写死，只适用当前机型。** 换一台接线不同的设备，界面会指错口——而且不会
报错，只是安静地把地址改到别的网口上。改机型必须同步改 `ports.go` 里的对应表。

**面板上的系统网口读不到时不给提示。** 那一行只是变空，和恒定为空的 `lan3` 长得一样。
分不清「这个口本来就不归工具管」和「这个口应该有但没读到」。

**只读网口在这个工具里没有任何改法。** 面板 `lan4`（以及无桥时的 `lan5`）能看不能改，
工具不提供任何绕过方式——要改只能上设备手工操作。

**只有 `persistIface` 会持久化。** 改 `br0` 会把地址写进 `restoreFile`，重启后仍然
生效；改其他网口只用 `ip` 命令改运行时状态，重启回到原样。没有为其他网口做持久化，
是因为通用做法各家设备差别太大（`/etc/network/interfaces`、netplan、自定义启动脚本），
猜不得。

**持久化的生效依赖设备自身的启动脚本。** 工具只负责把三行写进文件，控制器重启时会不会
读它、怎么读，是设备侧的事。写成功不等于重启后一定生效——这一点和「一键恢复」是同一个
边界：工具管文件，不管文件被怎么用。

**不校验主机密钥。** 用的是 `InsecureIgnoreHostKey`。面向内网设备，而且这工具本身
就在改设备 IP，固定主机密钥没有意义。不要拿它连公网主机。

**面板口不展示 MAC。** 现场改地址只看 IP/掩码/网关；MAC 仍在后端 `Port` 里，
需要排查时去设备上查。

**只显示每个网口的第一个 IPv4 地址。** 一个网口配了多个地址时，界面只展示第一个，
下发时 `ip addr flush` 会把该网口的地址全部清掉再加新的。

**恢复网络只保证文件被删掉，不保证网络真的恢复。** 删掉 `/opt/runtime/pi` 之后控制器
变成什么样，取决于设备自身的启动脚本，工具这边看不到也管不了。所以界面文案只说
「已恢复出厂网络配置」，没有承诺结果。

**改默认配置要重新构建。** 配置是编译进 exe 的，现场拿到成品改不了默认值。这是「产物是
单个免安装 exe」换来的代价：没有需要一起分发的外部配置文件。

**配置里的密码会明文进 exe。** `strings` 一翻就能看到，所以 `password` 默认留空，
别在配置里放生产密码。

**恢复网络不重启设备。** 删完文件要人工重启机器人控制器，工具只弹窗提醒，不下发
`reboot`，也不做重启后的探活或自动重连。

**WiFi 重启按设备脚本走，不另写一套。** 设备开机是 `/opt/autorun.sh` → `cd /opt; ./setWifi.sh`。
现行 `setWifi.sh` 先用 `fcu760k_init.sh` / `fcu760k_ap.sh` 起 AP，再改 hostapd 信道、
把 `wlan0` `brctl addif br0`，DHCP 改挂 `br0`。按钮必须先 `brctl delif` 再 `rmmod`，
否则桥还占着网卡，驱动卸不掉。脚本里 `wifiAp` 是相对路径，必须在 `/opt` 下执行。
成功后读 `brctl show` / `iw`，不回脚本 stdout（里面有密码）。

**没有真机验证。** 解析和校验逻辑有单元测试覆盖（`parse_test.go`，包含 busybox
与 iproute2 两种输出格式），但完整链路还没在真实设备上跑过——包括恢复网络，
它本身没有可单测的纯逻辑。

## 相关文件

```
internal/modules/netcfg/netcfg.go       模块入口、类型定义、对前端暴露的方法
internal/modules/netcfg/config/config.json  默认配置，编译进产物，改完要重新构建
internal/modules/netcfg/config.go       配置的嵌入、解析、校验与兜底
internal/modules/netcfg/config_test.go  配置解析与随包配置合法性的单元测试
internal/modules/netcfg/state.go        记住上次连通的设备地址
internal/modules/netcfg/state_test.go   地址记忆的单元测试
internal/modules/netcfg/persist_test.go 持久化写入命令的单元测试
internal/modules/netcfg/wifi.go         读改 wifiAp 信道与频段、后台重启 WiFi
internal/modules/netcfg/wifi_test.go    信道/频段解析、校验与写入命令的单元测试
internal/modules/netcfg/config/setBridge.sh  随包的网桥启动脚本，一键恢复时替换到 /opt
internal/modules/netcfg/config/setWifi.sh    随包的 WiFi 启动脚本，读 wifiAp 第 4 行频段
internal/modules/netcfg/ssh.go          SSH 连接与命令执行
internal/modules/netcfg/ssh_test.go     连接超时（含握手）的回归测试
internal/modules/netcfg/parse.go        输出解析与配置校验
internal/modules/netcfg/parse_test.go   解析与校验的单元测试
internal/modules/netcfg/ports.go        系统网口到机柜面板网口的对应关系
internal/modules/netcfg/ports_test.go   面板对应关系的单元测试
frontend/src/modules/netcfg/module.ts   模块清单
frontend/src/modules/netcfg/NetcfgView.vue  界面
```

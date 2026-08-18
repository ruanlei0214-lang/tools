# 网络检测（ping）

当前版本 **V1.1.2**，声明在 `frontend/src/modules/ping/module.ts`。

## 做什么

探测**本机所在网络**，不连控制器。页面分两个页签：

1. **Ping**：对单个地址（IP 或主机名）长 ping，每秒一个包，日志逐行显示，
   随时可停，停时给出收发统计和时延。
2. **扫描网段**：扫一个或多个网段（可指定范围），列出在线设备的 IP、设备名、
   MAC 和延迟，并检查 IP/MAC 冲突。

面向的场景是拉线现场：设备地址不确定、网段里接了几台机器不清楚时，先扫一遍再连；
链路不稳定时，长 ping 挂着看丢包。

## 界面操作

### Ping 页签

输入 IP 或主机名，回车或点「开始」。日志区每秒一行：`通，0.3ms` 或「超时」。
按钮变成红色「停止」，随时可点；停止后日志末尾给一行统计
（发出/收到/丢包率，有收到包时带最小/平均/最大时延）。

切到「扫描网段」页签不会掐断长 ping；切到别的模块（组件被卸载）会停掉。

### 扫描网段页签

扫描目标拆成三个框：**前三段 + 末段起点 + 末段终点**，默认扫整个 /24（1~254），
想扫哪一段改两个数字框即可。多块网卡时行首有下拉框（网卡名 + 本机 IP），选哪块
就把它的前三段填进第一个框，打开页面时默认选第一块。前三段里粘了完整 IP 也没事，
末段以两个数字框为准。

扫完状态栏报「在线 N 台 / 共扫 M 个地址，用时 Xs」，下面表格按 IP 排序列出
在线设备。设备名和 MAC 拿不到时显示「—」。

发现地址冲突时状态栏追加「发现 K 处冲突」，下面列出琥珀色的冲突明细，
表格里涉及的行同步高亮：

- **IP 冲突**：同一个 IP 在扫描期间被多个 MAC 应答——网段里有不止一台设备
  占着这个地址。
- **MAC 冲突**：同一个 MAC 对应多个在线 IP。可能是 MAC 仿冒，也可能是正常的
  一台设备配了多个地址（或路由器代理 ARP），报出来由现场判断。

## 后端接口

`internal/modules/ping` 的 `Service`，前端从 `wailsjs/go/ping/Service` 导入。

| 方法 | 签名 | 说明 |
| --- | --- | --- |
| `StartPing` | `(host string) => Promise<void>` | 开始长 ping，每秒一个包；已在跑的会被停掉重新开始 |
| `StopPing` | `() => Promise<void>` | 停止长 ping，不在跑时调用不算错误 |
| `ReadPing` | `() => Promise<PingLog>` | 取走上次以来攒下的日志行，`Running` 表示还在不在跑 |
| `Scan` | `(input string) => Promise<ScanResult>` | 扫网段，返回在线设备列表 |
| `LocalIfaces` | `() => Promise<LocalIface[]>` | 本机启用中的网卡及各自的 IPv4 /24 网段，给扫描框做网卡选择 |

```go
type PingLog struct {
	Lines   []string
	Running bool
}

// Name / MAC 拿不到时留空：跨网段的 MAC 本来就拿不到（那是网关的），
// 没有 PTR 记录的设备也没有名字。
type ScanHost struct {
	IP    string
	Name  string
	MAC   string
	RttMs float64
}

type ScanResult struct {
	Hosts     []ScanHost
	Conflicts []Conflict
	Total     int // 实际扫过的地址数
	ElapsedMs int64
}

// Kind 为 "ip"：同一个 IP 被多个 MAC 应答（Peers 是 MAC 列表）；
// Kind 为 "mac"：同一个 MAC 对应多个在线 IP（Peers 是 IP 列表）。
type Conflict struct {
	Kind  string
	Addr  string
	Peers []string
}

// 选哪块网卡，就扫它所在的网段。
type LocalIface struct {
	Name    string // 网卡名，如「以太网」
	IP      string // 本机在这个网段的地址
	Segment string // 前三段，如 192.168.1
}
```

## 实现要点

**Ping 走 Windows 的 `IcmpSendEcho`（iphlpapi.dll），不需要管理员权限。** 这是系统
`ping.exe` 自己用的 API。现成的 Go 库（go-ping、pro-bing）在 Windows 上都是开原始
套接字，必须管理员才能跑，现场不会接受「右键以管理员身份运行」。自己调
`IcmpSendEcho` 只有几十行（`echo_windows.go`），每次调用独立开句柄，扫描时上百个
goroutine 并发调它，不共享状态。

**目标地址的字节序是个坑。** `IcmpSendEcho` 的 `Destination` 参数要和 `inet_addr`
的返回一致：网络字节序的字节按小端装进 `ULONG`，对应 Go 里是
`binary.LittleEndian.Uint32(ip.To4())`。用 `BigEndian` 会把 `192.168.1.1` 发去
`1.1.168.192`——表现为大面积超时、偶尔收到一个 300ms 的公网回包。

**长 ping 的日志走轮询，不走事件。** 后端 goroutine 每秒 ping 一次、把日志行攒进
缓冲区，前端每 500ms 调 `ReadPing` 取走——和终端模块读输出的方式一样，不引入
第二套推送机制。缓冲区上限 500 行，防的是前端停在别的页签时日志无限攒。

**日志逐行渲染成 div，不用 pre。** 追加行只加新节点、不动已有节点，用户正在选择的
文字不会被新日志打断——终端模块在 pre 上踩过这个坑，那边的解法（选择期间缓冲输出）
在这里用不上，因为逐行 div 本来就不重排已有内容。

**扫描快靠并发，不靠压缩超时。** 128 个地址同时探，单个地址只发 1 个包、等 500ms。
一个 /24（254 个地址）两波就完事，连同设备名解析在内 1–2 秒出结果。超时再往下压
（比如 100ms）会把链路差的在线设备误判成不在线，不值。

**MAC 从 ARP 缓存读。** 扫完跑一遍 `arp -a` 解析——刚 ping 通的地址都在缓存里，这是
拿 MAC 最便宜的办法，不用自己实现 ARP 请求。解析按 IP/MAC 的格式匹配，与系统语言
无关（中文「动态」和英文 "dynamic" 都能匹配）。MAC 统一转大写。GUI 程序拉起
`arp.exe` 时用 `HideWindow` 挡住控制台窗口闪烁（`console_windows.go`）。

**冲突检查靠多次采 ARP 表。** 扫描前、ping 后、设备名反查后各采一次：同一个 IP 的
MAC 在采样间变了，就是有不止一台设备在应答它。原始 ARP 报文能直接看到多个应答，
但在 Windows 上要管理员/Npcap，不考虑。这个办法抓得住「正在打架」的冲突，抓不住
两台设备错开时间用同一个 IP 的情况。

**设备名做反向 DNS，并发加短超时。** 只对在线地址查，每个 300ms 超时，查不到留空。
逐个串行查的话，几个没 PTR 记录的地址就能把扫描拖慢好几秒。

**一次最多扫 2048 个地址。** `/16` 有六万多个地址，再并发也要扫一分多钟，直接拒绝并
提示把网段拆小。重复地址先去重再计数。

**CIDR 展开去掉网络号和广播号，区间展开不去。** `/24` 这类网段跳过一头一尾；
`/31`、`/32` 没有网络号/广播号的概念，全保留。`192.168.1.10-100` 是用户点名要扫的
范围，一个都不少。

## 已知限制

**设备名依赖反向 DNS。** 设备没注册 PTR 记录就查不到名字，显示「—」。Windows 的
NetBIOS 名字能不能解析到取决于系统解析器，不保证。

**跨网段拿不到 MAC。** 经过路由器的地址，ARP 缓存里只有网关的 MAC，扫描结果里这些
地址的 MAC 留空。这是 ARP 协议本身的边界，不是实现省略。

**ICMP 被防火墙挡住的设备扫不到。** 设备在线但不回 ping 时不会出现在结果里。

**IP 冲突检测会漏也会误报。** 它看的是扫描期间 ARP 缓存的变动：两台设备正在抢
同一个 IP 时抓得住；错开时间用同一个 IP 的抓不住。刚换过设备时旧 MAC 还留在缓存里，
会误报一次——再扫一遍，缓存已是新 MAC，就不再报了。

**只支持 IPv4。**

## 相关文件

```
internal/modules/ping/ping.go             模块入口、长 ping（StartPing/ReadPing/StopPing）、LocalIfaces
internal/modules/ping/scan.go             网段/区间解析、并发扫描、ARP 与设备名解析
internal/modules/ping/echo_windows.go     IcmpSendEcho 封装（Windows，不需要管理员）
internal/modules/ping/console_windows.go  拉起 arp.exe 时隐藏控制台窗口
internal/modules/ping/console_other.go    非 Windows 的空实现
internal/modules/ping/ping_test.go        网段/区间解析与 ARP 解析的单元测试
frontend/src/modules/ping/module.ts       模块清单
frontend/src/modules/ping/PingView.vue    页签外壳
frontend/src/modules/ping/PingPanel.vue   长 ping 页签
frontend/src/modules/ping/ScanPanel.vue   扫描网段页签
```

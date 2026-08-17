# 远程控制（remote）

当前版本 **V1.3.11**，声明在 `frontend/src/modules/remote/module.ts`。

## 做什么

通过控制器远程模式接口控制上位机。页面顶部是参数区，下面是若干标签页。
连接参数、IO 点位、寄存器点位**都在界面上改，改完立即生效**，不用重新构建也不用重启，
详见「配置文件」。

**传输层是 WebSocket**，地址是 `ws://<host>:<port><path>`，默认
`ws://192.168.1.136:9000/`。注意这一点和接口文档不一致：
`doc/api_documentation/远程模式接口说明.md` 写的是「使用 TCP/IP 协议…端口号为 9001」，
但现场这台控制器把远程接口挂在 WebSocket 上，文档没跟着更新。

**报文格式照文档**：一帧一条 JSON 文本，请求 `{"id","ty","db"}`，响应同结构、
失败时多一个 `err`。用到的 `ty` 见文档的「IO 相关接口」「寄存器相关接口」。

握手路径文档里没写。参数区「参数」里的路径是首选，连不上时会依次再试
`/`、`/ws`、`/websocket`、`/api`；真正连上的完整地址会显示在顶栏 WS 状态点的悬停提示里，
照着把它钉回 `path` 就不用每次再探。主机本身连不上时不做探测，直接报错。

## 界面操作

1. **连接区在顶栏，不在本页**。顶栏显示共享地址和 SSH / WS 两个状态点，一个「连接」
   按钮同时建立两条连接；地址在顶栏「凭据」弹层里改，全系列工具共用。
   本页顶部只剩本模块自己的东西：WS 端口、「参数」（路径、超时、刷新间隔）和状态槽。
   程序不会自动连——开机时设备还没起来，一打开就连会把程序卡住。
2. **IO 控制**标签页：操作栏放在点位上方，操作状态显示在同一行右侧；每个分组一列，左右并排（输出 DO 一列、
   输入 DI 一列，左右顺序就是配置里分组的顺序）。一个点位占一行：名称、点位号、
   动作、当前值，当前值恒在行尾。名称上悬停能看到完整名称和这一路的说明。
   所有点位默认一起自动刷新，间隔是配置里的 `refreshIntervalMs`（默认 1 秒）。
3. 开关量（IO 的 `DI` / `DO`、寄存器的 `BOOL`）点一下行尾那个 ON/OFF 就在两个值之间来回切换。
   配了 `pulseMs` 的点位在它左边还有一个「点动」按钮，写 `onValue`、等一会儿、再写回 `offValue`。
   输出随时能点；输入要先开强制。
4. 非开关量（IO 的 `AO`、寄存器的 `INT` / `FLOAT`）不走 ON/OFF。行尾是「输入框 + 下发 +
   当前值」：`INT` 填整数，`FLOAT` 填数字或任意文本（按字符串原样下发），`AO` 填数字。
   输入框预填配置里的 `value`，**自动刷新不会回填它**。`AI` 仍然只读。
5. 输入点位默认只读（值显示成虚线框）。顶部操作栏的「一键强制」会对配置里**所有 DI** 逐路发
   `SetIOForcedFlag`，打开之后才能切换和点动。再点一次变成「取消强制」。刷新只读当前值，
   不去问强制标志。断开时把本会话打开的强制全部清掉。详见「关于强制输入」。
6. **测试流程**在 IO 控制右侧单列，尽量少占地方。平时一行一个步骤，只显示名称（默认点动不标；ON / OFF / 下发才标出来），悬停看类型、端口和间隔。点某一步就从那里开始。「单步」触发当前高亮，「连续」按间隔接着跑，跑的时候同一个按钮变成「停止」。点「编辑」再「＋」，先选动作再点左侧点位名称加入（模拟量自动下发）。导入导出和恢复默认收在 ⋯ 里。DI 单步时会先开强制再写。
7. **寄存器**标签页和 IO 同一套界面：分组、自动刷新、切换、点动、手填下发都从配置里长出来。
   `BOOL` 显示 ON/OFF 并可切换，`INT` 填整数下发，`FLOAT` 填数字或文本下发。读写走
   `RegisterManager/GetRegisterValue` 和 `RegisterManager/SetRegisterValue`
   （见 `doc/api_documentation/远程模式接口说明.md`）。寄存器没有强制标志，配出来的地址都可以写。
8. 连接归顶栏管，切换模块不断开；要断开点顶栏「断开」，两条连接一起收。

### 改点位

1. 点操作栏的「编辑点位」进编辑态。这时点位的当前值换成编辑和删除两个图标，
   点位列表仍在原处——编辑时要对着它看。编辑不需要连接。
2. 点某一行的 ✎ 就地展开表单。开关量（`DI` / `DO` / `BOOL`）字段是名称、类型、端口/地址、
   ON 值、OFF 值、点动 ms、备注、醒目；`INT` / `FLOAT` / `AO` / `AI` 没有 ON/OFF 和点动，
   改成一个「默认值」（`INT` 整数，`FLOAT` 数字或文本）。名称留空会按类型和端口自动生成
   （`DO15`、`BOOL10000`）。
3. 组标题右边的 ＋ 往这一组末尾加点位，✕ 删掉整个分组（确认框里会说这一组有几个点位）。
   操作栏的「＋ 分组」加一个新分组，组名直接在标题那个输入框里改。
4. 「保存」把整份清单送回后端校验后落盘，当前页立刻按新清单重画；不合格会在状态栏说明
   哪一条不对，盘上那份不动。「取消」丢掉这次的改动。「恢复默认」删掉现场这一份，
   退回编译进产物的出厂默认。
5. 新加的或改过端口的点位在被刷新到之前显示 `—`，不显示 `OFF`：拿旧端口的读数
   冒充新点位的状态，是「界面显示 OFF、现场却是通的」这种最难查的假象的来源。
6. 寄存器页的编辑界面和 IO 页完全一样，只差类型可选值（`BOOL` / `INT` / `FLOAT`）
   和端口的叫法（「地址」）。
7. **导入 / 导出**在操作栏上，编辑态也能用，不需要连接。导出把当前这一页存成 JSON
   （和 exe 旁边 `remote-io.json` / `remote-register.json` 同一格式）。导入用选中的
   文件整份替换这一页，过不了校验就不写盘；把寄存器文件导进 IO 页（或反过来）会被拒。
   导入成功立刻生效。取消选文件等于什么都没做。

### 改连接参数

参数区的「参数」按钮在这一行下面展开路径、建连超时、请求超时、刷新间隔
（这几个值一年动不了一次，常显会把参数区撑回两行）。「保存」之后：

- 两个超时按新值算下一次请求，刷新间隔立刻换掉正在跑的定时器，操作栏那句
  「每 N 秒自动刷新」跟着变。
- 端口、路径只是存下来，**当前连接一动不动**，要用新端口得自己点顶栏「连接」。
  状态栏会提示这一点。不自动重连：这个页面上的按钮会动现场的气缸，换设备的时机由人决定。
- 地址不在这份参数里：它归顶栏「凭据」弹层，写进共享配置，本模块保存时不再碰它。

## 关于强制输入

直接 `SetIOValue` 写 DI **不会生效**：控制器接受请求，但值立刻被现场扫描盖掉。
要改 DI，得先发 `IOManager/SetIOForcedFlag`，再 `SetIOValue`。
控制器没有「全部 DI」这种批量指令，一键强制就是对配置里每一路 DI 各发一次。

```json
{
  "ty": "IOManager/SetIOForcedFlag",
  "db": { "type": "DI", "port": 3, "value": 1 },
  "id": "…"
}
```

`value` 为 `1` 打开这一路的强制，为 `0` 关掉。打开之后再 `SetIOValue` 写实际值。
界面上是点位上方的「一键强制」：对配置里每一路 DI 各发一次上面这条。刷新不再去读强制标志。
断开时把本会话打开过的标志全部发 `0` 清掉。

2026-08 在 `192.168.1.136` 上还实测过这些（都不是从文档推出来的）：

| 项 | 实测结果 |
| --- | --- |
| `SetIOValue` 认的端口号 | DI `0-24`、DO `0-17`、AI `0-3`、AO `0-3`；超出回 `1000/Failed to set IO value: invalid DI port.` |
| `GetIOValue` 对不存在的端口 | **照样回 `0`，不报错**（DI31 读得出来，写就被拒） |
| 未开强制就写 DI | 请求被接受，但**值不变** |

端口号超范围的点位在界面上是个正常的 `OFF`（读不报错），只有点下去才报
`invalid DI port`。写完输入点位仍会读回来核对一次，没开强制或强制没生效时会报
「已下发 1，但读回仍是 0：这一路没有真的被改动」。

## 配置文件

配置分三层：

| 层 | 位置 | 谁写 |
| --- | --- | --- |
| 共享配置 | exe 同目录的 `toolbox-config.json` | 三个模块共用，只放 `host` + SSH 凭据 |
| 现场配置 | exe 同目录的 `remote-config.json`、`remote-io.json`、`remote-register.json`、`remote-io-flow.json` | 界面上保存或导入时写，优先用 |
| 出厂默认 | `internal/modules/remote/config/config.json`、`io.json`、`register.json`、`io-flow.json` | 编译进产物，改它要重新构建 |

**地址只在一个地方改。** `toolbox-config.json` 里的 `host` 优先于 `remote-config.json` 里的；
界面上不再给 host 输入框，要改地址去顶栏「凭据」弹层，保存后全系列工具共用。
端口、路径、超时仍归本模块，在「参数」里改。

加载时三部分各自「现场那份有就用它，没有就用出厂默认」。干净的一台机器上一份现场配置都没有，
界面和出厂默认完全一致、没有告警；界面上第一次保存才生成对应文件。「恢复默认」做的事就是
删掉现场那一份。

分成三个文件而不是一份：逐份「恢复默认」不牵连另外两份，某一份坏掉时另两份照常可用。
合成一份之后，IO 点位表里少个逗号会把连接参数一起废掉。

某一份读不出来或过不了校验时，只有那一部分退回出厂默认，其余照常，顶部告警里带上
文件名和出错的行列号。**坏文件不会被覆盖**——里面可能还有能人工救回来的点位，
可以照着告警打开它自己修。

配置就在 exe 同目录（`netcfg` 记设备地址、`board` 存按钮清单也在这里）。
整夹拷走会一起带走；只拷一个 exe 文件则不会。界面上「配置存在本机」悬停能看到完整路径。

下面的例子就是三份出厂默认的样子，现场配置的格式完全相同。

`config.json` 只管连谁、超时多久：

```json
{
  "device": { "host": "192.168.1.136", "port": 9000, "path": "/" },
  "connectTimeoutSeconds": 5,
  "requestTimeoutSeconds": 8,
  "refreshIntervalMs": 1000
}
```

`io.json` 管 IO 按钮：

```json
{
  "id": "io",
  "title": "IO 控制",
  "groups": [
    {
      "title": "输出 DO",
      "points": [
        { "label": "上料前吹气", "type": "DO", "port": 0 },
        { "label": "开门指令", "type": "DO", "port": 3, "pulseMs": 500 },
        { "label": "反相点位", "type": "DO", "port": 4, "onValue": 0, "offValue": 1 },
        { "label": "急停", "type": "DO", "port": 9, "danger": true, "hint": "现场确认安全后再按" }
      ]
    },
    {
      "title": "输入 DI",
      "points": [{ "label": "安全OK", "type": "DI", "port": 3 }]
    }
  ]
}
```

`register.json` 管寄存器按钮：

```json
{
  "id": "register",
  "title": "寄存器",
  "groups": [
    {
      "title": "布尔 BOOL",
      "points": [
        { "label": "BOOL10000", "type": "BOOL", "port": 10000, "pulseMs": 500 }
      ]
    },
    {
      "title": "整数 INT",
      "points": [{ "label": "计数", "type": "INT", "port": 20000, "value": "0" }]
    }
  ]
}
```

`io-flow.json` 管测试流程：

```json
{
  "id": "io-flow",
  "title": "测试流程",
  "steps": [
    { "label": "开门指令", "type": "DO", "port": 3, "action": "pulse", "pulseMs": 500, "delayMs": 1000 },
    { "label": "卡盘夹紧", "type": "DO", "port": 6, "action": "on", "delayMs": 500 }
  ]
}
```

| 字段 | 说明 |
| --- | --- |
| `action` | `pulse` 点动、`on` / `off` 置位、`set` 只给 AO。省略时 DO/DI 按 `pulse`，AO 按 `set` |
| `pulseMs` | 点动持续时间，20–10000，省略按 300 |
| `delayMs` | 连续跑时本步完成后再等多久，0–60000。单步只作提示 |
| `value` | `set` 时要下发的数字 |

`config.json` 顶层字段：

| 字段 | 说明 |
| --- | --- |
| `connectTimeoutSeconds` | 建连超时，1–60，省略按 `5` |
| `requestTimeoutSeconds` | 单次请求等响应的上限，1–120，省略按 `8` |
| `refreshIntervalMs` | IO / 寄存器标签页自动刷新的间隔，200–60000，省略按 `1000`。界面上那句「每 N 秒自动刷新」跟着它变 |

这几个范围界面上保存时用的是同一套校验，越界会被拒，盘上那份不变。

`io.json` / `register.json` 字段：

| 字段 | 说明 |
| --- | --- |
| `id` | 省略时 IO 为 `io`、寄存器为 `register` |
| `title` | 标签文字，省略时用 `id` |
| `groups` | 不能为空，每组也不能一个点位都没有 |

点位字段。IO 和寄存器用同一份定义，界面上因此长得一样：

| 字段 | 说明 |
| --- | --- |
| `label` | 省略时显示成 `DI0` / `BOOL10000` 这样的名字 |
| `type` | IO：`DI` / `DO` / `AI` / `AO`。寄存器：`BOOL` / `INT` / `FLOAT`（对应指令文档的 bool/int/float） |
| `port` | IO 是端口号；寄存器是地址（`GetRegisterValue` 的 `address`），非负 |
| `onValue` / `offValue` | 只给开关量。切换时写的两个值，省略（或写成一样）时按 `1` / `0`。写成 `0` / `1` 就是反相点位 |
| `value` | 只给 `INT` / `FLOAT` / `AO` / `AI`。输入框的预填值，下发也用它。`INT` 必须是整数，`FLOAT` 可以是数字或任意文本 |
| `pulseMs` | 只给开关量。填了就多一个「点动」按钮：写 `onValue`，等这么久，再写回 `offValue`。范围 20–10000 |
| `danger` | 格子边框显示成红色，给急停一类的动作用 |
| `hint` | 覆盖下行那句自动生成的说明 |

没有「只读点位」这种单独配法：`DI` 默认不能改，需要时点「一键强制」。`AI` 保持只读。
寄存器没有强制接口，配出来的地址都可以写。开关量（`DI` / `DO` / `BOOL`）点一下切换，
非开关量（`AO` / `INT` / `FLOAT`）手填值下发。

## 后端接口

`internal/modules/remote` 的 `Service`，前端从 `wailsjs/go/remote/Service` 导入。

| 方法 | 签名 | 说明 |
| --- | --- | --- |
| `Config` | `() => Promise<Settings>` | 连接默认值 + 标签页定义 + 配置目录 + 配置告警 |
| `SaveDevice` | `(in: DeviceSettings) => Promise<Settings>` | 校验并落盘连接参数（端口、路径、超时），回整份配置；不碰当前连接，也不写共享配置的地址 |
| `SavePanel` | `(tab: Tab) => Promise<Settings>` | 校验并落盘整个点位面板，回整份配置 |
| `ResetDevice` | `() => Promise<Settings>` | 删掉现场那份连接参数，退回出厂默认 |
| `ResetPanel` | `(kind: string) => Promise<Settings>` | 删掉现场那份（`io` / `register` / `ioflow`），退回出厂默认 |
| `ExportPanel` | `(kind: string) => Promise<string>` | 弹出保存框，把当前这一页写成 JSON；取消返回空字符串 |
| `ImportPanel` | `(kind: string) => Promise<PanelFileResult>` | 弹出打开框，校验后整份替换这一页；取消时 `canceled` 为真、配置不动 |
| `Connect` | `(d: Device) => Promise<Status>` | 建 WebSocket 长连接，重复调用先断旧的 |
| `Disconnect` | `() => Promise<Status>` | 主动断开 |
| `Status` | `() => Promise<Status>` | 当前连接状态，顺带清理已断开的连接 |
| `GetIO` | `(points: IOPoint[]) => Promise<IOValue[]>` | `IOManager/GetIOValue` |
| `SetIO` | `(point: IOPoint, value: number) => Promise<void>` | `IOManager/SetIOValue` |
| `SetIOForced` | `(point: IOPoint, forced: boolean) => Promise<void>` | `IOManager/SetIOForcedFlag`，`value` 1 打开 / 0 关掉 |
| `SetIOForcedAll` | `(points: IOPoint[], forced: boolean) => Promise<void>` | 对一批 DI 逐路发 `SetIOForcedFlag` |
| `PulseIO` | `(point, value, offValue, pulseMs) => Promise<void>` | 写值、等待、恢复，等待在后端 |
| `RunFlowStep` | `(index: number) => Promise<void>` | 执行测试流程第 `index` 步（从 0 起），步骤从当前配置取 |
| `ToggleIO` | `(point, onValue, offValue) => Promise<number>` | 读回当前值再写反的那个，返回写入值 |
| `GetRegisters` | `(addresses: number[]) => Promise<RegisterValue[]>` | `RegisterManager/GetRegisterValue` |
| `SetRegister` | `(address: number, value: string) => Promise<void>` | `RegisterManager/SetRegisterValue` |
| `ToggleRegister` | `(address, onValue, offValue) => Promise<number>` | 读回当前值再写反的那个 |
| `PulseRegister` | `(address, value, offValue, pulseMs) => Promise<void>` | 写值、等待、恢复，等待在后端 |

`Device`：`{ host, port, path }`。
`DeviceSettings`：`{ device, connectTimeoutSeconds, requestTimeoutSeconds, refreshIntervalMs }`，
就是参数区能改的那一组值（`host` 只是随配置带上，改它要去顶栏「凭据」弹层）。
`IOPoint`：`{ type, port }`。`IOValue`：`{ type, port, value }`。
`RegisterValue`：`{ address, value }`，`value` 一律转成字符串方便展示。
`Status`：`{ connected, addr, error }`，`error` 是被动断开的原因。
`PanelFileResult`：`{ settings, path, canceled }`。

## 实现要点

- **长连接**。IO 按钮是连着点的，每次调用现握手既慢又让控制器不断新建会话。
  一条连接上跑一个读协程，按 `id` 把响应分发给各自的等待者；没人等的包是控制器的
  主动推送，丢掉。写请求串行加锁，gorilla 不允许并发写。
- **30 秒发一次 ping**。关掉自动刷新后这条连接可能长时间没有流量，中间隔着的反向
  代理或控制器自己的空闲超时会把它悄悄掐掉，等下次点按钮才发现。
- **连接状态显式管理**。不做自动重连：现场必须能看见「现在到底连没连上」，
  藏在每次调用背后重连会说不清刚才那一下有没有发出去。被动断开由 `Status` 如实上报。
- **发送失败不重发**。一次调用只发一帧。这些请求全是 IO 动作，重发等于让现场的气缸
  多动一次。发送失败说明连接已经不可用（写到一半断掉，帧边界就乱了），所以顺手把整条
  连接判死，界面如实退回未连接，要不要再试由人决定。
- **超时不拆连接**。单次请求超时只结束这次等待，也不补发，控制器慢一次不代表连接坏了。
- **脉冲的等待放在后端**。放前端的话，用户切标签页或切模块时组件一销毁定时器就没了，
  点位会一直停在按下的那个值上。
- **翻转前先读**。点位也可能被程序或别的上位机改掉，界面记的「上一次点了什么」
  一旦和现场对不上，按钮就要连点两下才动一次。
- **断开就清空读数**。留着上一次的值比显示 `—` 更危险，现场会当成当前状态。
- **一键强制所有 DI**。控制器没有批量指令，`SetIOForcedAll` 对配置里每一路 DI 逐个发
  `SetIOForcedFlag`。刷新只读 `GetIOValue`，不再问强制标志。断开时把本会话打开过的
  标志清掉。
- **写完输入点位读回来核对一次**。没开强制、或强制没生效时，控制器仍可能答应写入而值不动。
  不核对的话界面会显示一条绿色的「已写入 1」而现场那一路根本没动。第一次读回不对不立刻
  下结论，等 250ms 再读一次。输出点位不做这一步。
- **输入输出共用一份点位定义**。分成「可写的按钮」和「只读的监视点」是上一版的设计，
  结果同一块 IO 在界面上长两个样子，还得配两遍。现在只有点位，一起刷新，
  能不能写由类型加有没有开「一键强制」决定。
- **只读的值也占按钮的位置**。虚线框、不可点，但尺寸和可切换的完全一样，
  两列的行高才对得齐。
- **当前值恒在行尾**。有「点动」的点位和没有的混在一列里，把动作按钮排在值后面的话
  值就会跟着前面有没有按钮左右错开——而值是这一列里最常扫的东西。手填下发的那一行也照这个
  排：输入框和「下发」在中间，读回来的值仍在行尾。
- **待下发的值和读回来的值是两个格子**。合成一个可编辑的格子看着更省地方，但自动刷新
  一秒一次，会把人正在输的数字冲掉；分开之后还顺带把「填的是要写下去的」和
  「显示的是读回来的」分清了。输入框预填 `value`，刷新永远不碰它。
- **要不要手填只看类型，不看连没连上**。断线时布局不跟着变，控件只是灰掉；
  否则一次掉线会让整列行的样子都换一遍。
- **`hint` 和「切换 1 / 0」这类说明挪进了 `title`**。一个点位压到一行之后没地方摆它们。
  `danger` 点位靠那道红边和红字提醒，不指望现场去悬停。
- **IO 状态放在操作栏右侧并恒占位置**。没有消息时也留着空间，消息长了截断、
  完整内容挂在 `title` 上，操作时点位列表不会上下跳。
- **配置是可变状态，用独立的读写锁**。`settings` 从「构造时读一次的常量」变成能被界面改掉的
  东西。它由 `cfgMu` 保护，不复用管连接的 `s.mu`：两把锁职责不重叠且**永不嵌套**，
  否则哪天谁在持连接锁时顺手读一次配置就死锁。取超时的地方一律先拿快照。
- **保存以整份面板为单位**。前端把改完的整个标签页送回后端，后端跑与加载时同一套校验和
  归一化，通过才落盘。按点位增删改各开一个方法要写三遍校验，还得处理「改到一半失败」的
  中间态；整份替换只有成功和失败两种结果。校验也**不新写一套**：两套规则迟早分叉，
  那时候「能存进去但打不开」的配置就出现了。
- **前端拿后端写进去的那份当准**。归一化（类型转大写、名称补 `DO15`、开关量补 `onValue`/`offValue`
  为 `1`/`0`、非开关量清掉翻转值和点动）发生在后端，前端自己算一遍迟早对不上。
  保存后整份配置回传，界面照它重画。
- **写盘先写 `.tmp` 再改名**。同目录内改名是原子的。直接往目标文件上写，进程在写一半时
  挂掉就留下半份 JSON，下次打开整份配置读不出来。
- **「恢复默认」就是删掉现场那个文件**，加载逻辑自然退回出厂默认，不需要第二条
  「把默认值写进文件」的路径。
- **导入导出走系统文件对话框**。`Startup` 收下 Wails 上下文才能弹框。导出写用户选的
  路径，不改 exe 旁边那份；导入校验通过后才 `writeStore`，和「保存」同一条落盘路径。
- **编辑草稿是深拷贝**。直接改后端给的那份，保存失败后界面上留着的会是一份盘上并不存在的清单。
- **刷新定时器跟着间隔重建**。`setInterval` 起好之后改不了周期，间隔变了只能清掉重起。
- **编辑时刷新读的仍是后端那份清单**，不是草稿：草稿里的端口还没过校验，拿它去读会把
  控制器不认的端口一起发出去。
- **两页的编辑控件共用**（`PointEditor.vue` 与 `usePanelEdit.ts`）。各写一份就是让它们
  各自漂移——改了 IO 的字段顺序忘了改寄存器那边，界面立刻不一致。
- **测试流程挂在 IO 页右侧，不当独立标签**。步骤只显示名称，点一下点位就加入；
  导出、导入、恢复默认收进 ⋯，避免操作栏占两行。


## 已知限制

- 连接被控制器单方面掐掉时，来不及把强制标志清掉，物理输入会保持被盖住，
  只能重新连上再点一次「取消强制」，或在示教器上解除。
- 只有轮询，没有订阅式推送。间隔能在界面上调（200–60000 毫秒），但没有推送就得一直问。
- **界面保存或导入会整份覆盖盘上那份**。要手工改 exe 旁边那三个文件，就先关掉程序，
  或者改完先在界面上重开一次这个模块再动手——否则界面上那份保存下去会把手工改动冲掉。
  换一份点位清单更稳妥的做法是操作栏的「导入」。
- **只拷 exe 文件不会带走配置**。配置和 exe 在同一目录，整夹拷走会一起带走。
- 点位可以在界面上一直加，加多了一次读全部点位会变慢（症状是刷新变慢，不是请求堆积）。
  不设点位数量上限。
- 点位与分组不能在界面上调顺序，顺序就是清单顺序。要挪位置只能删了重建。
- 寄存器标签页和 IO 一样按配置铺按钮，批量读 `GetRegisterValue`。
  翻转、点动走 `SetRegisterValue`。未覆盖扩展数组（`setExtendArrayType` /
  `removeExtendArray`）与从站配置（`RegisterCommunicator/*`）。

## 相关文件

```
internal/modules/remote/remote.go             模块入口与对外方法
internal/modules/remote/client.go             WebSocket 长连接客户端、路径探测与解析
internal/modules/remote/config.go             配置结构与校验、现场配置与出厂默认的取舍
internal/modules/remote/store.go              现场配置的读、原子写、删
internal/modules/remote/config/config.json    出厂默认：连接地址与超时
internal/modules/remote/config/io.json        出厂默认：IO 点位
internal/modules/remote/config/register.json  出厂默认：寄存器点位
internal/modules/remote/config/io-flow.json   出厂默认：IO 测试流程
frontend/src/modules/remote/module.ts         模块清单
frontend/src/modules/remote/RemoteView.vue    参数区、连接参数编辑与标签页外壳
frontend/src/modules/remote/IoPanel.vue       IO 点位与状态，右侧挂测试流程
frontend/src/modules/remote/RegisterPanel.vue 寄存器点位与状态
frontend/src/modules/remote/FlowPanel.vue     IO 测试流程
frontend/src/modules/remote/PointEditor.vue   点位编辑表单，两页共用
frontend/src/modules/remote/usePanelEdit.ts   编辑状态与保存、恢复默认，两页共用
```

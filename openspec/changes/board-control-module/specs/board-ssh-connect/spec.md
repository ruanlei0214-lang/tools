## ADDED Requirements

### Requirement: 连接凭据全部显示在界面上

页面 SHALL 用四个可编辑的输入框显示 IP、端口、用户名、密码，四项都 SHALL 能在界面上改，
SHALL NOT 有任何一项只能在配置文件里改。密码框 SHALL 提供切换明文显示的入口——
现场核对的是「密码是不是空的」，一排圆点看不出这件事。

打开页面时四项 SHALL 填入 `internal/modules/board/config/config.json` 里的默认值。

#### Scenario: 打开页面

- **WHEN** 进入主板控制模块
- **THEN** IP 显示 `192.168.1.136`、端口 `22`、用户名 `root`、密码为空，四项都可编辑

#### Scenario: 空密码可以直接连接

- **WHEN** 密码框留空并点「连接」
- **THEN** 照常发起连接，界面 SHALL NOT 要求先填一个密码

#### Scenario: 查看密码

- **WHEN** 点密码框旁的显示切换
- **THEN** 密码以明文显示

### Requirement: 连接状态显式呈现

界面 SHALL 明确显示当前连没连上，以及连上的是哪个地址。连接 SHALL NOT 在每次操作背后
隐式建立或重连——现场必须能判断「刚才那条命令到底发出去了没有」。

连接被设备单方面断开时，界面 SHALL 在下一次操作失败后退回未连接状态并显示原因。

#### Scenario: 连接成功

- **WHEN** 点「连接」且握手与认证通过
- **THEN** 显示已连接及连上的 `user@host:port`，指令按钮与文件操作变为可用

#### Scenario: 连接失败

- **WHEN** 地址不可达、认证被拒或握手超时
- **THEN** 显示失败原因，状态保持未连接，指令按钮与文件操作保持不可用

#### Scenario: 连接中途被设备断开

- **WHEN** 连接已断，用户点某个指令按钮
- **THEN** 该操作报错，界面状态改回未连接，SHALL NOT 自动重连后当作成功

#### Scenario: 主动断开

- **WHEN** 点「断开」
- **THEN** 连接关闭，状态改回未连接

### Requirement: 连接超时有上限

建立连接 SHALL 在配置的超时内结束，且该超时 SHALL 覆盖 TCP 建连、SSH 握手、认证三段。
对着一个接受 TCP 连接却不说 SSH 协议的地址，界面 SHALL NOT 无限期停在「连接中」。

#### Scenario: 对方接受 TCP 但不说 SSH

- **WHEN** 目标端口上跑的是别的服务，握手一直不返回
- **THEN** 在配置的超时到达时放弃并报错

### Requirement: 离开模块时断开连接

切走页面或关闭程序时 SHALL 关闭连接，SHALL NOT 留下一条无人使用的 SSH 会话。

#### Scenario: 切到其他模块

- **WHEN** 从主板控制切到别的模块
- **THEN** 连接被关闭

## ADDED Requirements

### Requirement: 默认值来自随包配置
netcfg 模块 SHALL 把页面默认值放在模块内的 `config.json` 里，并用 `go:embed` 编译进产物。配置 MUST 覆盖：SSH 连接的设备地址、端口、用户名、密码，地址表单的默认子网掩码与默认网关，以及一键恢复要删除的文件路径。前端 MUST NOT 再自带这些默认值的字面量。

#### Scenario: 页面打开时套用配置
- **WHEN** 用户打开网络配置页面
- **THEN** 「设备地址」输入框填入配置里的值
- **AND** 地址表单的子网掩码与默认网关填入配置里的值
- **AND** 端口、用户名、密码取配置里的值用于连接，但不出现在界面上

#### Scenario: 改配置后重新构建生效
- **WHEN** 修改 `config.json` 并重新构建
- **THEN** 新产物打开页面时显示的默认值是修改后的值

#### Scenario: 选中网口时补默认掩码
- **WHEN** 用户选中一个读不到子网掩码的网口
- **THEN** 掩码输入框填入配置里的默认子网掩码

### Requirement: 连接参数中只有设备地址可在界面上改
连接卡片 SHALL 只提供「设备地址」一个输入框。端口、用户名、密码 MUST NOT 出现在界面上，它们只能通过修改配置并重新构建来改变。

#### Scenario: 界面上没有端口、用户名、密码输入框
- **WHEN** 用户打开网络配置页面
- **THEN** 连接卡片里只有「设备地址」一个输入框

#### Scenario: 不可见的参数照样生效
- **WHEN** 配置里端口是 2222、用户名是 `admin`，用户点「测试连接」
- **THEN** 系统用 2222 端口和 `admin` 账号去连设备

### Requirement: 恢复路径来自配置
一键恢复网络删除的文件路径 SHALL 取自配置，不得在代码里写死。界面上提到该路径的文案 MUST 显示配置里的实际值，MUST NOT 显示与实际删除目标不一致的路径。

#### Scenario: 按配置的路径删除
- **WHEN** 配置里的恢复路径是 `/opt/runtime/pi`，用户触发一键恢复网络
- **THEN** 系统删除设备上的 `/opt/runtime/pi`

#### Scenario: 换成别的路径
- **WHEN** 配置里的恢复路径改成 `/etc/robot/net.conf` 并重新构建
- **THEN** 一键恢复网络删除的是 `/etc/robot/net.conf`
- **AND** 按钮说明、结果提示与弹窗正文里显示的路径都是 `/etc/robot/net.conf`

### Requirement: 配置校验
加载配置时 SHALL 校验其取值。端口为 0 MUST 按 22 处理；端口不在 1-65535 之间 MUST 视为配置不可用；恢复路径 MUST 是绝对路径，空值或相对路径 MUST 视为配置不可用。

#### Scenario: 端口省略
- **WHEN** 配置里端口是 0 或没写
- **THEN** 按 22 处理，配置视为可用

#### Scenario: 端口越界
- **WHEN** 配置里端口是 70000
- **THEN** 配置视为不可用

#### Scenario: 恢复路径不是绝对路径
- **WHEN** 配置里的恢复路径是空字符串或 `opt/runtime/pi`
- **THEN** 配置视为不可用

### Requirement: 配置不可用时退回内置默认值
配置不可用时（JSON 解析失败或校验不通过），系统 MUST 改用内置默认值继续提供完整功能，MUST NOT 拒绝启动或让页面不可操作，并 SHALL 在界面上提示当前用的是内置默认值以及原因。

#### Scenario: 配置内容损坏
- **WHEN** 嵌入的 `config.json` 不是合法 JSON
- **THEN** 页面用内置默认值填好各输入框，功能照常可用
- **AND** 界面上显示一条提示，说明配置不可用及原因

#### Scenario: 配置正常时不打扰
- **WHEN** 配置可用
- **THEN** 界面上不出现任何配置相关的提示

### Requirement: 随包配置必须合法
仓库里随包发布的 `config.json` SHALL 由单元测试保证可解析且通过校验，使配置写错在 `go test` 阶段暴露，而不是等到运行时靠兜底掩盖。

#### Scenario: 测试守住随包配置
- **WHEN** 有人把 `config.json` 改成非法内容并运行 `go test ./...`
- **THEN** 测试失败

## ADDED Requirements

### Requirement: 一键恢复网络
netcfg 模块 SHALL 提供一个"一键恢复网络"操作，使用当前填写的 SSH 连接参数删除目标设备上的 `/opt/runtime/pi` 文件。删除文件 MUST 是该操作在设备上执行的唯一动作：MUST NOT 修改设备上的其他文件，MUST NOT 重启设备或重启任何服务。该操作 MUST 在文件不存在时同样视为成功。

#### Scenario: 文件存在并被删除
- **WHEN** 用户填好设备连接参数并触发"一键恢复网络"
- **THEN** 系统通过 SSH 删除 `/opt/runtime/pi`
- **AND** 界面提示恢复网络已完成

#### Scenario: 文件本来就不存在
- **WHEN** 目标设备上没有 `/opt/runtime/pi`
- **THEN** 系统不报错，按成功处理并给出同样的完成提示

#### Scenario: 连接失败
- **WHEN** SSH 连接不上目标设备
- **THEN** 系统不做任何删除动作
- **AND** 界面以错误提示展示连接失败原因

### Requirement: 删除后提示人工重启
删除成功后，系统 SHALL 弹出模态弹窗，提示用户需要重启机器人控制器才能生效，并在弹窗中写明已删除的文件路径。弹窗 MUST 只是提示：MUST NOT 提供由工具执行重启的入口。

#### Scenario: 删除成功后弹出提示
- **WHEN** 删除成功
- **THEN** 系统弹出模态弹窗，写明已删除 `/opt/runtime/pi` 且需要人工重启机器人控制器
- **AND** 弹窗中只有一个关闭它的按钮，没有任何执行重启的按钮

#### Scenario: 关闭提示
- **WHEN** 用户关闭该弹窗
- **THEN** 弹窗消失，界面上仍保留同样内容的删除成功提示

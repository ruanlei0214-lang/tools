## ADDED Requirements

### Requirement: 指令按钮可以在界面上增删改

用户 SHALL 能在界面上添加按钮，每个按钮至少包含一个名称和一条命令。已有按钮 SHALL 能改名、
改命令、删除。改动 SHALL 立即生效，SHALL NOT 需要重新构建或重启程序。

添加与编辑 SHALL 校验：名称与命令都不能为空白。删除 SHALL 先弹确认——按钮是攒出来的，
误删一个就得重新想起那条命令是什么。

#### Scenario: 添加按钮

- **WHEN** 填入名称「重启运行时」与命令 `systemctl restart runtime` 并保存
- **THEN** 按钮出现在按钮区，且立即可点

#### Scenario: 名称或命令为空

- **WHEN** 名称或命令留空（或只有空格）并保存
- **THEN** 拒绝保存并说明哪一项为空

#### Scenario: 编辑按钮

- **WHEN** 改掉某个按钮的命令并保存
- **THEN** 该按钮之后执行的是新命令

#### Scenario: 删除按钮

- **WHEN** 点某个按钮的删除并在确认框里确认
- **THEN** 该按钮从按钮区消失

#### Scenario: 取消删除

- **WHEN** 在确认框里取消
- **THEN** 按钮保留

### Requirement: 按钮清单持久化在用户配置目录

按钮清单 SHALL 存在 `%APPDATA%\embedtools\board-commands.json`，重开程序后 SHALL 还在。
SHALL NOT 存在 exe 旁边：exe 可能位于 Program Files 或只读共享盘，那里写不进去。

界面 SHALL 显示这个文件的完整路径——拷走 exe 不会带走它，要搬到另一台电脑得手动拷。

首次使用（文件不存在）时按钮区 SHALL 是空的，SHALL NOT 预置任何命令。

#### Scenario: 重开程序

- **WHEN** 添加几个按钮后关闭程序再打开
- **THEN** 这些按钮仍在，内容不变

#### Scenario: 首次使用

- **WHEN** `board-commands.json` 不存在
- **THEN** 按钮区为空，并提示可以从「添加」开始

#### Scenario: 清单文件损坏

- **WHEN** `board-commands.json` 内容不是合法 JSON
- **THEN** 按钮区退回空列表并显示一条告警，页面其余功能照常可用，
  SHALL NOT 覆盖掉那个坏文件——它里面可能还有能人工救回来的命令

#### Scenario: 写入失败

- **WHEN** 保存按钮时写文件失败（目录不可写等）
- **THEN** 明确报错说明没有保存成功，SHALL NOT 只在界面上留下一个下次打开就消失的按钮

### Requirement: 点按钮执行命令并回显输出

点一个按钮 SHALL 把它的命令发到主板上执行，并把标准输出与标准错误显示在日志区，
连同这次执行是成功还是失败。命令没有输出时 SHALL 也明确显示它已执行完毕。

执行期间 SHALL 禁用按钮区，避免同一条命令被连点多次重复下发。

#### Scenario: 命令执行成功

- **WHEN** 点一个按钮且命令退出码为 0
- **THEN** 日志区追加这条命令与它的输出，并标为成功

#### Scenario: 命令执行失败

- **WHEN** 命令退出码非 0
- **THEN** 日志区追加它的错误输出并标为失败，SHALL NOT 当作成功

#### Scenario: 命令没有输出

- **WHEN** 命令成功但没有任何输出
- **THEN** 日志区显示这条命令已执行完毕

#### Scenario: 未连接时

- **WHEN** 尚未连接
- **THEN** 按钮不可点

### Requirement: 命令原样下发

命令 SHALL 原样交给设备的 shell 执行，SHALL NOT 在本地解析、改写或过滤。用户写的是
给这台设备的命令，工具替他猜等于让界面上看到的和实际执行的不是一回事。

#### Scenario: 命令带管道与重定向

- **WHEN** 命令是 `ps | grep runtime > /tmp/p.txt`
- **THEN** 整条命令原样交给设备的 shell，管道与重定向由设备解释

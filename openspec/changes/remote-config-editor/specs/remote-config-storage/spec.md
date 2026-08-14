## ADDED Requirements

### Requirement: 现场配置存在用户配置目录

现场改出来的配置 SHALL 存在 `%APPDATA%\embedtools\` 下，三份文件分别对应连接参数、
IO 点位、寄存器点位。SHALL NOT 存在 exe 旁边：exe 可能位于 Program Files 或只读共享盘，
那里写不进去，`netcfg` 记设备地址和 `board` 存按钮清单时已经因为同样的理由这么选过。

界面 SHALL 显示这些文件所在的位置——拷走 exe 不会带走配置，要搬到另一台电脑得手动拷。

写盘 SHALL 是原子的：先写临时文件再改名。进程在写一半时挂掉不能留下半份 JSON，
否则下次打开整份配置都读不出来。

#### Scenario: 重开程序

- **WHEN** 改过点位与连接参数后关闭程序再打开
- **THEN** 三份改动都还在，内容不变

#### Scenario: 写入失败

- **WHEN** 保存时写文件失败（目录不可写等）
- **THEN** 明确报错说明没有保存成功，SHALL NOT 只在界面上留下一份下次打开就消失的配置

#### Scenario: 界面显示存放位置

- **WHEN** 打开配置编辑区
- **THEN** 能看到这份配置存在本机的哪个位置

### Requirement: 编译进产物的三份 JSON 降级为出厂默认

`internal/modules/remote/config/` 下的 `config.json`、`io.json`、`register.json`
SHALL 只作为出厂默认值：对应的 `%APPDATA%` 文件不存在时用它们播种。

出厂默认 SHALL 仍然编译进产物，SHALL NOT 依赖任何外部文件——干净的一台机器上第一次打开
就该有一份能用的点位表。

#### Scenario: 首次使用

- **WHEN** `%APPDATA%\embedtools\` 下没有 remote 的配置文件
- **THEN** 页面按编译进产物的出厂默认渲染，且不显示告警

#### Scenario: 首次保存

- **WHEN** 首次使用后改一个点位并保存
- **THEN** 在 `%APPDATA%` 下生成对应文件，编译进产物的那份不受影响

#### Scenario: 只有一份被改过

- **WHEN** 只保存过 IO 点位，寄存器的文件还不存在
- **THEN** IO 用现场那份，寄存器用出厂默认，两者互不影响

### Requirement: 每一份配置都能恢复默认

用户 SHALL 能把连接参数、IO 点位、寄存器点位各自恢复成出厂默认。恢复 SHALL 先弹确认框，
确认后 SHALL 立即生效，界面立刻按出厂默认重画。

三份 SHALL 互不牵连：恢复 IO 点位 SHALL NOT 连带重置寄存器点位或连接参数。

#### Scenario: 恢复 IO 点位

- **WHEN** 改乱 IO 点位后点「恢复默认」并确认
- **THEN** IO 页立刻显示出厂默认的那套点位，寄存器页和连接参数不变

#### Scenario: 取消恢复

- **WHEN** 在确认框里取消
- **THEN** 当前配置保留

#### Scenario: 恢复后重开

- **WHEN** 恢复默认后关闭程序再打开
- **THEN** 仍是出厂默认，SHALL NOT 又变回恢复前那份

### Requirement: 坏配置不让页面打不开

`%APPDATA%` 下某一份配置读不出来或过不了校验时，那一部分 SHALL 退回出厂默认，
页面其余部分 SHALL 照常可用，并 SHALL 在顶部显示一条说明是哪个文件、哪一行出了问题的告警。

SHALL NOT 覆盖掉那个坏文件——现场手改出的错，里面可能还有能人工救回来的点位。

告警 SHALL 指出具体行号，和现在解析配置文件时一样：一份几十行的点位表里少个逗号，
光说「JSON 语法错误」得一行行数过去。

#### Scenario: IO 配置是坏 JSON

- **WHEN** 手动把 `%APPDATA%` 下的 IO 配置改成不合法 JSON 后打开程序
- **THEN** IO 页显示出厂默认的点位，顶部告警指出是哪个文件第几行的问题，寄存器页正常

#### Scenario: 坏文件不被覆盖

- **WHEN** 因为坏文件而退回出厂默认
- **THEN** 那个坏文件仍留在盘上，内容一字不动

#### Scenario: 连接配置坏掉

- **WHEN** `%APPDATA%` 下的连接配置过不了范围校验
- **THEN** 连接参数退回出厂默认并告警，两个点位页照现场配置正常显示

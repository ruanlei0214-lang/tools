# 性能优先

## Purpose

本规格定义与模块独立性同级的硬约束：所有修改性能优先。任何改动先评估后台代价——
看不见的常驻开销（定时器、轮询、goroutine、缓冲区增长）不许留给停用状态的模块。
其中缓冲区上限由测试守住，其余靠评审与 AI 协助把关。

## Requirements

### Requirement: 所有修改性能优先

任何代码修改 SHALL 先评估后台代价：看不见的常驻开销（定时器、轮询、goroutine、
缓冲区增长）不许留给停用状态的模块。新模块引入后台活动时 SHALL 同时给出它的
暂停条件与资源上限。

#### Scenario: 新模块引入定时轮询

- **WHEN** 一个模块新增定时器或轮询循环
- **THEN** 它必须说明模块停用（keep-alive 切走）时该轮询的行为，且默认必须暂停

#### Scenario: 修改引入常驻 goroutine

- **WHEN** 一次修改在后端启动了长期运行的 goroutine
- **THEN** 它必须有明确的退出路径（context 取消或显式停止接口），不允许只进不出

### Requirement: 停用的模块暂停轮询

前端模块被 keep-alive 停用（切去别的模块）时，SHALL 暂停它所有的定时轮询与
IPC 调用；重新激活时 SHALL 立即补一轮再恢复节奏。这一行为 SHALL 经由共享层
`frontend/src/shell/polling.ts` 的 `useActivePolling` 实现，模块 SHALL NOT
各自裸写 `setInterval`。

#### Scenario: 切走正在轮询的模块

- **WHEN** 用户从终端模块切到别的模块
- **THEN** 终端输出轮询停止发起 IPC，设备上的输出暂存在后端缓冲区，切回时立即取走

#### Scenario: 模块被真正卸载

- **WHEN** 模块组件被卸载（不是停用）
- **THEN** 它的定时器随之销毁，不留回调

### Requirement: 累积型缓冲区必须有上限

任何随时间增长的缓冲区 SHALL 有明确上限，到达上限时丢弃较早内容并保留最新内容。
上限值 SHALL 写在代码里可查：终端后端积压 1MB、终端前端显示 20 万字符、
长 ping 日志 500 行。

#### Scenario: 设备持续刷输出

- **WHEN** 设备程序不间断向终端写输出，前端长时间不读取
- **THEN** 后端缓冲区截断到上限以内并插入省略提示，内存不随时间线性增长

### Requirement: 连接复用不多开

同一模块对同一台设备 SHALL 只维持一条长连接，其上的并发通道（SSH session、
SFTP、多个终端 PTY）SHALL 复用这条连接的多路复用能力。SHALL NOT 为每个
面板或每次操作各建一条连接。

#### Scenario: 终端分屏成四格

- **WHEN** 用户把终端分屏到四格
- **THEN** 四个 shell 会话跑在同一条 SSH 连接的四条 channel 上，不新增 TCP 连接

### Requirement: 批量探测必须并发

对一批网络目标的探测（ping 扫段、解析主机名）SHALL 并发执行，总耗时 SHALL
接近单个目标的最坏耗时而不是所有目标耗时之和。串行等待 SHALL NOT 出现在
批量路径上。

#### Scenario: 扫一个 /24 网段

- **WHEN** 用户扫描 254 个地址的网段
- **THEN** 探测并发发出，整体在秒级完成，而不是 254 次串行超时相加

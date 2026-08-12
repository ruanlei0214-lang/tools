## Context

这是一次纯清理，没有新功能、没有新依赖、不改用户可见行为。按 design.md 自己的收录标准（跨模块、新依赖、数据模型变更、安全或性能复杂度）本来都够不上，写这份文档的唯一价值在于把"哪些顺手能改的东西这次**不**改"记下来——项目的硬性约束是"最少改动、不顺手重构无关代码"，而清理类改动最容易在这条线上失控。

当前状态：`TestConnection` 返回 `(string, error)`，其中 string 是 `uname -a` 的输出，前端 `await TestConnection(device)` 直接丢弃；前端 `defaults.gateway` 在 `onMounted` 赋值后无任何读取点；`rememberHost` 写状态文件前先调 `loadLastHost()` 读回旧值比对。

## Goals / Non-Goals

**Goals:**

- 摘掉三处已确认无人消费的代码，且不改变任何用户可见行为。
- 让 `doc/netcfg.md` 与代码重新对齐。
- 给"连接测试"这块已经变过两轮却没进 spec 的行为补上规范。

**Non-Goals:**

- 不动 SSH 每次短连接的策略。
- 不动 `parse.go` 的解析与校验逻辑。
- 不改 `dial`/`run` 的错误提示文案。
- 不给 `loadSettings()` 加缓存。

## Decisions

### 保留命令执行，只去掉返回值

`TestConnection` 改成 `(Device) => error` 之后，仍然在设备上跑 `uname -a`，只是不再返回输出。

替代方案是既然不看输出，那连命令也别跑，`dial` 成功即算连上。否决：`dial` 成功只证明 TCP 通且 SSH 握手完成，而**认证失败也会在 dial 阶段暴露**——真正多出来的一层是"能不能开出 session 跑命令"。dropbear 这类轻量服务上，认证过了但 shell 不可用是真实存在的情况，而现场对"连不上"和"连上了但执行不了"的处理完全不同。为一次点击省一条命令的往返，换掉一层有意义的验证，不划算。

也考虑过把命令从 `uname -a` 换成更轻的 `true`。否决：省下的是几十字节，而 `uname -a` 在目标设备上已经被验证可用，换命令是拿零收益去冒回归的风险。

### `rememberHost` 直接写，不做去重

去掉写前的 `loadLastHost()` 比对。

原本加这个比对是想省掉"地址没变时的重复写"。但代价是每次成功操作都要读一次文件并解析 JSON，而省下的是一次 30 字节的写——读+解析比写更贵，这个"优化"方向是反的。去掉之后代码还短了一截。

### 不给 `loadSettings()` 加缓存

`loadSettings()` 每次调用都重新解析内嵌的 `config.json`，`Defaults()` 和 `RestoreNetwork()` 各调一次。用 `sync.OnceValue` 包一层就能只解析一次。

否决：一个会话里这个函数总共被调用个位数次，解析的是几百字节的常量，省下的时间不可测量。加缓存等于用"看起来在优化"换真实的零收益，还多一个包级状态——`config_test.go` 里两个用例都直接调 `loadSettings()`，缓存会让它们从"各自独立解析"变成"共享一次解析结果"，测试隔离性反而变差。判断标准是收益能不能说清楚，说不清就不做。

### 不改错误提示文案

`dial` 失败时会把 `golang.org/x/crypto/ssh` 的原始错误透出来，形如 `ssh: handshake failed: ssh: unable to authenticate`，对现场人员不够友好，可以映射成"认证失败／地址不可达／连接被拒绝"这类中文提示。

否决：这是改善可用性的新工作，不是删死代码，混进来会让这次变更的边界糊掉。已在 Open Questions 里记下。

## Risks / Trade-offs

- **改 Go 方法签名会重新生成 wails 绑定** → `frontend/wailsjs/go/netcfg/Service.{js,d.ts}` 是生成物，构建时自动更新。前端调用点写的是 `await TestConnection(device)`，没有接收返回值，签名变化不影响它。构建一次即可验证。
- **丢掉 `uname -a` 的输出，将来想显示设备信息要改回去** → 真要显示时，加回返回值是几行的事，而且那时才知道该显示什么字段。现在留着的是一个没有消费者的值，YAGNI。
- **`defaults.gateway` 可能被误判为死代码** → 已确认：全文件内只有 `onMounted` 里的一处赋值，无读取点；表单默认网关走的是 `form.gateway`，由同一次 `Defaults()` 调用单独赋值，不经过 `defaults`。删除后表单行为不变。

## Migration Plan

无数据迁移、无配置迁移。改完 `go test ./...` 加一次 `.\build.ps1` 即可；回滚就是还原这几个文件。

## Open Questions

- `dial`/`run` 的错误提示是否要映射成面向现场的中文？倾向于要，但应单开变更。

<!-- 原本这里还有一条「自动测试连接与读取网口是否合并」。已经定了：合并。
     那是用户可见的行为变更，与本变更「不改变任何用户可见行为」的边界冲突，
     所以另开 netcfg-connect-and-read 记录，不折进这里。 -->

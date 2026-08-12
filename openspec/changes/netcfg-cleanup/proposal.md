## Why

netcfg 最近几次改动（撤掉界面上的 `uname -a` 输出、撤掉弹窗正文、加上"记住上次用过的地址"）都是往前推功能，留下了三处已经没人消费的代码：后端还在把内核信息当返回值往上传、前端还在维护一个从不读取的 `defaults.gateway`、`rememberHost` 为了去重每次先把状态文件读回来解析一遍。这些东西不影响运行，但会让下一个人以为它们有用——`doc/netcfg.md` 里那句"成功会显示设备的 `uname -a`"就是被这个返回值误导写出来的，现在已经和界面对不上。

趁改动还新、上下文还在，把这三处摘掉。

## What Changes

- `Service.TestConnection` 的签名从 `(Device) => (string, error)` 改为 `(Device) => error`。仍然在设备上执行一条命令（连得上 TCP 不等于登得进去），只是不再把输出当返回值——前端从上次改动起就已经丢弃它了。**BREAKING**（仅限模块内部的前后端绑定，无外部使用者）。
- 前端 `defaults` 去掉 `gateway` 字段。它在 `onMounted` 里被赋值后再没被读过；表单的默认网关走的是 `form.gateway`，不受影响。
- `rememberHost` 不再先读旧值比对，直接写。省掉一次读 + 一次 JSON 解析，代码也短一截；写 30 字节比读回来比对更便宜。
- 同步订正 `doc/netcfg.md` 里关于 `TestConnection` 返回值的描述。

明确**不做**的事（避免这次清理长成重构）：不动 SSH 短连接策略、不动解析逻辑、不改错误提示文案。理由记在 design.md。

## Capabilities

### New Capabilities
- `netcfg-connection-test`: 连接测试这件事的行为契约——什么时候触发、验到什么程度、成功与失败各自向用户报告什么。这块行为在前几次改动里已经变过两轮（撤掉内核信息、页面打开自动连一次），但一直没有落到 spec 里，本次改的正是它的返回值，顺势补上。

### Modified Capabilities
<!-- 无。本次改动涉及的另外两处（默认值来源、恢复路径）其规范还在未归档的
     netcfg-default-config 与 netcfg-restore-network 变更里，尚未进入 openspec/specs/，
     且本次并不改变它们的任何需求，只动实现细节，因此不写 delta。 -->

## Impact

- `internal/modules/netcfg/netcfg.go`：`TestConnection` 改签名、去掉 `strings.TrimSpace` 包装。
- `internal/modules/netcfg/state.go`：`rememberHost` 去掉去重读取。
- `internal/modules/netcfg/state_test.go`：`TestRememberAndLoadHost` 覆盖写场景不变，仍需通过。
- `frontend/src/modules/netcfg/NetcfgView.vue`：`defaults` 去掉 `gateway` 字段及其赋值。
- `frontend/wailsjs/go/netcfg/Service.{js,d.ts}`：由 wails 重新生成的绑定，`TestConnection` 的返回类型随之变化。
- `doc/netcfg.md`：API 表格里 `TestConnection` 一行。
- 不涉及新依赖、不涉及其他模块、不改变任何用户可见行为。

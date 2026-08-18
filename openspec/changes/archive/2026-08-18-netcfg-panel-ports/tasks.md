## 1. 后端：面板映射

- [x] 1.1 新增 `internal/modules/netcfg/ports.go`：`Port` 类型、`bridgeIface` 常量、`bindings(hasBridge)` 对应表、`buildPorts(ifaces)`。
- [x] 1.2 `netcfg.go`：`ListInterfaces` 改名 `ListPorts`，返回 `[]Port`，内部把 `parseInterfaces` 的结果过一遍 `buildPorts`。
- [x] 1.3 `Iface` 保留为模块内部类型，不再出现在前端接口上。
- [x] 1.4 `mainIface` 常量与 `editableIface(hasBridge)`：有桥取 `br0`，无桥取 `lan1`；`buildPorts` 据此填 `Port.Editable`。

## 2. 后端：单元测试

- [x] 2.1 恒为五行且顺序固定（有桥、无桥、一个网口都没读到三种输入）。
- [x] 2.2 有 `br0` 时的对应关系。
- [x] 2.3 没有 `br0` 时的对应关系。
- [x] 2.4 面板 `lan3` 恒空——包括系统上恰好存在名为 `lan3` 的网口的情况。
- [x] 2.5 共用同一系统网口的面板口显示相同地址。
- [x] 2.6 对应表写了但设备上没有的网口，退化成占位行。
- [x] 2.7 有 `br0` 时可改的是 `lan1`/`lan2`/`lan5`，`lan4` 只读。
- [x] 2.8 无 `br0` 时可改的只有 `lan1`/`lan2`，`lan4`/`lan5` 只读。
- [x] 2.9 只读行仍然携带完整的地址信息。
- [x] 2.10 `br0` 与系统 `lan1` 都不存在时，一个口都不可改，但仍是五行。

## 3. 前端

- [x] 3.1 `ifaces` 改为 `ports`，调用 `ListPorts`；`current` computed 从面板名查回整行。
- [x] 3.2 `select()` 对不可改的行（只读与占位）直接返回，不选中。
- [x] 3.3 自动选中改为在可改的口里挑第一个 UP 的；一个可改的都没有时提示「没有可以修改地址的网口」。
- [x] 3.4 `apply()` 下发前先取出系统网口名（`selected` 会在函数体中途被清空）。
- [x] 3.5 `willPersist` 改为比较系统网口名。
- [x] 3.6 新增 `siblings` computed 与联动提示；删掉「下发配置」旁的持久化提示（下发成功的提示已经说明了结果），连带去掉 `persistPorts`。
- [x] 3.7 表格模板改用面板口；只读行加 `readonly` 类与「只读」标记，占位行再加 `blank` 类；`style.css` 区分"点不动"与"压暗"两种样式。
- [x] 3.8 表格上方列出当前可改的面板口（`editable` computed）。

## 4. 文档

- [x] 4.1 `doc/netcfg.md` 新增「面板网口对应关系」一节，含对应表与四点后果。
- [x] 4.2 更新「界面操作」第 2、3 条。
- [x] 4.3 更新「后端接口」表格与类型定义（`ListPorts`、`Port`、`Iface` 退回内部）。
- [x] 4.4 「已知限制」补三条：对应表只适用当前机型；读不到的口和恒空的 `lan3` 分不清；只读口在工具里没有任何改法。
- [x] 4.6 新增「只有主网口能改地址」一节。
- [x] 4.5 「相关文件」补 `ports.go` 与 `ports_test.go`。

## 5. 验证

- [x] 5.1 `go vet ./...` 与 `go test ./...` 全绿。
- [x] 5.2 `vue-tsc --noEmit` 通过，`wails build` 重新生成绑定后前端构建通过。
- [ ] 5.3 真机验证：桥接形态下表格五行、`lan1`/`lan2`/`lan5` 地址一致且可选、选中后有联动提示。
- [ ] 5.4 真机验证：无桥形态下 `lan1`/`lan2` 落在系统 `lan1` 且可改，`lan5` 显示系统 `lan4` 的信息但只读。
- [ ] 5.5 真机验证：面板 `lan4` 显示系统 `lan3` 的信息，带「只读」标记且点不动。
- [ ] 5.6 真机验证：在可改的口上下发地址，确认落到的是 `br0`（或无桥时的系统 `lan1`）。

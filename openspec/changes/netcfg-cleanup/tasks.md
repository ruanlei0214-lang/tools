## 1. 后端：去掉无人消费的返回值

- [x] 1.1 `internal/modules/netcfg/netcfg.go`：`TestConnection` 签名改为 `func (s *Service) TestConnection(d Device) error`，保留 `run(client, "uname -a")` 调用，丢弃其输出，去掉 `strings.TrimSpace` 包装；同步更新方法上方的注释（不再"返回设备的内核信息"）。
- [x] 1.2 确认 `netcfg.go` 的 `strings` 导入仍被 `applyScript` 里的 `strings.Join` 使用，未变成未使用导入。
- [x] 1.3 `go build ./...` 通过。

## 2. 后端：去掉写前的重复读

- [x] 2.1 `internal/modules/netcfg/state.go`：`rememberHost` 去掉 `host == loadLastHost()` 这个比对分支，保留 `net.ParseIP(host) == nil` 的合法性校验，直接写文件。
- [x] 2.2 `go test ./internal/modules/netcfg/` 通过，`TestRememberAndLoadHost`（含覆盖写）与 `TestRememberHostRejectsInvalid` 均不需修改即通过。

## 3. 前端：去掉死状态

- [x] 3.1 `frontend/src/modules/netcfg/NetcfgView.vue`：`defaults` 响应式对象去掉 `gateway` 字段，同时删掉 `onMounted` 里的 `defaults.gateway = s.gateway`。
- [x] 3.2 确认 `form.gateway = s.gateway` 一行保留不动——表单的默认网关来源于此，与 `defaults` 无关。
- [x] 3.3 全文件搜索 `defaults.` 确认剩余引用只有 `defaults.mask`（选中网口时的掩码兜底）与 `defaults.restoreFile`（删除成功的结果提示）。

## 4. 文档订正

- [x] 4.1 `doc/netcfg.md`：API 表格里 `TestConnection` 一行的签名改为 `(Device) => Promise<void>`，说明改为"验证连接是否可用，不返回设备信息"。
- [x] 4.2 全文搜索 `uname`，确认没有其他地方还在承诺界面会显示内核信息。

## 5. 验证

- [x] 5.1 `go test ./...` 全部通过（含 `internal/modules` 的模块边界测试）。
- [x] 5.2 `.\build.ps1` 构建通过，`frontend/wailsjs/go/netcfg/Service.d.ts` 里 `TestConnection` 已重新生成为 `Promise<void>`。
- [ ] 5.3 人工确认用户可见行为无变化：打开页面自动连一次并显示「连接成功」、表单掩码有默认值、一键恢复的结果提示里仍显示实际删除路径。

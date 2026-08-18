## 1. 规格落地

- [x] 1.1 新增 delta spec `specs/performance/spec.md`：性能优先总约束、停用暂停轮询、缓冲上限、连接复用、并发探测五条需求。
- [x] 1.2 `openspec/config.yaml` 项目上下文的硬性约束清单加「所有修改性能优先」。

## 2. 既有实现核对（规格对应的代码已在前几次提交完成）

- [x] 2.1 `frontend/src/shell/polling.ts`：`useActivePolling` 按 keep-alive 激活状态启停轮询；终端、远程控制 IO/寄存器、长 ping 均已接入。
- [x] 2.2 终端缓冲上限：后端 `maxTerminalBuffer`（1MB，带截断提示与测试），前端 20 万字符。
- [x] 2.3 长 ping 日志上限 500 行，代次机制防止旧 goroutine 串台。
- [x] 2.4 终端分屏四会话共享一条 SSH 连接（`terminals map[string]*terminalSession`，上限 4）。
- [x] 2.5 网段扫描并发 ping + ARP 表采样，/24 秒级完成。

## 3. 收尾

- [x] 3.1 `go test ./...` 与 `wails build` 通过。
- [x] 3.2 归档本变更，delta spec 同步进 `openspec/specs/performance/`。

## 1. 后端：模块骨架与连接

- [x] 1.1 新增 `internal/modules/board/config/config.json`：`device`（host `192.168.1.136`、port `22`、user `root`、password 空）、`connectTimeoutSeconds`、`commandTimeoutSeconds`、`defaultPath`（定 `/opt`，实测那儿是主战场）。
- [x] 1.2 新增 `internal/modules/board/config.go`：`Settings` / `Device` 类型、`//go:embed config/config.json`、`parseSettings` 的缺省填充与范围校验、解析失败时的 `builtinSettings` 兜底并带 `Warning`。
- [x] 1.3 新增 `internal/modules/board/board.go`：`Module`（`New`、`ID`、`Bindings`、`Startup(ctx)`）、`Service` 与它持有的连接和互斥锁、`Config` / `Connect` / `Disconnect` / `Status` 四个方法。
- [x] 1.4 新增 `internal/modules/board/ssh.go`：`dial`（密码 + keyboard-interactive、`InsecureIgnoreHostKey`）、`dialWithin`（超时覆盖 TCP 建连 + 握手 + 认证，握手后撤掉 deadline）、`run`（一条命令开一个 session，返回 stdout / stderr / 退出状态）。
  - 没有 `quote`：改用 SFTP 之后没有任何一处需要把路径拼进 shell 命令，指令按钮的命令是原样下发的。留一个没人调的转义函数只会让下一个人以为某处在用它。
- [x] 1.5 `Connect` 重复调用时先断旧连接；被动断开由后台 `conn.Wait()` 察觉后立刻改状态，不必等下一次调用。

## 2. 后端：指令按钮

- [x] 2.1 新增 `internal/modules/board/commands.go`：`Command` 类型（`id`、`name`、`command`）、清单文件定位（`os.UserConfigDir()` + `embedtools/board-commands.json`）。
- [x] 2.2 `ListCommands`：文件不存在返回空列表；解析失败返回空列表加告警文案，且不写回那个坏文件。
- [x] 2.3 `SaveCommands`：先写同目录临时文件再 `os.Rename`；写失败把错误抛回前端而不是只记日志。
- [x] 2.4 保存前校验名称与命令都非空白，并回报是哪一项为空。
- [x] 2.5 清单文件的完整路径随 `CommandList.Path` 一起返回，不单开一个 `CommandsFilePath` 方法——界面拿清单的同时就拿到了路径，多一个方法多一次往返。
- [x] 2.6 `RunCommand(id)`：按 id 取出命令原样下发，返回 stdout、stderr、是否成功；没有输出时由前端显示「执行完毕，没有输出」。

## 3. 后端：文件管理（SFTP）

- [x] 3.1 `go get github.com/pkg/sftp`（v1.13.11）。
- [x] 3.2 `Connect` 里在 SSH 连接上开 SFTP 客户端（`sftp.NewClient`），失败时的报错和「地址连不上」「认证被拒」是分开的三句话；`Disconnect` 里先关 SFTP 再关 SSH。
- [x] 3.3 新增 `internal/modules/board/files.go`：`Entry` 类型（`name`、`size`、`isDir`）、`ListDir(path)` 用 `client.ReadDir` 取回并转成 `[]Entry`，目录排在文件前面。
- [x] 3.4 `Upload(localPath, remoteDir, overwrite)`：写 `<目标>.tmp`（`io.Copy` 流式，不整份读进内存），`Close` 成功后顶替目标；任何一步失败都删掉 `.tmp` 并保证原文件没动过。
  - 顶替优先用 posix-rename 扩展（目标存在时原子替换），设备不支持才退回「删目标 + Rename」——后者中间有一小段目标不存在的窗口。
- [x] 3.5 `Download(remotePath, localPath)`：先 `client.Stat` 记下大小，流式写本地 `<localPath>.part`，核对字节数一致后 `os.Rename`；失败或字节数不符就删掉 `.part`。
- [x] 3.6 「覆盖同名文件」的确认走 `Upload` 的 `UploadResult.NeedsConfirm`，不单开一个 `Exists` 方法：分成两个调用的话，两次之间远端可能已经变了，而确认框问的是上一次看到的状态。
- [x] 3.7 `Delete(remotePath)`：`client.Remove`；不在本地判断是不是目录，把 SFTP 返回的错误原样上报。
- [x] 3.8 文件对话框：`Startup` 里存下 `ctx`，`PickLocalFile` / `PickSaveTarget` 用 `runtime.OpenFileDialog` / `runtime.SaveFileDialog` 选本地路径。

## 4. 后端：单元测试

- [x] 4.1 `config_test.go`：随包配置合法；缺省值填充；空密码不被拒；超时与端口越界被拒；兜底配置本身能过校验。
- [x] 4.2 `ssh_test.go`：连接超时覆盖握手——对着一个只接 TCP 不说 SSH 的监听端口，`dial` 在超时内返回；地址 / 用户名为空时报得清楚。
- [x] 4.3 `commands_test.go`：空名称 / 空命令被拒；写入后读回内容一致；编号递增且重复编号被重发；坏 JSON 退回空列表加告警且原文件未被覆盖；临时文件在成功后不残留。
- [x] 4.4 `files_test.go`：起一个本地 SFTP 服务端（`sftp.NewServer` 配一对内存管道）跑 `Upload` / `Download` 往返，断言 0x00–0xff 全字节值逐字节一致。
- [x] 4.5 `files_test.go`：上传失败时 `.tmp` 被清掉且原文件内容未变；下载目录被拒且本地不留 `.part`；覆盖同名文件后内容是新的。
  - 「字节数不符」没写测试：要让服务端谎报大小才能造出来，那得自己实现一个说谎的 SFTP 服务端。逻辑留在 `download` 里，靠真机验证（7.11 拔网线）覆盖。
- [x] 4.6 `files_test.go`：`ListDir` 把 `ReadDir` 的结果正确转成 `[]Entry`，目录与文件区分正确、目录排前，空目录返回空切片而不是 nil。

## 5. 前端

- [x] 5.1 新增 `frontend/src/modules/board/module.ts`：id `board`、名称「主板控制」、版本 `V1.0.0`。
- [x] 5.2 新增 `BoardView.vue`：连接区一行放 IP、端口、用户名、密码（带明文切换）与连接 / 断开按钮；下面是「指令」「文件」两个标签页。
- [x] 5.3 连接状态与操作结果共用一条固定占一行的状态栏，不用 `v-if` 让它进出 DOM。
- [x] 5.4 `onMounted` 读 `Config()` 填默认值；`onUnmounted` 调 `Disconnect()`。不自动连接：这个页面上的动作做过就回不去。
- [x] 5.5 新增 `CommandPanel.vue`：按钮网格、添加 / 编辑表单、删除确认；未连接时执行按钮不可点（编辑和删除不需要连接），执行期间禁用整个按钮区。
- [x] 5.6 `CommandPanel.vue`：日志区按时间倒序追加每次执行的命令与输出，成功和失败用左侧色条区分。
- [x] 5.7 `CommandPanel.vue`：底部显示清单文件的完整路径，并说明换电脑要手动拷这个文件。
- [x] 5.8 新增 `FilePanel.vue`：路径输入框加「列出」、条目表格（名称 / 类型 / 大小，目录加粗且可双击进入）、单选选中。
- [x] 5.9 `FilePanel.vue`：上传（同名先确认）、下载（选中目录时按钮直接禁用）、删除（确认框显示完整远端路径）；传输期间禁用操作区并显示正在传输；空目录明确提示。
- [x] 5.10 上传或删除成功后自动重新列一次当前目录；重列失败不推翻这次操作的成功提示。

## 6. 接线与文档

- [x] 6.1 `modules.json` 加 `board-only` profile（`all` 是 `["*"]`，board 自动进）；跑 `go run ./tools/genmodules -profile all`。
- [x] 6.2 `wails generate module` 生成 TS 绑定。
- [x] 6.3 新增 `doc/board.md`：做什么、界面操作、配置文件、按钮清单文件的位置与搬移方式、后端接口表、实现要点、已知限制、相关文件。
- [x] 6.4 `doc/README.md` 的模块表格加一行；根 `README.md` 的「已有模块」「目录结构」「profile 示例」都加上 board。
- [x] 6.5 已知限制里写清：不支持交互式命令与 tty；无传输进度与续传；上传不保留权限位；不支持递归删除与目录上传下载；依赖设备上有 `sftp-server`；长命令跑完前看不到输出；按钮清单不随 exe 迁移；不校验主机密钥。

## 7. 验证

- [x] 7.1 `go vet ./...` 与 `go test ./...` 全绿，含 `internal/modules/boundary_test.go` 的模块边界检查。
- [x] 7.2 `vue-tsc --noEmit` 与 `vite build` 通过；`wails build` 出产物，双击能起来。
- [x] 7.3 用 `board-only` profile 构建一次：bundle 里只搜得到「主板控制」，另两个模块的名字都不在；再用 `netcfg-only` 构建一次，确认 board 能被整块剔掉。
- [ ] 7.4 真机验证：空密码连上 `192.168.1.136`，四个输入框都显示且可改，明文切换能看到密码是空的。
- [ ] 7.5 真机验证：添加三个按钮，重开程序后仍在；改一个、删一个（确认框拦一次），文件内容跟着变。
- [ ] 7.6 真机验证：跑一条成功的命令和一条失败的命令，日志区区分得开；跑一条没有输出的命令也有明确回馈。
- [ ] 7.7 真机验证：列 `/opt`（应看到 `runtime/`、`autorun.sh`、`zlgmaster` 等）、列 `/`（含 `lib64 -> lib` 这类符号链接）、列一个空目录。
- [ ] 7.8 真机验证：上传一个二进制文件到 `/tmp` 后在设备上 `md5sum` 比对；覆盖同名文件走确认框；上传期间设备上看不到 `.tmp` 残留。
- [ ] 7.9 真机验证：下载刚上传的文件，本地 `md5sum` 一致；选中目录时下载按钮是灰的。
- [ ] 7.10 真机验证：删除 `/tmp` 里那个文件（确认框里显示完整路径），列表自动刷新；对一个非空目录点删除时看到设备返回的错误，目录仍在。
- [ ] 7.11 真机验证：传输中途拔网线，确认远端原文件没被截断、本地不留 `.part`，界面退回未连接。

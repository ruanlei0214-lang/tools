## Why

控制器主板上的日常操作现在全靠手边一台装了 SSH 客户端的电脑：敲 `ssh root@192.168.1.136`，
凭记忆输几条命令，再用另一个工具把文件传上去。三件事分散在三个地方，每件都要人记住细节——
哪条命令是重启进程的、运行时目录在哪、上传完要不要改权限。记错的代价由现场承担。

这些操作本身没有难度，难的是它们没有一个固定的落点。工具箱里已经有 netcfg 在管这台设备的网口，
主板控制是同一台设备、同一条 SSH 通道上的另一半工作，理应也有一个页面。

## What Changes

- 新增模块 `board`（主板控制），侧边栏显示为「主板控制」。它是一个独立模块，
  和 `netcfg`、`remote` 之间没有任何引用与交互。
- **SSH 连接区**：IP、端口、用户名、密码四个输入框全部显示在界面上，可改。
  出厂默认 `192.168.1.136` / `22` / `root` / 空密码，来自模块自己的
  `internal/modules/board/config/config.json`。空密码是这台设备的真实状态，
  界面不拦、不提示补一个。
- **指令按钮**：一格一个按钮，点一下把它带的命令发到主板上执行，输出显示在下方的日志区。
  按钮可以在界面上添加、编辑、删除，改完立刻生效，不需要重新构建。
- 按钮清单存在 `%APPDATA%\embedtools\board-commands.json`，和 netcfg 记地址那份文件同一个目录。
  **不放 exe 旁边**：exe 可能落在 Program Files 或只读共享盘上，那里写不进去（`netcfg/state.go`
  已经因为同样的理由这么选过一次）。代价是拷走 exe 不会带走按钮，所以界面上直接显示这个文件的
  完整路径，要搬到另一台电脑就手动拷它。
- 首次打开按钮区是空的，不预置任何命令。凭空给几条命令等于替现场决定该怎么操作这台设备。
- **文件管理**：填一个远端路径列出目录内容（默认 `/opt`），可以上传本地文件到该目录、
  把选中的文件下载到本地、删除选中的文件。上传下载的本地文件靠系统文件对话框选。
- 删除只能删列表里选中的那一行，界面上不提供「输入路径直接删」的入口，且删除前弹确认框。
  不支持递归删目录：非空目录会被设备直接拒绝。
- 文件操作走 SFTP。规划期间连真机探过：这台控制器跑的是 OpenSSH 8.6、
  `/usr/libexec/sftp-server` 在位、子系统握手成功，所以不需要为「设备可能没有 SFTP」
  准备第二套实现。理由与被推翻的原方案见 design.md。
- 新增文档 `doc/board.md`，并在 `doc/README.md` 的模块表格与根 `README.md` 的「已有模块」里各加一段。

## Capabilities

### New Capabilities

- `board-ssh-connect`: 主板 SSH 连接的建立、凭据在界面上的可见性与连接状态的呈现。
- `board-commands`: 自定义指令按钮的增删改、持久化位置，以及执行与输出回显。
- `board-files`: 远端目录列出、文件上传、下载、删除，及其安全边界。

### Modified Capabilities

<!-- 无。新增一个模块不改变模块独立性规格的任何要求，反而是它的一个新实例：
     board 自带前后端两半、不引用其他模块、能被 profile 单独剔掉。 -->

## Impact

- 新增 `internal/modules/board/`：`board.go`（模块入口与 `Service`）、`ssh.go`（连接与命令执行）、
  `commands.go`（按钮清单的读写）、`files.go`（SFTP 列目录与传输）、`config.go` + `config/config.json`
  （出厂默认值），以及各自的 `_test.go`。
- `ssh.go` 会和 `netcfg/ssh.go` 有一部分重复（`dialWithin` 那套「超时要盖住握手和认证」的处理）。
  这是模块独立约束的要求：不能 import 另一个模块，共享逻辑要么下沉共享层、要么各写一份。
  只有两个使用方且形态未必一致，暂不下沉——真下沉了就得先想清楚共享层要长什么样。
- 新增 `frontend/src/modules/board/`：`module.ts`、`BoardView.vue`（连接区与标签页外壳）、
  `CommandPanel.vue`、`FilePanel.vue`。
- `modules.json`：`all` profile 加上 `board`；需要单模块 profile 时另加。
  改完跑 `go run ./tools/genmodules -profile all` 重新接线。
- 新增一个 Go 依赖 `github.com/pkg/sftp`。它只依赖已经在用的 `golang.org/x/crypto/ssh`
  （netcfg 在用），不引入新的传递依赖树。
- 运行期会在 `%APPDATA%\embedtools\` 下多一个 `board-commands.json`。这是本模块独占的文件，
  netcfg 的 `netcfg-state.json` 与它互不相干——同目录不构成模块间交互，两个模块没有共享同一个文件。
- 不改动 `netcfg`、`remote` 与共享层的任何代码。

# 终端（board）

当前版本 **V1.2.0**，声明在 `frontend/src/modules/board/module.ts`。侧栏显示为「终端」。

## 做什么

通过 SSH 在控制器主板上干两件事：

1. **跑指令**。常用命令存成一个个按钮，点一下就执行，成败显示在顶部状态栏。
   按钮清单自己攒，存在本机，重开程序还在。
2. **传文件**。列远端目录，往里上传本地文件、把选中的文件下载到本地、删除选中的文件。

文件操作走 SFTP，指令走 SSH 的 exec 通道，两者共用同一条连接。默认目标是
`root@192.168.1.136:22`，空密码——实测这台设备就是这么登的。

## 界面操作

1. **连接区在顶栏，不在本页**。顶栏显示共享地址和 SSH / WS 两个状态点，一个「连接」
   按钮同时建立两条连接；地址、用户名、密码、私钥在 exe 同目录的
   `toolbox-config.json` 里改，全系列工具共用这一份。端口仍用配置默认值（22）。选了密钥就先走公钥登录，
   密码可兼作密钥口令，没有密钥时仍用密码（空密码也可以）。程序不会自动连接。
2. 本页左右分栏：**左边文件，右边终端**。指令按钮排在终端下方，小按钮，左键执行、右键编辑或删除。点「＋」展开名称和命令，保存即可。导入导出仍在工具行。
3. 连接后自动打开 SSH 终端。点指令会把命令送进去。点进终端可直接打字；`Ctrl+C` 无选中时中断当前命令，有选中时复制。程序正在往终端打字时，划选文字会暂停刷屏，取消选中后再把攒着的输出贴上去，避免选区被冲掉。
4. 终端最多分屏成四格：头部「⬌ 分屏」左右并排、「⬍ 分屏」上下并排，第三、四格自动成四宫格（三格时第三格独占底行）。格间分隔条可拖动调整比例。每格是设备上一个独立的 shell 会话；最后点过的格子是当前终端（蓝边），头部的 Ctrl+C、重开、清屏和指令按钮都作用在它身上。悬停格子右上角出现 ×，可关掉该格（最后一格不能关）。
5. 文件区地址栏可手改，回车或「刷新」打开。上一级、上传、粘贴在同一行。列表下方留白，右键可粘贴、新建文件夹。单击选中，Ctrl+单击加选，Shift+单击连选；多选后右键的复制、剪切、删除对整批生效（重命名、编辑、下载仍是单个）。双击目录进去；双击图片（png/jpg/gif/webp/bmp/svg）在终端下方预览，终端与图片之间的横条可拖动，其它文件编辑文本（二进制或超过 48KB 会拒）。复制移动删除走终端里的 `cp` / `mv` / `rm` / `mkdir` / `cat`，重命名在文件名上就地改（回车确认、Esc 取消），不弹窗。上传下载仍走 SFTP。左右中间的竖条可拖动。
5. 连接归顶栏管，切换模块不断开；要断开点顶栏「断开」，两条连接一起收。

## 配置文件

`internal/modules/board/config/config.json` 编译进产物，**改完要重新构建才生效**。
地址、用户名、密码、密钥在 exe 同目录的 `toolbox-config.json` 里改。端口和超时仍取这里的默认值。
配置解析失败不会让页面打不开，只会退回内置兜底并在顶部显示一条告警。

```json
{
  "device": { "host": "192.168.1.136", "port": 22, "user": "root", "password": "" },
  "connectTimeoutSeconds": 8,
  "commandTimeoutSeconds": 30,
  "defaultPath": "/opt"
}
```

| 字段 | 说明 |
| --- | --- |
| `device` | `host` / `user` / `password` 是没有共享配置时的出厂值；`port` 连的时候用，不在界面上。`password` 留空就是空密码 |
| `connectTimeoutSeconds` | 建连超时，1–120，省略按 `8`。它覆盖 TCP 建连 + SSH 握手 + 认证三步的总时长 |
| `commandTimeoutSeconds` | 单条指令的上限，1–600，省略按 `30` |
| `defaultPath` | 文件标签页打开时填在路径框里的远端目录，省略按 `/opt` |

**共享配置优先。** `Config()` 返回的地址、用户名、密码、密钥路径，优先用
exe 同目录的 `toolbox-config.json`（remote 和 netcfg 也读这份）；没有或坏掉才退回
上面这份编译进产物的 `config.json`。改凭据就改 exe 旁边那份共享配置，
全系列工具下次连接都用新值。

## 按钮清单文件

和远程控制的点位配置一样，分两层：

| 层 | 位置 | 谁写 |
| --- | --- | --- |
| 现场配置 | exe 同目录的 `board-commands.json` | 界面上保存或导入时写，优先用 |
| 出厂默认 | `internal/modules/board/config/commands.json` | 编译进产物，改它要重新构建 |

干净机器上还没有现场文件时，界面就是出厂那两份按钮（重启运行时、看进程），没有告警。
界面上第一次保存或导入才生成现场文件。「恢复默认」就是删掉现场那一份。
完整路径显示在指令标签页底部。整夹拷走会一起带走；只拷一个 exe 文件则不会。

文件是一个 JSON 数组，也可以用操作栏的导入导出换一份：

```json
[
  { "id": "c1", "name": "重启运行时", "command": "/opt/autorun.sh restart" },
  { "id": "c2", "name": "看进程", "command": "ps | grep zlgmaster" }
]
```

`id` 由程序发放，人工编辑时留空或者写重了会在下一次保存时重新发一个。
现场文件坏掉时界面退回出厂默认并给一条告警，**不会覆盖那个文件**——里面的命令
还有救回来的机会。

## 后端接口

`internal/modules/board` 的 `Service`，前端从 `wailsjs/go/board/Service` 导入。

| 方法 | 签名 | 说明 |
| --- | --- | --- |
| `Config` | `() => Promise<Settings>` | 界面默认值 + 配置告警；地址和 SSH 凭据优先用共享配置 `toolbox-config.json` |
| `Connect` | `(d: Device) => Promise<Status>` | 建 SSH 连接并在上面开 SFTP，重复调用先断旧的；连上后把地址写进共享配置 |
| `SaveDevice` | `(d: Device) => Promise<Settings>` | 把连接参数写进共享配置，全系列工具共用；界面不再调用，改凭据直接改文件 |
| `Disconnect` | `() => Promise<Status>` | 主动断开 |
| `Status` | `() => Promise<Status>` | 当前连接状态 |
| `ListCommands` | `() => Promise<CommandList>` | 读按钮清单（现场优先，没有就出厂默认），不需要连接 |
| `SaveCommands` | `(cmds: Command[]) => Promise<CommandList>` | 整份写回现场文件，返回补齐编号后的那份 |
| `ResetCommands` | `() => Promise<CommandList>` | 删掉现场清单，退回出厂默认 |
| `ExportCommands` | `() => Promise<string>` | 弹出保存框，把当前清单写成 JSON；取消返回空字符串 |
| `ImportCommands` | `() => Promise<CommandFileResult>` | 弹出打开框，校验后整份替换；取消时 `canceled` 为真、清单不动 |
| `RunCommand` | `(id: string) => Promise<CommandResult>` | 旧的一次性执行接口，仍保留 |
| `StartTerminal` | `(id: string) => Promise<void>` | 在当前 SSH 连接上为 id 打开持久 PTY；id 存活时复用，新会话最多 4 个 |
| `RunCommandInTerminal` | `(id, cmdId: string) => Promise<void>` | 从清单取出命令并写入 id 对应的终端 |
| `WriteTerminal` | `(id, text: string) => Promise<void>` | 原样写入 id 对应的终端 |
| `ReadTerminal` | `(id: string) => Promise<string>` | 取走 id 对应终端新产生的输出 |
| `CloseTerminal` | `(id: string) => Promise<void>` | 只关闭 id 对应的终端，不影响 SSH/SFTP 和其它终端 |
| `ListDir` | `(dir: string) => Promise<Entry[]>` | 列远端目录 |
| `ReadRemoteText` | `(path: string) => Promise<string>` | 读一份够小的文本，给编辑框用；二进制或超过 48KB 会拒 |
| `ReadRemoteBytes` | `(path: string) => Promise<string>` | 读一份不超过 4MB 的文件（前端拿到的是 base64），给终端下方预览图片用 |
| `PickKeyFile` | `() => Promise<string>` | 弹对话框选本机私钥，取消返回空串 |
| `PickLocalFile` | `() => Promise<string>` | 弹对话框选要上传的本地文件，取消返回空串 |
| `PickSaveTarget` | `(name: string) => Promise<string>` | 弹对话框选下载落点，取消返回空串 |
| `Upload` | `(local, remoteDir, overwrite) => Promise<UploadResult>` | 上传；`overwrite` 为假且同名文件已存在时只回报要确认 |
| `Download` | `(remotePath, localPath) => Promise<void>` | 下载 |
| `Delete` | `(remotePath: string) => Promise<void>` | 删一个远端文件，不递归 |

`Device`：`{ host, port, user, password, keyPath }`。`Status`：`{ connected, addr, error }`，
`error` 是被动断开的原因。`Command`：`{ id, name, command }`。
`CommandList`：`{ commands, path, warning }`，`path` 是现场清单的完整路径。
`CommandFileResult`：`{ list, path, canceled }`。
`CommandResult`：`{ command, stdout, stderr, success, error }`。
`Entry`：`{ name, size, isDir }`。`UploadResult`：`{ remotePath, needsConfirm }`。

## 实现要点

- **文件操作全部走 SFTP，不经过设备的 shell**。用户填的路径不会被拼进任何一条命令，
  带空格或引号的路径因此不需要转义，也不会因为转义写错而把操作落到别的文件上。
  列目录直接拿结构化结果，不用解析 `ls -lA` 的输出——那份输出里符号链接、带空格的
  文件名、各家实现的日期列都得单独处置，而这台设备的 `/` 下就摆着两个符号链接。
- **规划期间连真机探过**：OpenSSH 8.6、`/usr/libexec/sftp-server` 在位、
  子系统握手成功，所以不为「设备可能没有 SFTP」准备第二套传输实现。
  换了机型这一块会整体失效，因此 SFTP 起不来时的报错和「地址连不上」「认证被拒」
  是分开的三句话，现场能一眼看出坏在哪一层。
- **上传先写 `<目标>.tmp`，完整落盘后再顶替**。直接往目标上写的话，SFTP 会当场把它
  截断，传到一半断掉就留下半个文件——而那可能正是设备在跑的东西（`/opt` 下就摆着
  `zlgmaster`、`autorun.sh`）。任何一步失败都清掉临时文件，原文件一个字节没动。
  顶替优先用 posix-rename 扩展，它在目标存在时是原子的。
- **下载核对字节数**。流式读到 EOF 分不清「文件读完了」和「连接断了」，不比一次
  远端报告的大小就可能把半个文件当成完整的收下来。本地也是先写 `.part` 再改名，
  失败或字节数不符就删掉，不留一个内容不全的文件。
- **删除不在本地判断是不是目录**。界面上那个「是目录」来自上一次列目录的结果，
  可能已经过期。请求照发，让设备来拒。
- **连接超时覆盖握手和认证**。`ssh.Dial` 的 `Timeout` 只管 TCP 建连那一段，
  对着一个接受 TCP 却不说 SSH 的地址（透明代理、被占用的 22 端口）会一直等下去。
  这里在连接上设 deadline 走完三步，握手成功后再撤掉——留着的话之后每条命令的
  读写都会在那个时间点上一起失败。
- **执行终端是持久 PTY，且按会话 ID 多开**。连接后在同一条 SSH 连接上申请 `xterm` PTY 并启动 shell，
  指令按钮和手工输入都写进它；stdout/stderr 进入线程安全缓冲区，前端每 150ms 取一次。
  这样命令输出能实时显示，`cd`、环境变量等 shell 状态也会保留。
  分屏后每个格子是一个独立会话（`terminals map[string]*terminalSession`，上限 4），
  共享同一条 SSH 连接——多路复用是 SSH 原生能力，四个 shell 也只是四条 channel。
  前端每格一个 `TerminalPane` 组件，各自轮询自己的会话；布局（单屏/左右/上下/四宫格）
  和分隔条比例归 `CommandPanel`。
- **终端输出有上限**。后端积压超过 1MB 时丢弃较早的一半，前端最多保留约 20 万字符，
  防止持续输出把内存耗尽。
- **连接断了自己会知道**。后台等着这条连接结束，设备重启或网线被拔时状态立刻改回未连接，
  不用等下一次点按钮才发现。
- **按钮清单整份存整份取**。前端每次改动都送整份过来，由后端校验、补编号、落盘，
  再拿它写进去的那份当准。增删改各写一遍校验和落盘只会让它们慢慢长歪。
  落盘是先写同目录临时文件再改名：写一半挂掉的话，整个清单都会读不出来。
- **出厂默认与现场覆盖**。和 remote 的点位文件同一套取舍：exe 旁边有合法的
  `board-commands.json` 就用它，没有或坏掉就用编译进产物的 `commands.json`。
  「恢复默认」只删现场文件。导入导出弹系统对话框，校验不过不写盘。
- **状态栏恒占一行**。有状态槽的界面各留一条，没消息时也留着位置。让它按有没有消息进出 DOM 的话，
  每操作一下下面整片内容都会跟着上下跳一次，手指还停在原处按钮已经移开了。
  本页的连接状态槽随连接区一起收进了顶栏，只剩配置告警在有内容时占一行。
- **文件在左、终端在右，不再分标签**。中间竖条可拖。指令按钮压在终端下方——
  终端是这页的主角，按钮是顺手工具，不该一进门就占住顶部。
- **Tab 补全的响铃和半截转义丢掉，半个 UTF-8 也不先解码**，否则 `cd /opt/` 后面会冒问号方块。
- **文件页只列当前这一层**。地址栏可编辑。列表下方留白专门给右键粘贴。
  复制、移动、粘贴、重命名、删除把命令写进同一条 SSH 终端；列目录、上传下载、
  打开编辑框读内容仍走 SFTP。文件路径不跟终端的 `cd` 走：从回显里解析 cd 只能猜到
  直接敲的那几条，脚本和别名里的切换都会漏，跟错目录比不跟更害人。

## 已知限制

- 终端是文本回显区，不是完整的 xterm 模拟器。ANSI 颜色和光标控制码会被去掉，
  普通 shell、脚本和交互式问答可用，但 `vi`、`top` 这类全屏程序显示会错乱。
- 终端输入是一行文本；特殊键只单独提供了 `Ctrl+C`。
- **传输没有进度也不能续传**。大文件只能等它自己结束，中途失败要重新来。
- **上传不保留权限位**。传上去的脚本没有执行位，要的话用指令按钮跑一次 `chmod +x`。
- **上传下载不递归**，也不能整个目录上传或下载。右键删除目录会在终端里跑 `rm -rf`，
  复制目录跑 `cp -a`，这两条是递归的。
- **依赖设备上有 `sftp-server`**。没有的话文件标签页整个不可用，连接时就会报出来。
- **只拷 exe 文件不会带走现场清单**。出厂按钮编在产物里，现场改过的那份和 exe
  在同一目录，整夹拷走会一起带走。
- 主机密钥不校验（`InsecureIgnoreHostKey`）。内网嵌入式设备通常没有稳定的主机密钥，
  而且它的 IP 可能刚被 netcfg 改过。

## 相关文件

```
internal/modules/board/board.go              模块入口、Service 与连接管理
internal/modules/board/ssh.go                建连（超时覆盖握手）与命令执行
internal/modules/board/terminal.go           持久 SSH PTY、输入与输出缓冲
internal/modules/board/commands.go           按钮清单的读写与校验
internal/modules/board/files.go              SFTP 列目录与上传下载
internal/modules/board/config.go             配置结构与校验
internal/modules/board/config/config.json    连接默认值，编译进产物，改完要重新构建
internal/modules/board/config/commands.json  出厂指令清单，编译进产物
frontend/src/modules/board/module.ts         模块清单
frontend/src/modules/board/BoardView.vue     文件与终端的分栏外壳
frontend/src/modules/board/CommandPanel.vue  指令按钮、编辑表单与终端分屏布局
frontend/src/modules/board/TerminalPane.vue  单个终端格子：输出渲染、按键捕获、划词保持
frontend/src/modules/board/FilePanel.vue     远端资源管理器，右键操作走终端
frontend/src/modules/board/ContextMenu.vue   指令按钮与文件列表共用的右键菜单
```

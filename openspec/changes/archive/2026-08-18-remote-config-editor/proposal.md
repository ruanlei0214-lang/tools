## Why

远程控制的点位清单现在只活在编译进产物的 `internal/modules/remote/config/io.json` 与
`register.json` 里。现场想加一路 DO、改一个寄存器地址，得回到装了 Go 和 Wails 的机器上
改文件、重新构建、再把新的 exe 拷回去。而这些改动恰恰是最频繁的：换一台设备、
换一套夹具，点位表就得跟着动一遍。

主板控制的指令按钮早就是在界面上加的，改完立刻能用。远程控制的点位是同一类东西——
一张现场自己维护的清单——却还要靠重新构建才能改。这一版把它补齐。

## What Changes

- **IO 和寄存器两页各自带一个内联配置编辑器**，两页用同一套控件：分组的增删改、
  点位的增删改、拖不动的顺序就按清单顺序。IO 页配 `DI/DO/AI/AO` + 端口号，
  寄存器页配 `BOOL/INT/FLOAT` + 地址，除此之外字段、布局、交互完全一致。
  形态照 `board` 的指令编辑：在卡片内展开一行，不另占一大块区域。
- **保存后立即生效，不重新构建、不重启程序。** 保存即写盘、即重新加载，
  当前页立刻按新清单重画。
- **连接参数也可改并立即生效**：地址、端口、握手路径、建连超时、请求超时、自动刷新间隔。
  地址和端口本来就在连接区能改，这一版把它们连同其余四项一起存下来，
  下次打开还是上次那套值。超时对下一次请求生效，刷新间隔对下一次轮询生效。
- **配置存到 `%APPDATA%\embedtools\`**，三份文件分别对应现在的三份 JSON。
  编译进产物的那三份降级为出厂默认：`%APPDATA%` 里没有文件时用它们播种。
  和 `board` 的按钮清单同一个目录，理由也一样——exe 可能落在 Program Files
  或只读共享盘上，那里写不进去。代价是拷走 exe 不会带走配置，所以界面上显示这个文件的完整路径。
- **每一份都能「恢复默认」**：把 `%APPDATA%` 里那份删掉，退回编译进产物的出厂值。
  三份互不牵连，改坏了 IO 不用连带把寄存器也重置。
- **坏文件不让页面打不开**：`%APPDATA%` 里那份读不出来或过不了校验时，退回出厂默认并在顶部告警，
  同时保留坏文件不覆盖——现场手改出的错至少还能自己找回来。
- 保存前做和现在一样的校验（类型、端口非负、`pulseMs` 范围、分组非空），
  不合格就拒绝保存并把原因说在界面上，不写坏盘上那份。

## Capabilities

### New Capabilities

- `remote-point-config`: IO 与寄存器点位清单在界面上的增删改、两页共用同一套控件与校验，
  以及保存后立即生效的范围。
- `remote-connection-config`: 连接参数（地址、端口、路径、两个超时、刷新间隔）在界面上的可改、
  持久化与生效时机。
- `remote-config-storage`: 配置的存放位置、出厂默认值的播种、恢复默认、坏文件的兜底与告警。

### Modified Capabilities

<!-- 无。这一版只动 remote 模块自己的配置来源，不改变模块独立性规格的任何要求：
     配置仍留在本模块内，写的仍是本模块独占的文件，与 netcfg、board 没有共享状态。 -->

## Impact

- `internal/modules/remote/config.go`：`loadSettings` 从「只读内嵌」改成「先读 `%APPDATA%`、
  没有就用内嵌播种」；`parseRoot` / `parsePanel` 的校验被保存路径复用。
- 新增 `internal/modules/remote/store.go` + `store_test.go`：三份文件的读写、原子落盘
  （先写 `.tmp` 再改名，照 `board/commands.go`）、恢复默认。
- `internal/modules/remote/remote.go`：`Service.settings` 从「构造时读一次的值」改成可重载的状态，
  加 `SaveDevice` / `SavePanel` / `ResetPanel` 一类方法；已在跑的连接不因保存配置而断开，
  除非地址真的变了。
- `frontend/src/modules/remote/`：`IoPanel.vue` 与 `RegisterPanel.vue` 各加一个内联编辑区，
  共用同一套控件；`RemoteView.vue` 的连接区补上路径、超时、刷新间隔与保存。
  两个面板的编辑控件形态一致，但**不抽出跨模块的公共组件**——同模块内可以共用一个组件文件。
- `internal/modules/remote/config/`：三份 JSON 语义从「唯一配置」变成「出厂默认」，内容不动。
- `doc/remote.md`：配置文件一节重写（出厂默认 vs 现场配置、存放位置、立即生效、恢复默认），
  界面操作一节补编辑步骤；`frontend/src/modules/remote/module.ts` 版本号跟着跳。
- 运行期会在 `%APPDATA%\embedtools\` 下多三个文件。它们是 remote 模块独占的，
  与 `netcfg-state.json`、`board-commands.json` 互不相干。
- 不改动 `netcfg`、`board` 与共享层的任何代码。

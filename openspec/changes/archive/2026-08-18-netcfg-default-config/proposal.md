## Why

netcfg 页面的默认值现在写死在 `NetcfgView.vue` 里（设备地址 `192.168.1.100`、端口 22、用户名 `root`、掩码 `255.255.255.0`），一键恢复要删的 `/opt/runtime/pi` 写死在 Go 和 Vue 两处。不同现场的设备出厂地址、登录账号、控制器配置文件路径都可能不一样，改这些值现在得翻到源码里三个地方分别改，还容易漏掉 Vue 里那几处文案。把它们收进一份配置文件，改一处就够。

## What Changes

- netcfg 模块新增 `config.json`，用 `go:embed` 编译进程序。里面放页面的默认值：设备地址、端口、用户名、密码、默认子网掩码、默认网关，以及一键恢复要删除的文件路径。
- 页面启动时从后端取这份配置来填初始值，不再在前端写死。
- 连接卡片只保留「设备地址」输入框；端口、用户名、密码从界面上撤掉，只从配置取值。
- 一键恢复删除的路径改为取自配置；界面上原本硬编码 `/opt/runtime/pi` 的三处文案（按钮说明、结果提示、弹窗正文）同步改为显示配置里的路径。
- 配置解析失败时退回内置默认值继续可用，并在界面上给出提示，不让程序起不来。
- 界面文案（标题、按钮名、提示语）**不**进配置，仍写在组件里。

## Capabilities

### New Capabilities
- `netcfg-default-config`: netcfg 页面默认值与恢复路径的配置来源、校验规则和失效兜底。

### Modified Capabilities
<!-- 无法在此声明 delta：本次调整的"一键恢复删除固定路径"这条行为，其规范目前还在
     未归档的 netcfg-restore-network 变更里，尚未进入 openspec/specs/。
     该行为的新形态写在本变更的 spec 里，两个变更需按 netcfg-restore-network →
     netcfg-default-config 的顺序归档。 -->

## Impact

- `internal/modules/netcfg/config.json`：新增，编译进产物的默认配置。
- `internal/modules/netcfg/config.go`：新增，嵌入、解析、校验与兜底。
- `internal/modules/netcfg/config_test.go`：新增，保证随包发布的 config.json 是合法的。
- `internal/modules/netcfg/netcfg.go`：`Service` 新增 `Defaults` 方法；`RestoreNetwork` 改用配置里的路径，删掉写死的常量。
- `frontend/src/modules/netcfg/NetcfgView.vue`：启动拉取默认值、路径文案取自配置、配置异常提示。
- `frontend/wailsjs/go/netcfg/Service.{js,d.ts}` 与 `models.ts`：由 wails 重新生成的绑定。
- 不涉及新依赖，不涉及其他模块。改配置需要重新 `wails build`。

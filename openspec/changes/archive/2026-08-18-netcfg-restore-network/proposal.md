## Why

设备上的 `/opt/runtime/pi` 文件会让机器人控制器沿用一套已持久化的网络参数。当这套参数写错、或设备被搬到另一个网段后，控制器的网络就不通了，现场只能登录设备手工删文件再重启。netcfg 模块已经握有 SSH 连接，把这个动作做成一个按钮，可以让现场不必再开终端。

## What Changes

- netcfg 模块新增"一键恢复网络"按钮，通过已有的 SSH 连接参数删除设备上的 `/opt/runtime/pi`。
- 删除是这个功能在设备上做的唯一动作：不动其他文件，也不重启设备或服务。
- 删除成功后弹窗提示需要重启机器人控制器，重启由现场人工执行，工具不提供重启入口。

## Capabilities

### New Capabilities
- `netcfg-restore-network`: 删除设备上的网络持久化文件，并提示人工重启控制器让改动生效。

### Modified Capabilities
<!-- 无。module-independence 的要求没有变化。 -->

## Impact

- `internal/modules/netcfg/netcfg.go`：`Service` 新增一个方法 `RestoreNetwork`（删除文件）。
- `frontend/src/modules/netcfg/NetcfgView.vue`：新增按钮、重启提示弹窗与结果提示。
- `frontend/wailsjs/go/netcfg/Service.{js,d.ts}`：由 wails 重新生成的绑定。
- 不涉及新依赖，不涉及其他模块。

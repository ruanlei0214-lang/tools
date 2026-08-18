## 1. 后端

- [x] 1.1 在 `internal/modules/netcfg/netcfg.go` 添加 `RestoreNetwork(d Device) error`，通过 SSH 执行 `rm -f /opt/runtime/pi`
- [x] 1.2 确认删除是唯一的远程动作：模块不提供重启设备的方法

## 2. 前端

- [x] 2.1 重新生成 wails 绑定，让 `frontend/wailsjs/go/netcfg/Service.{js,d.ts}` 包含 `RestoreNetwork`
- [x] 2.2 在 `NetcfgView.vue` 的设备连接卡片加"一键恢复网络"按钮，hint 写明要删除的路径
- [x] 2.3 删除成功后弹出模态弹窗提示需人工重启机器人控制器，弹窗只有一个关闭按钮，同一提示同时写入 banner

## 3. 校验

- [x] 3.1 `go build ./...` 与 `go test ./...` 通过
- [x] 3.2 前端类型检查通过

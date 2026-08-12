## 1. 后端配置

- [x] 1.1 新增 `internal/modules/netcfg/config.json`，写入现有的默认值与 `/opt/runtime/pi`
- [x] 1.2 新增 `internal/modules/netcfg/config.go`：`go:embed` 嵌入、`Settings` 类型、解析与校验（端口范围、恢复路径必须绝对）、内置兜底值
- [x] 1.3 在 `netcfg.go` 的 `Service` 上加 `Defaults() Settings`
- [x] 1.4 `RestoreNetwork` 改用配置里的路径，删掉写死的 `runtimeConfigPath` 常量
- [x] 1.5 新增 `config_test.go`：随包配置可解析且通过校验，另覆盖坏 JSON、端口越界、相对路径三种不可用情形

## 2. 前端

- [x] 2.1 重新生成 wails 绑定，让前端能拿到 `Defaults` 与 `Settings` 类型
- [x] 2.2 `NetcfgView.vue` 去掉默认值字面量，改为 `onMounted` 里调 `Defaults()` 填 `device` 与表单
- [x] 2.3 `select()` 补掩码时改用配置里的默认掩码
- [x] 2.4 按钮说明、结果提示、弹窗正文三处的 `/opt/runtime/pi` 改为显示配置里的路径
- [x] 2.5 配置不可用时渲染一条常驻提示，与操作结果横幅互不覆盖
- [x] 2.6 连接卡片撤掉端口、用户名、密码三个输入框，只留设备地址

## 3. 文档与校验

- [x] 3.1 `doc/netcfg.md` 补配置文件说明：字段含义、改完要重新构建、密码明文进 exe 的注意事项、相关文件清单
- [x] 3.2 `go test ./...`、`go vet ./...`、前端类型检查通过
- [x] 3.3 实际构建一次，确认配置生效（改一个值验证，再改回来）

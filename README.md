# Estun Codroid 机器人工具箱

Go + Wails v2 桌面工具，用于处理 Estun Codroid 机器人控制器（嵌入式 Linux）的日常事务。
界面是 Vue 3 + TypeScript，依托系统 WebView2。构建产物是绿色版目录：exe、配置、
WebView2 缓存都在同一夹里，整夹拷走就能用，不用安装。

每个功能是一个**独立模块**：自带后端逻辑与前端界面，模块之间不互相引用。

这条约束由 `internal/modules/boundary_test.go` 强制执行——任何模块引用了另一个模块，
`go test ./...` 直接失败。需要共用的逻辑，要么下沉到 `internal/module`（后端）
和 `frontend/src/shell`（前端），要么就在各自模块里各写一份。

## 环境要求

| 依赖     | 版本                    |
| -------- | ----------------------- |
| Go       | 1.25+                   |
| Node.js  | 20+                     |
| Wails    | v2.14+                  |
| WebView2 | Windows 10/11 一般自带  |

安装 Wails CLI：

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@latest
wails doctor
```

## 开发与构建

```powershell
go run ./tools/buildui            # 图形界面；也可双击 tools\BuildUI.exe
.\build.ps1                      # 构建全部模块，绿色版目录 build\bin\C2toolsV1.0.4\
.\build.ps1 -Profile netcfg-only # 只编译指定 profile 的模块
go run ./tools/pickbuild         # 交互式挑模块再构建，不用先定义 profile
wails dev                        # 热重载开发，浏览器调试入口 http://localhost:34115
go test ./...                    # 后端单元测试
```

## 模块选装

哪些模块进入产物由 `modules.json` 的 profile 决定：

```json
{
  "profiles": {
    "all": ["*"],
    "netcfg-only": ["netcfg"],
    "board-only": ["board"]
  }
}
```

`build.ps1` 会先按 profile 生成接线代码，再构建。也可以分开跑：

```powershell
go run ./tools/genmodules -profile netcfg-only
wails build
```

生成器扫描模块目录、按 profile 过滤，写出 `internal/modules/modules_gen.go` 和
`frontend/src/shell/modules.gen.ts`。**没被选中的模块不会被任何代码 import，
因此 Go 代码不进 exe、界面代码不进 bundle**，不是只把入口藏起来。

生成的两个文件带 `DO NOT EDIT` 标记，不要手改；它们会提交进仓库，所以直接
`wails build` 也能用当前 profile 构建。

### 临时组合：交互式挑模块

组合只用一次，不值得往 `modules.json` 里加 profile 时，用交互式的挑选器：

```powershell
go run ./tools/pickbuild
```

```
可编译的模块：
  1) board
  2) netcfg
  3) remote

输入编号选择（逗号或空格分隔），直接回车=全选，q=退出：2
```

选完它就生成接线代码并 `wails build`，产物和路径与 `build.ps1` 一致。
固定下来的组合还是写成 profile 用 `build.ps1` 更省事——挑选器每次都要手动选，
不适合放进脚本或 CI。

它不改 `modules.json`，模块列表也不自己扫目录，而是转手给生成器：

```powershell
go run ./tools/genmodules -list                 # 打印可用模块，一行一个
go run ./tools/genmodules -modules remote,netcfg # 直接指定组合，不读 modules.json
```

这两个参数就是挑选器用的全部东西，也可以自己直接用。

## 目录结构

```
main.go                          应用入口：装配模块、启动窗口
doc/                             各模块的说明文档，一个模块一份
modules.json                     profile 配置：决定哪些模块进入产物
build.ps1                        按 profile 生成接线代码并构建
tools/BuildUI.exe                构建小窗口（双击即可）
tools/buildui/                   构建小窗口（Go）
tools/genmodules/                接线代码生成器
tools/pickbuild/                 交互式挑模块并构建
tools/packportable/              把 exe 收成绿色版目录
internal/module/                 模块接入契约（Module 接口）
internal/modules/modules_gen.go  生成的后端接线，勿手改
internal/modules/boundary_test.go 模块独立性检查
internal/modules/netcfg/         网络配置模块（后端）
internal/modules/remote/         远程控制模块（后端）
internal/modules/board/          终端模块（后端）
frontend/src/shell/registry.ts   前端模块清单
frontend/src/shell/modules.gen.ts 生成的前端接线，勿手改
frontend/src/modules/netcfg/     网络配置模块（前端）
frontend/src/modules/remote/     远程控制模块（前端）
frontend/src/modules/board/      终端模块（前端）
frontend/src/style.css           设计变量与通用控件样式
frontend/wailsjs/                Wails 自动生成的 TS 绑定，不要手改
```

## 新增一个模块

以 `foo` 模块为例。

**后端**：新建 `internal/modules/foo/foo.go`，实现 `module.Module`。

```go
package foo

type Module struct{ svc *Service }

func New() *Module          { return &Module{svc: &Service{}} }
func (m *Module) ID() string { return "foo" }
func (m *Module) Bindings() []any { return []any{m.svc} }

// Service 上所有导出方法都会暴露给前端。
type Service struct{}

func (s *Service) DoSomething(arg string) (string, error) { ... }
```

目录名必须与 Go 包名一致，构造函数必须叫 `New()`，生成器据此接线。

需要 Wails 运行时上下文（弹窗、事件等）的模块，额外实现 `Startup(ctx context.Context)` 即可，
框架会在启动时自动调用。

带配置文件的模块，配置一律放在 `internal/modules/foo/config/` 下，用
`//go:embed config/config.json` 嵌进来。单独一个目录是为了让现场一眼找到哪些文件是给人改的——
模块目录里 `.go` 文件越堆越多之后，一个 `config.json` 夹在中间并不显眼。
配置留在模块内而不是提到仓库根目录，是模块独立约束要求的：模块被 profile 剔掉时，
它的配置得跟着一起消失。

**前端**：新建目录 `frontend/src/modules/foo/`，放入 `module.ts` 和视图组件。

```ts
import type { ModuleManifest } from '../../shell/registry'
import FooView from './FooView.vue'

export default {
  id: 'foo',
  name: '模块名称',
  description: '一句话说明',
  version: 'V1.0.0',
  view: FooView,
} satisfies ModuleManifest
```

`version` 是这个模块自己的版本号，和别的模块无关，现场在顶部右侧「关于」里能看到。
约定见 [doc/README.md](doc/README.md#版本号)。

最后跑一次生成器把新模块接进来：

```powershell
go run ./tools/genmodules -profile all
```

再补一份文档 `doc/foo.md`，并在 [doc/README.md](doc/README.md) 的表格里加一行。
文档结构照抄现有的那两份。

模块自己的样式一律写在 SFC 的 `<style scoped>` 里。`style.css` 是所有模块共用的，
往里加模块专属规则会串到别的模块上。

视图里通过生成的绑定调用后端：

```ts
import { DoSomething } from '../../../wailsjs/go/foo/Service'
```

绑定由 `wails dev` / `wails build` 自动生成，也可以手动 `wails generate module`。
样式直接用 `style.css` 里的通用类（`card`、`field-row`、`banner`、`primary` 等），
保持各模块外观一致。

## 已有模块

### 远程控制（remote）

通过远程模式接口控制上位机的 IO 与寄存器。传输层是 WebSocket（默认
`ws://192.168.1.136:9000/`），报文沿用接口文档的 `{"id","ty","db"}` 结构。

- 点位和连接参数都在界面上改，改完立即生效，不用动代码也不用重新构建；
  现场配置存在 exe 同目录，编译进产物的出厂 JSON 可逐份恢复。
  IO / 寄存器点位也可以用操作栏的导入导出换一份 JSON。
  「测试流程」挂在 IO 控制右侧，从点位里选步骤，单步或按间隔连续触发，用来对节拍
- 开关量（DI/DO/BOOL）点一下翻转（先读回当前值再写反的）；非开关量（AO/INT/FLOAT）
  在行内填一个值再「下发」；配了 `pulseMs` 的点位还多一个点动（写完等一会儿自动恢复）
- 一条长连接跑到底，连接状态显式显示，不做自动重连；改了地址也要自己点「重新连接」

### 终端（board）

通过 SSH 在控制器主板上跑自定义指令、上传下载文件。界面填 IP、用户、密码，也可选本机私钥登录；端口用配置默认值。

- 连接后左边是文件、右边是终端；点进终端即可直接打字，也可发送 Ctrl+C
- 常用命令是终端上方的小按钮，点击后送入同一个终端；出厂清单编在产物里，现场改过的存在
  exe 同目录的 `board-commands.json`，也可以导入导出
- 文件操作走 SFTP：不解析 `ls` 的输出，用户填的路径也不会被拼进任何一条 shell 命令
- 上传先写 `.tmp`、下载先写 `.part`，核对无误后才顶替目标，中途断了不留半个文件
- 指令与文件共用一条 SSH 连接，连接断了后台立刻知道，界面跟着退回未连接

### 网络配置（netcfg）

通过 SSH 读取和修改设备网口的 IP、子网掩码、默认网关。

- 密码认证，同时支持 `password` 与 `keyboard-interactive`（兼容 dropbear）
- 用 `ip addr show` / `ip route show` 读取现状，兼容 busybox 与 iproute2
- 下发的命令会脱离 SSH 会话在后台执行，避免改地址时把自己的连接掐断
- 网关必须与新 IP 同网段，掩码必须连续，网口名做白名单校验

两个已知约束：

- **配置不持久化。** 只用 `ip` 命令改运行时状态，设备重启后失效。持久化方式各家设备不同
  （`/etc/network/interfaces`、netplan、自定义启动脚本），需要按目标设备补。
- **不校验主机密钥。** 面向内网设备，且工具本身就会改设备 IP，固定主机密钥没有意义。

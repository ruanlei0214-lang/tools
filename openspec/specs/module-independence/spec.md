# 模块独立性

## Purpose

本规格定义项目的核心架构约束：每个功能是一个独立模块，模块之间互不引用、互不交互，
代码保持精简，修改保持最小。其中"互不引用"由 `internal/modules/boundary_test.go`
强制执行，越界会导致 `go test ./...` 失败；其余三条靠评审与 AI 协助把关。

## Requirements

### Requirement: 模块之间不允许互相引用

任何模块的代码 SHALL NOT import 另一个模块。后端指 `internal/modules/<a>/` 引用
`internal/modules/<b>/`，前端指 `frontend/src/modules/<a>/` 里的 `.ts`/`.vue`
import 另一个模块目录。共享逻辑 SHALL 下沉到共享层（后端 `internal/module`、
前端 `frontend/src/shell`），或者在各自模块里各写一份。

#### Scenario: 后端模块引用了另一个模块

- **WHEN** `internal/modules/foo/` 下的任何 `.go` 文件 import 了 `embedtools/internal/modules/bar`
- **THEN** `go test ./internal/modules/` 失败，并指出违规的文件与被引用的模块

#### Scenario: 前端模块引用了另一个模块

- **WHEN** `frontend/src/modules/foo/` 下的任何 `.ts` 或 `.vue` 文件 import 了 `../bar/...`
- **THEN** `go test ./internal/modules/` 失败，并指出违规的文件与被引用的模块

#### Scenario: 模块引用共享层是允许的

- **WHEN** 一个模块 import 了 `internal/module`（后端）或 `../../shell/...`（前端）
- **THEN** 测试通过，因为共享层不属于任何模块

### Requirement: 模块之间不允许有任何交互

模块之间 SHALL NOT 通过任何机制交换数据或触发行为，包括但不限于：共享可变状态、
全局事件总线、Wails 运行时事件（`EventsEmit`/`EventsOn`）、写同一个文件、
以及通过后端单例间接通信。一个模块的增删 SHALL NOT 影响其余模块的编译与运行。

#### Scenario: 删除一个模块不影响其余模块

- **WHEN** 从 `internal/modules/` 和 `frontend/src/modules/` 删除某个模块目录，
  并重新运行 `go run ./tools/genmodules -profile all`
- **THEN** 项目照常 `wails build` 通过，其余模块功能不受影响

#### Scenario: 模块通过事件总线通信被禁止

- **WHEN** 模块 foo 用 `EventsEmit` 发出事件、模块 bar 用 `EventsOn` 监听同一事件
- **THEN** 这违反本约束，即使它不产生 import 关系也不允许

### Requirement: 模块可以按 profile 选装

构建时 SHALL 能选择只编译部分模块。未被选中的模块 SHALL NOT 出现在构建产物中——
既不能在 exe 里留下它的 Go 代码，也不能在前端 bundle 里留下它的界面代码。
profile 定义在 `modules.json`，接线代码由 `tools/genmodules` 从中生成。

#### Scenario: 用子集 profile 构建

- **WHEN** 执行 `.\build.ps1 -Profile netcfg-only`，而该 profile 只列了 `netcfg`
- **THEN** 产物里搜不到 `hello` 模块的任何字符串，侧边栏也不显示它

#### Scenario: profile 引用了不存在的模块

- **WHEN** `modules.json` 的某个 profile 列出了目录里不存在的模块名
- **THEN** 生成器报错并列出现有模块，不生成任何文件

#### Scenario: 模块只有一半

- **WHEN** 某个模块只有 `internal/modules/<name>/` 或只有 `frontend/src/modules/<name>/`
- **THEN** 生成器报错指出缺失的那一半，而不是等到构建时才失败

### Requirement: 代码要求精简

代码 SHALL 用能表达同一行为的最少代码。同一逻辑 SHALL 只保留一份实现；只有一个
使用方的逻辑 SHALL 放在使用方，出现第二个使用方时再上提到共享层。SHALL NOT 为
"以后可能用到"预留参数、配置项、抽象层或空函数。

#### Scenario: 为假想需求预留抽象

- **WHEN** 一个函数只有一个调用点，却预留了未被使用的选项参数或抽象层
- **THEN** 应当删除这些预留，只保留当前真实需要的签名

#### Scenario: 同一逻辑出现两份实现

- **WHEN** 某个逻辑在共享层已有一份实现，模块里又写了一份功能相同的
- **THEN** 应当删掉模块里的那份，改用共享层

### Requirement: 修改代码要求尽量少的改动

修改 SHALL 只改动为达成当前目标必须改动的代码。SHALL NOT 顺手重命名、顺手重排版、
顺手重构无关文件。修改完成后 SHALL 立刻删除因此失效的代码：不再被调用的函数、
不再被引用的变量与 CSS 规则、不再走到的分支、被新实现替换掉的旧实现。
SHALL NOT 保留注释掉的旧代码，SHALL NOT 加 deprecated 兼容层。

#### Scenario: 顺手重构无关代码

- **WHEN** 一次改动除了实现目标外，还重命名或重排了与目标无关的代码
- **THEN** 应当把这些无关改动从本次修改中剔除

#### Scenario: 留下失效代码

- **WHEN** 一次修改让某些函数、变量或分支不再被使用
- **THEN** 必须在同一次修改中把它们删除，而不是留到以后

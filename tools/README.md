# 构建工具

两个命令行工具，都**必须在仓库根目录运行**（它们用相对路径找模块目录）。

| 工具 | 干什么 | 什么时候用 |
| --- | --- | --- |
| [genmodules](#genmodules) | 按选定的模块生成接线代码 | 新增/删除模块后，或要换 profile 时 |
| [pickbuild](#pickbuild) | 交互式挑模块并构建 | 临时组合，懒得先去 `modules.json` 加 profile |

## 为什么需要它们

Go 和前端都是静态编译的，"哪些模块进产物"这件事没法在运行时决定，只能在编译前
把接线代码写死。`genmodules` 就是干这个的：它扫描模块目录，按你选的集合生成两个文件。

```
internal/modules/modules_gen.go     后端：只 import 选中模块，构造 All()
frontend/src/shell/modules.gen.ts   前端：只 import 选中模块的 module.ts
```

没被选中的模块不会被任何代码 import，所以 **Go 代码不进 exe、界面代码不进 bundle**，
不是把入口藏起来而已。

这两个文件带 `DO NOT EDIT` 标记，**不要手改**，改了下次生成就被覆盖。它们提交进仓库，
所以直接 `wails build` 也能用当前已生成的组合构建。

---

## genmodules

### 命令

```powershell
# 用 modules.json 里的 profile（不带 -profile 时默认 all）
go run ./tools/genmodules -profile all
go run ./tools/genmodules -profile netcfg-only

# 临时指定模块，不读 modules.json
go run ./tools/genmodules -modules netcfg
go run ./tools/genmodules -modules hello,netcfg

# 只打印可用模块，一行一个，不生成任何文件
go run ./tools/genmodules -list
```

| 参数 | 默认 | 说明 |
| --- | --- | --- |
| `-profile <名称>` | `all` | 用 `modules.json` 里定义的 profile |
| `-modules <a,b>` | 空 | 直接指定模块，逗号分隔。**给了它就完全不读 `modules.json`** |
| `-list` | false | 打印可用模块后退出 |

生成完只是改了源码，**还要自己跑 `wails build`**。想一步到位用根目录的 `build.ps1`
或下面的 `pickbuild`。

### 输出长这样

```
> go run ./tools/genmodules -list
hello
netcfg

> go run ./tools/genmodules -profile all
profile "all"：启用 2 个模块 [hello netcfg]

> go run ./tools/genmodules -modules netcfg
profile "自选"：启用 1 个模块 [netcfg]
```

用 `-modules` 时生成文件头部标的是 `profile: 自选`，看到这个就知道当前工作区是临时组合，
不对应任何一个 profile。

### 它会挡住哪些错误

前三条**报错退出，不生成文件**；第四条只警告，构建照常。

```
genmodules: modules.json 里没有 profile "nope"，可选：all、netcfg-only
genmodules: 指定的模块 "ghost" 不存在，现有模块：hello、netcfg
genmodules: 模块 foo 只有后端，缺少 frontend/src/modules/foo/
genmodules: 警告：模块 foo 缺少文档 doc/foo.md
```

还有两条隐性校验：目录名必须与 Go 包名一致（否则生成的 import 是错的），
构造函数必须叫 `New()`。这两条不满足会在生成或编译时暴露。

文档检查针对**所有发现到的模块**，不只是这次选中的——文档该不该有，跟这次装不装它无关。

---

## pickbuild

### 命令

```powershell
go run ./tools/pickbuild
```

列出模块让你按编号挑，然后自动跑 `genmodules` + `wails build`。

```
可编译的模块：
  1) hello
  2) netcfg

输入编号选择（逗号或空格分隔），直接回车=全选，q=退出：2
```

输入 `1,2` / `1 2` / `1、2` 都行，重复编号会自动去重。直接回车全选，`q` 退出且不构建。

### 和 build.ps1 的区别

| | `build.ps1` | `pickbuild` |
| --- | --- | --- |
| 模块从哪来 | `modules.json` 的 profile | 现场手选 |
| 适合 | 固定组合，比如给某客户的交付版本 | 临时试一下，用完就换 |
| 可重复 | 是，profile 名就是记录 | 否，靠你记得当时选了什么 |

**要交付给客户的组合请写成 profile**，别用 pickbuild——profile 名留在 `modules.json` 里
才说得清当时发的是哪个版本。

模块发现和合法性校验 pickbuild 自己不做，全部转交 `genmodules -list`，
免得同一套规则在两个地方各写一份、日后走样。

---

## 常见问题

**改了 `modules.json` 但产物没变** —— 光改 JSON 不生效，得跑一次生成器。真正决定产物的是
生成出来的那两个文件。

**`wails dev` 里看不到某个模块** —— 当前工作区的生成文件是上次生成的结果。看一眼
`internal/modules/modules_gen.go` 第二行的 `// profile:` 就知道现在是什么组合，
要全量就 `go run ./tools/genmodules -profile all`。

**报"读取 modules.json 失败"** —— 没在仓库根目录运行。两个工具都用相对路径。

**新增模块后没被扫到** —— 检查前后端两个目录是否都建了、后端目录里是否有 `.go` 文件。
只有一半的话生成器会明确报出来缺哪一半。

**改 `modules.json` 时能不能写注释** —— 不能，JSON 不支持。想说明某个 profile 给谁用，
写在 `doc/` 里或者用 profile 名表达。文件存成带 BOM 的 UTF-8 是没问题的，已经做了兼容。

---

相关文档：根目录 [README.md](../README.md)（整体架构与新增模块流程）、
[doc/](../doc/)（各模块说明）。

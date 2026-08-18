## Why

产物是单个 exe，现场拿到手上看不出装的是哪一版：exe 属性里没有版本号，界面上也没有任何地方写。出问题时第一句「你用的哪个版本」就问不下去，只能靠文件日期猜。

模块又是按 profile 选装的，不同客户手上的 exe 装的模块可能都不一样。一个笼统的"软件版本"说明不了"你这台上的网络配置模块是哪一版"，而现场报的问题基本都落在某个具体模块上。

## What Changes

- 每个模块声明自己的版本号，形如 `V1.0.1`，互相独立：改了一个模块不牵动其他模块的版本。版本写在模块目录的 `module.ts` 里，和 `name`、`description` 同一处。
- `ModuleManifest` 新增必填的 `version` 字段，漏写的模块编译不过。
- 工具箱整体另有一个版本（`frontend/src/shell/version.ts` 的 `APP_VERSION`），回答"这个 exe 是哪一版"。
- `wails.json` 补 `info` 段，让 exe 文件属性里也能看到版本；同时给 `build/windows/info.json` 补上 Wails 默认模板漏掉的 `FileVersion` 键——缺了它 Windows 会把整张版本字符串表一起丢掉。
- 产物改名为 `C2toolsV1.0.1.exe`（`wails.json` 的 `outputfilename`），文件名自带版本，不打开程序就能分辨手上是哪一版。`build.ps1` 与 `tools/pickbuild` 的完成提示改为从 `wails.json` 读这个名字，不再各写一份；`tools/appicon/dump_exe_icons.py` 的默认路径改为在 `build/bin` 里找 exe。
- 侧栏底部新增「关于」入口，点开的弹窗分两组：**工具箱**给名称与版本，**当前模块**给名称、标识、版本与一句说明。切到别的模块再打开，第二组整组跟着变。不列出所有模块的版本。
- 产品名提取为 `APP_NAME`，侧栏标题与「关于」共用一份，不再各写一遍。
- 现有两个模块与工具箱整体初始都是 `V1.0.1`。

## Capabilities

### New Capabilities
- `module-versioning`: 模块各自独立编号，并在「关于」里可查。

### Modified Capabilities
<!-- 无。模块的功能行为都不变。 -->

## Impact

- `frontend/src/shell/registry.ts`：`ModuleManifest` 加 `version`。
- `frontend/src/shell/version.ts`（新增）：`APP_VERSION`。
- `frontend/src/modules/{hello,netcfg}/module.ts`：各加一行 `version`。
- `frontend/src/App.vue`：侧栏「关于」按钮与弹窗，含 scoped 样式。
- `wails.json`：新增 `info` 段（产品名、版本、公司、版权）；`outputfilename` 改为 `C2toolsV1.0.1`。
- `build/windows/info.json`：补 `FileVersion` 与 `fixed.product_version`。
- `build.ps1`、`tools/pickbuild/main.go`、`tools/appicon/dump_exe_icons.py`：不再写死产物文件名。
- `README.md`、`doc/README.md`、`doc/hello.md`、`doc/netcfg.md`：版本约定与各模块当前版本。
- 后端不改，`tools/genmodules` 不改——版本在 manifest 内部，生成的接线代码整体导入 manifest，看不见这个字段。
- 不涉及新依赖、不破坏模块独立性：版本是模块的自我描述，没有引入任何跨模块引用。

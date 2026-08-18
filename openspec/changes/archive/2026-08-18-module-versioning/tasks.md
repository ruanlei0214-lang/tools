## 1. 版本声明

- [x] 1.1 `ModuleManifest` 新增必填的 `version: string`，注释说明为什么只声明这一处。
- [x] 1.2 `modules/hello/module.ts`、`modules/netcfg/module.ts` 各加 `version: 'V1.0.1'`。
- [x] 1.3 新增 `shell/version.ts` 导出 `APP_VERSION = 'V1.0.1'`，注释写明要和 `wails.json` 手工同步。
- [x] 1.4 `wails.json` 补 `info` 段：`productName`、`productVersion`、`companyName`、`copyright`。
- [x] 1.5 `build/windows/info.json` 补 `FileVersion` 字符串键与 `fixed.product_version`。Wails 的默认模板漏了 `FileVersion`，缺了它 Windows 读版本信息时会把整张字符串表一起丢掉。
- [x] 1.6 `wails.json` 的 `outputfilename` 改为 `C2toolsV1.0.1`，产物文件名自带版本。
- [x] 1.7 `build.ps1` 与 `tools/pickbuild` 的完成提示改为从 `wails.json` 读 `outputfilename`；`tools/appicon/dump_exe_icons.py` 默认路径改为在 `build/bin` 里找 exe。

## 2. 「关于」界面

- [x] 2.1 `App.vue` 侧栏底部加「关于」按钮，`margin-top: auto` 顶到底，不进模块导航列表。
- [x] 2.2 弹窗分两组：工具箱（名称、版本）与当前模块（名称、标识、版本、说明）；没有当前模块时只留第一组。
- [x] 2.3 遮罩点击与「知道了」都能关闭。
- [x] 2.4 弹窗样式 scoped 在 `App.vue`，不提取共享组件。
- [x] 2.5 产品名提取为 `APP_NAME`，侧栏标题与「关于」共用。

## 3. 文档

- [x] 3.1 `doc/README.md` 新增「版本号」一节：独立编号的理由、声明位置、与 `wails.json` 的同步要求、现场怎么看。
- [x] 3.2 `doc/README.md` 模块表格加版本列。
- [x] 3.3 `doc/hello.md`、`doc/netcfg.md` 标题下标注当前版本与声明位置。
- [x] 3.4 根 `README.md` 的 `module.ts` 模板加 `version`，并指向版本约定。

## 4. 验证

- [x] 4.1 `vue-tsc --noEmit` 在 netcfg-only 与 all 两种 profile 下都通过。
- [x] 4.2 `wails build` 通过，Wails 绑定无需改动。
- [x] 4.3 截图确认：「关于」在侧栏底部，弹窗分两组；切到 Hello World 后当前模块那组整组跟着变。
- [x] 4.4 确认 exe 文件属性里的 ProductName / ProductVersion / FileVersion / CompanyName 都读得出来。
- [x] 4.5 清空 `build/bin` 后重新构建，确认产物就叫 `C2toolsV1.0.1.exe`。

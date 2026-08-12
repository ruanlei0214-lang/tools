## Why

网络配置页刚做完一轮排版（三步流程、竖排网口、固定状态栏、紧凑间距）。功能是对的，但顺手留下几处已经没人用的残骸，以及一处和「状态栏固定」原则打架的地方：

- 全局样式里还有给旧表格用的 `readonly` / `blank` 行样式，网口已经改成竖排列表，仓库里没有任何 `<table>` 再带这些 class。
- `NetcfgView.vue` 注释还在说「表格」「空表格」，和下方真实 UI 对不上，下一眼读代码会被带偏。
- 配置告警（`configWarning`）还是 `v-if` 出现/消失，操作结果状态栏已经固定占位了，告警一闪下方还是会跳——同一页两种规则。
- 同文件里 `.banner` 与 `.status-slot` 两套样式并存，告警走前者、结果走后者。

越往后改越难分清哪些是现役、哪些是上一版的影子。排版刚落定，上下文还热，把这些清掉。

## What Changes

- 删掉 `frontend/src/style.css` 里已无引用的 `tbody tr.readonly` / `tbody tr.blank`（及对应 hover）。表格基础样式留给以后模块，不整段删表。
- `NetcfgView.vue` 注释里「表格」改成「列表」，与竖排 UI 一致。
- 配置告警并进固定高度的状态槽：有告警显示告警，没有告警也不挤占/释放高度；操作结果仍走同一条状态槽。`configWarning` 与操作 `banner` 的优先级写清楚：告警未被操作冲掉（操作只清 `banner`）。
- 去掉仅服务于旧 `v-if` banner 的重复样式，状态槽样式收成一套。
- 同步 `doc/netcfg.md` 里若仍写「表格展示」且指界面的表述。

明确**不做**：不抽共享弹窗组件、不拆 `NetcfgView` 的 CSS 到独立文件、不改后端、不改网口映射/可改性、不把「无网口时第 2 步整块消失」改成占位（那是另一次 UX 决策）。

## Capabilities

### New Capabilities
- `netcfg-status-bar`: 网络配置页状态区的占位与内容规则——有无文案都占同一高度，配置告警与操作结果如何共用、谁优先。

### Modified Capabilities
<!-- 无。模块独立性等主规范不改；面板网口等 delta 仍在未归档变更里，本变更不碰它们的需求。 -->

## Impact

- `frontend/src/modules/netcfg/NetcfgView.vue`：状态槽用法、注释、scoped 样式整理。
- `frontend/src/style.css`：删掉无引用的表格行修饰。
- `doc/netcfg.md`：界面描述与状态栏行为对齐（若有过时「表格」字样）。
- 不涉及新依赖、不涉及其他模块、不改 Go / Wails 绑定。

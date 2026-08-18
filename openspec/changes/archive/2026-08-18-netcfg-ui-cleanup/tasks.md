## 1. 状态槽统一

- [x] 1.1 `NetcfgView.vue`：用 computed（如 `status`）合并 `configWarning` 与 `banner`——告警优先；两者皆空则为 `null`（模板走 `idle`）。
- [x] 1.2 模板去掉独立的 `v-if="configWarning"` 横幅；连接区下方只保留一条 `.status-slot`，绑定合并后的状态。
- [x] 1.3 确认 `call()` 只清 `banner`、不清 `configWarning`，与 spec「告警不被操作冲掉」一致。
- [x] 1.4 删掉仅服务于旧横幅的重复 `.banner` scoped 样式（若已无引用）。

## 2. 死代码与过时表述

- [x] 2.1 `frontend/src/style.css`：删除无引用的 `tbody tr.readonly`、`tbody tr.blank` 及其 hover；保留表格基础样式与 `.selected`。
- [x] 2.2 `NetcfgView.vue` 脚本注释：把「表格 / 空表格」改成「列表」等与竖排 UI 一致的说法。
- [x] 2.3 `doc/netcfg.md`：若界面描述仍把网口区叫「表格」且指当前 UI，改为列表/竖排表述；补一句状态区固定占位（有无文案高度不变）。

## 3. 验证

- [x] 3.1 `npx vue-tsc --noEmit` 通过。
- [x] 3.2 `wails build` 通过（若 exe 被占用先结束进程再编）。
- [ ] 3.3 目测：无消息时状态槽仍在；点「测试连接」时下方网口区不纵向跳动；人为制造配置告警时告警占同一槽且操作不清掉它。

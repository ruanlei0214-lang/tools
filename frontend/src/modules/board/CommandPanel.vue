<script lang="ts" setup>
import { computed, onMounted, reactive, ref, shallowReactive } from 'vue'
import {
  ExportCommands,
  ImportCommands,
  ListCommands,
  RunCommandInTerminal,
  SaveCommands,
} from '../../../wailsjs/go/board/Service'
import type { board } from '../../../wailsjs/go/models'
import ContextMenu, { type MenuItem } from './ContextMenu.vue'
import TerminalPane from './TerminalPane.vue'

const props = defineProps<{ connected: boolean }>()
const emit = defineEmits<{
  (e: 'refresh-status'): void
}>()

const commands = ref<board.Command[]>([])
const listWarning = ref('')
const busy = ref('')
// title 是悬停提示：导出导入的结果文案只显示文件名，完整路径挂在 title 上。
const banner = ref<{ kind: 'ok' | 'err' | 'info'; text: string; title?: string } | null>(null)

// ---- 终端分屏 ----
// 最多四格。第二格由用户选左右还是上下；第三、四格固定四宫格
// （三格时第三格独占底行）。格子大小拖分隔条调，比例存 colRatio/rowRatio。
interface PaneApi {
  ready: boolean
  ensure: () => Promise<void>
  reopen: () => Promise<void>
  send: (text: string) => void
  clear: () => void
  focus: () => void
}

const panes = ref<string[]>(['t1'])
let paneSeq = 1
const activeId = ref('t1')
const splitDir = ref<'cols' | 'rows'>('cols')
const colRatio = ref(0.5)
const rowRatio = ref(0.5)
const dragging = ref(false)
const paneArea = ref<HTMLElement | null>(null)
const paneRefs = shallowReactive(new Map<string, PaneApi>())

const layout = computed<'single' | 'cols' | 'rows' | 'grid'>(() => {
  if (panes.value.length === 1) return 'single'
  if (panes.value.length === 2) return splitDir.value
  return 'grid'
})

const activePane = computed(() => paneRefs.get(activeId.value))
const activeReady = computed(() => activePane.value?.ready ?? false)
const showVDivider = computed(() => layout.value === 'cols' || layout.value === 'grid')
const showHDivider = computed(() => layout.value === 'rows' || layout.value === 'grid')

// 格子摆进带分隔条轨道的 grid：第 2 列/第 2 行是 6px 的拖拽缝。
const gridStyle = computed(() => {
  const cols = `${colRatio.value}fr 6px ${1 - colRatio.value}fr`
  const rows = `${rowRatio.value}fr 6px ${1 - rowRatio.value}fr`
  switch (layout.value) {
    case 'cols':
      return { gridTemplateColumns: cols, gridTemplateRows: '1fr' }
    case 'rows':
      return { gridTemplateColumns: '1fr', gridTemplateRows: rows }
    case 'grid':
      return { gridTemplateColumns: cols, gridTemplateRows: rows }
    default:
      return { gridTemplateColumns: '1fr', gridTemplateRows: '1fr' }
  }
})

function paneStyle(i: number) {
  const n = panes.value.length
  if (n === 1) return { gridColumn: '1', gridRow: '1' }
  if (n === 2) {
    return splitDir.value === 'cols'
      ? { gridColumn: i === 0 ? '1' : '3', gridRow: '1' }
      : { gridColumn: '1', gridRow: i === 0 ? '1' : '3' }
  }
  const spots = [
    { gridColumn: '1', gridRow: '1' },
    { gridColumn: '3', gridRow: '1' },
    n === 3 ? { gridColumn: '1 / -1', gridRow: '3' } : { gridColumn: '1', gridRow: '3' },
    { gridColumn: '3', gridRow: '3' },
  ]
  return spots[i]
}

const vDividerStyle = computed(() => ({
  gridColumn: '2',
  gridRow: layout.value === 'grid' && panes.value.length > 3 ? '1 / -1' : '1',
}))
const hDividerStyle = computed(() => ({ gridColumn: '1 / -1', gridRow: '2' }))

function paneRefSetter(id: string) {
  return (el: unknown) => {
    if (el) paneRefs.set(id, el as PaneApi)
    else paneRefs.delete(id)
  }
}

function addPane(dir: 'cols' | 'rows') {
  if (panes.value.length >= 4) return
  if (panes.value.length === 1) splitDir.value = dir
  const id = `t${++paneSeq}`
  panes.value.push(id)
  activeId.value = id
}

function closePane(id: string) {
  if (panes.value.length <= 1) return
  const at = panes.value.indexOf(id)
  if (at < 0) return
  // 后端会话由 TerminalPane 的卸载钩子关掉，这里只管布局。
  panes.value.splice(at, 1)
  if (activeId.value === id) activeId.value = panes.value[panes.value.length - 1]
}

function startDrag(which: 'col' | 'row', e: MouseEvent) {
  const area = paneArea.value
  if (!area) return
  dragging.value = true
  const rect = area.getBoundingClientRect()
  const onMove = (ev: MouseEvent) => {
    const ratio =
      which === 'col' ? (ev.clientX - rect.left) / rect.width : (ev.clientY - rect.top) / rect.height
    // 钳在 15%~85%：再小终端就剩不下几列字，等于变相关屏却没有关屏的入口。
    const clamped = Math.min(Math.max(ratio, 0.15), 0.85)
    if (which === 'col') colRatio.value = clamped
    else rowRatio.value = clamped
  }
  const onUp = () => {
    dragging.value = false
    window.removeEventListener('mousemove', onMove)
    window.removeEventListener('mouseup', onUp)
  }
  window.addEventListener('mousemove', onMove)
  window.addEventListener('mouseup', onUp)
}

function sendCtrlC() {
  const pane = activePane.value
  pane?.send('\u0003')
  pane?.focus()
}

function reopenTerminal() {
  return act('terminal-reopen', async () => {
    await activePane.value?.reopen()
    banner.value = { kind: 'info', text: '终端已重新打开' }
  })
}

// 编辑中的那一条。id 为空表示新增。
const draft = reactive({ id: '', name: '', command: '' })
const editing = ref(false)

const canEdit = computed(() => draft.name.trim() !== '' && draft.command.trim() !== '')
const menu = ref<{ x: number; y: number; cmd: board.Command } | null>(null)
const menuItems: MenuItem[] = [
  { id: 'edit', label: '编辑' },
  { id: 'delete', label: '删除', danger: true },
]

// 清单在本机文件里，读它不需要连接，所以一进来就读。
onMounted(async () => {
  try {
    const list = await ListCommands()
    commands.value = list.commands ?? []
    listWarning.value = list.warning
  } catch (e) {
    listWarning.value = `读取按钮清单失败：${String(e)}`
  }
})

async function act(op: string, fn: () => Promise<void>) {
  busy.value = op
  try {
    await fn()
  } catch (e) {
    banner.value = { kind: 'err', text: String(e) }
    emit('refresh-status')
  } finally {
    busy.value = ''
  }
}

function run(c: board.Command) {
  return act(`run-${c.id}`, async () => {
    const pane = activePane.value
    if (!pane) return
    await pane.ensure()
    await RunCommandInTerminal(activeId.value, c.id)
    pane.focus()
  })
}

function startAdd() {
  draft.id = ''
  draft.name = ''
  draft.command = ''
  editing.value = true
}

function startEdit(c: board.Command) {
  draft.id = c.id
  draft.name = c.name
  draft.command = c.command
  editing.value = true
}

function save() {
  return act('save', async () => {
    const next = commands.value.map((c) => ({ ...c }))
    const at = next.findIndex((c) => c.id === draft.id)
    const row = { id: draft.id, name: draft.name.trim(), command: draft.command.trim() }
    if (at >= 0) {
      next[at] = row
    } else {
      next.push(row)
    }
    // 整份送回后端，由它校验、补编号、落盘，再拿它写进去的那份当准。
    // 本地先改一份再指望两边一致，迟早会对不上。
    const saved = await SaveCommands(next)
    applyList(saved)
    editing.value = false
    banner.value = { kind: 'ok', text: at >= 0 ? `已保存「${row.name}」` : `已添加「${row.name}」` }
  })
}

function remove(c: board.Command) {
  // 删按钮只是删清单里的一行，不动设备上任何东西，但攒了一串按钮之后误删也挺烦。
  if (!window.confirm(`删除按钮「${c.name}」？\n\n${c.command}`)) {
    return
  }
  return act(`del-${c.id}`, async () => {
    const saved = await SaveCommands(commands.value.filter((x) => x.id !== c.id).map((x) => ({ ...x })))
    applyList(saved)
    if (draft.id === c.id) {
      editing.value = false
    }
    banner.value = { kind: 'info', text: `已删除「${c.name}」` }
  })
}

function applyList(list: board.CommandList) {
  commands.value = list.commands ?? []
  listWarning.value = list.warning
}

function exportList() {
  return act('export', async () => {
    const path = await ExportCommands()
    if (!path) return
    banner.value = { kind: 'ok', text: `已导出：${fileName(path)}`, title: path }
  })
}

function importList() {
  const msg = editing.value
    ? '导入会替换当前指令清单，未保存的编辑会丢掉。继续？'
    : '导入会替换当前指令清单，继续？'
  if (!window.confirm(msg)) return
  return act('import', async () => {
    const r = await ImportCommands()
    if (r.canceled) return
    applyList(r.list)
    editing.value = false
    banner.value = { kind: 'ok', text: `已导入：${fileName(r.path)}`, title: r.path }
  })
}

function openMenu(e: MouseEvent, c: board.Command) {
  menu.value = { x: e.clientX, y: e.clientY, cmd: c }
}

function onMenu(id: string) {
  const c = menu.value?.cmd
  menu.value = null
  if (!c) return
  if (id === 'edit') startEdit(c)
  if (id === 'delete') remove(c)
}

function fileName(p: string): string {
  const i = Math.max(p.lastIndexOf('/'), p.lastIndexOf('\\'))
  return i >= 0 ? p.slice(i + 1) : p
}
</script>

<template>
  <section class="card command-card">
    <div class="terminal-panel">
      <div class="terminal-head">
        <span class="terminal-title">终端</span>
        <span class="terminal-state" :class="{ online: activeReady }">
          {{ activeReady ? '已打开' : props.connected ? '…' : '未连接' }}
        </span>
        <button
          class="terminal-tool"
          :disabled="!props.connected || panes.length >= 4"
          title="在右侧加开一个终端"
          @click="addPane('cols')"
        >
          ⬌ 分屏
        </button>
        <button
          class="terminal-tool"
          :disabled="!props.connected || panes.length >= 4"
          title="在下方加开一个终端"
          @click="addPane('rows')"
        >
          ⬍ 分屏
        </button>
        <button class="terminal-tool" :disabled="!props.connected" title="向当前终端发送 Ctrl+C" @click="sendCtrlC">Ctrl+C</button>
        <button class="terminal-tool" :disabled="!props.connected || !!busy" title="重开当前终端" @click="reopenTerminal">重开</button>
        <button class="terminal-tool" title="清空当前终端的显示" @click="activePane?.clear()">清屏</button>
      </div>
      <div ref="paneArea" class="pane-area" :class="{ dragging }" :style="gridStyle">
        <TerminalPane
          v-for="(id, i) in panes"
          :key="id"
          :ref="paneRefSetter(id)"
          :session-id="id"
          :connected="props.connected"
          :active="id === activeId"
          :closable="panes.length > 1"
          :style="paneStyle(i)"
          @activate="activeId = id"
          @close="closePane(id)"
          @error="(text) => (banner = { kind: 'err', text })"
          @refresh-status="emit('refresh-status')"
        />
        <div
          v-if="showVDivider"
          class="divider divider-v"
          :style="vDividerStyle"
          title="拖动调整左右比例"
          @mousedown.prevent="startDrag('col', $event)"
        />
        <div
          v-if="showHDivider"
          class="divider divider-h"
          :style="hDividerStyle"
          title="拖动调整上下比例"
          @mousedown.prevent="startDrag('row', $event)"
        />
      </div>
    </div>

    <!-- 指令区单独成一块、不许被压扁。终端输出再长也只在自己那格里滚，
         不能把下面的按钮挤出可视区域——现场就是这么坏的。 -->
    <div class="command-footer">
      <div class="command-toolbar">
        <button class="tool-btn add-command" :disabled="!!busy" title="新增指令按钮" @click="startAdd">＋ 新增</button>
        <span class="tool-sep" />
        <button class="tool-btn" :disabled="!!busy" title="把清单存成 JSON" @click="exportList">导出</button>
        <button class="tool-btn" :disabled="!!busy" title="从 JSON 整份替换清单" @click="importList">导入</button>
        <div
          v-if="banner || listWarning"
          class="status"
          :class="banner?.kind || 'err'"
          :title="banner?.title || banner?.text || listWarning"
        >
          {{ banner?.text || listWarning }}
        </div>
      </div>

      <div v-if="editing" class="editor-row">
        <input
          v-model="draft.name"
          class="editor-name"
          aria-label="名称"
          placeholder="名称"
          autofocus
          :disabled="!!busy"
          @keyup.enter="canEdit && save()"
          @keyup.esc="editing = false"
        />
        <input
          v-model="draft.command"
          class="editor-command"
          aria-label="命令"
          placeholder="命令"
          :disabled="!!busy"
          @keyup.enter="canEdit && save()"
          @keyup.esc="editing = false"
        />
        <button class="primary editor-action" :disabled="!!busy || !canEdit" @click="save">保存</button>
        <button class="editor-action" :disabled="!!busy" @click="editing = false">取消</button>
      </div>

      <div v-if="commands.length" class="cmd-grid">
        <button
          v-for="c in commands"
          :key="c.id"
          class="cmd-run"
          :disabled="!props.connected || !!busy"
          :title="c.command"
          @click="run(c)"
          @contextmenu.prevent="openMenu($event, c)"
        >
          {{ busy === `run-${c.id}` ? '…' : c.name }}
        </button>
      </div>
    </div>
    <ContextMenu
      v-if="menu"
      :x="menu.x"
      :y="menu.y"
      :items="menuItems"
      @pick="onMenu"
      @close="menu = null"
    />
  </section>
</template>

<style scoped>
.command-card {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  margin: 0;
  padding: 8px;
  overflow: hidden;
}

/* 工具行 + 按钮整块锁死高度：flex-shrink 为 0，终端再长也挤不动它。 */
.command-footer {
  flex: 0 0 auto;
  min-height: 0;
}

/* 工具行和终端之间划一条细线：它是终端的附属区，不是和终端平起平坐的一块。 */
.command-toolbar {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-top: 6px;
  padding-top: 8px;
  border-top: 1px solid var(--border);
}

/* 幽灵按钮：平时只有字，悬停才浮出底色。这一行是顺手工具，不该用实心按钮抢眼。 */
.tool-btn {
  min-height: 24px;
  padding: 0 9px;
  border-color: transparent;
  background: transparent;
  color: var(--text-dim);
  font-size: 12px;
}

.tool-btn:hover:not(:disabled) {
  border-color: var(--border);
  background: var(--bg);
  color: var(--text);
}

.add-command {
  color: var(--accent);
  font-weight: 600;
}

.add-command:hover:not(:disabled) {
  border-color: var(--accent);
  background: var(--accent-soft);
  color: var(--accent);
}

.tool-sep {
  flex: 0 0 auto;
  width: 1px;
  height: 14px;
  margin: 0 3px;
  background: var(--border);
}

/* 状态条限宽靠右：导出路径这类长消息截断显示，完整内容挂 title，
   不许把整行撑成一条大横幅。 */
.status {
  flex: 0 1 auto;
  max-width: 45%;
  min-width: 0;
  height: 24px;
  margin-left: auto;
  padding: 0 8px;
  border-radius: 5px;
  overflow: hidden;
  font-size: 11px;
  line-height: 24px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.status.ok {
  background: var(--ok-soft);
  color: var(--ok);
}

.status.err {
  background: var(--err-soft);
  color: var(--err);
}

.status.info {
  background: var(--accent-soft);
  color: var(--accent);
}

.editor-row {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-top: 6px;
}

.editor-row input {
  padding: 4px 6px;
  font-size: 12px;
}

.editor-name {
  flex: 0 1 7rem;
  min-width: 5rem;
}

.editor-command {
  flex: 1 1 10rem;
  min-width: 6rem;
  font-family: ui-monospace, Consolas, monospace;
}

.editor-action {
  flex: 0 0 auto;
  min-height: 26px;
  padding: 0 8px;
  font-size: 12px;
}

.cmd-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  max-height: 4.6rem;
  margin-top: 6px;
  overflow: auto;
}

.cmd-run {
  min-width: 0;
  min-height: 26px;
  padding: 0 8px;
  border-color: #93b4f8;
  background: #2563eb;
  color: #fff;
  font-size: 12px;
}

.cmd-run:hover:not(:disabled) {
  border-color: #1d4ed8;
  background: #1d4ed8;
  color: #fff;
}

.cmd-run:disabled {
  border-color: #c5d4f5;
  background: #9bb6ee;
  color: #eef3fc;
}

.terminal-panel {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
  border: 1px solid #303640;
  border-radius: 7px;
  background: #15181d;
}

.terminal-head {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 6px;
  min-height: 28px;
  padding: 3px 6px 3px 10px;
  border-bottom: 1px solid #303640;
  background: #20242b;
}

.terminal-title {
  font-size: 12px;
  font-weight: 600;
  color: #e5e7eb;
}

.terminal-state {
  margin-right: auto;
  font-size: 10px;
  color: #9ca3af;
}

.terminal-state.online {
  color: #6ee7a0;
}

.terminal-tool {
  min-height: 22px;
  padding: 0 7px;
  border-color: #424955;
  background: #2a2f38;
  color: #cbd5e1;
  font-size: 10px;
}

.terminal-tool:hover:not(:disabled) {
  border-color: #64748b;
  color: #fff;
}

/* 格子区：grid 的第 2 列/第 2 行是 6px 分隔条轨道，比例由 colRatio/rowRatio 决定。 */
.pane-area {
  display: grid;
  flex: 1 1 auto;
  min-height: 0;
  padding: 4px;
}

/* 拖分隔条时禁止选中：鼠标会扫过终端文本，不拦的话拖完多出一段选区。 */
.pane-area.dragging {
  user-select: none;
}

.divider {
  z-index: 2;
  border-radius: 3px;
  background: #303640;
}

.divider-v {
  cursor: col-resize;
}

.divider-h {
  cursor: row-resize;
}

.divider:hover,
.pane-area.dragging .divider {
  background: #3b82f6;
}
</style>

<script lang="ts" setup>
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import {
  CloseTerminal,
  ExportCommands,
  ImportCommands,
  ListCommands,
  ReadTerminal,
  RunCommandInTerminal,
  SaveCommands,
  StartTerminal,
  WriteTerminal,
} from '../../../wailsjs/go/board/Service'
import type { board } from '../../../wailsjs/go/models'
import ContextMenu, { type MenuItem } from './ContextMenu.vue'

const props = defineProps<{ connected: boolean }>()
const emit = defineEmits<{
  (e: 'refresh-status'): void
}>()

const commands = ref<board.Command[]>([])
const listWarning = ref('')
const busy = ref('')
// title 是悬停提示：导出导入的结果文案只显示文件名，完整路径挂在 title 上。
const banner = ref<{ kind: 'ok' | 'err' | 'info'; text: string; title?: string } | null>(null)
const terminalReady = ref(false)
const terminalOutput = ref('')
const terminalScreen = ref<HTMLElement | null>(null)
const terminalCapture = ref<HTMLTextAreaElement | null>(null)
let terminalTimer: number | undefined
let terminalReading = false
let terminalStart: Promise<void> | null = null
let writeChain = Promise.resolve()
let composing = false
let skipNextKey = false
let escapeHold = ''

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
  terminalTimer = window.setInterval(() => {
    void pullTerminal()
  }, 150)
})

onUnmounted(() => {
  if (terminalTimer !== undefined) window.clearInterval(terminalTimer)
  void CloseTerminal()
})

watch(
  () => props.connected,
  (connected) => {
    if (connected) {
      void ensureTerminal().catch((e) => {
        banner.value = { kind: 'err', text: String(e) }
      })
    } else {
      terminalReady.value = false
      escapeHold = ''
    }
  },
  { immediate: true },
)

watch(terminalReady, (ready) => {
  if (ready) {
    void nextTick(() => terminalCapture.value?.focus())
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
    await ensureTerminal()
    await RunCommandInTerminal(c.id)
    await pullTerminal()
    terminalCapture.value?.focus()
  })
}

function ensureTerminal(): Promise<void> {
  if (terminalReady.value) return Promise.resolve()
  if (terminalStart) return terminalStart

  terminalStart = (async () => {
    if (!props.connected) throw new Error('尚未连接主板')
    await StartTerminal()
    terminalReady.value = true
    await pullTerminal()
  })().finally(() => {
    terminalStart = null
  })
  return terminalStart
}

async function pullTerminal() {
  if (!terminalReady.value || terminalReading) return
  terminalReading = true
  try {
    const chunk = await ReadTerminal()
    if (!chunk) return
    appendTerminal(chunk)
    if (chunk.includes('[终端已关闭')) {
      terminalReady.value = false
    }
    await nextTick()
    if (terminalScreen.value) {
      terminalScreen.value.scrollTop = terminalScreen.value.scrollHeight
    }
  } catch (e) {
    terminalReady.value = false
    banner.value = { kind: 'err', text: `读取终端失败：${String(e)}` }
    emit('refresh-status')
  } finally {
    terminalReading = false
  }
}

function appendTerminal(chunk: string) {
  const raw = holdIncompleteEscape(escapeHold + chunk)
  const clean = sanitizeTerminal(raw).replace(/\r\n/g, '\n')
  let s = terminalOutput.value
  for (const ch of clean) {
    if (ch === '\b' || ch === '\x7f') {
      if (s.length && !s.endsWith('\n')) s = s.slice(0, -1)
    } else if (ch === '\r') {
      const i = s.lastIndexOf('\n')
      s = i >= 0 ? s.slice(0, i + 1) : ''
    } else {
      s += ch
    }
  }
  terminalOutput.value = s.slice(-200_000)
}

// Tab 补全常夹着响铃、半截 CSI、8 位 C1。漏掉就会在 cd /opt/ 后面冒出一个问号方块。
function sanitizeTerminal(s: string) {
  return s
    .replace(/\u001B\][^\u0007\u001B]*(?:\u0007|\u001B\\)/g, '')
    .replace(/\u001B\[[0-?]*[ -/]*[@-~]/g, '')
    .replace(/\u001B[@-Z\\-_]/g, '')
    .replace(/[\u0000-\u0007\u000B\u000C\u000E-\u001A\u001C-\u001F\u0080-\u009F\uFFFD]/g, '')
}

function holdIncompleteEscape(s: string) {
  const esc = s.lastIndexOf('\u001B')
  if (esc < 0) {
    escapeHold = ''
    return s
  }
  const tail = s.slice(esc)
  if (/^\u001B(?:$|\[(?:[0-?]*[ -/]*)?|\][^\u0007\u001B]*)$/.test(tail)) {
    escapeHold = tail
    return s.slice(0, esc)
  }
  escapeHold = ''
  return s
}

function sendKeys(text: string) {
  if (!props.connected || !text) return
  writeChain = writeChain
    .then(async () => {
      await ensureTerminal()
      await WriteTerminal(text)
    })
    .catch((e) => {
      banner.value = { kind: 'err', text: String(e) }
      emit('refresh-status')
    })
}

const specialKeys: Record<string, string> = {
  Enter: '\n',
  Backspace: '\x7f',
  Tab: '\t',
  Escape: '\x1b',
  Delete: '\x1b[3~',
  ArrowUp: '\x1b[A',
  ArrowDown: '\x1b[B',
  ArrowRight: '\x1b[C',
  ArrowLeft: '\x1b[D',
  Home: '\x1b[H',
  End: '\x1b[F',
}

function focusCapture() {
  if (props.connected) terminalCapture.value?.focus()
}

function onTerminalKey(e: KeyboardEvent) {
  if (!props.connected) return
  if (e.isComposing || composing) return
  if (skipNextKey && e.key.length === 1) {
    skipNextKey = false
    e.preventDefault()
    return
  }
  skipNextKey = false

  if (e.ctrlKey || e.metaKey) {
    const k = e.key.toLowerCase()
    if (k === 'c') {
      const sel = window.getSelection()?.toString()
      if (sel) {
        e.preventDefault()
        void navigator.clipboard.writeText(sel)
        return
      }
      e.preventDefault()
      sendKeys('\u0003')
      return
    }
    if (k === 'v') {
      e.preventDefault()
      void navigator.clipboard.readText().then((t) => sendKeys(t))
      return
    }
    return
  }
  if (e.altKey) return

  if (specialKeys[e.key]) {
    e.preventDefault()
    sendKeys(specialKeys[e.key])
    return
  }
  if (e.key.length === 1) {
    e.preventDefault()
    sendKeys(e.key)
  }
}

function onBeforeInput(e: InputEvent) {
  if (e.inputType === 'insertFromPaste' || e.inputType === 'insertFromDrop') {
    e.preventDefault()
    if (e.data) sendKeys(e.data)
  }
}

function onCompositionStart() {
  composing = true
}

function onCompositionEnd(e: CompositionEvent) {
  composing = false
  if (e.data) {
    sendKeys(e.data)
    skipNextKey = true
  }
}

function sendCtrlC() {
  sendKeys('\u0003')
  terminalCapture.value?.focus()
}

function reopenTerminal() {
  return act('terminal-reopen', async () => {
    await CloseTerminal()
    terminalReady.value = false
    await ensureTerminal()
    banner.value = { kind: 'info', text: '终端已重新打开' }
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
        <span class="terminal-state" :class="{ online: terminalReady }">
          {{ terminalReady ? '已打开' : connected ? '…' : '未连接' }}
        </span>
        <button class="terminal-tool" :disabled="!connected" @click="sendCtrlC">Ctrl+C</button>
        <button class="terminal-tool" :disabled="!connected || !!busy" @click="reopenTerminal">重开</button>
        <button class="terminal-tool" :disabled="!terminalOutput" @click="terminalOutput = ''">清屏</button>
      </div>
      <div class="terminal-body" @click="focusCapture">
        <pre ref="terminalScreen" class="terminal-screen" :class="{ live: terminalReady }">{{ terminalOutput }}</pre>
        <textarea
          ref="terminalCapture"
          class="terminal-capture"
          aria-label="终端输入"
          autocomplete="off"
          spellcheck="false"
          :disabled="!connected"
          @keydown="onTerminalKey"
          @beforeinput="onBeforeInput"
          @compositionstart="onCompositionStart"
          @compositionend="onCompositionEnd"
        />
      </div>
    </div>

    <!-- 指令区压在终端下面：终端是这页的主角，按钮是顺手工具，不该一进门就占住顶部。 -->
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
        :disabled="!connected || !!busy"
        :title="c.command"
        @click="run(c)"
        @contextmenu.prevent="openMenu($event, c)"
      >
        {{ busy === `run-${c.id}` ? '…' : c.name }}
      </button>
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
}

/* 工具行和终端之间划一条细线：它是终端的附属区，不是和终端平起平坐的一块。 */
.command-toolbar {
  display: flex;
  flex: 0 0 auto;
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
  flex: 0 0 auto;
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
  flex: 0 1 auto;
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

.terminal-body {
  position: relative;
  flex: 1 1 auto;
  min-height: 0;
}

.terminal-screen {
  height: 100%;
  margin: 0;
  padding: 8px 10px;
  overflow: auto;
  background: #111318;
  color: #d8dee9;
  font-family: Consolas, "Cascadia Mono", "Courier New", monospace;
  font-size: 12px;
  line-height: 1.45;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  user-select: text;
  cursor: text;
}

.terminal-body:focus-within .terminal-screen {
  box-shadow: inset 0 0 0 1px #3b82f6;
}

.terminal-body:focus-within .terminal-screen.live::after {
  content: '█';
  color: #6ee7a0;
  animation: terminal-blink 1s step-end infinite;
}

.terminal-capture {
  position: absolute;
  left: 8px;
  bottom: 8px;
  width: 1px;
  height: 1px;
  margin: 0;
  padding: 0;
  overflow: hidden;
  border: none;
  opacity: 0;
  resize: none;
}

@keyframes terminal-blink {
  50% {
    opacity: 0;
  }
}
</style>

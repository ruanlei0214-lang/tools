<script lang="ts" setup>
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import {
  CloseTerminal,
  ExportCommands,
  ImportCommands,
  ListCommands,
  ReadTerminal,
  ResetCommands,
  RunCommandInTerminal,
  SaveCommands,
  StartTerminal,
  WriteTerminal,
} from '../../../wailsjs/go/board/Service'
import type { board } from '../../../wailsjs/go/models'

const props = defineProps<{ connected: boolean }>()
const emit = defineEmits<{ (e: 'refresh-status'): void }>()

const commands = ref<board.Command[]>([])
const filePath = ref('')
const listWarning = ref('')
const busy = ref('')
const banner = ref<{ kind: 'ok' | 'err' | 'info'; text: string } | null>(null)
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

// 编辑中的那一条。id 为空表示新增。
const draft = reactive({ id: '', name: '', command: '' })
const editing = ref(false)

const canEdit = computed(() => draft.name.trim() !== '' && draft.command.trim() !== '')

// 清单在本机文件里，读它不需要连接，所以一进来就读。
onMounted(async () => {
  try {
    const list = await ListCommands()
    commands.value = list.commands ?? []
    filePath.value = list.path
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
    banner.value = { kind: 'ok', text: `${c.name} 已发送到终端` }
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
  const clean = chunk
    .replace(/\u001B\][^\u0007]*(?:\u0007|\u001B\\)/g, '')
    .replace(/\u001B\[[0-?]*[ -/]*[@-~]/g, '')
    .replace(/\r\n/g, '\n')
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
  filePath.value = list.path
  listWarning.value = list.warning
}

function exportList() {
  return act('export', async () => {
    const path = await ExportCommands()
    if (!path) return
    banner.value = { kind: 'ok', text: `已导出到 ${path}` }
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
    banner.value = { kind: 'ok', text: `已从 ${fileName(r.path)} 导入，已经生效` }
  })
}

function resetList() {
  if (!window.confirm('把指令清单恢复成出厂默认？现场改过的这一份会被删掉。')) return
  return act('reset', async () => {
    applyList(await ResetCommands())
    editing.value = false
    banner.value = { kind: 'info', text: '已恢复出厂默认指令' }
  })
}

function fileName(p: string): string {
  const i = Math.max(p.lastIndexOf('/'), p.lastIndexOf('\\'))
  return i >= 0 ? p.slice(i + 1) : p
}
</script>

<!-- 整个面板包在一个根元素里：父组件用 v-show 切标签页，而 v-show 作用在多根组件上
     会被 Vue 丢掉，两个面板就会同时显示。 -->
<template>
  <div>
    <section class="card command-card">
      <div class="command-toolbar">
        <button class="add-command" :disabled="!!busy" @click="startAdd">
          <span aria-hidden="true">＋</span> 添加
        </button>
        <button :disabled="!!busy" title="把当前清单存成 JSON 文件" @click="exportList">
          {{ busy === 'export' ? '导出中…' : '导出' }}
        </button>
        <button :disabled="!!busy" title="从 JSON 文件替换当前清单" @click="importList">
          {{ busy === 'import' ? '导入中…' : '导入' }}
        </button>
        <button :disabled="!!busy" title="删掉现场清单，退回出厂默认" @click="resetList">
          {{ busy === 'reset' ? '恢复中…' : '恢复默认' }}
        </button>
        <div class="status" :class="banner?.kind" :title="banner?.text">{{ banner?.text }}</div>
      </div>

      <div v-if="listWarning" class="banner err warn">{{ listWarning }}</div>

      <div v-if="editing" class="editor-row">
        <span class="editor-mode">{{ draft.id ? '编辑' : '新增' }}</span>
        <input
          v-model="draft.name"
          class="editor-name"
          aria-label="按钮名称"
          placeholder="按钮名称"
          autofocus
          :disabled="!!busy"
          @keyup.enter="canEdit && save()"
          @keyup.esc="editing = false"
        />
        <input
          v-model="draft.command"
          class="editor-command"
          aria-label="执行命令"
          placeholder="命令，例如 /opt/autorun.sh restart"
          :disabled="!!busy"
          @keyup.enter="canEdit && save()"
          @keyup.esc="editing = false"
        />
        <button class="primary editor-action" :disabled="!!busy || !canEdit" @click="save">
          {{ busy === 'save' ? '保存中…' : '保存' }}
        </button>
        <button class="editor-action" :disabled="!!busy" @click="editing = false">取消</button>
      </div>

      <p v-if="!commands.length" class="empty command-empty">还没有指令，点「添加」或「导入」。</p>
      <div v-else class="cmd-grid">
        <div v-for="c in commands" :key="c.id" class="cmd">
          <button
            class="cmd-run"
            :disabled="!connected || !!busy"
            :title="connected ? c.command : '尚未连接主板'"
            @click="run(c)"
          >
            {{ busy === `run-${c.id}` ? '执行中…' : c.name }}
          </button>
          <button
            class="cmd-icon"
            :disabled="!!busy"
            :aria-label="`编辑 ${c.name}`"
            title="编辑"
            @click="startEdit(c)"
          >
            <svg viewBox="0 0 24 24" aria-hidden="true">
              <path d="m4 20 4.5-1 10-10-3.5-3.5-10 10L4 20Z" />
              <path d="m13.5 7 3.5 3.5" />
            </svg>
          </button>
          <button
            class="cmd-icon cmd-delete"
            :disabled="!!busy"
            :aria-label="`删除 ${c.name}`"
            title="删除"
            @click="remove(c)"
          >
            <svg viewBox="0 0 24 24" aria-hidden="true">
              <path d="M4 7h16M9 7V4h6v3m3 0-1 13H7L6 7m4 4v5m4-5v5" />
            </svg>
          </button>
        </div>
      </div>

      <div class="command-foot">
        <span>编辑清单不需要连接，执行时才需要。</span>
        <span v-if="filePath" class="config-file" :title="filePath">清单保存在本机</span>
      </div>

      <div class="terminal-panel">
        <div class="terminal-head">
          <span class="terminal-title">执行终端</span>
          <span class="terminal-state" :class="{ online: terminalReady }">
            {{ terminalReady ? '已打开' : connected ? '正在打开…' : '未连接' }}
          </span>
          <button
            class="terminal-tool"
            :disabled="!connected"
            title="向终端发送 Ctrl+C，停止当前命令"
            @click="sendCtrlC"
          >
            Ctrl+C
          </button>
          <button class="terminal-tool" :disabled="!connected || !!busy" @click="reopenTerminal">重开</button>
          <button class="terminal-tool" :disabled="!terminalOutput" @click="terminalOutput = ''">清屏</button>
        </div>
        <div class="terminal-body" @click="focusCapture">
          <pre ref="terminalScreen" class="terminal-screen" :class="{ live: terminalReady }">{{
            terminalOutput ||
            (connected ? '点这里后直接输入…' : '连接主板后可使用终端')
          }}</pre>
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
    </section>
  </div>
</template>

<style scoped>
.command-card {
  padding: 10px 12px;
}

.command-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 8px;
}

.add-command {
  flex: 0 0 auto;
  padding: 5px 9px;
  color: var(--accent);
}

/* 操作结果放进工具行，不额外占一整条。 */
.status {
  flex: 1 1 auto;
  min-width: 0;
  height: 28px;
  margin-left: auto;
  padding: 0 10px;
  border-radius: 6px;
  overflow: hidden;
  font-size: 12px;
  line-height: 28px;
  text-overflow: ellipsis;
  white-space: nowrap;
  user-select: text;
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

.warn {
  margin-bottom: 8px;
  padding: 6px 8px;
}

.editor-row {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
  padding: 7px;
  border: 1px solid var(--accent);
  border-radius: 7px;
  background: var(--accent-soft);
}

.editor-row input {
  padding: 6px 8px;
}

.editor-mode {
  flex: 0 0 auto;
  font-size: 11px;
  font-weight: 600;
  color: var(--accent);
}

.editor-name {
  flex: 0 1 10rem;
  min-width: 7rem;
}

.editor-command {
  flex: 1 1 18rem;
  min-width: 10rem;
  font-family: ui-monospace, Consolas, monospace;
}

.editor-action {
  flex: 0 0 auto;
  padding: 6px 10px;
}

.command-empty {
  margin: 10px 0;
}

.cmd-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  gap: 6px;
}

.cmd {
  display: flex;
  gap: 3px;
  min-width: 0;
}

.cmd-run {
  flex: 1 1 auto;
  min-width: 0;
  padding: 7px 10px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.cmd-icon {
  flex: 0 0 auto;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 31px;
  padding: 0;
  color: var(--text-dim);
}

.cmd-icon svg {
  width: 14px;
  height: 14px;
  fill: none;
  stroke: currentColor;
  stroke-width: 1.8;
  stroke-linecap: round;
  stroke-linejoin: round;
}

.cmd-icon:hover:not(:disabled) {
  border-color: var(--accent);
  color: var(--accent);
}

.cmd-delete:hover:not(:disabled) {
  border-color: var(--err);
  color: var(--err);
}

.command-foot {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  margin-top: 8px;
  padding-top: 7px;
  border-top: 1px solid var(--border);
  font-size: 11px;
  color: var(--text-dim);
}

.config-file {
  flex: 0 0 auto;
  cursor: help;
  text-decoration: underline dotted;
  text-underline-offset: 2px;
}

.terminal-panel {
  margin-top: 9px;
  overflow: hidden;
  border: 1px solid #303640;
  border-radius: 7px;
  background: #15181d;
}

.terminal-head {
  display: flex;
  align-items: center;
  gap: 6px;
  min-height: 31px;
  padding: 4px 6px 4px 10px;
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
  min-height: 23px;
  padding: 2px 7px;
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
}

.terminal-screen {
  height: 248px;
  margin: 0;
  padding: 9px 11px;
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

@media (max-width: 720px) {
  .editor-row {
    flex-wrap: wrap;
  }

  .editor-name,
  .editor-command {
    flex: 1 1 100%;
  }
}
</style>

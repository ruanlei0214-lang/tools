<script lang="ts" setup>
import { nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import {
  CloseTerminal,
  ReadTerminal,
  StartTerminal,
  WriteTerminal,
} from '../../../wailsjs/go/board/Service'
import { useActivePolling } from '../../shell/polling'
import { appendAnsi, clearTermBuf, newTermBuf, renderAnsi } from './terminalAnsi'

// 一个分屏格子对应设备上一个独立的 shell 会话（按 sessionId 区分）。
// 输出渲染、按键捕获、划词保持都收在这里；摆格子、分屏、分隔条归 CommandPanel。
const props = defineProps<{
  sessionId: string
  connected: boolean
  active: boolean
  closable: boolean
}>()

const emit = defineEmits<{
  (e: 'activate'): void
  (e: 'close'): void
  (e: 'error', text: string): void
  (e: 'refresh-status'): void
}>()

const FONT_MIN = 9
const FONT_MAX = 28
const FONT_DEFAULT = 12

const terminalReady = ref(false)
const terminalHtml = ref('')
const termBuf = newTermBuf()
const fontSize = ref(FONT_DEFAULT)
const sizeHint = ref('')
const terminalScreen = ref<HTMLElement | null>(null)
const terminalCapture = ref<HTMLTextAreaElement | null>(null)
const paneRoot = ref<HTMLElement | null>(null)
let terminalReading = false
let terminalStart: Promise<void> | null = null
let writeChain = Promise.resolve()
let composing = false
let skipNextKey = false
let escapeHold = ''
// 选中文字时不能改 pre 的内容，否则选择会被冲掉。输出先攒着，松开再贴上去。
let heldOutput = ''
let selecting = false

// 切到别的模块时轮询暂停；这期间设备上的输出攒在后端缓冲区（有 1MB 上限），
// 切回来立即补一轮取走。
useActivePolling(() => {
  void pullTerminal()
}, () => 150)

onMounted(() => {
  window.addEventListener('mouseup', onSelectEnd)
  window.addEventListener('keydown', onWindowKey)
  // 必须非 passive：要拦住 WebView 的 Ctrl+滚轮整页缩放，并改成只动本格字号。
  paneRoot.value?.addEventListener('wheel', onWheel, { passive: false })
})

onUnmounted(() => {
  window.removeEventListener('mouseup', onSelectEnd)
  window.removeEventListener('keydown', onWindowKey)
  paneRoot.value?.removeEventListener('wheel', onWheel)
  window.clearTimeout(hintTimer)
  // 分屏被关掉时把设备上对应的 shell 也收了，不留孤儿进程。
  void CloseTerminal(props.sessionId)
})

watch(
  () => props.connected,
  (connected) => {
    if (connected) {
      void ensure().catch((e) => emit('error', String(e)))
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

function fail(text: string) {
  emit('error', text)
  emit('refresh-status')
}

function ensure(): Promise<void> {
  if (terminalReady.value) return Promise.resolve()
  if (terminalStart) return terminalStart

  terminalStart = (async () => {
    if (!props.connected) throw new Error('尚未连接主板')
    await StartTerminal(props.sessionId)
    terminalReady.value = true
    await pullTerminal()
  })().finally(() => {
    terminalStart = null
  })
  return terminalStart
}

function terminalHasSelection() {
  const el = terminalScreen.value
  const sel = window.getSelection()
  if (!el || !sel || sel.isCollapsed || !sel.rangeCount) return false
  const node = sel.anchorNode
  return !!(node && el.contains(node))
}

function shouldHoldOutput() {
  return selecting || terminalHasSelection()
}

function onSelectStart() {
  selecting = true
}

function onSelectEnd() {
  selecting = false
}

async function pullTerminal() {
  if (!terminalReady.value || terminalReading) return
  terminalReading = true
  try {
    const chunk = await ReadTerminal(props.sessionId)
    if (shouldHoldOutput()) {
      if (chunk) heldOutput += chunk
      return
    }
    const pending = heldOutput + (chunk || '')
    heldOutput = ''
    if (!pending) return
    appendTerminal(pending)
    if (pending.includes('[终端已关闭')) {
      terminalReady.value = false
    }
    await nextTick()
    if (terminalScreen.value) {
      terminalScreen.value.scrollTop = terminalScreen.value.scrollHeight
    }
  } catch (e) {
    terminalReady.value = false
    fail(`读取终端失败：${String(e)}`)
  } finally {
    terminalReading = false
  }
}

function appendTerminal(chunk: string) {
  const raw = holdIncompleteEscape(escapeHold + chunk)
  appendAnsi(termBuf, raw)
  terminalHtml.value = renderAnsi(termBuf)
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
      await ensure()
      await WriteTerminal(props.sessionId, text)
    })
    .catch((e) => {
      fail(String(e))
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

let hintTimer = 0

function showSizeHint() {
  sizeHint.value = `${fontSize.value}px`
  window.clearTimeout(hintTimer)
  hintTimer = window.setTimeout(() => {
    sizeHint.value = ''
  }, 800)
}

function zoomBy(step: number) {
  const el = terminalScreen.value
  const atBottom = !el || el.scrollHeight - el.scrollTop - el.clientHeight < 16
  const next = Math.min(FONT_MAX, Math.max(FONT_MIN, fontSize.value + step))
  if (next === fontSize.value) return
  fontSize.value = next
  showSizeHint()
  if (atBottom) {
    void nextTick(() => {
      if (el) el.scrollTop = el.scrollHeight
    })
  }
}

function resetFont() {
  if (fontSize.value === FONT_DEFAULT) return
  const el = terminalScreen.value
  const atBottom = !el || el.scrollHeight - el.scrollTop - el.clientHeight < 16
  fontSize.value = FONT_DEFAULT
  showSizeHint()
  if (atBottom) {
    void nextTick(() => {
      if (el) el.scrollTop = el.scrollHeight
    })
  }
}

// 滚轮改本格字号。按住 Shift 仍翻历史，避免长输出没法看。
function onWheel(e: WheelEvent) {
  if (e.shiftKey) {
    const el = terminalScreen.value
    if (el) {
      el.scrollTop += e.deltaY
      e.preventDefault()
    }
    return
  }
  e.preventDefault()
  if (e.deltaY === 0) return
  zoomBy(e.deltaY > 0 ? -1 : 1)
}

function focusCapture() {
  // 点一下是为了接着打字；已经划了字就别抢焦点，否则选区会被清掉。
  if (selecting || terminalHasSelection()) return
  if (props.connected) terminalCapture.value?.focus()
}

function onWindowKey(e: KeyboardEvent) {
  if (!terminalHasSelection()) return
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === 'c') {
    const sel = window.getSelection()?.toString()
    if (!sel) return
    e.preventDefault()
    void navigator.clipboard.writeText(sel)
  }
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
    if (k === '=' || k === '+') {
      e.preventDefault()
      zoomBy(1)
      return
    }
    if (k === '-' || k === '_') {
      e.preventDefault()
      zoomBy(-1)
      return
    }
    if (k === '0') {
      e.preventDefault()
      resetFont()
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

async function reopen() {
  await CloseTerminal(props.sessionId)
  terminalReady.value = false
  await ensure()
}

defineExpose({
  ready: terminalReady,
  ensure,
  reopen,
  send: sendKeys,
  clear: () => {
    clearTermBuf(termBuf)
    terminalHtml.value = ''
  },
  focus: focusCapture,
})
</script>

<template>
  <div
    ref="paneRoot"
    class="pane-root"
    :class="{ active: active && closable }"
    @mousedown="emit('activate')"
  >
    <div class="terminal-body" @mousedown="onSelectStart" @click="focusCapture">
      <pre
        ref="terminalScreen"
        class="terminal-screen"
        :class="{ live: terminalReady }"
        :style="{ fontSize: `${fontSize}px` }"
        title="滚轮放大缩小文字，Shift+滚轮翻历史"
        v-html="terminalHtml"
      />
      <div v-if="sizeHint" class="size-hint">{{ sizeHint }}</div>
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
      <button
        v-if="closable"
        class="pane-close"
        title="关闭这个终端"
        @mousedown.stop
        @click.stop="emit('close')"
      >
        ×
      </button>
    </div>
  </div>
</template>

<style scoped>
.pane-root {
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  border: 1px solid transparent;
  border-radius: 4px;
}

/* 多屏时给「当前终端」一个常亮边：头部按钮（Ctrl+C、重开、清屏）和指令按钮
   都作用在它身上，得看得出来是谁。 */
.pane-root.active {
  border-color: #3b82f6;
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
  border-radius: 3px;
  background: #111318;
  color: #d8dee9;
  font-family: Consolas, "Cascadia Mono", "Courier New", monospace;
  line-height: 1.45;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  user-select: text;
  cursor: text;
}

.terminal-screen :deep(span) {
  white-space: inherit;
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

.size-hint {
  position: absolute;
  top: 8px;
  left: 50%;
  z-index: 4;
  transform: translateX(-50%);
  padding: 2px 8px;
  border-radius: 4px;
  background: rgba(15, 23, 42, 0.82);
  color: #e5e7eb;
  font-size: 12px;
  pointer-events: none;
}

/* 关闭钮平时藏起来，悬停格子才浮出来：终端里每个像素都是输出区域。 */
.pane-close {
  position: absolute;
  top: 4px;
  right: 6px;
  z-index: 3;
  min-height: 18px;
  padding: 0 6px;
  border-color: #424955;
  background: rgba(42, 47, 56, 0.92);
  color: #cbd5e1;
  font-size: 11px;
  line-height: 1;
  opacity: 0;
  transition: opacity 0.12s;
}

.pane-root:hover .pane-close {
  opacity: 1;
}

.pane-close:hover {
  border-color: #64748b;
  color: #fff;
}

@keyframes terminal-blink {
  50% {
    opacity: 0;
  }
}
</style>

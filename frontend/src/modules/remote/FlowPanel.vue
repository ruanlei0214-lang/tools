<script lang="ts" setup>
import { computed, ref } from 'vue'
import {
  ExportPanel,
  ImportPanel,
  ResetPanel,
  RunFlowStep,
  SavePanel,
} from '../../../wailsjs/go/remote/Service'
import type { remote } from '../../../wailsjs/go/models'

const props = defineProps<{
  tab: remote.Tab
  connected: boolean
  configDir: string
  points: remote.Point[]
}>()
const emit = defineEmits<{
  (e: 'refresh-status'): void
  (e: 'config-updated', cfg: remote.Settings): void
}>()

const busy = ref('')
const banner = ref<{ kind: 'ok' | 'err' | 'info'; text: string } | null>(null)
const editing = ref(false)
const draft = ref<remote.FlowStep[]>([])
const cursor = ref(0)
const running = ref(false)
const moreOpen = ref(false)
const pickerOpen = ref(false)
const addAction = ref('pulse')
let stopRun = false

const steps = computed(() => (editing.value ? draft.value : (props.tab.steps ?? [])))

// AI 只读。名称和类型从左侧点位带过来，点一下就加入，不手填。
const selectablePoints = computed(() => (props.points ?? []).filter((p) => p.type !== 'AI'))

function pointKey(p: { type: string; port: number }) {
  return `${p.type}:${p.port}`
}

function cloneSteps(): remote.FlowStep[] {
  return (props.tab.steps ?? []).map((s) => ({ ...s }))
}

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

function startEdit() {
  draft.value = cloneSteps()
  editing.value = true
  pickerOpen.value = false
  moreOpen.value = false
  banner.value = null
}

function cancelEdit() {
  editing.value = false
  draft.value = []
  pickerOpen.value = false
  moreOpen.value = false
}

function togglePicker() {
  pickerOpen.value = !pickerOpen.value
  addAction.value = 'pulse'
}

function addPoint(p: remote.Point) {
  const analog = p.type === 'AO'
  const action = analog ? 'set' : addAction.value
  draft.value.push({
    label: p.label,
    type: p.type,
    port: p.port,
    action,
    value: analog ? p.value || '0' : '',
    onValue: p.onValue,
    offValue: p.offValue,
    pulseMs: analog || action !== 'pulse' ? 0 : p.pulseMs || 500,
    delayMs: 1000,
    hint: p.hint || '',
  } as remote.FlowStep)
}

function removeStep(i: number) {
  draft.value.splice(i, 1)
}

function moveStep(i: number, dir: -1 | 1) {
  const j = i + dir
  if (j < 0 || j >= draft.value.length) return
  const row = draft.value.splice(i, 1)[0]
  draft.value.splice(j, 0, row)
}

function onActionChange(s: remote.FlowStep) {
  if (s.action === 'pulse' && !s.pulseMs) {
    const p = selectablePoints.value.find((x) => x.type === s.type && x.port === s.port)
    s.pulseMs = p?.pulseMs || 500
  }
}

function saveFlow() {
  moreOpen.value = false
  return act('save', async () => {
    emit(
      'config-updated',
      await SavePanel({
        id: props.tab.id,
        title: props.tab.title,
        kind: props.tab.kind,
        description: props.tab.description,
        groups: [],
        steps: draft.value,
      } as unknown as remote.Tab),
    )
    editing.value = false
    draft.value = []
    pickerOpen.value = false
    if (cursor.value >= (props.tab.steps?.length ?? 0)) cursor.value = 0
    banner.value = { kind: 'ok', text: '已保存' }
  })
}

function resetFlow() {
  moreOpen.value = false
  if (!window.confirm('把测试流程恢复成出厂默认？现场改过的这一份会被删掉。')) return
  return act('reset', async () => {
    emit('config-updated', await ResetPanel(props.tab.kind))
    editing.value = false
    draft.value = []
    cursor.value = 0
    pickerOpen.value = false
    banner.value = { kind: 'info', text: '已恢复默认' }
  })
}

function exportFlow() {
  moreOpen.value = false
  return act('export', async () => {
    const path = await ExportPanel(props.tab.kind)
    if (!path) return
    banner.value = { kind: 'ok', text: `已导出 ${fileName(path)}` }
  })
}

function importFlow() {
  moreOpen.value = false
  const msg = editing.value
    ? '导入会替换当前测试流程，未保存的编辑会丢掉。继续？'
    : '导入会替换当前测试流程，继续？'
  if (!window.confirm(msg)) return
  return act('import', async () => {
    const r = await ImportPanel(props.tab.kind)
    if (r.canceled) return
    editing.value = false
    draft.value = []
    cursor.value = 0
    pickerOpen.value = false
    emit('config-updated', r.settings)
    banner.value = { kind: 'ok', text: `已导入 ${fileName(r.path)}` }
  })
}

function fileName(p: string): string {
  const i = Math.max(p.lastIndexOf('/'), p.lastIndexOf('\\'))
  return i >= 0 ? p.slice(i + 1) : p
}

function jumpTo(i: number) {
  if (editing.value || running.value) return
  cursor.value = i
  banner.value = null
}

async function stepOnce() {
  const list = props.tab.steps ?? []
  if (!list.length) {
    banner.value = { kind: 'err', text: '还没有步骤' }
    return
  }
  if (cursor.value >= list.length) cursor.value = 0
  const i = cursor.value
  const s = list[i]
  await RunFlowStep(i)
  cursor.value = i + 1
  banner.value = { kind: 'ok', text: s.label }
}

function singleStep() {
  return act('step', stepOnce)
}

async function runAll() {
  const list = props.tab.steps ?? []
  if (!list.length) {
    banner.value = { kind: 'err', text: '还没有步骤' }
    return
  }
  stopRun = false
  running.value = true
  busy.value = 'run'
  try {
    for (let i = cursor.value; i < list.length; i++) {
      if (stopRun) {
        banner.value = { kind: 'info', text: '已停止' }
        return
      }
      cursor.value = i
      await RunFlowStep(i)
      cursor.value = i + 1
      const wait = list[i].delayMs ?? 0
      banner.value = { kind: 'ok', text: list[i].label }
      if (wait > 0 && i < list.length - 1 && !stopRun) {
        await sleep(wait)
      }
    }
    banner.value = { kind: 'ok', text: '跑完了' }
  } catch (e) {
    banner.value = { kind: 'err', text: String(e) }
    emit('refresh-status')
  } finally {
    running.value = false
    busy.value = ''
    stopRun = false
  }
}

function stop() {
  stopRun = true
}

function sleep(ms: number) {
  return new Promise<void>((resolve) => window.setTimeout(resolve, ms))
}

function actionLabel(a: string) {
  switch (a) {
    case 'on':
      return 'ON'
    case 'off':
      return 'OFF'
    case 'set':
      return '下发'
    default:
      return ''
  }
}

function stepTitle(s: remote.FlowStep) {
  const bits = [`${s.type}${s.port}`, actionLabel(s.action) || '点动']
  if (s.action === 'pulse' && s.pulseMs) bits.push(`${s.pulseMs}ms`)
  if (s.action === 'set' && s.value) bits.push(s.value)
  if (s.delayMs) bits.push(`间隔 ${s.delayMs}ms`)
  if (s.hint) bits.push(s.hint)
  return bits.join(' · ')
}
</script>

<template>
  <aside class="flow-col">
    <div class="flow-bar">
      <h2
        class="flow-title"
        :title="configDir ? `${configDir}\n和 exe 在同一目录，整夹拷走会一起带走。` : undefined"
      >
        {{ tab.title || '测试流程' }}
      </h2>
      <template v-if="!editing">
        <button class="primary" :disabled="!connected || !!busy || !steps.length" @click="singleStep">
          单步
        </button>
        <button
          v-if="!running"
          :disabled="!connected || !!busy || !steps.length"
          @click="runAll"
        >
          连续
        </button>
        <button v-else @click="stop">停止</button>
        <button :disabled="!!busy" title="增删步骤" @click="startEdit">编辑</button>
      </template>
      <template v-else>
        <button :disabled="!!busy" :class="{ primary: pickerOpen }" title="点选点位加入" @click="togglePicker">
          ＋
        </button>
        <button class="primary" :disabled="!!busy" @click="saveFlow">保存</button>
        <button :disabled="!!busy" @click="cancelEdit">取消</button>
      </template>
      <div class="flow-more">
        <button :disabled="!!busy || running" title="更多" @click="moreOpen = !moreOpen">⋯</button>
        <div v-if="moreOpen" class="flow-more-menu">
          <button :disabled="!!busy" @click="exportFlow">导出</button>
          <button :disabled="!!busy" @click="importFlow">导入</button>
          <button :disabled="!!busy" @click="resetFlow">恢复默认</button>
        </div>
      </div>
    </div>

    <div v-if="banner" class="status" :class="banner.kind" :title="banner.text">{{ banner.text }}</div>

    <div v-if="pickerOpen" class="flow-picker">
      <div class="flow-picker-bar">
        <select v-model="addAction" aria-label="动作" title="多选时动作必须相同，点一下加入">
          <option value="pulse">点动</option>
          <option value="on">ON</option>
          <option value="off">OFF</option>
        </select>
        <span class="flow-hint">点名称加入，模拟量自动下发</span>
      </div>
      <p v-if="!selectablePoints.length" class="empty">左侧还没有可写点位</p>
      <button
        v-for="p in selectablePoints"
        :key="pointKey(p)"
        class="flow-pick"
        type="button"
        :title="`${p.type}${p.port}`"
        @click="addPoint(p)"
      >
        <span class="flow-pick-name">{{ p.label }}</span>
        <span class="flow-pick-type">{{ p.type }}{{ p.port }}</span>
      </button>
    </div>

    <div class="flow-list">
      <p v-if="!steps.length" class="empty">点「编辑」再「＋」选点位</p>
      <div
        v-for="(s, i) in steps"
        :key="i"
        class="flow-row"
        :class="{ current: !editing && i === cursor, edit: editing }"
        :title="stepTitle(s)"
        :role="editing ? undefined : 'button'"
        :tabindex="editing ? undefined : 0"
        @click="jumpTo(i)"
      >
        <span class="flow-idx">{{ i + 1 }}</span>
        <span class="flow-name">{{ s.label }}</span>
        <template v-if="editing">
          <select
            v-if="s.type !== 'AO'"
            v-model="s.action"
            aria-label="动作"
            @click.stop
            @change="onActionChange(s)"
          >
            <option value="pulse">点动</option>
            <option value="on">ON</option>
            <option value="off">OFF</option>
          </select>
          <span v-else class="flow-act">下发</span>
          <span class="flow-ops">
            <button type="button" :disabled="i === 0" title="上移" @click.stop="moveStep(i, -1)">↑</button>
            <button type="button" :disabled="i === steps.length - 1" title="下移" @click.stop="moveStep(i, 1)">
              ↓
            </button>
            <button type="button" class="io-del" title="删除" @click.stop="removeStep(i)">✕</button>
          </span>
        </template>
        <span v-else-if="actionLabel(s.action)" class="flow-act">{{ actionLabel(s.action) }}</span>
      </div>
    </div>
  </aside>
</template>

<style scoped>
.flow-col {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}

.flow-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
}

.flow-title {
  flex: 1 1 auto;
  margin: 0;
  min-width: 0;
  overflow: hidden;
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.flow-bar button {
  min-height: 26px;
  padding: 0 7px;
  font-size: 12px;
}

.flow-more {
  position: relative;
}

.flow-more-menu {
  position: absolute;
  top: 100%;
  right: 0;
  z-index: 2;
  display: grid;
  min-width: 6.5rem;
  padding: 4px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--panel);
  box-shadow: 0 4px 12px rgb(0 0 0 / 12%);
}

.flow-more-menu button {
  justify-content: flex-start;
  width: 100%;
}

.status {
  min-width: 0;
  padding: 0 6px;
  border-radius: 4px;
  overflow: hidden;
  font-size: 11px;
  line-height: 22px;
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

.flow-picker {
  display: grid;
  gap: 2px;
  max-height: 14rem;
  overflow: auto;
  padding: 4px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--panel);
}

.flow-picker-bar {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 2px;
}

.flow-picker-bar select {
  width: 4.5rem;
  padding: 2px 4px;
  font-size: 12px;
}

.flow-hint,
.empty {
  margin: 4px;
  color: var(--text-dim);
  font-size: 11px;
}

.flow-pick,
.flow-row {
  display: grid;
  align-items: center;
  gap: 6px;
  width: 100%;
  min-height: 26px;
  padding: 2px 6px;
  border: none;
  border-radius: 4px;
  background: none;
  color: inherit;
  text-align: left;
}

.flow-pick {
  grid-template-columns: 1fr auto;
}

.flow-pick:hover,
.flow-row:hover:not(:disabled) {
  background: var(--accent-soft);
}

.flow-pick-name,
.flow-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 12px;
}

.flow-pick-type,
.flow-act {
  color: var(--text-dim);
  font-size: 11px;
}

.flow-list {
  display: grid;
  gap: 1px;
}

.flow-row {
  grid-template-columns: 1.2rem minmax(0, 1fr) auto;
}

.flow-row.edit {
  grid-template-columns: 1.2rem minmax(0, 1fr) 3.6rem auto;
}

.flow-row.current {
  background: var(--accent-soft);
  outline: 1px solid var(--accent);
}

.flow-idx {
  color: var(--text-dim);
  font-size: 11px;
}

.flow-row select {
  width: 100%;
  padding: 1px 2px;
  font-size: 11px;
}

.flow-ops {
  display: flex;
  gap: 0;
}

.flow-ops button {
  width: 20px;
  min-height: 22px;
  padding: 0;
  font-size: 11px;
}

.io-del:hover:not(:disabled) {
  border-color: var(--err);
  color: var(--err);
}
</style>

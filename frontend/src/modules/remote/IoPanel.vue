<script lang="ts" setup>
import { computed, nextTick, ref, watch } from 'vue'
import { GetIO, PulseIO, SetIO, SetIOForcedAll, ToggleIO } from '../../../wailsjs/go/remote/Service'
import type { remote } from '../../../wailsjs/go/models'
import { useActivePolling } from '../../shell/polling'
import { confirmDialog } from '../../shell/dialog'
import PointEditor from './PointEditor.vue'
import { usePanelEdit } from './usePanelEdit'

// intervalMs 是刷新间隔，父组件从配置里读出来传进来。它能在界面上被改掉，
// 所以定时器跟着它重建，见下面的 watch。
const props = defineProps<{
  tab: remote.Tab
  connected: boolean
  intervalMs: number
  configDir: string
}>()
const emit = defineEmits<{
  (e: 'refresh-status'): void
  (e: 'config-updated', cfg: remote.Settings): void
}>()

const busy = ref('')
const banner = ref<{ kind: 'ok' | 'err' | 'info'; text: string } | null>(null)
const values = ref<Record<string, number>>({})
const autoRefresh = ref(true)
// 一键强制打开后，配置里所有 DI 都能写。刷新只读当前值，不再去问强制标志。
const allForced = ref(false)

// 编辑动作与寄存器页共用，见 usePanelEdit。groups 在编辑时看草稿、平时看后端那份。
const {
  editing,
  groups,
  editingPoint,
  isEditingAt,
  start: startEdit,
  stop: cancelEdit,
  addGroup,
  removeGroup,
  addPoint,
  editPoint,
  applyPoint,
  cancelPoint,
  removePoint,
  save: saveDraft,
  reset: resetDraft,
  exportFile,
  importFile,
} = usePanelEdit(() => props.tab, 'DO')

const ioTypes = ['DI', 'DO', 'AI', 'AO']

// 刷新读的始终是后端那份清单，不是编辑草稿：草稿里的端口还没过校验，
// 拿它去读会把控制器不认的端口一起发出去。
const savedPoints = computed(() => (props.tab.groups ?? []).flatMap((g) => g.points ?? []))
const diPoints = computed(() => savedPoints.value.filter((p) => p.type === 'DI'))

const pointKey = (type: string, port: number) => `${type}:${port}`

// setpoints 是模拟量点位输入框里待下发的值。它和读回来的 values 分开存，
// 自动刷新只写 values：一秒一次的回填会把人正在输的数字冲掉。
// 这也让「填的是要写下去的值」和「行尾显示的是读回来的值」在界面上分得清。
const setpoints = ref<Record<string, string>>({})

watch(
  savedPoints,
  (points) => {
    const next: Record<string, string> = {}
    for (const p of points) {
      const k = pointKey(p.type, p.port)
      // 保留人已经填过的：改一次点位清单不该把没下发的那些数字清空。
      next[k] = setpoints.value[k] ?? p.value ?? ''
    }
    setpoints.value = next
  },
  { immediate: true },
)

// 复选框上那句话得跟着配置走，写死「每秒」的话改完间隔界面就在说谎。
const intervalText = computed(() =>
  props.intervalMs % 1000 === 0 ? `${props.intervalMs / 1000} 秒` : `${props.intervalMs} 毫秒`,
)

// 上一轮还没回来就跳过这一轮：控制器慢的时候不能让请求越堆越多。
let polling = false

// 切到别的模块时暂停刷新，切回来立即补一轮。间隔在界面上改完按新值重建。
const { restart: restartPolling } = useActivePolling(
  () => {
    if (autoRefresh.value) void refresh(true)
  },
  () => props.intervalMs,
)

watch(() => props.intervalMs, restartPolling)

// 断开时把读数清空。留着上一次的值比显示「—」更危险，现场会当成当前状态。
watch(
  () => props.connected,
  (now) => {
    if (!now) {
      values.value = {}
      allForced.value = false
    } else {
      void refresh()
    }
  },
)

function toggleAllForce() {
  return act('force-all', async () => {
    const next = !allForced.value
    await SetIOForcedAll(
      diPoints.value.map((p) => ({ type: p.type, port: p.port })),
      next,
    )
    allForced.value = next
    banner.value = {
      kind: 'ok',
      text: next ? `已强制 ${diPoints.value.length} 路 DI` : '已取消全部 DI 强制',
    }
  })
}

// 不在开头清空 banner：状态栏是固定一行的，清了只会让文字闪一下，
// 结果出来时直接覆盖掉上一条就够了。
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

// silent 是轮询用的：定时刷新失败不该把用户刚看到的操作结果冲掉，
// 也不该每秒弹一次红条。
async function refresh(silent = false) {
  if (!props.connected || savedPoints.value.length === 0) return
  if (silent && polling) return
  polling = true
  try {
    const points = savedPoints.value.map((p) => ({ type: p.type, port: p.port }))
    const rows = await GetIO(points)
    const next: Record<string, number> = {}
    for (const row of rows) next[pointKey(row.type, row.port)] = row.value
    values.value = next
  } catch (e) {
    if (!silent) {
      banner.value = { kind: 'err', text: String(e) }
    }
    emit('refresh-status')
  } finally {
    polling = false
  }
}

// 点值切换。读回当前值再写反的那个，输入输出一视同仁。
function toggle(p: remote.Point, id: string) {
  if (!writable(p)) return
  return act(id, async () => {
    const written = await ToggleIO({ type: p.type, port: p.port }, p.onValue, p.offValue)
    banner.value = { kind: 'ok', text: `${p.label}：${p.type}${p.port} = ${written}` }
    await refresh(true)
  })
}

// 手填值下发。模拟量在两个值之间翻转没什么意义，要的是写一个具体的数。
function writeValue(p: remote.Point, id: string) {
  if (!writable(p)) return
  const raw = (setpoints.value[pointKey(p.type, p.port)] ?? '').trim()
  const v = Number(raw)
  if (raw === '' || !Number.isFinite(v)) {
    banner.value = { kind: 'err', text: `${p.label}：先填一个数值` }
    return
  }
  return act(id, async () => {
    await SetIO({ type: p.type, port: p.port }, v)
    banner.value = { kind: 'ok', text: `${p.label}：${p.type}${p.port} 写入 ${v}` }
    await refresh(true)
  })
}

function pulse(p: remote.Point, id: string) {
  return act(id, async () => {
    await PulseIO({ type: p.type, port: p.port }, p.onValue, p.offValue, p.pulseMs)
    banner.value = { kind: 'ok', text: `${p.label}：${p.type}${p.port} 点动 ${p.pulseMs}ms` }
    await refresh(true)
  })
}

// 保存整份清单，拿后端写进去的那份当准。它只动本机那份清单：不断连接，
// 也不向控制器发任何写请求。
function savePanel() {
  return act('save', async () => {
    emit('config-updated', await saveDraft())
    banner.value = { kind: 'ok', text: '点位清单已保存，已经生效' }
    await reread()
  })
}

async function resetPanel() {
  const ok = await confirmDialog('把这一页的点位清单恢复成出厂默认？现场改过的这一份会被删掉。', {
    title: '恢复默认',
    danger: true,
    confirmText: '恢复默认',
  })
  if (!ok) return
  return act('reset', async () => {
    emit('config-updated', await resetDraft())
    banner.value = { kind: 'info', text: '已恢复出厂默认点位' }
    await reread()
  })
}

function exportPanel() {
  return act('export', async () => {
    const path = await exportFile()
    if (!path) return
    banner.value = { kind: 'ok', text: `已导出到 ${path}` }
  })
}

async function importPanel() {
  const msg = editing.value
    ? '导入会替换当前这一页的点位清单，未保存的编辑会丢掉。继续？'
    : '导入会替换当前这一页的点位清单，继续？'
  if (!(await confirmDialog(msg, { title: '导入点位' }))) return
  return act('import', async () => {
    const r = await importFile()
    if (r.canceled) return
    emit('config-updated', r.settings)
    banner.value = { kind: 'ok', text: `已从 ${fileName(r.path)} 导入，已经生效` }
    await reread()
  })
}

function fileName(p: string): string {
  const i = Math.max(p.lastIndexOf('/'), p.lastIndexOf('\\'))
  return i >= 0 ? p.slice(i + 1) : p
}

// 清空旧读数再读一次。改过端口的点位不能拿上一个端口的读数冒充自己的状态——
// 那正是「界面显示 OFF、现场却是通的」这种最难查的假象。
//
// 先等一次 nextTick：新清单是父组件收到 config-updated 之后才灌下来的，
// 不等的话这一次读的还是改之前那批端口。
async function reread() {
  values.value = {}
  await nextTick()
  await refresh(true)
}

function isDigital(p: remote.Point): boolean {
  return p.type === 'DI' || p.type === 'DO'
}

// manual 决定这一行是「ON/OFF 翻转」还是「填个值再下发」。
//
// 只给 AO：AI 是输入，一键强制只对 DI 发标志，AI 那一路走不通，所以它跟以前一样只读。
// 判断只看类型不看连不连得上，断线时布局不会跟着变，控件只是灰掉。
function manual(p: remote.Point): boolean {
  return p.type === 'AO'
}

// 输出随时可写；DI 要先一键强制。AI 不走强制，保持只读。
function writable(p: remote.Point): boolean {
  if (!props.connected) return false
  if (p.type === 'DI') return allForced.value
  if (p.type === 'AI') return false
  return true
}

function display(p: remote.Point): string {
  const v = values.value[pointKey(p.type, p.port)]
  if (v === undefined) return '—'
  if (isDigital(p)) return v ? 'ON' : 'OFF'
  return String(v)
}

function isOn(p: remote.Point): boolean {
  const v = values.value[pointKey(p.type, p.port)]
  return v !== undefined && v !== 0
}

function meta(p: remote.Point): string {
  if (p.hint) return p.hint
  if (p.type === 'DI' && !allForced.value) return '只读'
  if (manual(p)) return p.value ? `默认 ${p.value}，改完点「下发」` : '填一个值再点「下发」'
  return `切换 ${p.onValue} / ${p.offValue}`
}

// 灰着的按钮要说清为什么灰。输入点位的点动要先开强制开关，
// 不写明的话看起来就像这个按钮坏了。
function pulseTitle(p: remote.Point): string {
  if (p.type === 'DI' && !allForced.value) {
    return '先点「一键强制」'
  }
  return `写 ${p.onValue}，${p.pulseMs}ms 后写回 ${p.offValue}`
}
</script>

<template>
  <div class="io-page">
    <div class="io-main">
  <div class="actions io-toolbar">
    <template v-if="!editing">
      <button :disabled="!connected || !!busy" @click="refresh()">刷新</button>
      <button
        :class="{ primary: !allForced }"
        :disabled="!connected || !!busy || !diPoints.length"
        :title="allForced ? '关闭配置里所有 DI 的强制' : '打开配置里所有 DI 的强制'"
        @click="toggleAllForce"
      >
        {{ busy === 'force-all' ? '处理中…' : allForced ? '取消强制' : '一键强制' }}
      </button>
      <label class="opt" title="间隔在上面的连接区里改，改完立即生效">
        <input v-model="autoRefresh" type="checkbox" :disabled="!connected" />
        每 {{ intervalText }}自动刷新
      </label>
      <button :disabled="!!busy" title="增删改这一页的点位，不需要连接" @click="startEdit">
        编辑点位
      </button>
    </template>
    <template v-else>
      <button :disabled="!!busy" @click="addGroup">＋ 分组</button>
      <button class="primary" :disabled="!!busy" @click="savePanel">
        {{ busy === 'save' ? '保存中…' : '保存' }}
      </button>
      <button :disabled="!!busy" @click="cancelEdit">取消</button>
      <button :disabled="!!busy" @click="resetPanel">
        {{ busy === 'reset' ? '恢复中…' : '恢复默认' }}
      </button>
      <span
        v-if="configDir"
        class="config-file"
        :title="`${configDir}\n和 exe 在同一目录，整夹拷走会一起带走。`"
      >
        配置存在本机
      </span>
    </template>
    <button :disabled="!!busy" title="把当前这一页的点位存成 JSON 文件" @click="exportPanel">
      {{ busy === 'export' ? '导出中…' : '导出' }}
    </button>
    <button :disabled="!!busy" title="从 JSON 文件替换当前这一页的点位" @click="importPanel">
      {{ busy === 'import' ? '导入中…' : '导入' }}
    </button>
    <div class="status" :class="banner?.kind" :title="banner?.text">{{ banner?.text }}</div>
  </div>

  <div class="io-cols">
    <section v-for="(group, gi) in groups" :key="gi" class="card">
      <div class="group-head">
        <h2 v-if="!editing" class="card-title">{{ group.title || '点位' }}</h2>
        <input
          v-else
          v-model="group.title"
          class="group-title"
          aria-label="分组名称"
          placeholder="分组名称"
        />
        <template v-if="editing">
          <button class="io-icon" :disabled="!!busy" title="添加点位" @click="addPoint(gi)">
            ＋
          </button>
          <button
            class="io-icon io-del"
            :disabled="!!busy"
            title="删除整个分组"
            @click="removeGroup(gi)"
          >
            ✕
          </button>
        </template>
      </div>
      <template v-for="(p, pi) in group.points" :key="`${gi}-${pi}`">
        <PointEditor
          v-if="isEditingAt(gi, pi) && editingPoint"
          :point="editingPoint"
          :types="ioTypes"
          port-label="端口"
          @submit="applyPoint"
          @cancel="cancelPoint"
        />
        <div v-else class="io-item" :class="{ danger: p.danger }">
          <span class="io-name" :title="`${p.label} · ${meta(p)}`">{{ p.label }}</span>
          <span class="io-port">{{ p.type }}{{ p.port }}</span>
          <template v-if="editing">
            <button class="io-icon" :disabled="!!busy" title="编辑" @click="editPoint(gi, pi)">
              ✎
            </button>
            <button
              class="io-icon io-del"
              :disabled="!!busy"
              title="删除"
              @click="removePoint(gi, pi)"
            >
              ✕
            </button>
          </template>
          <template v-else>
            <button
              v-if="isDigital(p) && p.pulseMs > 0"
              class="io-pulse"
              :disabled="!writable(p) || !!busy"
              :title="pulseTitle(p)"
              :aria-label="`${p.label} 点动`"
              @click="pulse(p, `p-${gi}-${pi}`)"
            >
              {{ busy === `p-${gi}-${pi}` ? '…' : '点动' }}
            </button>
            <template v-if="manual(p)">
              <input
                v-model="setpoints[pointKey(p.type, p.port)]"
                class="io-set"
                type="number"
                step="any"
                :disabled="!writable(p) || !!busy"
                :aria-label="`${p.label} 要写入的值`"
                title="要写下去的值，回车或点「下发」生效"
                @keyup.enter="writeValue(p, `w-${gi}-${pi}`)"
              />
              <button
                class="io-send"
                :disabled="!writable(p) || !!busy"
                :title="`把左边填的值写到 ${p.type}${p.port}`"
                @click="writeValue(p, `w-${gi}-${pi}`)"
              >
                {{ busy === `w-${gi}-${pi}` ? '…' : '下发' }}
              </button>
              <span class="io-value readonly" :title="`读回来的当前值`">{{ display(p) }}</span>
            </template>
            <button
              v-else
              class="io-value"
              :class="{ on: isOn(p), readonly: !writable(p) }"
              :disabled="!writable(p) || !!busy"
              :title="writable(p) ? `点一下写 ${isOn(p) ? p.offValue : p.onValue}` : '只读'"
              :aria-label="`${p.label} 当前 ${display(p)}，点击切换`"
              :aria-pressed="isOn(p)"
              @click="toggle(p, `t-${gi}-${pi}`)"
            >
              {{ busy === `t-${gi}-${pi}` ? '…' : display(p) }}
            </button>
          </template>
        </div>
      </template>
      <p v-if="editing && !group.points?.length" class="empty group-empty">
        这一组还没有点位，点＋添加。空分组保存不了。
      </p>
    </section>
  </div>
    </div>
  </div>
</template>

<style scoped>
.io-page {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 12px;
  align-items: start;
}

.io-main {
  min-width: 0;
}

/* 状态栏在操作行右侧恒占一块位置；消息长了截断，完整内容挂在 title 上。 */
.status {
  flex: 1 1 220px;
  min-width: 0;
  height: 34px;
  margin-left: auto;
  padding: 0 10px;
  border-radius: 6px;
  overflow: hidden;
  font-size: 12px;
  line-height: 34px;
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

/* 输出一列、输入一列，左右并排；列的顺序就是 config.json 里分组的顺序。
   窄到放不下两列时自动落回一列。align-items 不拉伸，两组点位数不一样时
   短的那张卡片不会被抻出一片空白。 */
.io-cols {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(340px, 1fr));
  gap: 12px;
  align-items: start;
}

/* 工具栏放在点位上方，一进标签页就能刷新或开强制，不用滚到列表末尾。 */
.io-toolbar {
  flex-wrap: wrap;
  margin: 0 0 10px;
  padding: 8px 10px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--panel);
}

.io-toolbar button {
  min-height: 34px;
}

/* 仍保持一行一个点位，但把可点击区域扩到正常按钮尺寸。 */
.card {
  margin-bottom: 0;
  padding: 10px 12px;
}

.card-title {
  margin-bottom: 8px;
}

/* 组名和这一组的编辑按钮同一行。组名在编辑时换成输入框，位置和尺寸不变，
   编辑前后卡片不跳动。 */
.group-head {
  display: flex;
  align-items: center;
  gap: 6px;
  margin-bottom: 8px;
}

.group-head .card-title {
  flex: 1 1 auto;
  min-width: 0;
  margin-bottom: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.group-title {
  flex: 1 1 auto;
  min-width: 0;
  padding: 4px 7px;
  font-size: 13px;
  font-weight: 600;
}

.io-icon {
  flex: 0 0 auto;
  width: 30px;
  min-height: 30px;
  padding: 0;
  border-radius: 7px;
  font-size: 13px;
  color: var(--text-dim);
}

.io-icon:hover:not(:disabled) {
  border-color: var(--accent);
  color: var(--accent);
}

.io-del:hover:not(:disabled) {
  border-color: var(--err);
  color: var(--err);
}

.group-empty {
  margin: 6px 0 2px;
  font-size: 12px;
}

/* 配置目录挂在 title 上而不是铺在界面里：路径很长，平时不用看见，
   要拷文件的时候鼠标一停就有。 */
.config-file {
  flex: 0 0 auto;
  font-size: 11px;
  color: var(--text-dim);
  cursor: help;
  text-decoration: underline dotted;
  text-underline-offset: 2px;
}

/* 一个点位一行。名称占满剩下的宽度，点位号、动作、当前值依次靠右，
   当前值恒在最右边——它是最常扫的一列，对齐了才好一眼看完一整组。 */
.io-item {
  display: flex;
  align-items: center;
  gap: 8px;
  min-height: 42px;
  padding: 5px 4px;
  border-top: 1px solid var(--border);
  transition: background-color 120ms ease;
}

.io-item:hover {
  background: var(--bg);
}

.io-item:first-child {
  border-top: none;
}

/* 用内阴影而不是左边框画这道红：边框会把这一行的名称往右挤 2px，
   一列里只有急停那几行错开，看着比不标更乱。 */
.io-item.danger {
  box-shadow: inset 2px 0 0 var(--err);
}

.io-item.danger .io-name {
  color: var(--err);
}

.io-name {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  font-size: 13px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.io-port {
  flex: 0 0 auto;
  min-width: 2.7rem;
  font-family: Consolas, "Courier New", monospace;
  font-size: 11px;
  color: var(--text-dim);
  text-align: center;
}

.io-value {
  flex: 0 0 auto;
  min-width: 4rem;
  min-height: 32px;
  padding: 5px 12px;
  border-radius: 7px;
  background: var(--bg);
  font-size: 12px;
  font-weight: 600;
  color: var(--text-dim);
  text-align: center;
}

.io-value.on {
  border-color: var(--ok);
  background: var(--ok-soft);
  color: var(--ok);
}

/* 只读的值不做成按钮样子，但位置和尺寸跟可切换的一模一样。 */
.io-value.readonly {
  border-style: dashed;
  cursor: default;
  opacity: 0.72;
}

.io-pulse {
  flex: 0 0 auto;
  min-width: 3.5rem;
  min-height: 32px;
  padding: 5px 10px;
  border-radius: 7px;
  font-size: 12px;
}

/* 待下发的值和读回来的值分两个格子：左边是要写下去的，行尾还是读回来的当前值。
   合成一个可编辑的格子的话，一秒一次的自动刷新会把人正在输的数字冲掉。 */
.io-set {
  flex: 0 0 3.9rem;
  min-height: 32px;
  padding: 5px 6px;
  border-radius: 7px;
  font-family: Consolas, "Courier New", monospace;
  font-size: 12px;
  text-align: right;
}

.io-send {
  flex: 0 0 auto;
  min-width: 3.2rem;
  min-height: 32px;
  padding: 5px 8px;
  border-radius: 7px;
  font-size: 12px;
}

/* 行尾这个值是 span 不是按钮（点不动它），补上按钮那圈边框好让两种行对齐。 */
span.io-value {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  box-sizing: border-box;
  border: 1px solid var(--border);
}

.io-value:hover:not(:disabled),
.io-send:hover:not(:disabled),
.io-pulse:hover:not(:disabled) {
  border-color: var(--accent);
  box-shadow: 0 0 0 2px var(--accent-soft);
}

.io-value:active:not(:disabled),
.io-send:active:not(:disabled),
.io-pulse:active:not(:disabled) {
  transform: translateY(1px);
}

.io-value:focus-visible,
.io-send:focus-visible,
.io-pulse:focus-visible {
  outline: 2px solid var(--accent);
  outline-offset: 2px;
}

.opt {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--text-dim);
}

.opt input {
  width: auto;
}
</style>

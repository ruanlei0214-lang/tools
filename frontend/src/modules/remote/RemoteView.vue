<script lang="ts" setup>
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import {
  Config,
  Connect,
  Disconnect,
  ResetDevice,
  SaveDevice,
  Status,
} from '../../../wailsjs/go/remote/Service'
import type { remote } from '../../../wailsjs/go/models'
import IoPanel from './IoPanel.vue'
import RegisterPanel from './RegisterPanel.vue'

// 标签页与点位来自后端的配置，这里负责渲染、连接，以及连接参数的编辑。
const device = reactive<remote.Device>({ host: '', port: 9000, path: '/' })
const tabs = ref<remote.Tab[]>([])
const refreshIntervalMs = ref(1000)
const connectTimeoutSeconds = ref(5)
const requestTimeoutSeconds = ref(8)
const configDir = ref('')
const activeId = ref('')
const connected = ref(false)
const busy = ref('')
const configWarning = ref('')
const banner = ref<{ kind: 'ok' | 'err' | 'info'; text: string } | null>(null)
// 连接参数平时收起来。连接区刚压缩成一行，把两个超时和刷新间隔常显会把它撑回两行，
// 而这几个值一年动不了一次。
const paramsOpen = ref(false)

// 配置告警与操作结果共用一条状态槽：告警优先，操作只清 banner，冲不掉告警。
const status = computed(() => {
  if (configWarning.value) return { kind: 'err' as const, text: configWarning.value }
  return banner.value
})

const activeTab = computed(() => tabs.value.find((t) => t.id === activeId.value) ?? null)

// applyConfig 是配置进入界面的唯一入口：初次加载、保存连接参数、保存点位、恢复默认
// 都走它，界面上那份于是永远等于后端写进去的那份。
function applyConfig(cfg: remote.Settings) {
  device.host = cfg.device.host
  device.port = cfg.device.port || 9000
  device.path = cfg.device.path || '/'
  connectTimeoutSeconds.value = cfg.connectTimeoutSeconds
  requestTimeoutSeconds.value = cfg.requestTimeoutSeconds
  refreshIntervalMs.value = cfg.refreshIntervalMs
  configDir.value = cfg.configDir
  tabs.value = cfg.tabs ?? []
  configWarning.value = cfg.warning
  // 保存点位不该把人踢回第一页；那一页真的没了才换。
  if (!tabs.value.some((t) => t.id === activeId.value)) {
    activeId.value = tabs.value[0]?.id ?? ''
  }
}

onMounted(async () => {
  try {
    applyConfig(await Config())
  } catch (e) {
    configWarning.value = `读取配置失败：${String(e)}`
  }
  // 不自动连接。开机时设备还没起来，一打开就连会把整个程序卡住。
})

// 连接活在后端，组件销毁不会自动收；不断开的话切走页面还挂着一条 socket。
onUnmounted(() => {
  void Disconnect()
})

async function call(op: string, fn: () => Promise<void>) {
  busy.value = op
  banner.value = null
  try {
    await fn()
  } catch (e) {
    banner.value = { kind: 'err', text: String(e) }
  } finally {
    busy.value = ''
  }
}

function connect() {
  return call('connect', async () => {
    connected.value = false
    const st = await Connect({
      host: device.host.trim(),
      port: Number(device.port) || 9000,
      path: device.path,
    })
    connected.value = st.connected
    banner.value = { kind: 'ok', text: `已连接 ${st.addr}` }
  })
}

function disconnect() {
  return call('disconnect', async () => {
    await Disconnect()
    connected.value = false
    banner.value = { kind: 'info', text: '已断开连接' }
  })
}

// 保存连接参数。超时和刷新间隔存下去就立刻算新的；地址、端口、路径只是存下来，
// 当前连接一动不动——这个页面上的按钮会动现场的气缸，换设备的时机必须由人决定。
function saveDevice() {
  return call('save-device', async () => {
    const connectedTo = connected.value ? await currentAddr() : ''
    const cfg = await SaveDevice({
      device: {
        host: device.host.trim(),
        port: Number(device.port) || 0,
        path: device.path.trim(),
      },
      connectTimeoutSeconds: Number(connectTimeoutSeconds.value) || 0,
      requestTimeoutSeconds: Number(requestTimeoutSeconds.value) || 0,
      refreshIntervalMs: Number(refreshIntervalMs.value) || 0,
    } as remote.DeviceSettings)
    applyConfig(cfg)

    const target = `${cfg.device.host}:${cfg.device.port}${cfg.device.path}`
    banner.value =
      connectedTo && !connectedTo.includes(target)
        ? { kind: 'info', text: `已保存。当前仍连着 ${connectedTo}，点「重新连接」才换到 ${target}` }
        : { kind: 'ok', text: '已保存，立即生效' }
  })
}

function resetDevice() {
  if (!window.confirm('把连接参数恢复成出厂默认？现场改过的这一份会被删掉。')) return
  return call('reset-device', async () => {
    applyConfig(await ResetDevice())
    banner.value = { kind: 'info', text: '连接参数已恢复出厂默认，地址要点「重新连接」才换过去' }
  })
}

async function currentAddr(): Promise<string> {
  try {
    return (await Status()).addr
  } catch {
    return ''
  }
}

// 点位面板保存或恢复默认之后，整份配置由它们回传，这里照样走 applyConfig。
function onConfigUpdated(cfg: remote.Settings) {
  applyConfig(cfg)
}

// 子面板每次调用失败都会喊一声：连接可能是被控制器单方面断掉的，
// 那种情况下只有后端知道，界面得跟着把状态改回未连接。
async function syncStatus() {
  try {
    const st: remote.Status = await Status()
    connected.value = st.connected
    if (!st.connected && st.error) {
      banner.value = { kind: 'err', text: st.error }
    }
  } catch {
    connected.value = false
  }
}
</script>

<template>
  <section class="card conn-card">
    <!-- 标题、地址、按钮、状态同一行：连接区在这三个模块里长一个样，
         状态紧跟在操作按钮后面，不再单独占一块横幅。 -->
    <div class="conn-row">
      <h2 class="card-title">控制器连接</h2>
      <input
        id="remote-host"
        v-model.trim="device.host"
        class="conn-host"
        aria-label="IP"
        placeholder="IP"
        :disabled="!!busy"
      />
      <input
        id="remote-port"
        v-model.number="device.port"
        class="conn-port"
        aria-label="端口"
        type="number"
        min="1"
        max="65535"
        placeholder="端口"
        :disabled="!!busy"
      />
      <button class="primary conn-btn" :disabled="!!busy || !device.host.trim()" @click="connect">
        {{ busy === 'connect' ? '连接中…' : connected ? '重新连接' : '连接' }}
      </button>
      <button class="conn-btn" :disabled="!!busy || !connected" @click="disconnect">断开</button>
      <button
        class="conn-btn"
        :title="paramsOpen ? '收起参数' : '路径、超时、刷新间隔'"
        @click="paramsOpen = !paramsOpen"
      >
        参数 {{ paramsOpen ? '▲' : '▼' }}
      </button>
      <div class="status" :class="status?.kind" :title="status?.text">{{ status?.text }}</div>
    </div>

    <div v-if="paramsOpen" class="conn-row params-row">
      <div class="field">
        <label for="remote-path">路径</label>
        <input id="remote-path" v-model.trim="device.path" placeholder="/" :disabled="!!busy" />
      </div>
      <div class="field field-narrow">
        <label for="remote-ct">建连超时 秒</label>
        <input
          id="remote-ct"
          v-model.number="connectTimeoutSeconds"
          type="number"
          min="1"
          max="60"
          :disabled="!!busy"
        />
      </div>
      <div class="field field-narrow">
        <label for="remote-rt">请求超时 秒</label>
        <input
          id="remote-rt"
          v-model.number="requestTimeoutSeconds"
          type="number"
          min="1"
          max="120"
          :disabled="!!busy"
        />
      </div>
      <div class="field field-narrow">
        <label for="remote-ri">刷新间隔 毫秒</label>
        <input
          id="remote-ri"
          v-model.number="refreshIntervalMs"
          type="number"
          min="200"
          max="60000"
          step="100"
          :disabled="!!busy"
        />
      </div>
      <button class="primary" :disabled="!!busy || !device.host.trim()" @click="saveDevice">
        {{ busy === 'save-device' ? '保存中…' : '保存' }}
      </button>
      <button :disabled="!!busy" @click="resetDevice">
        {{ busy === 'reset-device' ? '恢复中…' : '恢复默认' }}
      </button>
      <span
        v-if="configDir"
        class="config-file"
        :title="`${configDir}\n和 exe 在同一目录，整夹拷走会一起带走。`"
      >
        配置存在本机
      </span>
    </div>
  </section>

  <nav v-if="tabs.length" class="tabs">
    <button
      v-for="t in tabs"
      :key="t.id"
      class="tab"
      :class="{ active: t.id === activeId }"
      @click="activeId = t.id"
    >
      {{ t.title }}
    </button>
  </nav>

  <template v-if="activeTab">
    <IoPanel
      v-if="activeTab.kind === 'io'"
      :key="activeTab.id"
      :tab="activeTab"
      :connected="connected"
      :interval-ms="refreshIntervalMs"
      :config-dir="configDir"
      @refresh-status="syncStatus"
      @config-updated="onConfigUpdated"
    />
    <RegisterPanel
      v-else-if="activeTab.kind === 'register'"
      :key="activeTab.id"
      :tab="activeTab"
      :connected="connected"
      :interval-ms="refreshIntervalMs"
      :config-dir="configDir"
      @refresh-status="syncStatus"
      @config-updated="onConfigUpdated"
    />
    <p v-else class="empty">标签页类型 {{ activeTab.kind }} 还没有对应界面。</p>
  </template>
  <p v-else class="empty">配置里没有任何标签页。</p>
</template>

<style scoped>
.conn-card {
  margin-bottom: 8px;
  padding: 6px 8px;
}

/* 标题、输入、按钮、状态挤在一行，不折行：折了就等于又占两行，
   连接区压到一行的意义就没了。地址框可以被压窄，状态栏吃掉剩下的宽度。 */
.conn-row {
  display: flex;
  flex-wrap: nowrap;
  align-items: center;
  gap: 6px;
}

.conn-row .card-title {
  flex: 0 0 auto;
  margin: 0;
  font-size: 12px;
  white-space: nowrap;
}

.conn-row input {
  padding: 4px 7px;
  font-size: 12px;
}

.conn-host {
  flex: 0 1 9.5rem;
  width: 9.5rem;
}

.conn-port {
  flex: 0 0 4.2rem;
  width: 4.2rem;
  appearance: textfield;
}

.conn-port::-webkit-inner-spin-button,
.conn-port::-webkit-outer-spin-button {
  appearance: none;
}

.conn-btn {
  flex: 0 0 auto;
  padding: 4px 10px;
  font-size: 12px;
}

/* 状态占掉这一行剩下的宽度。消息长了截断，完整内容挂在 title 上——
   这一行的高度不能跟着消息变，否则下面的标签页会上下跳。 */
.status {
  flex: 1 1 auto;
  min-width: 0;
  height: 26px;
  padding: 0 8px;
  border-radius: 5px;
  overflow: hidden;
  font-size: 12px;
  line-height: 26px;
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

/* 参数行展开在连接那一行下面。这一行的输入框带标签，靠底对齐才不会错位。 */
.params-row {
  flex-wrap: wrap;
  align-items: flex-end;
  gap: 10px;
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px solid var(--border);
}

.params-row .field {
  flex: 1 1 160px;
}

.field-narrow {
  flex: 0 0 7rem;
}

/* 配置目录挂在 title 上而不是铺在界面里：路径很长，平时不用看见，
   要拷文件的时候鼠标一停就有。 */
.config-file {
  flex: 0 0 auto;
  align-self: center;
  font-size: 11px;
  color: var(--text-dim);
  cursor: help;
  text-decoration: underline dotted;
  text-underline-offset: 2px;
}

.tabs {
  display: flex;
  gap: 2px;
  margin-bottom: 16px;
  border-bottom: 1px solid var(--border);
}

.tab {
  margin-bottom: -1px;
  padding: 8px 14px;
  border: none;
  border-bottom: 2px solid transparent;
  border-radius: 0;
  background: none;
  color: var(--text-dim);
}

.tab:hover:not(.active) {
  color: var(--text);
}

.tab.active {
  border-bottom-color: var(--accent);
  color: var(--accent);
  font-weight: 600;
}
</style>

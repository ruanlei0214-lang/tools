<script lang="ts" setup>
import { computed, onMounted, reactive, ref } from 'vue'
import {
  ApplyConfig,
  ApplyWifi,
  Defaults,
  GetWifiAp,
  ListPorts,
  RestoreNetwork,
  TestConnection,
} from '../../../wailsjs/go/netcfg/Service'
import type { netcfg } from '../../../wailsjs/go/models'
import { loadShared } from '../../shell/connection'

// 四个字段都由后端 Defaults() 填入：地址和凭据来自共享配置 toolbox-config.json。
// 界面上不暴露编辑，要改去 exe 同目录改那份文件。
const device = reactive<netcfg.Device>({ host: '', port: 0, user: '', password: '' })
const form = reactive({ ip: '', mask: '', gateway: '' })
const defaults = reactive({ mask: '', persistIface: '' })
const wifiSSID = ref('')
const wifiChannel = ref('')
// wifiBand 是设备当前频段（只用于展示），bandChoice 是下拉框里要切到的频段。
const wifiBand = ref<'5G' | '2.4G'>('5G')
const bandChoice = ref<'5G' | '2.4G'>('5G')
const configWarning = ref('')

onMounted(async () => {
  try {
    const s = await Defaults()
    Object.assign(device, s.device)
    defaults.mask = s.mask
    defaults.persistIface = s.persistIface
    form.mask = s.mask
    form.gateway = s.gateway
    configWarning.value = s.warning
  } catch (e) {
    configWarning.value = `读取默认配置失败：${String(e)}`
  }
  // 不自动连接。开机时设备还没起来，一打开就连会把整个程序卡住。
})

// 列表里是机柜面板上的口（lan1..lan5），不是系统网口。selected 存的是面板口名，
// 下发配置时要换成它背后的系统网口名。
const ports = ref<netcfg.Port[]>([])
const selected = ref('')
const busy = ref('')
const confirming = ref(false)
const rebootNotice = ref(false)
const banner = ref<{ kind: 'ok' | 'err' | 'info'; text: string } | null>(null)
// 一键恢复、重启 WiFi 都要先能 SSH 上设备。没连过就摆着，误点要么改错机，要么空等超时。
const restorable = ref(false)

// 配置告警与操作结果共用一条状态槽：告警优先，操作只清 banner，冲不掉告警。
const status = computed(() => {
  if (configWarning.value) return { kind: 'err' as const, text: configWarning.value }
  return banner.value
})

const current = computed(() => ports.value.find((p) => p.name === selected.value) ?? null)

// 只有落在设备主网口上的面板口能改地址，其余只读。
const editable = computed(() => ports.value.filter((p) => p.editable))

const canApply = computed(() => current.value !== null && form.ip !== '' && form.mask !== '')

const channels5G = [36, 40, 44, 48, 149, 153, 157, 161, 165]

const channelHint = computed(() =>
  bandChoice.value === '2.4G' ? '2.4G 可用信道：1-13' : '5G 可用信道：36 / 40 / 44 / 48 / 149 / 153 / 157 / 161 / 165',
)

// 信道留空表示保持现状；填了就必须落在所选频段的合法范围里。
const canApplyWifi = computed(() => {
  if (wifiChannel.value === '') return true
  const n = Number(wifiChannel.value)
  if (!Number.isInteger(n)) return false
  return bandChoice.value === '2.4G' ? n >= 1 && n <= 13 : channels5G.includes(n)
})

// DFS 信道（52-64、100-144）单独点名：它不是"不在列表里"，是雷达避让会让热点长时间不可用。
const dfsChannel = computed(() => {
  const n = Number(wifiChannel.value)
  if (!Number.isInteger(n) || bandChoice.value !== '5G') return false
  return (n >= 52 && n <= 64) || (n >= 100 && n <= 144)
})

// 只有持久化网口改完能留住，其他网口重启就没了。这两种结果差别太大，
// 不能用同一句话糊过去。比的是系统网口名，面板名和它对不上。
const willPersist = computed(
  () => defaults.persistIface !== '' && current.value?.iface === defaults.persistIface,
)

// 几个面板口共用一个系统网口时（桥接形态下 lan1/lan2/lan5 都是 br0），改一个就是
// 改全部。这事必须说出来，否则现场以为只动了自己选的那个口。
const siblings = computed(() => {
  const iface = current.value?.iface
  if (!iface) return []
  const names = ports.value.filter((p) => p.iface === iface).map((p) => p.name)
  return names.length > 1 ? names : []
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

// refreshDevice 重新取一次共享配置里的地址和凭据。页面是 keep-alive 的，
// 文件改过的值不会自己长进来，每次动设备前先对齐。
async function refreshDevice() {
  const s = await Defaults()
  Object.assign(device, s.device)
}

// 连得上就顺手把网口读回来：点「刷新网络配置」的人下一步一定是要看网口，中间再让他点一次
// 没有意义。读网口失败时错误会盖掉「连接成功」，这是对的——连上了但读不到网口，
// 能做的事和没连上一样。
function resetView() {
  ports.value = []
  selected.value = ''
  confirming.value = false
  restorable.value = false
  wifiSSID.value = ''
  wifiChannel.value = ''
  wifiBand.value = '5G'
  bandChoice.value = '5G'
}

function test() {
  return call('test', async () => {
    // 先清掉上一次连接读到的网口。连不上却还留着旧设备的列表，会按错机改地址。
    resetView()

    await refreshDevice()
    await TestConnection(device)
    ports.value = await ListPorts(device)
    restorable.value = true
    try {
      const wifi = await GetWifiAp(device)
      wifiSSID.value = wifi.ssid
      wifiChannel.value = wifi.channel > 0 ? String(wifi.channel) : ''
      wifiBand.value = wifi.band === '2.4G' ? '2.4G' : '5G'
      bandChoice.value = wifiBand.value
    } catch {
      // 网口已经读到了，WiFi 读失败不挡改地址。
    }

    // 面板口恒为五个（wlan 另算），所以"有没有东西可配"要看有没有能改的口，而不是读到了几行。
    if (editable.value.length === 0) {
      banner.value = { kind: 'info', text: '连接成功，但这台设备上没有可以修改地址的网口。' }
      return
    }
    select(editable.value.find((p) => p.up) ?? editable.value[0])
    banner.value = { kind: 'ok', text: '连接成功' }
  })
}

function select(port: netcfg.Port) {
  // 只读行和占位行都点不动：前者有信息但不归这里改，后者连信息都没有。
  if (!port.editable) return
  selected.value = port.name
  form.ip = port.ip
  form.mask = port.mask || defaults.mask
  form.gateway = port.gateway
  confirming.value = false
}

function apply() {
  // 先存下来：下面会清空 selected 和表单，那之后这两个值就取不到了。
  const target = form.ip
  const persisted = willPersist.value
  // 下发用的是系统网口名，不是面板名——设备只认前者。
  const iface = current.value?.iface ?? ''
  return call('apply', async () => {
    await ApplyConfig(device, { iface, ...form })
    resetView()
    device.host = target
    // 后端已把新地址写进共享配置，顶栏的地址跟着换过去。
    await loadShared()
    banner.value = {
      kind: 'ok',
      text: persisted ? '配置已下发并持久化，请重新刷新确认' : '配置已下发，请重新刷新确认',
    }
  })
}

function applyWifi() {
  return call('wifi', async () => {
    await refreshDevice()
    const ch = wifiChannel.value === '' ? 0 : Number(wifiChannel.value)
    const out = await ApplyWifi(device, bandChoice.value, ch)
    wifiBand.value = bandChoice.value
    banner.value = { kind: 'ok', text: out || 'WiFi 正在后台重启' }
  })
}

function restore() {
  return call('restore', async () => {
    await refreshDevice()
    await RestoreNetwork(device)
    banner.value = { kind: 'ok', text: '已恢复出厂网络配置，请重启控制器' }
    rebootNotice.value = true
  })
}
</script>

<template>
  <div class="page">
    <section class="panel">
      <div class="block-head">
        <div class="step-label">设备</div>
        <p class="step-hint">先刷新，再改网口或 WiFi。地址在顶栏。</p>
      </div>
      <div class="toolbar">
        <button class="primary" :disabled="busy !== ''" @click="test">
          {{ busy === 'test' ? '刷新中…' : '刷新网络配置' }}
        </button>
        <button
          v-if="restorable"
          class="danger"
          :disabled="busy !== ''"
          title="删除地址持久化文件，并替换 /opt/setBridge.sh、/opt/setWifi.sh 为出厂版本"
          @click="restore"
        >
          {{ busy === 'restore' ? '恢复中…' : '一键恢复网络' }}
        </button>
        <div class="status" :class="status?.kind" :title="status?.text" aria-live="polite">
          {{ status?.text ?? '尚未刷新' }}
        </div>
      </div>

      <div v-if="restorable" class="wifi-block">
        <div class="wifi-head">
          <div class="wifi-title">WiFi 设置</div>
          <span v-if="wifiSSID" class="wifi-ssid" :title="wifiSSID">{{ wifiSSID }}</span>
        </div>
        <div class="toolbar">
          <div class="field band-field">
            <label for="band">频段（当前 {{ wifiBand }}）</label>
            <select id="band" v-model="bandChoice">
              <option value="5G">5G</option>
              <option value="2.4G">2.4G</option>
            </select>
          </div>
          <div class="field chan-field">
            <label for="chan">信道</label>
            <input
              id="chan"
              v-model.trim="wifiChannel"
              inputmode="numeric"
              :placeholder="bandChoice === '2.4G' ? '1-13' : '149'"
              :title="channelHint"
            />
          </div>
          <button
            class="primary"
            :disabled="!canApplyWifi || busy !== ''"
            title="写入频段/信道并后台重启 WiFi，约 10 秒；信道留空表示保持现状"
            @click="applyWifi"
          >
            {{ busy === 'wifi' ? '应用中…' : '应用并重启' }}
          </button>
        </div>
        <p class="chan-hint" :class="{ err: dfsChannel }">
          <template v-if="dfsChannel">信道 {{ wifiChannel }} 是 DFS 信道，雷达避让会让热点长时间不可用，禁止使用</template>
          <template v-else>{{ channelHint }}</template>
        </p>
      </div>
    </section>

    <section v-if="ports.length" class="panel">
      <div class="block-head">
        <div class="step-label">网口</div>
        <p v-if="editable.length" class="step-hint">
          点可改的口改地址。可改：{{ editable.map((p) => p.name).join('、') }}
        </p>
        <p v-else class="step-hint">这台设备上没有可以在这里改地址的网口</p>
      </div>

      <div class="port-list">
        <div class="port-row port-head">
          <span>口名</span>
          <span>IP</span>
          <span>掩码</span>
          <span>网关</span>
        </div>
        <button
          v-for="p in ports"
          :key="p.name"
          type="button"
          class="port-row"
          :class="{
            selected: p.name === selected,
            editable: p.editable,
            readonly: p.iface && !p.editable,
            blank: !p.iface,
          }"
          :disabled="!p.editable"
          @click="select(p)"
        >
          <span class="port-name">{{ p.name }}</span>
          <div class="port-ip">{{ p.ip || '—' }}</div>
          <div class="port-mask">{{ p.iface ? p.mask || '—' : '—' }}</div>
          <div class="port-gw">{{ p.iface ? p.gateway || '—' : '—' }}</div>
        </button>
      </div>
    </section>

    <section v-if="selected" class="panel edit-panel">
      <div class="block-head">
        <div class="step-label">修改 {{ selected }}</div>
        <p v-if="siblings.length" class="step-hint">
          {{ siblings.join('、') }} 是同一网口，改一个另外几个一起变。
        </p>
      </div>

      <div class="field-row">
        <div class="field">
          <label for="ip">IP 地址</label>
          <input id="ip" v-model.trim="form.ip" placeholder="192.168.1.100" />
        </div>
        <div class="field">
          <label for="mask">子网掩码</label>
          <input id="mask" v-model.trim="form.mask" placeholder="255.255.255.0" />
        </div>
        <div class="field">
          <label for="gateway">默认网关</label>
          <input id="gateway" v-model.trim="form.gateway" placeholder="留空表示不改默认路由" />
        </div>
      </div>

      <div class="actions">
        <template v-if="confirming">
          <button class="danger" :disabled="busy !== ''" @click="apply">
            {{ busy === 'apply' ? '下发中…' : '确认下发' }}
          </button>
          <button :disabled="busy !== ''" @click="confirming = false">取消</button>
          <span class="hint">下发后连接会断开，请确认新地址与本机同网段。</span>
        </template>
        <template v-else>
          <button class="primary" :disabled="!canApply || busy !== ''" @click="confirming = true">
            下发配置
          </button>
        </template>
      </div>
    </section>

    <div v-if="rebootNotice" class="modal-mask">
      <div class="modal">
        <h2 class="modal-title">请重启机器人控制器</h2>
        <div class="modal-actions">
          <button class="primary" @click="rebootNotice = false">知道了</button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.page {
  max-width: 880px;
}

.panel {
  margin-bottom: 12px;
  padding: 14px 16px;
  background: var(--panel);
  border: 1px solid var(--border);
  border-radius: var(--radius);
}

.block-head {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 4px 12px;
  margin-bottom: 10px;
}

.step-label {
  margin: 0;
  font-size: 13px;
  font-weight: 700;
  color: var(--text);
}

.step-hint {
  margin: 0;
  font-size: 12px;
  color: var(--text-dim);
}

.toolbar {
  display: flex;
  align-items: flex-end;
  flex-wrap: wrap;
  gap: 8px 10px;
}

.toolbar > button {
  flex: 0 0 auto;
  height: 32px;
  padding: 0 12px;
  font-size: 12px;
  font-weight: 600;
}

.wifi-block {
  margin-top: 12px;
  padding-top: 12px;
  border-top: 1px solid var(--border);
}

.wifi-head {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 4px 12px;
  margin-bottom: 8px;
}

.wifi-title {
  font-size: 12px;
  font-weight: 700;
  color: var(--text);
}

.band-field {
  flex: 0 0 150px;
}

.chan-field {
  flex: 0 0 88px;
}

.chan-hint {
  margin: 6px 0 0;
  font-size: 11px;
  color: var(--text-dim);
}

.chan-hint.err {
  color: var(--err);
}

.wifi-ssid {
  min-width: 0;
  max-width: 180px;
  overflow: hidden;
  font-size: 13px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* 竖排列表：行距压紧，四列对齐表头。 */
.port-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.port-head {
  padding: 0 10px 4px;
  border: 0;
  background: transparent;
  color: var(--text-dim);
  font-size: 11px;
  font-weight: 600;
}

.port-row {
  display: grid;
  grid-template-columns: 72px minmax(110px, 1.2fr) minmax(100px, 1fr) minmax(110px, 1.2fr);
  align-items: center;
  gap: 6px 12px;
  width: 100%;
  padding: 7px 10px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: #fff;
  text-align: left;
  font-family: inherit;
  color: inherit;
}

.port-row.editable {
  cursor: pointer;
}

.port-row.editable:hover {
  border-color: #c8ccd3;
  background: var(--bg);
}

.port-row.selected {
  border-color: var(--accent);
  background: var(--accent-soft);
  box-shadow: inset 0 0 0 1px var(--accent);
}

.port-row.readonly,
.port-row.blank {
  cursor: default;
  background: #fafbfc;
  color: var(--text-dim);
}

.port-row:disabled {
  cursor: default;
}

.port-name {
  font-size: 13px;
  font-weight: 700;
  color: var(--text);
}

.port-row.readonly .port-name,
.port-row.blank .port-name {
  color: var(--text-dim);
}

.port-ip {
  font-size: 13px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  letter-spacing: 0.01em;
  color: var(--text);
  word-break: break-all;
}

.port-row.readonly .port-ip,
.port-row.blank .port-ip {
  color: var(--text-dim);
  font-weight: 500;
}

.port-mask,
.port-gw {
  font-size: 12px;
  color: var(--text-dim);
  font-variant-numeric: tabular-nums;
  word-break: break-all;
}

.edit-panel {
  border-color: #c9d8f5;
  background: #f8faff;
}

.actions {
  margin-top: 10px;
  gap: 8px;
}

/* 状态占掉连接行剩下的宽度。这一格的高度不能跟着消息变，
   否则下面的网口列表会上下跳，所以长消息截断、完整内容挂 title。 */
.status {
  flex: 1 1 12rem;
  min-width: 8rem;
  height: 32px;
  padding: 0 10px;
  border-radius: 5px;
  overflow: hidden;
  font-size: 12px;
  line-height: 32px;
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

.modal-mask {
  position: fixed;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(31, 36, 48, 0.45);
}

.modal {
  width: 380px;
  padding: 18px 20px;
  background: var(--panel);
  border-radius: var(--radius);
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.18);
}

.modal-title {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 18px;
}

@media (max-width: 720px) {
  .port-head {
    display: none;
  }

  .port-row {
    grid-template-columns: 56px 1fr;
    grid-template-areas:
      "name ip"
      "name meta";
    padding: 6px 8px;
  }

  .port-name {
    grid-area: name;
  }

  .port-ip {
    grid-area: ip;
  }

  .port-mask,
  .port-gw {
    grid-area: meta;
  }

  .port-mask {
    display: none;
  }

  .status {
    flex: 1 1 100%;
  }
}
</style>

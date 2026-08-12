<script lang="ts" setup>
import { computed, onMounted, reactive, ref } from 'vue'
import {
  ApplyConfig,
  Defaults,
  ListPorts,
  RestoreNetwork,
  TestConnection,
} from '../../../wailsjs/go/netcfg/Service'
import type { netcfg } from '../../../wailsjs/go/models'

// 四个字段都由 onMounted 从 config.json 填入，这里只占位。
// 端口、用户名、密码不在界面上暴露，要改只能改配置后重新构建。
const device = reactive<netcfg.Device>({ host: '', port: 0, user: '', password: '' })
const form = reactive({ ip: '', mask: '', gateway: '' })
const defaults = reactive({ mask: '', restoreFile: '', persistIface: '' })
const configWarning = ref('')

onMounted(async () => {
  try {
    const s = await Defaults()
    Object.assign(device, s.device)
    defaults.mask = s.mask
    defaults.restoreFile = s.restoreFile
    defaults.persistIface = s.persistIface
    form.mask = s.mask
    form.gateway = s.gateway
    configWarning.value = s.warning
  } catch (e) {
    configWarning.value = `读取默认配置失败：${String(e)}`
    return
  }
  // 打开页面就连一次，省掉每次手点「测试连接」。配置没读到就没有地址可连，上面已经 return。
  await test()
})

// 表格里是机柜面板上的口（lan1..lan5），不是系统网口。selected 存的是面板口名，
// 下发配置时要换成它背后的系统网口名。
const ports = ref<netcfg.Port[]>([])
const selected = ref('')
const busy = ref('')
const confirming = ref(false)
const rebootNotice = ref(false)
const banner = ref<{ kind: 'ok' | 'err' | 'info'; text: string } | null>(null)

const current = computed(() => ports.value.find((p) => p.name === selected.value) ?? null)

// 只有落在设备主网口上的面板口能改地址，其余只读。
const editable = computed(() => ports.value.filter((p) => p.editable))

const canApply = computed(() => current.value !== null && form.ip !== '' && form.mask !== '')

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

// 连得上就顺手把网口读回来：点「测试连接」的人下一步一定是要看网口，中间再让他点一次
// 没有意义。读网口失败时错误会盖掉「连接成功」，这是对的——连上了但读不到网口，
// 能做的事和没连上一样。
function test() {
  return call('test', async () => {
    // 先清掉上一次连接读到的网口。连不上（超时、认证失败、地址不可达都算）却还留着旧
    // 设备的网口列表，用户会当成当前设备的状态，照着它改地址就改到别处去了。放在开头
    // 而不是 catch 里：连接期间状态本来就是未知的，空表格比一份过期的表格诚实。
    ports.value = []
    selected.value = ''
    confirming.value = false

    await TestConnection(device)
    ports.value = await ListPorts(device)

    // 面板口恒为五个，所以"有没有东西可配"要看有没有能改的口，而不是读到了几行。
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
    confirming.value = false
    ports.value = []
    selected.value = ''
    device.host = target
    banner.value = {
      kind: 'ok',
      text: persisted
        ? `配置已下发并写入 ${defaults.restoreFile}，重启后仍然生效。设备地址已切换到 ${target}，连接地址同步更新，请重新点「测试连接」确认。`
        : `配置已下发。设备地址已切换到 ${target}，连接地址同步更新，请重新点「测试连接」确认。`,
    }
  })
}

function restore() {
  return call('restore', async () => {
    await RestoreNetwork(device)
    banner.value = {
      kind: 'ok',
      text: `已删除 ${defaults.restoreFile}。改动要等机器人控制器重启后才会生效。`,
    }
    rebootNotice.value = true
  })
}
</script>

<template>
  <div class="page">
    <header class="page-head">
      <h1 class="page-title">网络配置</h1>
      <p class="page-sub">按机柜面板网口改地址：先连设备，再点可改的网口，最后下发。</p>
    </header>

    <!-- 配置告警独立一条，不与操作结果的 banner 抢位置，否则点一次操作就被冲掉。 -->
    <div v-if="configWarning" class="banner err">{{ configWarning }}</div>

    <!-- ① 连接：地址在左，两个主操作按钮并排在右。「一键恢复」是现场重点工具，
         用实心危险按钮突出，不能缩成文字链。 -->
    <section class="panel">
      <div class="step-label">1 · 连接设备</div>
      <div class="connect-row">
        <div class="field grow">
          <label for="host">设备地址</label>
          <input id="host" v-model.trim="device.host" placeholder="192.168.1.100" />
        </div>
        <div class="connect-actions">
          <button class="primary connect-btn" :disabled="busy !== ''" @click="test">
            {{ busy === 'test' ? '连接中…' : '测试连接' }}
          </button>
          <button class="danger connect-btn restore-btn" :disabled="busy !== ''" @click="restore">
            {{ busy === 'restore' ? '恢复中…' : '一键恢复网络' }}
          </button>
        </div>
      </div>
    </section>

    <div v-if="banner" class="banner" :class="banner.kind">{{ banner.text }}</div>

    <!-- ② 选口：五个面板口并排，IP 放大；可改的才像能点，只读的压暗。 -->
    <section v-if="ports.length" class="panel">
      <div class="step-head">
        <div class="step-label">2 · 选择网口</div>
        <p v-if="editable.length" class="step-hint">
          可改：{{ editable.map((p) => p.name).join('、') }} · 其余只读
        </p>
        <p v-else class="step-hint">这台设备上没有可以在这里改地址的网口</p>
      </div>

      <div class="port-grid">
        <button
          v-for="p in ports"
          :key="p.name"
          type="button"
          class="port-card"
          :class="{
            selected: p.name === selected,
            editable: p.editable,
            readonly: p.iface && !p.editable,
            blank: !p.iface,
          }"
          :disabled="!p.editable"
          @click="select(p)"
        >
          <div class="port-top">
            <span class="port-name">{{ p.name }}</span>
            <span v-if="p.iface" class="port-status" :class="{ up: p.up }">
              {{ p.up ? 'UP' : 'DOWN' }}
            </span>
            <span v-else class="port-status muted">—</span>
          </div>

          <div class="port-ip">{{ p.ip || '—' }}</div>

          <div class="port-meta">
            <template v-if="p.iface">
              <span>{{ p.mask || '—' }}</span>
              <span v-if="p.gateway">网关 {{ p.gateway }}</span>
            </template>
            <template v-else>不由本工具管理</template>
          </div>

          <div class="port-badge">
            <template v-if="p.editable">可改</template>
            <template v-else-if="p.iface">只读</template>
            <template v-else>空</template>
          </div>
        </button>
      </div>
    </section>

    <!-- ③ 改地址：选中后才出现，标题直接点名网口。 -->
    <section v-if="selected" class="panel edit-panel">
      <div class="step-label">3 · 修改 {{ selected }} 的地址</div>
      <p v-if="siblings.length" class="sibling-note">
        {{ siblings.join('、') }} 是同一网口，改一个另外几个一起变。
      </p>

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
          <span class="hint">下发后当前连接会立即断开，请确认新地址与本机在同一网段。</span>
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
  max-width: 920px;
}

.page-head {
  margin-bottom: 18px;
}

.page-title {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
}

.page-sub {
  margin: 6px 0 0;
  font-size: 13px;
  color: var(--text-dim);
  line-height: 1.45;
}

.panel {
  margin-bottom: 14px;
  padding: 16px 18px;
  background: var(--panel);
  border: 1px solid var(--border);
  border-radius: var(--radius);
}

.step-label {
  margin: 0 0 12px;
  font-size: 13px;
  font-weight: 700;
  color: var(--text);
  letter-spacing: 0.02em;
}

.step-head {
  display: flex;
  align-items: baseline;
  flex-wrap: wrap;
  gap: 8px 14px;
  margin-bottom: 12px;
}

.step-head .step-label {
  margin: 0;
}

.step-hint {
  margin: 0;
  font-size: 12px;
  color: var(--text-dim);
}

.connect-row {
  display: flex;
  align-items: flex-end;
  flex-wrap: wrap;
  gap: 10px 12px;
}

.field.grow {
  flex: 1 1 240px;
  max-width: 320px;
}

.connect-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
}

.connect-btn {
  flex-shrink: 0;
  height: 36px;
  padding: 0 18px;
  font-size: 13px;
  font-weight: 600;
}

/* 现场重点工具：比普通 danger 再大一点、字重更重，一眼能找到。 */
.restore-btn {
  min-width: 132px;
  letter-spacing: 0.02em;
}

.port-grid {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 10px;
}

.port-card {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 8px;
  min-height: 118px;
  padding: 12px;
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: #fff;
  text-align: left;
  font-family: inherit;
  color: inherit;
}

.port-card.editable {
  cursor: pointer;
}

.port-card.editable:hover {
  border-color: #c8ccd3;
  background: var(--bg);
}

.port-card.selected {
  border-color: var(--accent);
  background: var(--accent-soft);
  box-shadow: inset 0 0 0 1px var(--accent);
}

.port-card.readonly,
.port-card.blank {
  cursor: default;
  background: #fafbfc;
  color: var(--text-dim);
}

.port-card:disabled {
  cursor: default;
}

.port-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
}

.port-name {
  font-size: 14px;
  font-weight: 700;
  color: var(--text);
}

.port-card.readonly .port-name,
.port-card.blank .port-name {
  color: var(--text-dim);
}

.port-status {
  padding: 1px 7px;
  border-radius: 10px;
  font-size: 11px;
  background: var(--bg);
  color: var(--text-dim);
}

.port-status.up {
  background: var(--ok-soft);
  color: var(--ok);
}

.port-status.muted {
  background: transparent;
  padding: 0;
}

/* IP 是现场最要看的数，放大并排成等宽数字。 */
.port-ip {
  font-size: 15px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
  letter-spacing: 0.01em;
  color: var(--text);
  word-break: break-all;
}

.port-card.readonly .port-ip,
.port-card.blank .port-ip {
  color: var(--text-dim);
  font-weight: 500;
}

.port-meta {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 11px;
  line-height: 1.35;
  color: var(--text-dim);
}

.port-badge {
  margin-top: auto;
  font-size: 11px;
  font-weight: 600;
}

.port-card.editable .port-badge {
  color: var(--accent);
}

.port-card.selected .port-badge {
  color: var(--accent);
}

.port-card.readonly .port-badge,
.port-card.blank .port-badge {
  color: var(--text-dim);
  font-weight: 500;
}

.edit-panel {
  border-color: #c9d8f5;
  background: #f8faff;
}

.sibling-note {
  margin: -4px 0 14px;
  font-size: 12px;
  color: var(--text-dim);
}

.banner {
  margin-bottom: 14px;
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

@media (max-width: 900px) {
  .port-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }
}

@media (max-width: 640px) {
  .port-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .connect-row {
    flex-wrap: wrap;
  }

  .field.grow {
    max-width: none;
  }
}
</style>

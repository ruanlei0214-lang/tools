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

// 提示里说的是面板口名。持久化配置项写的是系统网口名（br0），那个名字界面上不该出现。
const persistPorts = computed(() =>
  defaults.persistIface === ''
    ? []
    : ports.value.filter((p) => p.iface === defaults.persistIface).map((p) => p.name),
)

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
  <div class="module-head">
    <h1 class="module-title">网络配置</h1>
  </div>

  <!-- 配置告警独立一条，不与操作结果的 banner 抢位置，否则点一次操作就被冲掉。 -->
  <div v-if="configWarning" class="banner err config-warning">{{ configWarning }}</div>

  <section class="card">
    <h2 class="card-title">设备连接</h2>
    <div class="field-row">
      <div class="field" style="flex: 0 1 240px">
        <label for="host">设备地址</label>
        <input id="host" v-model.trim="device.host" placeholder="192.168.1.100" />
      </div>
    </div>
    <div class="actions">
      <button class="primary" :disabled="busy !== ''" @click="test">
        {{ busy === 'test' ? '连接中…' : '测试连接' }}
      </button>
      <button class="danger" :disabled="busy !== ''" @click="restore">
        {{ busy === 'restore' ? '恢复中…' : '一键恢复网络' }}
      </button>
    </div>
  </section>

  <section v-if="ports.length" class="card">
    <h2 class="card-title">网口（点击选择要修改的网口）</h2>
    <p class="hint table-note">
      <template v-if="editable.length">
        这台设备上只有 {{ editable.map((p) => p.name).join('、') }} 可以在这里改地址，其余网口只读。
      </template>
      <template v-else>这台设备上没有可以在这里改地址的网口。</template>
    </p>
    <table>
      <thead>
        <tr>
          <th>网口</th>
          <th>状态</th>
          <th>IP 地址</th>
          <th>子网掩码</th>
          <th>网关</th>
          <th>MAC</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="p in ports"
          :key="p.name"
          :class="{ selected: p.name === selected, blank: !p.iface, readonly: !p.editable }"
          @click="select(p)"
        >
          <td>
            {{ p.name }}
            <span v-if="p.iface && !p.editable" class="ro-tag">只读</span>
          </td>
          <td>
            <span v-if="p.iface" class="tag" :class="{ up: p.up }">{{ p.up ? 'UP' : 'DOWN' }}</span>
            <span v-else>—</span>
          </td>
          <td>{{ p.ip || '—' }}</td>
          <td>{{ p.mask || '—' }}</td>
          <td>{{ p.gateway || '—' }}</td>
          <td>{{ p.mac || '—' }}</td>
        </tr>
      </tbody>
    </table>
  </section>

  <section v-if="selected" class="card">
    <h2 class="card-title">修改 {{ selected }} 的地址</h2>
    <p v-if="siblings.length" class="hint sibling-note">
      {{ siblings.join('、') }} 在这台设备上是同一个网口，改一个，另外几个跟着一起变。
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
        <span v-if="willPersist" class="hint">
          配置会写入设备上的 {{ defaults.restoreFile }}，重启后仍然生效。
        </span>
        <span v-else class="hint">
          配置只在设备运行期间生效，重启后就没了<template v-if="persistPorts.length">（只有
          {{ persistPorts.join('、') }} 会被持久化）</template>。
        </span>
      </template>
    </div>
  </section>

  <div v-if="banner" class="banner" :class="banner.kind">{{ banner.text }}</div>

  <div v-if="rebootNotice" class="modal-mask">
    <div class="modal">
      <h2 class="modal-title">请重启机器人控制器</h2>
      <div class="modal-actions">
        <button class="primary" @click="rebootNotice = false">知道了</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.config-warning {
  margin-bottom: 16px;
}

.sibling-note {
  margin: -4px 0 14px;
}

.table-note {
  margin: -4px 0 12px;
}

.ro-tag {
  margin-left: 6px;
  font-size: 11px;
  color: var(--text-dim);
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
</style>

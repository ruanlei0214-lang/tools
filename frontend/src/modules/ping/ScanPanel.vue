<script lang="ts" setup>
import { computed, onMounted, ref } from 'vue'
import { LocalIfaces, Scan } from '../../../wailsjs/go/ping/Service'
import type { ping } from '../../../wailsjs/go/models'

// 扫描目标拆成「前三段 + 末段起止」三个框：网卡下拉负责填前三段，
// 末段范围默认 1–254，想扫哪一段改数字就行。前端拼成 192.168.1.10-100
// 交给后端，区间解析后端已经会做。
const ifaces = ref<ping.LocalIface[]>([])
const prefix = ref('')
const rangeFrom = ref('1')
const rangeTo = ref('254')
const scanBusy = ref(false)
const scanStatus = ref<{ kind: 'ok' | 'err' | 'info'; text: string } | null>(null)
const hosts = ref<ping.ScanHost[]>([])
const conflicts = ref<ping.Conflict[]>([])

// 冲突涉及的 IP / MAC 各收成一个集合，表格里对应的行高亮。
const conflictIPs = computed(
  () => new Set(conflicts.value.flatMap((c) => (c.kind === 'ip' ? [c.addr] : c.peers))),
)
const conflictMACs = computed(
  () => new Set(conflicts.value.flatMap((c) => (c.kind === 'mac' ? [c.addr] : c.peers))),
)

function conflictText(c: ping.Conflict) {
  return c.kind === 'ip'
    ? `IP 冲突：${c.addr} 被多个 MAC 应答：${c.peers.join('、')}`
    : `MAC 冲突：${c.addr} 对应多个 IP：${c.peers.join('、')}`
}

onMounted(async () => {
  try {
    ifaces.value = (await LocalIfaces()) ?? []
    if (ifaces.value.length) prefix.value = ifaces.value[0].segment
  } catch {
    // 预填不上就留空，不挡用户手输。
  }
})

// composeTarget 把三个框拼成后端认识的区间写法。前三段里多带了末段
// （比如直接粘了个完整 IP）就砍掉，以两个数字框为准。
function composeTarget(): string {
  const parts = prefix.value.trim().split('.')
  const base = parts.slice(0, 3).join('.')
  const from = rangeFrom.value.trim() || '1'
  const to = rangeTo.value.trim() || '254'
  return `${base}.${from}-${to}`
}

async function doScan() {
  if (scanBusy.value) return
  const target = composeTarget()
  scanBusy.value = true
  hosts.value = []
  conflicts.value = []
  scanStatus.value = { kind: 'info', text: '扫描中…' }
  try {
    const r = await Scan(target)
    hosts.value = r.hosts ?? []
    conflicts.value = r.conflicts ?? []
    const base = `在线 ${hosts.value.length} 台 / 共扫 ${r.total} 个地址，用时 ${(r.elapsedMs / 1000).toFixed(1)}s`
    scanStatus.value = conflicts.value.length
      ? { kind: 'err', text: `${base}，发现 ${conflicts.value.length} 处冲突` }
      : { kind: hosts.value.length ? 'ok' : 'err', text: base }
  } catch (e) {
    scanStatus.value = { kind: 'err', text: String(e) }
  } finally {
    scanBusy.value = false
  }
}
</script>

<template>
  <section class="panel">
    <div class="op-row">
      <select
        v-if="ifaces.length"
        v-model="prefix"
        class="iface-select"
        title="选择要扫的网卡，选中后填入它的网段"
      >
        <option v-for="f in ifaces" :key="f.name + f.ip" :value="f.segment">
          {{ f.name }}（{{ f.ip }}）
        </option>
      </select>
      <input
        v-model="prefix"
        class="prefix-input"
        placeholder="192.168.1"
        @keydown.enter="doScan"
      />
      <span class="range-dot">.</span>
      <input
        v-model="rangeFrom"
        class="range-input"
        type="number"
        min="0"
        max="255"
        title="末段起点"
        @keydown.enter="doScan"
      />
      <span class="range-sep">~</span>
      <input
        v-model="rangeTo"
        class="range-input"
        type="number"
        min="0"
        max="255"
        title="末段终点"
        @keydown.enter="doScan"
      />
      <button class="primary op-btn" :disabled="scanBusy || !prefix.trim()" @click="doScan">
        {{ scanBusy ? '扫描中…' : '开始扫描' }}
      </button>
      <div class="status" :class="scanStatus?.kind" :title="scanStatus?.text" aria-live="polite">
        {{ scanStatus?.text ?? '' }}
      </div>
    </div>

    <div v-if="conflicts.length" class="conflict-list">
      <div v-for="c in conflicts" :key="c.kind + c.addr" class="conflict-row">
        {{ conflictText(c) }}
      </div>
    </div>

    <table v-if="hosts.length" class="result-table">
      <thead>
        <tr>
          <th>IP</th>
          <th>设备名</th>
          <th>MAC</th>
          <th>延迟</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="h in hosts"
          :key="h.ip"
          :class="{ bad: conflictIPs.has(h.ip) || (!!h.mac && conflictMACs.has(h.mac)) }"
        >
          <td class="mono ip">{{ h.ip }}</td>
          <td>{{ h.name || '—' }}</td>
          <td class="mono">{{ h.mac || '—' }}</td>
          <td class="mono">{{ h.rttMs.toFixed(1) }}ms</td>
        </tr>
      </tbody>
    </table>
  </section>
</template>

<style scoped>
.panel {
  padding: 12px 14px;
  background: var(--panel);
  border: 1px solid var(--border);
  border-radius: var(--radius);
}

.op-row {
  display: flex;
  align-items: center;
  flex-wrap: nowrap;
  gap: 8px;
}

.prefix-input {
  width: 88px;
  flex-shrink: 0;
}

/* 网卡下拉：宽度跟着内容走，别挤占状态栏。 */
.iface-select {
  flex-shrink: 0;
  max-width: 200px;
  height: 30px;
  font-size: 12px;
}

.range-dot,
.range-sep {
  flex-shrink: 0;
  margin: 0 -4px;
  color: var(--text-dim);
  font-size: 12px;
}

.range-input {
  width: 64px;
  flex-shrink: 0;
  text-align: center;
}

.op-btn {
  flex-shrink: 0;
  height: 30px;
  padding: 0 14px;
  font-size: 12px;
  font-weight: 600;
}

/* 状态占掉操作行剩下的宽度，高度固定，长消息截断、完整内容挂 title。 */
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

.result-table {
  width: 100%;
  margin-top: 10px;
  border-collapse: collapse;
  font-size: 12px;
}

.result-table th {
  padding: 4px 10px;
  text-align: left;
  font-weight: 600;
  color: var(--text-dim);
  border-bottom: 1px solid var(--border);
}

.result-table td {
  padding: 5px 10px;
  border-bottom: 1px solid var(--border);
  color: var(--text);
  user-select: text;
}

.result-table tbody tr:last-child td {
  border-bottom: none;
}

.mono {
  font-variant-numeric: tabular-nums;
}

.result-table .ip {
  font-weight: 600;
}

/* 冲突提示用琥珀色：是要人处理的警告，但不是操作失败。 */
.conflict-list {
  margin-top: 10px;
  padding: 6px 10px;
  background: #fef3c7;
  border: 1px solid #fcd34d;
  border-radius: 6px;
}

.conflict-row {
  font-size: 12px;
  line-height: 1.8;
  color: #92400e;
  user-select: text;
}

.result-table tr.bad td {
  background: #fef3c7;
}

@media (max-width: 720px) {
  .op-row {
    flex-wrap: wrap;
  }

  .status {
    flex: 1 1 100%;
  }
}
</style>

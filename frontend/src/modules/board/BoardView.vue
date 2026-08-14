<script lang="ts" setup>
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { Config, Connect, Disconnect, PickKeyFile, Status } from '../../../wailsjs/go/board/Service'
import type { board } from '../../../wailsjs/go/models'
import CommandPanel from './CommandPanel.vue'
import FilePanel from './FilePanel.vue'

const device = reactive<board.Device>({ host: '', port: 22, user: 'root', password: '', keyPath: '' })
const defaultPath = ref('/opt')
const syncPath = ref('')
const fileWidth = ref(320)
const splitting = ref(false)
const connected = ref(false)
const busy = ref('')
const configWarning = ref('')
const banner = ref<{ kind: 'ok' | 'err' | 'info'; text: string } | null>(null)

// 配置告警与操作结果共用一条状态槽：告警优先，操作只清 banner，冲不掉告警。
const status = computed(() => {
  if (configWarning.value) return { kind: 'err' as const, text: configWarning.value }
  return banner.value
})

onMounted(async () => {
  try {
    const cfg = await Config()
    device.host = cfg.device.host
    device.port = cfg.device.port || 22
    device.user = cfg.device.user
    device.password = cfg.device.password
    device.keyPath = cfg.device.keyPath || ''
    defaultPath.value = cfg.defaultPath
    configWarning.value = cfg.warning
  } catch (e) {
    configWarning.value = `读取配置失败：${String(e)}`
    return
  }
  // 不自动连接。开机时设备还没起来，一打开就连会把整个程序卡住；
  // 这里的按钮又是重启进程、删文件这类做过就回不去的事，得人自己按。
  await syncStatus()
})

// 连接活在后端，组件销毁不会自动收；不断开的话切走页面还挂着一条 SSH 连接。
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
      port: Number(device.port) || 22,
      user: device.user.trim(),
      password: device.password,
      keyPath: device.keyPath,
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

// 子面板每次调用失败都会喊一声：连接可能是被设备单方面断掉的（重启、拔网线），
// 那种情况下只有后端知道，界面得跟着把状态改回未连接。
async function syncStatus() {
  try {
    const st: board.Status = await Status()
    connected.value = st.connected
    if (!st.connected && st.error) {
      banner.value = { kind: 'err', text: st.error }
    }
  } catch {
    connected.value = false
  }
}

const keyName = computed(() => {
  const p = device.keyPath
  if (!p) return ''
  const i = Math.max(p.lastIndexOf('/'), p.lastIndexOf('\\'))
  return i >= 0 ? p.slice(i + 1) : p
})

function pickKey() {
  return call('key', async () => {
    const path = await PickKeyFile()
    if (!path) return
    device.keyPath = path
  })
}

function onCwd(p: string) {
  syncPath.value = p
}

function onSplitDown(e: MouseEvent) {
  splitting.value = true
  const startX = e.clientX
  const startW = fileWidth.value
  const onMove = (ev: MouseEvent) => {
    fileWidth.value = Math.min(Math.max(startW + ev.clientX - startX, 200), window.innerWidth - 380)
  }
  const onUp = () => {
    splitting.value = false
    window.removeEventListener('mousemove', onMove)
    window.removeEventListener('mouseup', onUp)
  }
  window.addEventListener('mousemove', onMove)
  window.addEventListener('mouseup', onUp)
}
</script>

<template>
  <div class="board-page">
  <section class="card conn-card">
    <div class="conn-row">
      <h2 class="card-title">主板连接</h2>
      <input
        id="board-host"
        v-model.trim="device.host"
        class="conn-host"
        aria-label="IP"
        placeholder="IP"
        :disabled="!!busy"
      />
      <input
        id="board-user"
        v-model.trim="device.user"
        class="conn-user"
        aria-label="用户"
        placeholder="用户"
        :disabled="!!busy"
      />
      <input
        id="board-pass"
        v-model="device.password"
        class="conn-pass"
        aria-label="密码"
        type="password"
        placeholder="密码"
        :disabled="!!busy"
      />
      <button
        class="conn-btn conn-key"
        :class="{ on: !!device.keyPath }"
        :title="device.keyPath || '选择私钥'"
        :disabled="!!busy"
        @click="pickKey"
      >
        {{ keyName || '密钥' }}
      </button>
      <button
        v-if="device.keyPath"
        class="conn-btn"
        title="清除密钥"
        :disabled="!!busy"
        @click="device.keyPath = ''"
      >
        ×
      </button>
      <button
        class="primary conn-btn"
        :disabled="!!busy || !device.host.trim() || !device.user.trim()"
        @click="connect"
      >
        {{ busy === 'connect' ? '连接中…' : connected ? '重连' : '连接' }}
      </button>
      <button class="conn-btn" :disabled="!!busy || !connected" @click="disconnect">断开</button>
      <div class="status" :class="status?.kind" :title="status?.text">{{ status?.text }}</div>
    </div>
  </section>

  <div class="workspace" :class="{ splitting }">
    <div class="pane pane-file" :style="{ width: `${fileWidth}px` }">
      <FilePanel
        :connected="connected"
        :default-path="defaultPath"
        :sync-path="syncPath"
        @refresh-status="syncStatus"
      />
    </div>
    <div class="splitter" title="拖动调整宽度" @mousedown.prevent="onSplitDown" />
    <div class="pane pane-term">
      <CommandPanel :connected="connected" @refresh-status="syncStatus" @cwd="onCwd" />
    </div>
  </div>
  </div>
</template>

<style scoped>
.board-page {
  display: flex;
  flex-direction: column;
  gap: 8px;
  height: calc(100vh - 88px);
  min-height: 0;
}

.workspace {
  display: flex;
  flex: 1 1 auto;
  min-height: 0;
}

.workspace.splitting {
  user-select: none;
  cursor: col-resize;
}

.pane {
  min-height: 0;
  height: 100%;
}

.pane-file {
  flex: 0 0 auto;
}

.pane-term {
  flex: 1 1 auto;
  min-width: 280px;
}

.pane > :deep(*) {
  height: 100%;
}

.splitter {
  flex: 0 0 6px;
  margin: 0 2px;
  border-radius: 3px;
  background: var(--border);
  cursor: col-resize;
}

.splitter:hover,
.workspace.splitting .splitter {
  background: var(--accent);
}

.conn-card {
  margin-bottom: 8px;
  padding: 6px 8px;
}

.conn-row {
  display: flex;
  flex-wrap: nowrap;
  align-items: center;
  gap: 6px;
}

/* 标题占定宽不参与压缩：这一行挤的时候先压地址框，标题被压成半个字最难看。 */
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
  flex: 0 1 8.5rem;
  width: 8.5rem;
}

.conn-user {
  flex: 0 1 5rem;
  width: 5rem;
}

.conn-pass {
  flex: 0 1 6rem;
  width: 6rem;
}

.conn-key {
  max-width: 7rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.conn-key.on {
  border-color: var(--accent);
  color: var(--accent);
}

.conn-btn {
  flex: 0 0 auto;
  padding: 4px 10px;
  font-size: 12px;
}

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
</style>

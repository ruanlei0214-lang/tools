<script lang="ts" setup>
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { Config, Connect, Disconnect, Status } from '../../../wailsjs/go/board/Service'
import type { board } from '../../../wailsjs/go/models'
import CommandPanel from './CommandPanel.vue'
import FilePanel from './FilePanel.vue'

// 四项都放在界面上而不是只藏在配置里：现场换设备、换端口、换登录用户是常事。
const device = reactive<board.Device>({ host: '', port: 22, user: 'root', password: '' })
const showPassword = ref(false)
const defaultPath = ref('/opt')
const activeId = ref<'command' | 'file'>('command')
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
</script>

<template>
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
        id="board-port"
        v-model.number="device.port"
        class="conn-port"
        aria-label="端口"
        type="number"
        min="1"
        max="65535"
        placeholder="端口"
        :disabled="!!busy"
      />
      <input
        id="board-user"
        v-model.trim="device.user"
        class="conn-user"
        aria-label="用户名"
        placeholder="用户"
        :disabled="!!busy"
      />
      <div class="pass-box">
        <input
          id="board-pass"
          v-model="device.password"
          aria-label="密码"
          :type="showPassword ? 'text' : 'password'"
          placeholder="密码"
          :disabled="!!busy"
        />
        <!-- 空密码在密码框里和「没填」长得一模一样，得能看一眼确认。 -->
        <button
          class="peek"
          type="button"
          :title="showPassword ? '隐藏密码' : '显示密码'"
          :aria-label="showPassword ? '隐藏密码' : '显示密码'"
          @click="showPassword = !showPassword"
        >
          <svg
            viewBox="0 0 24 24"
            width="14"
            height="14"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
          >
            <path d="M2 12s3.6-6.5 10-6.5S22 12 22 12s-3.6 6.5-10 6.5S2 12 2 12z" />
            <circle cx="12" cy="12" r="2.6" />
            <path v-if="showPassword" d="M4.5 19.5 19.5 4.5" />
          </svg>
        </button>
      </div>
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

  <nav class="tabs">
    <button class="tab" :class="{ active: activeId === 'command' }" @click="activeId = 'command'">
      指令
    </button>
    <button class="tab" :class="{ active: activeId === 'file' }" @click="activeId = 'file'">
      文件
    </button>
  </nav>

  <!-- 用 v-show 不用 v-if：切一下标签页就把执行日志和列好的目录清空，
       而这两样正是要对着看的东西。两个面板都不在挂载时联网，留着不占什么。 -->
  <CommandPanel v-show="activeId === 'command'" :connected="connected" @refresh-status="syncStatus" />
  <FilePanel
    v-show="activeId === 'file'"
    :connected="connected"
    :default-path="defaultPath"
    @refresh-status="syncStatus"
  />
</template>

<style scoped>
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

/* 标题占定宽不参与压缩：这一行挤的时候先压地址和密码框，标题被压成半个字最难看。 */
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

.conn-user {
  flex: 0 1 5.5rem;
  width: 5.5rem;
}

.pass-box {
  position: relative;
  flex: 0 1 7.5rem;
  width: 7.5rem;
}

.pass-box input {
  padding-right: 26px;
}

.peek {
  position: absolute;
  top: 1px;
  right: 1px;
  bottom: 1px;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 24px;
  padding: 0;
  border: none;
  border-radius: 0 5px 5px 0;
  background: none;
  color: var(--text-dim);
}

.peek:hover {
  color: var(--text);
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

.tabs {
  display: flex;
  gap: 2px;
  margin-bottom: 10px;
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

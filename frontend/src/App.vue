<script lang="ts" setup>
import { computed, onMounted, ref } from 'vue'
import { WindowSetAlwaysOnTop } from '../wailsjs/runtime/runtime'
import { modules } from './shell/registry'
import { APP_NAME, APP_VERSION } from './shell/version'
import { conn, connectAll, disconnectAll, loadShared, persistHost } from './shell/connection'

const activeId = ref(modules.find((m) => m.id === 'netcfg')?.id ?? modules[0]?.id ?? '')
const active = computed(() => modules.find((m) => m.id === activeId.value))
const showAbout = ref(false)
const alwaysOnTopKey = 'embedtools.alwaysOnTop'
const alwaysOnTop = ref(localStorage.getItem(alwaysOnTopKey) === '1')
const connMsg = ref<{ kind: 'ok' | 'err' | 'info'; text: string } | null>(null)

// 一条连接还挂着就算「已连接」：按钮文案跟着变成断开，点一下两条一起收。
const anyConnected = computed(() => conn.sshConnected || conn.wsConnected)

onMounted(async () => {
  applyAlwaysOnTop(alwaysOnTop.value)
  // 不自动连接。开机时设备还没起来，一打开就连会把整个程序卡住；
  // 这里的按钮连着重启进程、删文件这类做过就回不去的事，得人自己按。
  try {
    await loadShared()
  } catch (e) {
    connMsg.value = { kind: 'err', text: `读取共享配置失败：${String(e)}` }
  }
})

function toggleAlwaysOnTop() {
  const next = !alwaysOnTop.value
  applyAlwaysOnTop(next)
  alwaysOnTop.value = next
  localStorage.setItem(alwaysOnTopKey, next ? '1' : '0')
}

function applyAlwaysOnTop(on: boolean) {
  WindowSetAlwaysOnTop(on)
}

async function commitHost() {
  const next = conn.host.trim()
  if (!next) {
    connMsg.value = { kind: 'err', text: '请填写设备地址' }
    try {
      await loadShared()
    } catch {
      conn.host = ''
    }
    return
  }
  try {
    await persistHost(next)
  } catch (e) {
    connMsg.value = { kind: 'err', text: `写入地址失败：${String(e)}` }
  }
}

function toggleConnect() {
  connMsg.value = null
  if (anyConnected.value) {
    // 断开是正常操作，状态灯自己会变灰，不用再说一句。
    disconnectAll()
    return
  }
  connectAll().then((r) => {
    // 成功不提示，灯亮了就够了；只有失败才占这一格。
    if (r.kind === 'err') connMsg.value = r
  })
}
</script>

<template>
  <div class="shell">
    <header class="topbar">
      <nav class="top-nav" aria-label="模块">
        <button
          v-for="m in modules"
          :key="m.id"
          class="module-shortcut"
          :class="{ active: m.id === activeId }"
          :title="m.description"
          @click="activeId = m.id"
        >
          {{ m.name }}
        </button>
      </nav>

      <!-- 全系列工具共用一台控制器：地址只在这里显示，连接也只在这里按。
           SSH 和 WS 各一个状态点，灰的没连、绿的已连、红的刚断过。
           被 profile 裁掉的协议不显示（比如 netcfg-only 产物没有这两个点）。 -->
      <div v-if="conn.hasSsh || conn.hasWs" class="conn-cluster">
        <span
          v-if="conn.hasSsh"
          class="proto"
          :class="{ on: conn.sshConnected, err: !conn.sshConnected && !!conn.sshError }"
          :title="conn.sshConnected ? `SSH 已连接 ${conn.sshAddr}` : conn.sshError || 'SSH 未连接'"
        >
          <span class="dot" />SSH
        </span>
        <span
          v-if="conn.hasWs"
          class="proto"
          :class="{ on: conn.wsConnected, err: !conn.wsConnected && !!conn.wsError }"
          :title="conn.wsConnected ? `WS 已连接 ${conn.wsAddr}` : conn.wsError || 'WS 未连接'"
        >
          <span class="dot" />WS
        </span>
        <input
          v-model="conn.host"
          class="conn-host"
          aria-label="设备地址"
          placeholder="设备 IP"
          spellcheck="false"
          :disabled="!!conn.busy"
          title="改完回车或点别处即写入 toolbox-config.json，重新连接后生效"
          @change="commitHost"
          @keyup.enter="commitHost"
        />
        <button
          class="primary conn-toggle"
          :disabled="!!conn.busy || (!anyConnected && (!conn.host.trim() || !conn.user.trim()))"
          @click="toggleConnect"
        >
          {{ conn.busy === 'connect' ? '连接中…' : conn.busy === 'disconnect' ? '断开中…' : anyConnected ? '断开' : '连接' }}
        </button>
        <span v-if="connMsg" class="conn-msg" :class="connMsg.kind" :title="connMsg.text">{{ connMsg.text }}</span>
      </div>

      <div class="top-actions">
        <button class="about-entry" title="关于工具箱" @click="showAbout = true">关于</button>
        <button
          class="topmost"
          :class="{ active: alwaysOnTop }"
          type="button"
          :title="alwaysOnTop ? '取消窗口置顶' : '让窗口始终显示在最上层'"
          :aria-label="alwaysOnTop ? '取消窗口置顶' : '窗口置顶'"
          :aria-pressed="alwaysOnTop"
          @click="toggleAlwaysOnTop"
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
            <path :fill="alwaysOnTop ? 'currentColor' : 'none'" d="m9 4 6 0-1 5 3 3H7l3-3-1-5Z" />
            <path d="M12 12v8" />
          </svg>
        </button>
      </div>
    </header>

    <main class="content">
      <keep-alive>
        <component :is="active.view" v-if="active" :key="active.id" />
      </keep-alive>
      <p v-if="!active" class="empty">还没有任何模块，在 src/modules/ 下新建一个目录即可。</p>
    </main>
  </div>

  <div v-if="showAbout" class="modal-mask" @click.self="showAbout = false">
    <div class="modal">
      <h2 class="modal-title">关于</h2>
      <!-- 分两组：工具箱整体，和当前所在的模块。只讲当前模块——模块各自独立编号，
           把没在用的模块也铺出来，等于让人在一堆数字里找自己要的那个。 -->
      <section class="about-group">
        <h3 class="about-group-title">工具箱</h3>
        <dl class="about-list">
          <dt>名称</dt>
          <dd>{{ APP_NAME }}</dd>
          <dt>版本</dt>
          <dd class="about-ver">{{ APP_VERSION }}</dd>
        </dl>
      </section>
      <section v-if="active" class="about-group divided">
        <h3 class="about-group-title">当前模块</h3>
        <dl class="about-list">
          <dt>名称</dt>
          <dd>{{ active.name }}</dd>
          <dt>标识</dt>
          <dd class="about-id">{{ active.id }}</dd>
          <dt>版本</dt>
          <dd class="about-ver">{{ active.version }}</dd>
          <dt>说明</dt>
          <dd>{{ active.description }}</dd>
        </dl>
      </section>
      <div class="modal-actions">
        <button class="primary" @click="showAbout = false">知道了</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.topbar {
  display: flex;
  align-items: center;
  min-height: 50px;
  padding: 7px 12px 7px 16px;
  border-bottom: 1px solid var(--border);
  background: var(--panel);
}

.top-nav {
  display: flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
  overflow-x: auto;
}

.module-shortcut {
  flex: 0 0 auto;
  padding: 7px 13px;
  border-color: transparent;
  background: transparent;
  color: var(--text-dim);
}

.module-shortcut:hover:not(:disabled) {
  border-color: var(--border);
  color: var(--text);
}

.module-shortcut.active {
  border-color: var(--accent);
  background: var(--accent-soft);
  color: var(--accent);
  font-weight: 600;
}

/* 连接区在导航和右侧按钮之间，吃掉中间剩下的宽度。
   消息长了截断，完整内容挂 title——顶栏高度不能跟着消息变。 */
.conn-cluster {
  display: flex;
  flex: 1 1 auto;
  align-items: center;
  gap: 8px;
  min-width: 0;
  margin-left: 12px;
  padding-left: 12px;
  border-left: 1px solid var(--border);
}

.proto {
  display: inline-flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 4px;
  font-size: 11px;
  font-weight: 600;
  color: var(--text-dim);
  cursor: default;
}

.proto .dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #c8ccd3;
}

.proto.on {
  color: var(--ok);
}

.proto.on .dot {
  background: var(--ok);
}

.proto.err .dot {
  background: var(--err);
}

.conn-host {
  flex: 0 1 10rem;
  min-width: 7rem;
  width: 10rem;
  padding: 3px 7px;
  border: 1px solid var(--border);
  border-radius: 5px;
  background: #fff;
  overflow: hidden;
  font-size: 12px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.conn-host:focus {
  outline: none;
  border-color: var(--accent);
  box-shadow: 0 0 0 3px var(--accent-soft);
}

.conn-host:disabled {
  background: var(--bg);
  color: var(--text-dim);
}

.conn-toggle {
  flex: 0 0 auto;
  padding: 4px 14px;
  font-size: 12px;
}

.conn-msg {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  font-size: 11px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.conn-msg.ok {
  color: var(--ok);
}

.conn-msg.err {
  color: var(--err);
}

.conn-msg.info {
  color: var(--accent);
}

.top-actions {
  display: flex;
  align-items: center;
  gap: 5px;
  margin-left: auto;
  padding-left: 10px;
}

.topmost {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  padding: 0;
  border-color: var(--border);
  background: rgba(255, 255, 255, 0.94);
  color: var(--text-dim);
}

.topmost:hover:not(:disabled) {
  border-color: var(--accent);
  color: var(--accent);
}

.topmost.active {
  border-color: var(--accent);
  background: var(--accent);
  color: #fff;
}

.topmost.active:hover:not(:disabled) {
  border-color: #1d4ed8;
  background: #1d4ed8;
  color: #fff;
}

.about-entry {
  padding: 5px 8px;
  border-color: transparent;
  background: transparent;
  color: var(--text-dim);
  font-size: 11px;
}

.about-entry:hover:not(:disabled) {
  border-color: var(--border);
  color: var(--text);
  background: var(--bg);
}

/* 弹窗样式没进全局，和 netcfg 里那个各留一份。项目没有 UI 组件库，
   把它提成共享控件就等于凭空造一层所有人都得依赖的东西。 */
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

.about-group {
  margin-top: 16px;
}

.about-group.divided {
  padding-top: 14px;
  border-top: 1px solid var(--border);
}

.about-group-title {
  margin: 0 0 8px;
  font-size: 12px;
  font-weight: 600;
  color: var(--text-dim);
}

/* 标签窄列在左、值左对齐：说明这类长文本右对齐会散，而且值一多就对不齐。 */
.about-list {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 6px 14px;
  margin: 0;
  font-size: 13px;
  line-height: 1.5;
}

.about-list dt {
  color: var(--text-dim);
}

.about-list dd {
  margin: 0;
}

.about-ver {
  font-variant-numeric: tabular-nums;
}

.about-id {
  font-family: ui-monospace, Consolas, monospace;
  font-size: 12px;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 18px;
}
</style>

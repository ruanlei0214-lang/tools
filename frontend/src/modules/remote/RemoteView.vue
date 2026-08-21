<script lang="ts" setup>
import { computed, onMounted, ref } from 'vue'
import { Config } from '../../../wailsjs/go/remote/Service'
import type { remote } from '../../../wailsjs/go/models'
import { conn, refreshStatus } from '../../shell/connection'
import IoPanel from './IoPanel.vue'
import RegisterPanel from './RegisterPanel.vue'
import CommandPanel from './CommandPanel.vue'

// 标签页与点位来自后端的配置。端口、路径、超时在 remote-config.json 里改，
// 页面不再摆编辑区——一年动不了一次，摆出来只会占地方。
const tabs = ref<remote.Tab[]>([])
const refreshIntervalMs = ref(1000)
const configDir = ref('')
const activeId = ref('')
const connected = computed(() => conn.wsConnected)
const configWarning = ref('')

const activeTab = computed(() => tabs.value.find((t) => t.id === activeId.value) ?? null)

// applyConfig 是配置进入界面的唯一入口：初次加载、保存点位、恢复默认都走它。
function applyConfig(cfg: remote.Settings) {
  refreshIntervalMs.value = cfg.refreshIntervalMs
  configDir.value = cfg.configDir
  tabs.value = cfg.tabs ?? []
  configWarning.value = cfg.warning
  // 保存点位不该把人踢回第一页。
  if (!(cfg.tabs ?? []).some((t) => t.id === activeId.value)) {
    activeId.value = cfg.tabs?.[0]?.id ?? ''
  }
}

onMounted(async () => {
  try {
    applyConfig(await Config())
  } catch (e) {
    configWarning.value = `读取配置失败：${String(e)}`
  }
  await syncStatus()
})

// 点位面板保存或恢复默认之后，整份配置由它们回传，这里照样走 applyConfig。
function onConfigUpdated(cfg: remote.Settings) {
  applyConfig(cfg)
}

// 子面板每次调用失败都会喊一声：连接可能是被控制器单方面断掉的，
// 那种情况下只有后端知道，顶栏的状态点得跟着改回未连接。
async function syncStatus() {
  await refreshStatus()
}
</script>

<template>
  <p v-if="configWarning" class="config-warning" :title="configWarning">{{ configWarning }}</p>

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
    <CommandPanel
      v-else-if="activeTab.kind === 'command'"
      :key="activeTab.id"
      :connected="connected"
      @refresh-status="syncStatus"
    />
    <p v-else class="empty">标签页类型 {{ activeTab.kind }} 还没有对应界面。</p>
  </template>
  <p v-else class="empty">配置里没有任何标签页。</p>
</template>

<style scoped>
.config-warning {
  margin: 0 0 8px;
  padding: 0 8px;
  border-radius: 5px;
  overflow: hidden;
  background: var(--err-soft);
  color: var(--err);
  font-size: 12px;
  line-height: 26px;
  text-overflow: ellipsis;
  white-space: nowrap;
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

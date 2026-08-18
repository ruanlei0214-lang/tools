<script lang="ts" setup>
import { ref } from 'vue'
import PingPanel from './PingPanel.vue'
import ScanPanel from './ScanPanel.vue'

// 两个页签都用 v-show 保活：切去看扫描结果时，长 ping 不该被掐断。
const tab = ref<'ping' | 'scan'>('ping')
</script>

<template>
  <div class="page">
    <nav class="tabs">
      <button class="tab" :class="{ active: tab === 'ping' }" @click="tab = 'ping'">Ping</button>
      <button class="tab" :class="{ active: tab === 'scan' }" @click="tab = 'scan'">
        扫描网段
      </button>
    </nav>

    <PingPanel v-show="tab === 'ping'" />
    <ScanPanel v-show="tab === 'scan'" />
  </div>
</template>

<style scoped>
.page {
  max-width: 880px;
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

<script lang="ts" setup>
import { computed, onMounted, ref } from 'vue'
import { Config } from '../../../wailsjs/go/board/Service'
import { conn, refreshStatus } from '../../shell/connection'
import CommandPanel from './CommandPanel.vue'
import FilePanel from './FilePanel.vue'

// 连接状态归顶栏的全局连接区管：这里只读它的 SSH 状态，不再有自己的连接按钮。
const connected = computed(() => conn.sshConnected)
const defaultPath = ref('/opt')
const fileWidth = ref(320)
const splitting = ref(false)
const imagePreview = ref<{ name: string; src: string } | null>(null)
const imageHeight = ref(240)
const vSplitting = ref(false)
const configWarning = ref('')

onMounted(async () => {
  try {
    const cfg = await Config()
    defaultPath.value = cfg.defaultPath
    configWarning.value = cfg.warning
  } catch (e) {
    configWarning.value = `读取配置失败：${String(e)}`
    return
  }
  await syncStatus()
})

// 子面板每次调用失败都会喊一声：连接可能是被设备单方面断掉的（重启、拔网线），
// 那种情况下只有后端知道，顶栏的状态点得跟着改回未连接。
async function syncStatus() {
  await refreshStatus()
}

function onPreviewImage(payload: { name: string; mime: string; data: string }) {
  imagePreview.value = { name: payload.name, src: `data:${payload.mime};base64,${payload.data}` }
}

function onVSplitDown(e: MouseEvent) {
  vSplitting.value = true
  const startY = e.clientY
  const startH = imageHeight.value
  const onMove = (ev: MouseEvent) => {
    imageHeight.value = Math.min(Math.max(startH + startY - ev.clientY, 120), window.innerHeight - 320)
  }
  const onUp = () => {
    vSplitting.value = false
    window.removeEventListener('mousemove', onMove)
    window.removeEventListener('mouseup', onUp)
  }
  window.addEventListener('mousemove', onMove)
  window.addEventListener('mouseup', onUp)
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
  <!-- 配置告警只在有内容时占一行：连接区收进顶栏之后，这一页平时不该再有横幅。 -->
  <div v-if="configWarning" class="config-warning" :title="configWarning">{{ configWarning }}</div>

  <div class="workspace" :class="{ splitting }">
    <div class="pane pane-file" :style="{ width: `${fileWidth}px` }">
      <FilePanel
        :connected="connected"
        :default-path="defaultPath"
        @refresh-status="syncStatus"
        @preview-image="onPreviewImage"
      />
    </div>
    <div class="splitter" title="拖动调整宽度" @mousedown.prevent="onSplitDown" />
    <div class="pane pane-term">
      <div class="term-stack" :class="{ vsplitting: vSplitting }">
        <div class="term-main">
          <CommandPanel :connected="connected" @refresh-status="syncStatus" />
        </div>
        <template v-if="imagePreview">
          <div class="vsplitter" title="拖动调整高度" @mousedown.prevent="onVSplitDown" />
          <div class="image-pane" :style="{ height: `${imageHeight}px` }">
            <div class="image-head">
              <span class="image-name" :title="imagePreview.name">{{ imagePreview.name }}</span>
              <button type="button" @click="imagePreview = null">关闭</button>
            </div>
            <div class="image-body">
              <img :src="imagePreview.src" :alt="imagePreview.name" />
            </div>
          </div>
        </template>
      </div>
    </div>
  </div>
  </div>
</template>

<style scoped>
.board-page {
  display: flex;
  flex-direction: column;
  gap: 8px;
  /* 跟着内容区走，不再用 100vh 减一个估出来的顶栏高度——
     顶栏加了连接区之后那 88px 已经对不上，底部会被裁掉一截。 */
  height: 100%;
  min-height: 0;
  overflow: hidden;
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

.term-stack {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}

.term-stack.vsplitting {
  user-select: none;
  cursor: row-resize;
}

.term-main {
  flex: 1 1 auto;
  min-height: 140px;
}

.term-main > :deep(*) {
  height: 100%;
}

.vsplitter {
  flex: 0 0 6px;
  margin: 2px 0;
  border-radius: 3px;
  background: var(--border);
  cursor: row-resize;
}

.vsplitter:hover,
.term-stack.vsplitting .vsplitter {
  background: var(--accent);
}

.image-pane {
  display: flex;
  flex: 0 0 auto;
  flex-direction: column;
  min-height: 120px;
  overflow: hidden;
  border: 1px solid var(--border);
  border-radius: 7px;
  background: #111318;
}

.image-head {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 8px;
  min-height: 28px;
  padding: 3px 6px 3px 10px;
  border-bottom: 1px solid #303640;
  background: #20242b;
}

.image-name {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  color: #e5e7eb;
  font-size: 12px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.image-head button {
  flex: 0 0 auto;
  min-height: 22px;
  padding: 0 7px;
  border-color: #424955;
  background: #2a2f38;
  color: #cbd5e1;
  font-size: 10px;
}

.image-body {
  display: grid;
  flex: 1 1 auto;
  min-height: 0;
  place-items: center;
  padding: 8px;
}

.image-body img {
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
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

.config-warning {
  flex: 0 0 auto;
  padding: 0 8px;
  border-radius: 5px;
  overflow: hidden;
  background: var(--err-soft);
  color: var(--err);
  font-size: 12px;
  line-height: 26px;
  text-overflow: ellipsis;
  white-space: nowrap;
  user-select: text;
}
</style>

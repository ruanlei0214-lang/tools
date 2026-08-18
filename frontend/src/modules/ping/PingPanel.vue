<script lang="ts" setup>
import { nextTick, onBeforeUnmount, ref } from 'vue'
import { ReadPing, StartPing, StopPing } from '../../../wailsjs/go/ping/Service'

// 长 ping：开始后后端每秒一个包，这里半秒取一次日志。日志逐行渲染成 div，
// 追加行不动已有节点，正在进行的文字选择不会被新日志打断（终端踩过这个坑）。
const host = ref('')
const running = ref(false)
const lines = ref<string[]>([])
const logEl = ref<HTMLElement>()

// maxLines 是前端显示的上限，超了从头上丢。一千行约等于 16 分钟的日志。
const maxLines = 1000
let timer: number | undefined

async function start() {
  const target = host.value.trim()
  if (!target || running.value) return
  try {
    await StartPing(target)
    lines.value = []
    running.value = true
    poll()
  } catch (e) {
    lines.value = [String(e)]
  }
}

async function stop() {
  // 停的结果（统计行和 running=false）由下一轮 poll 带回来，不用在这里处理。
  await StopPing()
}

async function poll() {
  try {
    const r = await ReadPing()
    if (r.lines?.length) {
      lines.value = lines.value.concat(r.lines).slice(-maxLines)
      await nextTick()
      if (logEl.value) logEl.value.scrollTop = logEl.value.scrollHeight
    }
    running.value = r.running
  } catch {
    // 取日志失败不致命，下一轮再试。
  }
  if (running.value) {
    timer = window.setTimeout(poll, 500)
  }
}

onBeforeUnmount(() => {
  window.clearTimeout(timer)
  // 模块切换走 keep-alive，是停用不是卸载，不会进这里——长 ping 在后台照跑，
  // 日志照攒（缓冲区有上限），切回来接着看。这里只管整个视图被销毁的时候
  // （比如程序退出），把后端的长 ping 收掉，别让它对着没人看的缓冲区跑。
  if (running.value) StopPing()
})
</script>

<template>
  <section class="panel">
    <div class="op-row">
      <input
        v-model="host"
        class="host-input"
        placeholder="IP 或主机名，如 192.168.1.136"
        :disabled="running"
        @keydown.enter="start"
      />
      <button v-if="!running" class="primary op-btn" :disabled="!host.trim()" @click="start">
        开始
      </button>
      <button v-else class="danger op-btn" @click="stop">停止</button>
    </div>
    <div ref="logEl" class="ping-log" :class="{ empty: !lines.length }">
      <div v-for="(l, i) in lines" :key="i">{{ l }}</div>
      <div v-if="!lines.length" class="log-hint">输入地址点「开始」，每秒一行，随时可停。</div>
    </div>
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
  gap: 8px;
}

.host-input {
  width: 260px;
  flex-shrink: 0;
}

.op-btn {
  flex-shrink: 0;
  height: 30px;
  padding: 0 14px;
  font-size: 12px;
  font-weight: 600;
}

.ping-log {
  height: 320px;
  margin-top: 10px;
  padding: 8px 10px;
  overflow-y: auto;
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 6px;
  font-family: Consolas, 'Courier New', monospace;
  font-size: 12px;
  line-height: 1.7;
  color: var(--text);
  user-select: text;
  white-space: pre-wrap;
  word-break: break-all;
}

.ping-log.empty {
  display: flex;
  align-items: center;
  justify-content: center;
}

.log-hint {
  color: var(--text-dim);
  font-family: inherit;
}

@media (max-width: 720px) {
  .host-input {
    width: 100%;
  }
}
</style>

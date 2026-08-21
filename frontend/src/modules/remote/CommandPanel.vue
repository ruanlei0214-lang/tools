<script lang="ts" setup>
import { ref } from 'vue'
import { GetRobotStatus, RebootController } from '../../../wailsjs/go/remote/Service'
import type { remote } from '../../../wailsjs/go/models'

// 指令页：重启控制器。页面上不读状态——状态只在点重启这一刻由后端读：
// 读不到、读出来不是「未使能」，都不许重启。重启指令本身不显示给用户。
defineProps<{
  connected: boolean
}>()
const emit = defineEmits<{
  (e: 'refresh-status'): void
}>()

const busy = ref(false)
const banner = ref<{ kind: 'ok' | 'err'; text: string } | null>(null)

// 禁止重启是安全相关的拦截：弹窗警告 + 状态条各来一份——只滚一行小字在现场容易被忽略。
function refuse(msg: string) {
  banner.value = { kind: 'err', text: msg }
  window.alert(msg)
}

async function reboot() {
  if (busy.value) return
  busy.value = true
  try {
    // 读不到状态就不许重启：状态确认不了时宁可误拦。
    let st: remote.RobotStatus
    try {
      st = await GetRobotStatus()
    } catch (e) {
      refuse(`获取不到机器人状态，禁止重启：${String(e)}`)
      emit('refresh-status')
      return
    }
    // 放行条件和后端 checkRebootAllowed 保持一致：state 为 0，或控制器自己报「未使能」。
    // 文档的状态表和真机对不上（这台 state=2 叫「已使能」），单看 state 不够。
    if (st.state !== 0 && st.stateName !== '未使能') {
      refuse(`机器人当前处于「${st.stateName}」，不允许重启：请先把机器人打到未使能`)
      return
    }
    const ok = window.confirm(
      '确认重启控制器？\n\n' +
        '· 重启只作用于控制器，机器人本体不会随之重启\n' +
        '· 请确认现场已处于安全状态\n' +
        '· 重启后连接会断开，需要等控制器起来后重新连接',
    )
    if (!ok) return
    // 确认之后后端还会再查一次状态，防的是确认期间有人把使能打开。
    await RebootController()
    banner.value = { kind: 'ok', text: '重启指令已下发，控制器正在重启，连接会断开' }
    // 重启会把两条连接都带走，稍等一下再同步顶栏的状态点。
    window.setTimeout(() => emit('refresh-status'), 3000)
  } catch (e) {
    const msg = String(e)
    // 后端复查拦下的「不允许重启」同样是安全拦截，和前端预检一样弹窗。
    if (msg.includes('不允许重启')) {
      refuse(msg)
    } else {
      banner.value = { kind: 'err', text: msg }
    }
    emit('refresh-status')
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <div class="cmd-page">
    <section class="card">
      <h2 class="card-title">重启控制器</h2>
      <p class="hint">
        只重启控制器，机器人本体不会随之重启。只有机器人处于「未使能」时才能重启；
        重启后连接会断开，需要等控制器起来后重新连接。
      </p>
      <button
        class="danger"
        :disabled="!connected || busy"
        :title="connected ? '重启前会先确认机器人处于未使能' : '先连接控制器'"
        @click="reboot"
      >
        {{ busy ? '处理中…' : '重启控制器' }}
      </button>
    </section>
    <div class="status" :class="banner?.kind" :title="banner?.text">{{ banner?.text }}</div>
  </div>
</template>

<style scoped>
.cmd-page {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 12px;
  align-items: start;
  max-width: 520px;
}

.card {
  margin-bottom: 0;
  padding: 10px 12px;
}

.hint {
  margin: 0 0 10px;
  font-size: 12px;
  line-height: 1.7;
  color: var(--text-dim);
}

/* 结果条恒占一行：没有消息时也留着位置，操作结果出来页面不跳。 */
.status {
  height: 34px;
  padding: 0 10px;
  border-radius: 6px;
  overflow: hidden;
  font-size: 12px;
  line-height: 34px;
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
</style>

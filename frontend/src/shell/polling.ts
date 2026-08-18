import { onActivated, onDeactivated, onMounted, onUnmounted } from 'vue'

/**
 * 模块激活期间每 intervalMs() 调一次 fn，启动时先立即调一轮。
 *
 * 模块切换走 keep-alive：停用的页签 DOM 被摘掉，但组件实例活着，定时器
 * 不拦的话会在后台一直空转 IPC、更新一棵没人看的 DOM 树。所以停用时暂停，
 * 切回来时立即补一轮再进节奏。
 *
 * intervalMs 是函数而不是值：间隔在界面上可改，每次起定时器取最新值；
 * 改了间隔调返回的 restart 重建（setInterval 起好之后改不了周期）。
 */
export function useActivePolling(fn: () => void, intervalMs: () => number) {
  let timer: number | undefined
  let active = false

  const stop = () => {
    if (timer !== undefined) window.clearInterval(timer)
    timer = undefined
  }
  const start = () => {
    // onMounted 和 onActivated 在首次挂载都会触发，别起两轮。
    if (timer !== undefined) return
    fn()
    timer = window.setInterval(fn, intervalMs())
  }

  onMounted(() => {
    active = true
    start()
  })
  onActivated(() => {
    active = true
    start()
  })
  onDeactivated(() => {
    active = false
    stop()
  })
  onUnmounted(() => {
    active = false
    stop()
  })

  // 停用期间改了间隔不重建，切回来时 activated 会按新值起。
  const restart = () => {
    stop()
    if (active) start()
  }
  return { restart }
}

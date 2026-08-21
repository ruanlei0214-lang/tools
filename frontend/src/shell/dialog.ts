import { reactive } from 'vue'

// 应用内弹窗，替代 window.alert / window.confirm：原生弹窗是操作系统样式，
// 和界面不搭，多行文字的排版也不受控。状态是单例，AppDialog 挂在 App.vue 里渲染。
interface DialogState {
  visible: boolean
  title: string
  text: string
  // isConfirm 为真时有「取消」，返回用户点没点确认；否则只是提示，只有一个按钮。
  isConfirm: boolean
  // danger 把确认按钮标红，给删除、重启这类做过就回不去的动作用。
  danger: boolean
  confirmText: string
  resolve: ((ok: boolean) => void) | null
}

export const dialog = reactive<DialogState>({
  visible: false,
  title: '',
  text: '',
  isConfirm: false,
  danger: false,
  confirmText: '确定',
  resolve: null,
})

export function closeDialog(ok: boolean) {
  dialog.visible = false
  dialog.resolve?.(ok)
  dialog.resolve = null
}

function show(patch: Partial<DialogState>, resolve: ((ok: boolean) => void) | null) {
  // 上一个还开着就先按「取消」了结它，别让它的调用方永远等下去。
  if (dialog.resolve) closeDialog(false)
  Object.assign(dialog, patch, { visible: true, resolve })
}

// alertDialog 替代 window.alert：一条消息，一个「知道了」。text 里的换行原样保留。
export function alertDialog(text: string, title = '提示') {
  show({ title, text, isConfirm: false, danger: false, confirmText: '知道了' }, null)
}

// confirmDialog 替代 window.confirm，返回用户点没点确认。
export function confirmDialog(
  text: string,
  opts: { title?: string; danger?: boolean; confirmText?: string } = {},
): Promise<boolean> {
  return new Promise((resolve) => {
    show(
      {
        title: opts.title ?? '确认',
        text,
        isConfirm: true,
        danger: opts.danger ?? false,
        confirmText: opts.confirmText ?? '确定',
      },
      resolve,
    )
  })
}

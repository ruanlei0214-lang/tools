import type { ModuleManifest } from '../../shell/registry'
import RemoteView from './RemoteView.vue'

export default {
  id: 'remote',
  name: '远程控制',
  description: '按配置渲染标签页，控制上位机 IO 与寄存器',
  version: 'V1.4.1',
  guide: [
    '先在顶栏点「连接」，状态点变绿后再操作',
    '指令页「重启控制器」只在机器人未使能时可用，已使能会被拦下',
  ],
  view: RemoteView,
} satisfies ModuleManifest

import type { ModuleManifest } from '../../shell/registry'
import RemoteView from './RemoteView.vue'

export default {
  id: 'remote',
  name: '远程控制',
  description: '按配置渲染标签页，控制上位机 IO 与寄存器',
  version: 'V1.4.1',
  view: RemoteView,
} satisfies ModuleManifest

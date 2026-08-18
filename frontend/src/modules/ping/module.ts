import type { ModuleManifest } from '../../shell/registry'
import PingView from './PingView.vue'

export default {
  id: 'ping',
  name: '网络检测',
  description: '长 ping 单个地址，扫描网段里的在线设备（IP、设备名、MAC）',
  version: 'V1.1.2',
  view: PingView,
} satisfies ModuleManifest

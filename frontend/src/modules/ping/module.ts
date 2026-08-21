import type { ModuleManifest } from '../../shell/registry'
import PingView from './PingView.vue'

export default {
  id: 'ping',
  name: '网络检测',
  description: '长 ping 单个地址，扫描网段里的在线设备（IP、设备名、MAC）',
  version: 'V1.1.3',
  guide: [
    'Ping：填一个地址开始长 ping，看时延和丢包',
    '扫描网段：列出在线设备的 IP、设备名、MAC，冲突的行会高亮',
  ],
  view: PingView,
} satisfies ModuleManifest

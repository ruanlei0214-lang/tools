import type { ModuleManifest } from '../../shell/registry'
import BoardView from './BoardView.vue'

export default {
  id: 'board',
  name: '终端',
  description: 'SSH 终端、自定义指令与文件传输',
  version: 'V1.1.10',
  view: BoardView,
} satisfies ModuleManifest

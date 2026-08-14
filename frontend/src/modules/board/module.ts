import type { ModuleManifest } from '../../shell/registry'
import BoardView from './BoardView.vue'

export default {
  id: 'board',
  name: '主板控制',
  description: '在主板上跑指令、上传下载文件',
  version: 'V1.1.4',
  view: BoardView,
} satisfies ModuleManifest

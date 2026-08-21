import type { ModuleManifest } from '../../shell/registry'
import BoardView from './BoardView.vue'

export default {
  id: 'board',
  name: '终端',
  description: 'SSH 终端、自定义指令与文件传输',
  version: 'V1.2.4',
  guide: [
    '顶栏「连接」后即可使用 SSH 终端',
    '自定义指令点一下直接下发，指令清单可编辑、导入导出',
    '文件区支持上传、下载，可整目录操作',
  ],
  view: BoardView,
} satisfies ModuleManifest

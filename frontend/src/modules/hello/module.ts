import type { ModuleManifest } from '../../shell/registry'
import HelloView from './HelloView.vue'

export default {
  id: 'hello',
  name: 'Hello World',
  description: '模块框架的最小示例',
  version: 'V1.0.1',
  view: HelloView,
} satisfies ModuleManifest

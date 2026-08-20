import type { ModuleManifest } from '../../shell/registry'
import NetcfgView from './NetcfgView.vue'

export default {
  id: 'netcfg',
  name: '网络配置',
  description: '远程修改设备的 IP、掩码与网关',
  version: 'V1.0.23',
  view: NetcfgView,
} satisfies ModuleManifest

import type { ModuleManifest } from '../../shell/registry'
import NetcfgView from './NetcfgView.vue'

export default {
  id: 'netcfg',
  name: '网络配置',
  description: '远程修改设备的 IP、掩码与网关',
  version: 'V1.0.24',
  guide: [
    '打开页面自动连接设备并读出网口现状',
    '网络：支持修改lan1的IP / 掩码 / 网关，「下发配置」二次确认后生效',
    'WiFi：支持修改热点名称、密码、频段、信道，应用后自动重启热点',
    '一键恢复网络：清掉现场改坏的配置，需手动重启控制器',
  ],
  view: NetcfgView,
} satisfies ModuleManifest

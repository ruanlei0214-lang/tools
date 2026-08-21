import type { Component } from 'vue'
import { enabledModules } from './modules.gen'

/** 一个模块对外的自我描述，由模块目录下的 module.ts 默认导出。 */
export interface ModuleManifest {
  id: string
  name: string
  description: string
  /**
   * 模块自己的版本，形如 `V1.0.1`。
   *
   * 每个模块独立编号，互不牵连：改了网络配置不该让别的模块的版本跟着跳。
   * 版本只声明在这里，后端不再存一份——两处各写一遍迟早对不上，而对不上的时候
   * 没有任何机制会发现。
   */
  version: string
  /**
   * 简要操作说明，一行一条，显示在「关于」弹窗的当前模块组里。
   * 只写最容易问出口的那几步，不是文档的替代品。
   */
  guide?: string[]
  view: Component
}

/** 当前 profile 启用的模块，列表由 tools/genmodules 生成。 */
export const modules: ModuleManifest[] = [...enabledModules].sort((a, b) =>
  a.name.localeCompare(b.name, 'zh-CN'),
)

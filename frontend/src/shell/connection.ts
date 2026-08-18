import { reactive } from 'vue'
import {
  Config as boardConfig,
  Connect as boardConnect,
  Disconnect as boardDisconnect,
  Status as boardStatus,
} from '../../wailsjs/go/board/Service'
import {
  Config as remoteConfig,
  Connect as remoteConnect,
  Disconnect as remoteDisconnect,
  Status as remoteStatus,
} from '../../wailsjs/go/remote/Service'

// 全系列工具共用一台控制器：地址和 SSH 凭据只有这一份（共享配置
// toolbox-config.json），SSH（board）和 WebSocket（remote）都连它。
// 顶栏的连接按钮同时建这两条连接，状态点分开显示；各模块页面不再有自己的
// 连接按钮，只从这里读状态。
//
// profile 可能把 board 或 remote 裁掉（比如 netcfg-only），那时对应的后端
// 绑定在运行时不存在。调之前先探一下，裁掉的协议就不连也不显示。
declare global {
  interface Window {
    go?: { board?: { Service?: unknown }; remote?: { Service?: unknown } }
  }
}
const hasBoard = () => !!window.go?.board?.Service
const hasRemote = () => !!window.go?.remote?.Service

export const conn = reactive({
  host: '',
  user: '',
  password: '',
  keyPath: '',
  sshPort: 22,
  // 协议有没有进产物：被 profile 裁掉的那个不显示状态点。
  hasSsh: hasBoard(),
  hasWs: hasRemote(),
  sshConnected: false,
  wsConnected: false,
  sshAddr: '',
  wsAddr: '',
  sshError: '',
  wsError: '',
  busy: '' as '' | 'connect' | 'disconnect',
  loaded: false,
})

// loadShared 把共享配置读进界面。启动时调一次；点「连接」还会再调，
// 这样改完 exe 旁边的 toolbox-config.json 不用重启就能用新凭据。
export async function loadShared() {
  if (hasBoard()) {
    const cfg = await boardConfig()
    conn.host = cfg.device.host
    conn.user = cfg.device.user
    conn.password = cfg.device.password
    conn.keyPath = cfg.device.keyPath || ''
    conn.sshPort = cfg.device.port || 22
  }
  conn.loaded = true
  await refreshStatus()
}

// refreshStatus 对齐两条连接的真实状态。连接可能被设备单方面断掉（重启、
// 拔网线），那种情况只有后端知道，所以各面板调用失败时也会喊它来同步。
export async function refreshStatus() {
  const [ssh, ws] = await Promise.allSettled([
    hasBoard() ? boardStatus() : Promise.resolve(null),
    hasRemote() ? remoteStatus() : Promise.resolve(null),
  ])
  if (ssh.status === 'fulfilled' && ssh.value) {
    conn.sshConnected = ssh.value.connected
    conn.sshAddr = ssh.value.addr
    conn.sshError = ssh.value.error
  } else if (!hasBoard()) {
    conn.sshConnected = false
  }
  if (ws.status === 'fulfilled' && ws.value) {
    conn.wsConnected = ws.value.connected
    conn.wsAddr = ws.value.addr
    conn.wsError = ws.value.error
  } else if (!hasRemote()) {
    conn.wsConnected = false
  }
}

// connectAll 一键建 SSH 和 WS 两条连接。两条互不等待：WS 连不上不该拖着
// SSH 也连不上。成功不返回文案（状态灯够了），失败只报哪条没连上，
// 详细原因留在状态点的悬停提示里。
export async function connectAll(): Promise<{ kind: 'ok' | 'err'; text: string }> {
  conn.busy = 'connect'
  conn.sshConnected = false
  conn.wsConnected = false
  try {
    // 先读一遍文件：现场改完 toolbox-config.json 点连接就能用，不用重启。
    await loadShared()
    const device = {
      host: conn.host.trim(),
      user: conn.user.trim(),
      password: conn.password,
      keyPath: conn.keyPath,
    }
    // WS 的端口和路径归 remote 模块自己管，建连前取它当前的那份。
    const wsTarget = hasRemote()
      ? await remoteConfig().then((c) => ({
          host: device.host,
          port: c.device.port || 9000,
          path: c.device.path || '/',
        }))
      : null

    const [ssh, ws] = await Promise.allSettled([
      hasBoard() ? boardConnect({ ...device, port: conn.sshPort || 22 }) : Promise.resolve(null),
      wsTarget ? remoteConnect(wsTarget) : Promise.resolve(null),
    ])

    const parts: string[] = []
    if (ssh.status === 'fulfilled' && ssh.value) {
      conn.sshConnected = ssh.value.connected
      conn.sshAddr = ssh.value.addr
      conn.sshError = ''
    } else if (ssh.status === 'rejected') {
      conn.sshError = String(ssh.reason)
      parts.push('SSH')
    }
    if (ws.status === 'fulfilled' && ws.value) {
      conn.wsConnected = ws.value.connected
      conn.wsAddr = ws.value.addr
      conn.wsError = ''
    } else if (ws.status === 'rejected') {
      conn.wsError = String(ws.reason)
      parts.push('WS')
    }
    // 成功不说话，状态灯够了；失败只报哪条没连上，详细原因挂在状态点的悬停提示里。
    const anyOk = (ssh.status === 'fulfilled' && ssh.value) || (ws.status === 'fulfilled' && ws.value)
    if (!parts.length && anyOk) return { kind: 'ok' as const, text: '' }
    if (parts.length) return { kind: 'err' as const, text: `${parts.join('、')} 连接失败` }
    return { kind: 'err' as const, text: '这个产物里没有可连接的模块' }
  } finally {
    conn.busy = ''
  }
}

export async function disconnectAll(): Promise<void> {
  conn.busy = 'disconnect'
  try {
    await Promise.allSettled([
      hasBoard() ? boardDisconnect() : Promise.resolve(),
      hasRemote() ? remoteDisconnect() : Promise.resolve(),
    ])
  } finally {
    conn.sshConnected = false
    conn.wsConnected = false
    conn.sshAddr = ''
    conn.wsAddr = ''
    conn.sshError = ''
    conn.wsError = ''
    conn.busy = ''
  }
}

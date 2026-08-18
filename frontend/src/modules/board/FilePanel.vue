<script lang="ts" setup>
import { computed, nextTick, ref, watch } from 'vue'
import {
  Download,
  ListDir,
  PickLocalFile,
  PickSaveTarget,
  ReadRemoteBytes,
  ReadRemoteText,
  StartTerminal,
  Upload,
  WriteTerminal,
} from '../../../wailsjs/go/board/Service'
import type { board } from '../../../wailsjs/go/models'
import ContextMenu, { type MenuItem } from './ContextMenu.vue'

const props = defineProps<{ connected: boolean; defaultPath: string }>()
const emit = defineEmits<{
  (e: 'refresh-status'): void
  (e: 'preview-image', payload: { name: string; mime: string; data: string }): void
}>()

const path = ref('')
const listedPath = ref('')
const entries = ref<board.Entry[]>([])
// 多选：selNames 是选中的名字集，anchorIdx 是 Shift 范围选的锚点（entries 里的下标）。
const selNames = ref<string[]>([])
let anchorIdx = -1
const busy = ref('')
const banner = ref<{ kind: 'ok' | 'err' | 'info'; text: string } | null>(null)
const menu = ref<{ x: number; y: number; entry: board.Entry | null } | null>(null)
const editor = ref<{ path: string; name: string; text: string } | null>(null)
const clip = ref<{ path: string; name: string; cut: boolean }[]>([])
const renaming = ref<{ from: string; draft: string } | null>(null)
const renameInput = ref<HTMLInputElement | null>(null)

watch(
  () => props.defaultPath,
  (p) => {
    if (p && !path.value) path.value = p
  },
  { immediate: true },
)

watch(
  () => props.connected,
  (ok) => {
    if (ok && path.value.trim() && !listedPath.value) void list()
  },
)

const canOperate = computed(() => props.connected && !busy.value)

const selectedEntries = computed(() => entries.value.filter((e) => selNames.value.includes(e.name)))

// 右键点中已选中的行就作用于整个选择；点中没选中的行则先把它变成唯一选中。
function menuTargets(): board.Entry[] {
  const e = menu.value?.entry
  if (!e) return []
  const sel = selectedEntries.value
  return sel.some((x) => x.name === e.name) ? sel : [e]
}

const menuItems = computed<MenuItem[]>(() => {
  const e = menu.value?.entry
  if (!e) {
    return [
      { id: 'paste', label: '粘贴', disabled: !clip.value.length || !listedPath.value },
      { id: 'mkdir', label: '新建文件夹', disabled: !listedPath.value },
      { id: 'upload', label: '上传' },
      { id: 'refresh', label: '刷新' },
    ]
  }
  const targets = menuTargets()
  if (targets.length > 1) {
    return [
      { id: 'copy', label: `复制 ${targets.length} 项` },
      { id: 'cut', label: `剪切 ${targets.length} 项` },
      { id: 'delete', label: `删除 ${targets.length} 项`, danger: true },
    ]
  }
  if (e.isDir) {
    return [
      { id: 'open', label: '打开' },
      { id: 'copy', label: '复制' },
      { id: 'cut', label: '剪切' },
      { id: 'paste', label: '粘贴', disabled: !clip.value.length },
      { id: 'rename', label: '重命名' },
      { id: 'delete', label: '删除', danger: true },
    ]
  }
  if (imageMime(e.name)) {
    return [
      { id: 'preview', label: '预览' },
      { id: 'copy', label: '复制' },
      { id: 'cut', label: '剪切' },
      { id: 'rename', label: '重命名' },
      { id: 'download', label: '下载' },
      { id: 'delete', label: '删除', danger: true },
    ]
  }
  return [
    { id: 'edit', label: '编辑' },
    { id: 'copy', label: '复制' },
    { id: 'cut', label: '剪切' },
    { id: 'rename', label: '重命名' },
    { id: 'download', label: '下载' },
    { id: 'delete', label: '删除', danger: true },
  ]
})

async function act(op: string, fn: () => Promise<void>) {
  busy.value = op
  try {
    await fn()
  } catch (e) {
    banner.value = { kind: 'err', text: String(e) }
    emit('refresh-status')
  } finally {
    busy.value = ''
  }
}

function list(dir?: string) {
  const next = (dir ?? path.value).trim()
  if (!next) return
  path.value = next
  return act('list', async () => {
    const rows = await ListDir(next)
    entries.value = rows
    listedPath.value = next
    clearSelection()
    renaming.value = null
    banner.value = null
  })
}

function clearSelection() {
  selNames.value = []
  anchorIdx = -1
}

// 单击选一个，Ctrl+单击加选/减选，Shift+单击从锚点选到这一行。
function onRowClick(ev: MouseEvent, e: board.Entry, i: number) {
  if (renaming.value) return
  if (ev.shiftKey && anchorIdx >= 0 && anchorIdx < entries.value.length) {
    const [a, b] = anchorIdx < i ? [anchorIdx, i] : [i, anchorIdx]
    selNames.value = entries.value.slice(a, b + 1).map((x) => x.name)
    return
  }
  if (ev.ctrlKey || ev.metaKey) {
    const next = selNames.value.slice()
    const at = next.indexOf(e.name)
    if (at >= 0) next.splice(at, 1)
    else next.push(e.name)
    selNames.value = next
    anchorIdx = i
    return
  }
  selNames.value = [e.name]
  anchorIdx = i
}

function imageMime(name: string) {
  const ext = name.slice(name.lastIndexOf('.') + 1).toLowerCase()
  const mime: Record<string, string> = {
    png: 'image/png',
    jpg: 'image/jpeg',
    jpeg: 'image/jpeg',
    gif: 'image/gif',
    webp: 'image/webp',
    bmp: 'image/bmp',
    svg: 'image/svg+xml',
  }
  return mime[ext] ?? ''
}

function open(e: board.Entry) {
  if (!e.isDir) {
    if (imageMime(e.name)) {
      void previewImage(e)
      return
    }
    void startEdit(e)
    return
  }
  return list(joinPath(listedPath.value, e.name))
}

function goUp() {
  if (!listedPath.value || listedPath.value === '/') return
  return list(parentPath(listedPath.value))
}

function joinPath(dir: string, name: string) {
  if (!dir || dir === '/') return `/${name}`
  return dir.endsWith('/') ? `${dir}${name}` : `${dir}/${name}`
}

function parentPath(dir: string) {
  const t = dir.replace(/\/+$/, '') || '/'
  if (t === '/') return '/'
  const i = t.lastIndexOf('/')
  return i <= 0 ? '/' : t.slice(0, i)
}

function shQuote(s: string) {
  return `'${s.replace(/'/g, `'"'"'`)}'`
}

function remoteOf(e: board.Entry) {
  return joinPath(listedPath.value, e.name)
}

// 终端分屏后写固定进第一格（t1）：它关不掉、始终存在，上传后的解压之类的
// 辅助命令落在那里，和用户自己敲命令的当前格子互不干扰。
async function sendShell(cmd: string) {
  await StartTerminal('t1')
  await WriteTerminal('t1', cmd.endsWith('\n') ? cmd : `${cmd}\n`)
}

function sleep(ms: number) {
  return new Promise<void>((resolve) => window.setTimeout(resolve, ms))
}

function upload() {
  return act('upload', async () => {
    if (!listedPath.value) throw new Error('先打开一个目录，再往里上传')
    const local = await PickLocalFile()
    if (!local) return
    let res = await Upload(local, listedPath.value, false)
    if (res.needsConfirm) {
      if (!window.confirm(`设备上已有同名文件，覆盖它？\n\n${res.remotePath}`)) {
        banner.value = { kind: 'info', text: '已取消上传' }
        return
      }
      res = await Upload(local, listedPath.value, true)
    }
    banner.value = { kind: 'ok', text: `已上传到 ${res.remotePath}` }
    await relist()
  })
}

function download(row: board.Entry | null) {
  if (!row || row.isDir) return
  return act('download', async () => {
    const local = await PickSaveTarget(row.name)
    if (!local) return
    await Download(remoteOf(row), local)
    banner.value = { kind: 'ok', text: `已保存到 ${local}` }
  })
}

function copy(list: board.Entry[]) {
  if (!list.length) return
  clip.value = list.map((e) => ({ path: remoteOf(e), name: e.name, cut: false }))
  banner.value = {
    kind: 'info',
    text: list.length > 1 ? `已复制 ${list.length} 项` : `已复制 ${list[0].name}`,
  }
}

function cut(list: board.Entry[]) {
  if (!list.length) return
  clip.value = list.map((e) => ({ path: remoteOf(e), name: e.name, cut: true }))
  banner.value = {
    kind: 'info',
    text: list.length > 1 ? `已剪切 ${list.length} 项` : `已剪切 ${list[0].name}`,
  }
}

// 同目录粘贴撞名时自动加「-副本」。taken 是目标目录里已占用的名字，
// 批量粘贴时逐个往里放，后贴的不会顶掉先贴的。
function pasteName(intoDir: string, name: string, taken: Set<string>) {
  if (intoDir !== listedPath.value || !taken.has(name)) return name
  const dot = name.lastIndexOf('.')
  const stem = dot > 0 ? name.slice(0, dot) : name
  const ext = dot > 0 ? name.slice(dot) : ''
  let out = `${stem}-副本${ext}`
  let n = 2
  while (taken.has(out)) {
    out = `${stem}-副本${n}${ext}`
    n++
  }
  return out
}

function paste(into?: board.Entry) {
  if (!clip.value.length) return
  const destDir = into?.isDir ? remoteOf(into) : listedPath.value
  if (!destDir) return
  const items = clip.value
  const taken = new Set(entries.value.map((x) => x.name))
  const cmds: string[] = []
  for (const item of items) {
    const name = pasteName(destDir, item.name, taken)
    taken.add(name)
    const dest = joinPath(destDir, name)
    if (dest === item.path) continue
    cmds.push(`${item.cut ? 'mv' : 'cp -a'} ${shQuote(item.path)} ${shQuote(dest)}`)
  }
  if (!cmds.length) {
    banner.value = { kind: 'info', text: '源和目标相同' }
    return
  }
  return act('paste', async () => {
    await sendShell(cmds.join('\n'))
    if (items.some((i) => i.cut)) clip.value = []
    banner.value = { kind: 'ok', text: `已在终端执行 ${cmds.length} 条命令` }
    await sleep(400)
    await relist()
  })
}

function startRename(e: board.Entry) {
  renaming.value = { from: e.name, draft: e.name }
  void nextTick(() => {
    renameInput.value?.focus()
    renameInput.value?.select()
  })
}

function commitRename() {
  const r = renaming.value
  if (!r) return
  const name = r.draft.trim()
  renaming.value = null
  if (!name || name === r.from) return
  if (name.includes('/') || name.includes('\\')) {
    banner.value = { kind: 'err', text: '名称里不能有斜杠' }
    return
  }
  const cmd = `mv ${shQuote(joinPath(listedPath.value, r.from))} ${shQuote(joinPath(listedPath.value, name))}`
  return act('rename', async () => {
    await sendShell(cmd)
    banner.value = { kind: 'ok', text: `已在终端执行：${cmd}` }
    await sleep(400)
    await relist()
  })
}

function cancelRename() {
  renaming.value = null
}

function mkdir() {
  const name = window.prompt('文件夹名称')?.trim()
  if (!name) return
  if (name.includes('/') || name.includes('\\')) {
    banner.value = { kind: 'err', text: '名称里不能有斜杠' }
    return
  }
  const cmd = `mkdir -p ${shQuote(joinPath(listedPath.value, name))}`
  return act('mkdir', async () => {
    await sendShell(cmd)
    banner.value = { kind: 'ok', text: `已在终端执行：${cmd}` }
    await sleep(400)
    await relist()
  })
}

function remove(list: board.Entry[]) {
  if (!list.length) return
  const multi = list.length > 1
  const hint = multi
    ? `将删除这 ${list.length} 项（目录含里面的所有文件）`
    : list[0].isDir
      ? '将删除整个目录（含里面的文件）'
      : '删除这个文件'
  const detail = multi ? list.map((e) => e.name).join('、') : remoteOf(list[0])
  if (!window.confirm(`${hint}？\n\n${detail}`)) return
  const cmd =
    multi || list[0].isDir
      ? `rm -rf ${list.map((e) => shQuote(remoteOf(e))).join(' ')}`
      : `rm -f ${shQuote(remoteOf(list[0]))}`
  return act('delete', async () => {
    await sendShell(cmd)
    banner.value = { kind: 'ok', text: `已在终端执行：${cmd}` }
    await sleep(400)
    await relist()
  })
}

function previewImage(e: board.Entry) {
  const mime = imageMime(e.name)
  if (!mime) return
  return act('preview', async () => {
    const data = await ReadRemoteBytes(remoteOf(e))
    emit('preview-image', { name: e.name, mime, data })
  })
}

function startEdit(e: board.Entry) {
  if (e.isDir) return
  return act('edit', async () => {
    const remote = remoteOf(e)
    editor.value = { path: remote, name: e.name, text: await ReadRemoteText(remote) }
  })
}

function saveEdit() {
  const ed = editor.value
  if (!ed) return
  if (ed.text.length > 48 * 1024) {
    banner.value = { kind: 'err', text: '内容超过 48KB，请改小再保存' }
    return
  }
  let delim = `C2EOF_${Date.now()}`
  while (ed.text.includes(delim)) delim += 'X'
  const cmd = `cat > ${shQuote(ed.path)} << '${delim}'\n${ed.text}\n${delim}`
  return act('save-edit', async () => {
    await sendShell(cmd)
    editor.value = null
    banner.value = { kind: 'ok', text: `已把 ${ed.name} 的写入命令送到终端` }
    await sleep(400)
    await relist()
  })
}

async function relist() {
  if (!listedPath.value) return
  try {
    entries.value = await ListDir(listedPath.value)
    clearSelection()
  } catch {
    /* 命令可能还在跑，列表下次再刷 */
  }
}

function openMenu(e: MouseEvent, entry: board.Entry | null) {
  if (entry && !selNames.value.includes(entry.name)) {
    selNames.value = [entry.name]
    anchorIdx = entries.value.findIndex((x) => x.name === entry.name)
  }
  menu.value = { x: e.clientX, y: e.clientY, entry }
}

function onMenu(id: string) {
  const e = menu.value?.entry
  const targets = menuTargets()
  menu.value = null
  switch (id) {
    case 'open':
      if (e) void open(e)
      break
    case 'preview':
      if (e) void previewImage(e)
      break
    case 'edit':
      if (e) void startEdit(e)
      break
    case 'copy':
      copy(targets)
      break
    case 'cut':
      cut(targets)
      break
    case 'paste':
      void paste(e ?? undefined)
      break
    case 'rename':
      if (e) startRename(e)
      break
    case 'delete':
      remove(targets)
      break
    case 'download':
      if (e) void download(e)
      break
    case 'mkdir':
      mkdir()
      break
    case 'upload':
      void upload()
      break
    case 'refresh':
      void list()
      break
  }
}

function humanSize(n: number) {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}
</script>

<template>
  <section class="card explorer">
    <div class="explorer-bar">
      <button :disabled="!canOperate || !listedPath || listedPath === '/'" title="上一级" @click="goUp">
        ↑
      </button>
      <input
        v-model="path"
        class="addr"
        aria-label="路径"
        :disabled="!!busy"
        @keyup.enter="list()"
      />
      <button :disabled="!canOperate || !path.trim()" @click="list()">刷新</button>
      <button :disabled="!canOperate || !listedPath" @click="upload">上传</button>
      <button :disabled="!canOperate || !listedPath || !clip.length" @click="paste()">粘贴</button>
    </div>
    <div v-if="banner" class="status" :class="banner.kind" :title="banner.text">{{ banner.text }}</div>

    <div class="explorer-body" @contextmenu.prevent="openMenu($event, null)">
      <div class="explorer-head">
        <span>名称</span>
        <span class="col-size">大小</span>
      </div>
      <div
        v-for="(e, i) in entries"
        :key="e.name"
        class="row"
        :class="{ selected: selNames.includes(e.name), dir: e.isDir }"
        role="button"
        tabindex="0"
        @click="onRowClick($event, e, i)"
        @dblclick="renaming?.from === e.name ? undefined : open(e)"
        @contextmenu.prevent.stop="openMenu($event, e)"
      >
        <span class="icon" :class="e.isDir ? 'icon-dir' : 'icon-file'" aria-hidden="true" />
        <input
          v-if="renaming?.from === e.name"
          ref="renameInput"
          v-model="renaming.draft"
          class="rename-input"
          spellcheck="false"
          @click.stop
          @dblclick.stop
          @keydown.enter.prevent="commitRename"
          @keydown.esc.prevent="cancelRename"
          @blur="commitRename"
        />
        <span v-else class="name">{{ e.name }}</span>
        <span class="col-size">{{ e.isDir ? '' : humanSize(e.size) }}</span>
      </div>
      <div class="explorer-pad" />
    </div>

    <ContextMenu
      v-if="menu"
      :x="menu.x"
      :y="menu.y"
      :items="menuItems"
      @pick="onMenu"
      @close="menu = null"
    />

    <div v-if="editor" class="mask" @click.self="editor = null">
      <div class="edit-box">
        <header>{{ editor.name }}</header>
        <textarea v-model="editor.text" spellcheck="false" />
        <footer>
          <button class="primary" :disabled="!!busy" @click="saveEdit">
            {{ busy === 'save-edit' ? '写入中…' : '保存' }}
          </button>
          <button :disabled="!!busy" @click="editor = null">取消</button>
        </footer>
      </div>
    </div>
  </section>
</template>

<style scoped>
.status {
  flex: 0 0 auto;
  min-height: 22px;
  margin-bottom: 6px;
  padding: 0 8px;
  border-radius: 5px;
  overflow: hidden;
  font-size: 12px;
  line-height: 22px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.status.ok {
  background: var(--ok-soft);
  color: var(--ok);
}

.status.err {
  background: var(--err-soft);
  color: var(--err);
}

.status.info {
  background: var(--accent-soft);
  color: var(--accent);
}

.explorer {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  margin: 0;
  padding: 8px;
}

.explorer-bar {
  display: flex;
  flex: 0 0 auto;
  align-items: center;
  gap: 4px;
  margin-bottom: 6px;
}

.explorer-bar > button {
  flex: 0 0 auto;
  min-height: 26px;
  padding: 0 8px;
  font-size: 12px;
}

.addr {
  flex: 1 1 auto;
  min-width: 0;
  padding: 4px 7px;
  font-size: 12px;
}

.explorer-body {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  min-height: 0;
  overflow: auto;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: #fff;
}

.explorer-head,
.row {
  display: grid;
  grid-template-columns: 18px minmax(0, 1fr) 5.5rem;
  align-items: center;
  gap: 8px;
  width: 100%;
  min-height: 28px;
  padding: 0 10px;
}

.explorer-head {
  position: sticky;
  top: 0;
  border-bottom: 1px solid var(--border);
  background: #f7f8fa;
  color: var(--text-dim);
  font-size: 11px;
}

.row {
  border: none;
  border-radius: 0;
  background: none;
  text-align: left;
  cursor: pointer;
}

.row:nth-child(odd) {
  background: #fafbfc;
}

.explorer-pad {
  flex: 1 1 auto;
  min-height: 120px;
}

.row:hover,
.row.selected {
  background: var(--accent-soft);
}

.row.dir .name {
  font-weight: 600;
}

.icon {
  width: 14px;
  height: 12px;
  border-radius: 2px;
}

.icon-dir {
  background: #f5c14a;
  box-shadow: inset 0 3px 0 #e0a020;
}

.icon-file {
  background: #d7dde6;
  box-shadow: inset 3px 0 0 #b7c0cc;
}

.rename-input {
  min-width: 0;
  width: 100%;
  padding: 1px 4px;
  border: 1px solid var(--accent);
  border-radius: 3px;
  background: #fff;
  font: inherit;
  font-size: 12px;
}

.name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
}

.col-size {
  color: var(--text-dim);
  font-size: 12px;
  text-align: right;
}

.mask {
  position: fixed;
  inset: 0;
  z-index: 30;
  display: grid;
  place-items: center;
  background: rgb(15 23 42 / 35%);
}

.edit-box {
  display: grid;
  grid-template-rows: auto 1fr auto;
  width: min(720px, 92vw);
  height: min(480px, 80vh);
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--panel);
  box-shadow: 0 12px 40px rgb(0 0 0 / 20%);
}

.edit-box header {
  margin-bottom: 8px;
  font-weight: 600;
}

.edit-box textarea {
  width: 100%;
  height: 100%;
  padding: 8px;
  border: 1px solid var(--border);
  border-radius: 6px;
  resize: none;
  font-family: Consolas, "Cascadia Mono", monospace;
  font-size: 12px;
  line-height: 1.45;
}

.edit-box footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 8px;
}
</style>

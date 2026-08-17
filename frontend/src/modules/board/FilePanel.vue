<script lang="ts" setup>
import { computed, ref, watch } from 'vue'
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
const selected = ref<board.Entry | null>(null)
const busy = ref('')
const banner = ref<{ kind: 'ok' | 'err' | 'info'; text: string } | null>(null)
const menu = ref<{ x: number; y: number; entry: board.Entry | null } | null>(null)
const editor = ref<{ path: string; name: string; text: string } | null>(null)
const clip = ref<{ path: string; name: string; cut: boolean } | null>(null)

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

const menuItems = computed<MenuItem[]>(() => {
  const e = menu.value?.entry
  if (!e) {
    return [
      { id: 'paste', label: '粘贴', disabled: !clip.value || !listedPath.value },
      { id: 'mkdir', label: '新建文件夹', disabled: !listedPath.value },
      { id: 'upload', label: '上传' },
      { id: 'refresh', label: '刷新' },
    ]
  }
  if (e.isDir) {
    return [
      { id: 'open', label: '打开' },
      { id: 'copy', label: '复制' },
      { id: 'cut', label: '剪切' },
      { id: 'paste', label: '粘贴', disabled: !clip.value },
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
    selected.value = null
    banner.value = null
  })
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

async function sendShell(cmd: string) {
  await StartTerminal()
  await WriteTerminal(cmd.endsWith('\n') ? cmd : `${cmd}\n`)
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

function download(row = selected.value) {
  if (!row || row.isDir) return
  return act('download', async () => {
    const local = await PickSaveTarget(row.name)
    if (!local) return
    await Download(remoteOf(row), local)
    banner.value = { kind: 'ok', text: `已保存到 ${local}` }
  })
}

function copy(e: board.Entry) {
  clip.value = { path: remoteOf(e), name: e.name, cut: false }
  banner.value = { kind: 'info', text: `已复制 ${e.name}` }
}

function cut(e: board.Entry) {
  clip.value = { path: remoteOf(e), name: e.name, cut: true }
  banner.value = { kind: 'info', text: `已剪切 ${e.name}` }
}

function pasteName(intoDir: string) {
  if (!clip.value) return ''
  const exists = intoDir === listedPath.value && entries.value.some((x) => x.name === clip.value?.name)
  if (!exists) return clip.value.name
  const dot = clip.value.name.lastIndexOf('.')
  if (dot > 0) return `${clip.value.name.slice(0, dot)}-副本${clip.value.name.slice(dot)}`
  return `${clip.value.name}-副本`
}

function paste(into?: board.Entry) {
  if (!clip.value) return
  const destDir = into?.isDir ? remoteOf(into) : listedPath.value
  if (!destDir) return
  const dest = joinPath(destDir, pasteName(destDir))
  if (dest === clip.value.path) {
    banner.value = { kind: 'info', text: '源和目标相同' }
    return
  }
  const op = clip.value.cut ? 'mv' : 'cp -a'
  const cmd = `${op} ${shQuote(clip.value.path)} ${shQuote(dest)}`
  return act('paste', async () => {
    await sendShell(cmd)
    if (clip.value?.cut) clip.value = null
    banner.value = { kind: 'ok', text: `已在终端执行：${cmd}` }
    await sleep(400)
    await relist()
  })
}

function rename(e: board.Entry) {
  const name = window.prompt('新名称', e.name)?.trim()
  if (!name || name === e.name) return
  if (name.includes('/') || name.includes('\\')) {
    banner.value = { kind: 'err', text: '名称里不能有斜杠' }
    return
  }
  const cmd = `mv ${shQuote(remoteOf(e))} ${shQuote(joinPath(listedPath.value, name))}`
  return act('rename', async () => {
    await sendShell(cmd)
    banner.value = { kind: 'ok', text: `已在终端执行：${cmd}` }
    await sleep(400)
    await relist()
  })
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

function remove(e: board.Entry) {
  const remote = remoteOf(e)
  const hint = e.isDir ? '将删除整个目录（含里面的文件）' : '删除这个文件'
  if (!window.confirm(`${hint}？\n\n${remote}`)) return
  const cmd = e.isDir ? `rm -rf ${shQuote(remote)}` : `rm -f ${shQuote(remote)}`
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
    selected.value = null
  } catch {
    /* 命令可能还在跑，列表下次再刷 */
  }
}

function openMenu(e: MouseEvent, entry: board.Entry | null) {
  if (entry) selected.value = entry
  menu.value = { x: e.clientX, y: e.clientY, entry }
}

function onMenu(id: string) {
  const e = menu.value?.entry
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
      if (e) copy(e)
      break
    case 'cut':
      if (e) cut(e)
      break
    case 'paste':
      void paste(e ?? undefined)
      break
    case 'rename':
      if (e) rename(e)
      break
    case 'delete':
      if (e) remove(e)
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
      <button :disabled="!canOperate || !listedPath || !clip" @click="paste()">粘贴</button>
    </div>
    <div v-if="banner" class="status" :class="banner.kind" :title="banner.text">{{ banner.text }}</div>

    <div class="explorer-body" @contextmenu.prevent="openMenu($event, null)">
      <div class="explorer-head">
        <span>名称</span>
        <span class="col-size">大小</span>
      </div>
      <button
        v-for="e in entries"
        :key="e.name"
        class="row"
        :class="{ selected: selected?.name === e.name, dir: e.isDir }"
        type="button"
        @click="selected = e"
        @dblclick="open(e)"
        @contextmenu.prevent.stop="openMenu($event, e)"
      >
        <span class="icon" :class="e.isDir ? 'icon-dir' : 'icon-file'" aria-hidden="true" />
        <span class="name">{{ e.name }}</span>
        <span class="col-size">{{ e.isDir ? '' : humanSize(e.size) }}</span>
      </button>
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

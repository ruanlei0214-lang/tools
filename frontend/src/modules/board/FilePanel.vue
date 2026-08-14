<script lang="ts" setup>
import { computed, ref, watch } from 'vue'
import {
  Delete,
  Download,
  ListDir,
  PickLocalFile,
  PickSaveTarget,
  Upload,
} from '../../../wailsjs/go/board/Service'
import type { board } from '../../../wailsjs/go/models'

const props = defineProps<{ connected: boolean; defaultPath: string }>()
const emit = defineEmits<{ (e: 'refresh-status'): void }>()

const path = ref('')
// listedPath 是当前这份列表来自哪个目录。上传、删除都落在它上面，
// 而不是落在输入框里那个——输入框可能已经被改成别的目录了，只是还没点「列出」。
const listedPath = ref('')
const entries = ref<board.Entry[]>([])
const selected = ref<board.Entry | null>(null)
const busy = ref('')
const banner = ref<{ kind: 'ok' | 'err' | 'info'; text: string } | null>(null)

// 默认路径来自配置，等它到了再填进输入框。
watch(
  () => props.defaultPath,
  (p) => {
    if (p && !path.value) path.value = p
  },
  { immediate: true },
)

const canOperate = computed(() => props.connected && !busy.value)

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

function list() {
  return act('list', async () => {
    const dir = path.value.trim()
    const rows = await ListDir(dir)
    entries.value = rows
    listedPath.value = dir
    selected.value = null
    banner.value = rows.length
      ? { kind: 'ok', text: `${dir}：${rows.length} 个条目` }
      : { kind: 'info', text: `${dir} 是空目录` }
  })
}

// 双击目录进去。手打路径太累，而这是列表里唯一还算显然的下钻方式。
function open(e: board.Entry) {
  if (!e.isDir) return
  path.value = joinPath(listedPath.value, e.name)
  return list()
}

function joinPath(dir: string, name: string) {
  return dir.endsWith('/') ? `${dir}${name}` : `${dir}/${name}`
}

function upload() {
  return act('upload', async () => {
    if (!listedPath.value) {
      throw new Error('先列出一个目录，再往里上传')
    }
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

function download() {
  const row = selected.value
  if (!row) return
  return act('download', async () => {
    if (row.isDir) {
      throw new Error(`${row.name} 是目录，不支持下载目录`)
    }
    const local = await PickSaveTarget(row.name)
    if (!local) return

    const remote = joinPath(listedPath.value, row.name)
    await Download(remote, local)
    banner.value = { kind: 'ok', text: `已保存到 ${local}` }
  })
}

function remove() {
  const row = selected.value
  if (!row) return
  const remote = joinPath(listedPath.value, row.name)
  // 确认框里放完整远端路径：光看文件名分不清删的是哪个目录下的那一个。
  if (!window.confirm(`删除设备上的这个文件？\n\n${remote}`)) {
    return
  }
  return act('delete', async () => {
    await Delete(remote)
    banner.value = { kind: 'ok', text: `已删除 ${remote}` }
    await relist()
  })
}

// 传完、删完自动刷一次：列表已经和设备不一致了，留着旧的容易接着对错的那一行操作。
// 重列失败不算这次操作失败——文件确实已经传/删掉了。
async function relist() {
  try {
    entries.value = await ListDir(listedPath.value)
    selected.value = null
  } catch {
    /* 保留上一条成功提示，列表下次再刷 */
  }
}

function humanSize(n: number) {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}
</script>

<!-- 单根元素，理由同 CommandPanel：父组件靠 v-show 切标签页。 -->
<template>
  <div>
    <div class="status" :class="banner?.kind" :title="banner?.text">{{ banner?.text }}</div>

    <section class="card">
      <div class="path-row">
        <div class="field">
          <label for="board-path">远端路径</label>
          <input
            id="board-path"
            v-model.trim="path"
            placeholder="/opt"
            :disabled="!!busy"
            @keyup.enter="list"
          />
        </div>
        <button class="primary" :disabled="!canOperate || !path.trim()" @click="list">
          {{ busy === 'list' ? '读取中…' : '列出' }}
        </button>
        <button :disabled="!canOperate || !listedPath" @click="upload">
          {{ busy === 'upload' ? '上传中…' : '上传文件' }}
        </button>
        <button :disabled="!canOperate || !selected || selected.isDir" @click="download">
          {{ busy === 'download' ? '下载中…' : '下载' }}
        </button>
        <button class="danger" :disabled="!canOperate || !selected" @click="remove">
          {{ busy === 'delete' ? '删除中…' : '删除' }}
        </button>
      </div>

      <p v-if="!listedPath" class="empty list-note">填一个目录，点「列出」。</p>
      <p v-else-if="!entries.length" class="empty list-note">{{ listedPath }} 是空目录。</p>
      <table v-else class="list">
        <thead>
          <tr>
            <th>名称</th>
            <th class="col-type">类型</th>
            <th class="col-size">大小</th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="e in entries"
            :key="e.name"
            :class="{ selected: selected?.name === e.name }"
            @click="selected = e"
            @dblclick="open(e)"
          >
            <td class="col-name" :class="{ dir: e.isDir }">{{ e.name }}</td>
            <td class="col-type">
              <span class="tag">{{ e.isDir ? '目录' : '文件' }}</span>
            </td>
            <td class="col-size">{{ e.isDir ? '—' : humanSize(e.size) }}</td>
          </tr>
        </tbody>
      </table>

      <p class="hint">
        上传落在当前列出的目录里；双击目录进去。传输没有进度条，大文件请等它自己结束。
      </p>
    </section>
  </div>
</template>

<style scoped>
/* 与 BoardView 里那条同一个道理：恒占一行，不让它进出 DOM 把下面的表格顶上顶下。 */
.status {
  height: 24px;
  margin-bottom: 8px;
  padding: 0 10px;
  border-radius: 6px;
  overflow: hidden;
  font-size: 12px;
  line-height: 24px;
  text-overflow: ellipsis;
  white-space: nowrap;
  user-select: text;
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

/* 路径和四个操作排一行。标题和输入框高度不一样，靠底对齐才不会错位。 */
.path-row {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: 8px;
}

.path-row .field {
  flex: 1 1 220px;
}

/* 目录可能很长，给它一个上限自己滚，别把整页撑长。 */
.list {
  display: block;
  max-height: 360px;
  margin-top: 12px;
  overflow-y: auto;
}

.list thead,
.list tbody,
.list tr {
  display: table;
  width: 100%;
  table-layout: fixed;
}

/* 表头跟着滚出去的话，滚到下面就分不清哪列是哪列了。 */
.list thead {
  position: sticky;
  top: 0;
  background: var(--panel);
}

.col-name {
  overflow: hidden;
  text-overflow: ellipsis;
  user-select: text;
}

/* 目录加粗，扫一眼就能把它们和文件分开——类型那一列是用来确认的，不是用来找的。 */
.col-name.dir {
  font-weight: 600;
}

.col-type {
  width: 5rem;
}

.col-size {
  width: 6.5rem;
  text-align: right;
}

.list-note {
  margin: 14px 0 0;
}

.hint {
  display: block;
  margin-top: 12px;
}
</style>

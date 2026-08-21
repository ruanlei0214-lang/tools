import { computed, ref } from 'vue'
import { ExportPanel, ImportPanel, ResetPanel, SavePanel } from '../../../wailsjs/go/remote/Service'
import type { remote } from '../../../wailsjs/go/models'
import { confirmDialog } from '../../shell/dialog'

// IO 页与寄存器页的编辑动作完全一样（增删点位、增删分组、改组名、保存、恢复默认），
// 只有读写点位的方式不同。这些动作放在这里共用，两页各写一份的话改了一边忘了另一边，
// 而「两页对齐」是明确要求。
//
// 草稿是深拷贝：直接改 props.tab 的话，保存失败后界面上留着的是一份盘上并不存在的清单，
// 而后端写进去的那份才是准。
function cloneTab(t: remote.Tab): remote.Tab {
  return {
    ...t,
    groups: (t.groups ?? []).map((g) => ({
      title: g.title,
      points: (g.points ?? []).map((p) => ({ ...p })),
    })),
  } as remote.Tab
}

function blankPoint(type: string): remote.Point {
  return {
    label: '',
    type,
    port: 0,
    onValue: 1,
    offValue: 0,
    value: '',
    pulseMs: 0,
    danger: false,
    hint: '',
  } as remote.Point
}

export function usePanelEdit(getTab: () => remote.Tab, defaultType: string) {
  const editing = ref(false)
  const draft = ref<remote.Tab | null>(null)
  // editAt 是正在展开编辑的那个点位的位置，null 表示没有展开的编辑行。
  const editAt = ref<{ g: number; p: number } | null>(null)
  // 「添加点位」是先往草稿里放一行再展开编辑它，取消时要把这一行撤掉，
  // 否则列表里会留下一个空点位。
  const adding = ref(false)

  // 编辑中看草稿，平时看后端那份。
  const groups = computed(() => (editing.value ? (draft.value?.groups ?? []) : (getTab().groups ?? [])))

  const editingPoint = computed(() => {
    const at = editAt.value
    if (!at || !draft.value) return null
    return draft.value.groups?.[at.g]?.points?.[at.p] ?? null
  })

  function isEditingAt(g: number, p: number): boolean {
    return editAt.value?.g === g && editAt.value?.p === p
  }

  function start() {
    draft.value = cloneTab(getTab())
    editAt.value = null
    adding.value = false
    editing.value = true
  }

  function stop() {
    editing.value = false
    draft.value = null
    editAt.value = null
    adding.value = false
  }

  function addGroup() {
    if (!draft.value) return
    const list = draft.value.groups ?? (draft.value.groups = [])
    list.push({ title: '新分组', points: [blankPoint(defaultType)] } as remote.Group)
    // 新分组自带一个点位：后端不收一个点位都没有的分组，加完组直接保存会被拒。
    editAt.value = { g: list.length - 1, p: 0 }
    adding.value = true
  }

  async function removeGroup(gi: number): Promise<boolean> {
    const g = draft.value?.groups?.[gi]
    if (!g) return false
    const n = g.points?.length ?? 0
    const ok = await confirmDialog(
      `删除分组「${g.title || '未命名'}」？这一组有 ${n} 个点位，会一起删掉。`,
      { title: '删除分组', danger: true, confirmText: '删除' },
    )
    if (!ok) return false
    draft.value!.groups!.splice(gi, 1)
    editAt.value = null
    adding.value = false
    return true
  }

  function addPoint(gi: number) {
    const g = draft.value?.groups?.[gi]
    if (!g) return
    const points = g.points ?? (g.points = [])
    points.push(blankPoint(defaultType))
    editAt.value = { g: gi, p: points.length - 1 }
    adding.value = true
  }

  function editPoint(gi: number, pi: number) {
    editAt.value = { g: gi, p: pi }
    adding.value = false
  }

  function applyPoint(p: remote.Point) {
    const at = editAt.value
    if (!at || !draft.value) return
    draft.value.groups![at.g].points![at.p] = p
    editAt.value = null
    adding.value = false
  }

  function cancelPoint() {
    const at = editAt.value
    if (at && adding.value && draft.value) {
      draft.value.groups![at.g].points!.splice(at.p, 1)
    }
    editAt.value = null
    adding.value = false
  }

  async function removePoint(gi: number, pi: number): Promise<boolean> {
    const p = draft.value?.groups?.[gi]?.points?.[pi]
    if (!p) return false
    const ok = await confirmDialog(`删除点位「${p.label || `${p.type}${p.port}`}」？`, {
      title: '删除点位',
      danger: true,
      confirmText: '删除',
    })
    if (!ok) return false
    draft.value!.groups![gi].points!.splice(pi, 1)
    editAt.value = null
    adding.value = false
    return true
  }

  // save / reset 都把后端返回的整份配置交回调用方，由它更新页面——
  // 归一化（类型转大写、名称缺省补 DO15、onValue/offValue 补 1/0）发生在后端，
  // 前端自己算一遍迟早和后端对不上。
  async function save(): Promise<remote.Settings> {
    const cfg = await SavePanel(draft.value as remote.Tab)
    stop()
    return cfg
  }

  async function reset(): Promise<remote.Settings> {
    const cfg = await ResetPanel(getTab().kind)
    stop()
    return cfg
  }

  // 导入导出不经过编辑草稿：导出的是后端正在用的那份，导入成功就丢掉未保存的改动。
  async function exportFile(): Promise<string> {
    return ExportPanel(getTab().kind)
  }

  async function importFile(): Promise<remote.PanelFileResult> {
    const r = await ImportPanel(getTab().kind)
    if (!r.canceled) stop()
    return r
  }

  return {
    editing,
    draft,
    groups,
    editAt,
    editingPoint,
    isEditingAt,
    start,
    stop,
    addGroup,
    removeGroup,
    addPoint,
    editPoint,
    applyPoint,
    cancelPoint,
    removePoint,
    save,
    reset,
    exportFile,
    importFile,
  }
}

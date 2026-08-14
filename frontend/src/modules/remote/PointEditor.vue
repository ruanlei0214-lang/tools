<script lang="ts" setup>
import { computed, reactive, ref, watch } from 'vue'
import type { remote } from '../../../wailsjs/go/models'

// IO 页和寄存器页共用这一个表单。两页只差类型可选值和端口的叫法，
// 各写一份的话改了一边忘了另一边，界面立刻不对齐——而「两页对齐」是明确要求。
const props = defineProps<{
  point: remote.Point
  types: string[]
  portLabel: string
}>()

const emit = defineEmits<{
  (e: 'submit', point: remote.Point): void
  (e: 'cancel'): void
}>()

// 字段就是 JSON 里那几个，一个不多一个不少。
const draft = reactive({
  label: '',
  type: '',
  port: 0,
  onValue: 1,
  offValue: 0,
  value: '',
  pulseMs: 0,
  danger: false,
  hint: '',
})

const first = ref<HTMLInputElement | null>(null)

// 开关量才有 ON/OFF 和点动。INT / FLOAT / AO / AI 配的是一个可填值，
// 再摆两个翻转值只会让人以为下发的还是 ON/OFF。
const digital = computed(() => {
  const t = draft.type
  return t === 'DI' || t === 'DO' || t === 'BOOL'
})

watch(
  () => props.point,
  (p) => {
    draft.label = p.label ?? ''
    draft.type = p.type || props.types[0]
    draft.port = p.port ?? 0
    draft.onValue = p.onValue ?? 1
    draft.offValue = p.offValue ?? 0
    draft.value = p.value ?? ''
    draft.pulseMs = p.pulseMs ?? 0
    draft.danger = !!p.danger
    draft.hint = p.hint ?? ''
    // 展开就把光标放在名称上：改点位十次里有九次是改名字。
    void Promise.resolve().then(() => first.value?.focus())
  },
  { immediate: true },
)

// 名称留空不在这里拦。后端会按类型加端口补一个（DO15、BOOL10000），
// 前端自己算一遍迟早和后端对不上。
function submit() {
  emit('submit', {
    label: draft.label.trim(),
    type: draft.type,
    port: Number(draft.port) || 0,
    onValue: digital.value ? Number(draft.onValue) : 0,
    offValue: digital.value ? Number(draft.offValue) : 0,
    value: digital.value ? '' : draft.value.trim(),
    pulseMs: digital.value ? Number(draft.pulseMs) || 0 : 0,
    danger: draft.danger,
    hint: draft.hint.trim(),
  } as remote.Point)
}
</script>

<template>
  <form class="pe" @submit.prevent="submit" @keyup.esc="emit('cancel')">
    <label class="pe-f pe-name">
      <span>名称</span>
      <input ref="first" v-model="draft.label" placeholder="留空按类型和端口生成" />
    </label>
    <label class="pe-f pe-type">
      <span>类型</span>
      <select v-model="draft.type">
        <option v-for="t in types" :key="t" :value="t">{{ t }}</option>
      </select>
    </label>
    <label class="pe-f pe-port">
      <span>{{ portLabel }}</span>
      <input v-model.number="draft.port" type="number" min="0" />
    </label>
    <template v-if="digital">
      <label class="pe-f pe-num">
        <span>ON 值</span>
        <input v-model.number="draft.onValue" type="number" step="any" />
      </label>
      <label class="pe-f pe-num">
        <span>OFF 值</span>
        <input v-model.number="draft.offValue" type="number" step="any" />
      </label>
      <label class="pe-f pe-num" title="填 0 就没有点动按钮；要有的话填 20-10000 毫秒">
        <span>点动 ms</span>
        <input v-model.number="draft.pulseMs" type="number" min="0" max="10000" />
      </label>
    </template>
    <label v-else class="pe-f pe-value" :title="draft.type === 'FLOAT' ? '按字符串原样下发' : '预填到下发框里的默认值'">
      <span>默认值</span>
      <input
        v-model="draft.value"
        :type="draft.type === 'INT' ? 'number' : 'text'"
        :step="draft.type === 'INT' ? 1 : undefined"
        :placeholder="draft.type === 'INT' ? '整数' : draft.type === 'FLOAT' ? '数字或文本' : '数字'"
      />
    </label>
    <label class="pe-f pe-hint">
      <span>备注</span>
      <input v-model="draft.hint" placeholder="鼠标悬停时显示" />
    </label>
    <label class="pe-f pe-flag" title="标红，给急停一类的动作用">
      <span>醒目</span>
      <input v-model="draft.danger" type="checkbox" />
    </label>
    <div class="pe-act">
      <button class="primary" type="submit">确定</button>
      <button type="button" @click="emit('cancel')">取消</button>
    </div>
  </form>
</template>

<style scoped>
/* 编辑区在卡片里就地展开，占掉那一行点位的位置——点位列表是编辑时要对着看的东西，
   所以不弹窗、不另占一块区域。字段多，窄列里自动折成两行。 */
.pe {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  gap: 6px;
  margin: 4px 0;
  padding: 7px;
  border: 1px solid var(--accent);
  border-radius: 7px;
  background: var(--accent-soft);
}

.pe-f {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  font-size: 11px;
  color: var(--text-dim);
}

.pe-f input,
.pe-f select {
  padding: 5px 7px;
  font-size: 12px;
}

.pe-name {
  flex: 1 1 8rem;
  min-width: 6rem;
}

.pe-hint {
  flex: 1 1 9rem;
  min-width: 6rem;
}

.pe-type {
  flex: 0 0 5.2rem;
}

.pe-port {
  flex: 0 0 5.5rem;
}

.pe-num {
  flex: 0 0 4.6rem;
}

.pe-value {
  flex: 1 1 7rem;
  min-width: 5rem;
}

.pe-flag {
  flex: 0 0 auto;
  align-items: center;
}

.pe-flag input {
  width: auto;
  margin-top: 2px;
}

.pe-act {
  display: flex;
  flex: 0 0 auto;
  gap: 5px;
  margin-left: auto;
}

.pe-act button {
  padding: 6px 12px;
  font-size: 12px;
}
</style>

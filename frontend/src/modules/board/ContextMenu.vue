<script lang="ts" setup>
import { onMounted, onUnmounted } from 'vue'

export type MenuItem = {
  id: string
  label: string
  danger?: boolean
  disabled?: boolean
}

const props = defineProps<{
  x: number
  y: number
  items: MenuItem[]
}>()
const emit = defineEmits<{
  (e: 'pick', id: string): void
  (e: 'close'): void
}>()

function onClose() {
  emit('close')
}

function onKey(e: KeyboardEvent) {
  if (e.key === 'Escape') emit('close')
}

onMounted(() => {
  requestAnimationFrame(() => {
    window.addEventListener('click', onClose)
    window.addEventListener('contextmenu', onClose)
    window.addEventListener('keydown', onKey)
  })
})

onUnmounted(() => {
  window.removeEventListener('click', onClose)
  window.removeEventListener('contextmenu', onClose)
  window.removeEventListener('keydown', onKey)
})

function pick(item: MenuItem) {
  if (item.disabled) return
  emit('pick', item.id)
}

const left = Math.min(props.x, window.innerWidth - 168)
const top = Math.min(props.y, window.innerHeight - 12 - props.items.length * 28)
</script>

<template>
  <div class="ctx" :style="{ left: `${left}px`, top: `${top}px` }" @click.stop @contextmenu.prevent>
    <button
      v-for="item in items"
      :key="item.id"
      :class="{ harm: item.danger }"
      :disabled="item.disabled"
      @click="pick(item)"
    >
      {{ item.label }}
    </button>
  </div>
</template>

<style scoped>
.ctx {
  position: fixed;
  z-index: 40;
  display: grid;
  min-width: 9rem;
  padding: 4px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: var(--panel);
  box-shadow: 0 8px 24px rgb(0 0 0 / 14%);
}

.ctx button {
  justify-content: flex-start;
  width: 100%;
  min-height: 28px;
  padding: 0 10px;
  border: none;
  border-radius: 4px;
  background: none;
  color: var(--text);
  text-align: left;
}

.ctx button.harm {
  color: var(--err);
}

.ctx button:hover:not(:disabled) {
  background: var(--accent-soft);
  color: var(--accent);
}

.ctx button.harm:hover:not(:disabled) {
  background: var(--err-soft);
  color: var(--err);
}
</style>

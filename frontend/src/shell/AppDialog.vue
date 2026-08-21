<script lang="ts" setup>
import { closeDialog, dialog } from './dialog'

// 应用内弹窗的宿主，挂在 App.vue 根部。样式和「关于」弹窗同一套：
// 遮罩点外面相当于取消。
</script>

<template>
  <div v-if="dialog.visible" class="modal-mask" @click.self="closeDialog(false)">
    <div class="modal" role="dialog" :aria-label="dialog.title">
      <h2 class="modal-title">{{ dialog.title }}</h2>
      <p class="dialog-text">{{ dialog.text }}</p>
      <div class="modal-actions">
        <button v-if="dialog.isConfirm" @click="closeDialog(false)">取消</button>
        <button
          class="primary"
          :class="{ danger: dialog.danger }"
          autofocus
          @click="closeDialog(true)"
        >
          {{ dialog.confirmText }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.modal-mask {
  position: fixed;
  inset: 0;
  z-index: 20;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(31, 36, 48, 0.45);
}

.modal {
  width: 400px;
  max-width: calc(100vw - 48px);
  padding: 16px 20px;
  background: var(--panel);
  border-radius: var(--radius);
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.18);
}

.modal-title {
  margin: 0;
  font-size: 14px;
  font-weight: 600;
}

/* 多行文本按原样排版：要点一行一条，靠换行而不是塞进一段里。 */
.dialog-text {
  margin: 10px 0 0;
  font-size: 12px;
  line-height: 1.8;
  white-space: pre-wrap;
  word-break: break-word;
  user-select: text;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 16px;
}
</style>

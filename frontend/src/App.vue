<script lang="ts" setup>
import { computed, ref } from 'vue'
import { modules } from './shell/registry'
import { APP_VERSION } from './shell/version'

const activeId = ref(modules[0]?.id ?? '')
const active = computed(() => modules.find((m) => m.id === activeId.value))
const showAbout = ref(false)
</script>

<template>
  <div class="shell">
    <aside class="sidebar">
      <div class="brand">
        <div class="brand-title">Estun Codroid 机器人工具箱</div>
        <div class="brand-sub">{{ modules.length }} 个模块</div>
      </div>
      <nav class="nav">
        <button
          v-for="m in modules"
          :key="m.id"
          class="nav-item"
          :class="{ active: m.id === activeId }"
          @click="activeId = m.id"
        >
          <span class="nav-name">{{ m.name }}</span>
          <span class="nav-desc">{{ m.description }}</span>
        </button>
      </nav>
      <button class="about-entry" @click="showAbout = true">关于</button>
    </aside>

    <main class="content">
      <keep-alive>
        <component :is="active.view" v-if="active" :key="active.id" />
      </keep-alive>
      <p v-if="!active" class="empty">还没有任何模块，在 src/modules/ 下新建一个目录即可。</p>
    </main>
  </div>

  <div v-if="showAbout" class="modal-mask" @click.self="showAbout = false">
    <div class="modal">
      <h2 class="modal-title">关于</h2>
      <!-- 只列工具箱和当前模块两行。模块各自独立编号，把没在用的模块版本
           一起铺出来，等于让人在一堆数字里找自己要的那个。 -->
      <dl class="about-list">
        <dt>Estun Codroid 机器人工具箱</dt>
        <dd>{{ APP_VERSION }}</dd>
        <template v-if="active">
          <dt>{{ active.name }}</dt>
          <dd>{{ active.version }}</dd>
        </template>
      </dl>
      <div class="modal-actions">
        <button class="primary" @click="showAbout = false">知道了</button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.about-entry {
  /* 顶到侧栏底部：它不是导航项，不该混在模块列表里争注意力。 */
  margin-top: auto;
  padding: 11px 16px;
  border: none;
  border-top: 1px solid var(--border);
  background: transparent;
  color: var(--text-dim);
  font-family: inherit;
  font-size: 12px;
  text-align: left;
  cursor: pointer;
}

.about-entry:hover {
  color: var(--text);
  background: var(--bg);
}

/* 弹窗样式没进全局，和 netcfg 里那个各留一份。项目没有 UI 组件库，
   把它提成共享控件就等于凭空造一层所有人都得依赖的东西。 */
.modal-mask {
  position: fixed;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(31, 36, 48, 0.45);
}

.modal {
  width: 320px;
  padding: 18px 20px;
  background: var(--panel);
  border-radius: var(--radius);
  box-shadow: 0 10px 30px rgba(0, 0, 0, 0.18);
}

.modal-title {
  margin: 0;
  font-size: 15px;
  font-weight: 600;
}

.about-list {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: 8px 16px;
  margin: 16px 0 0;
  font-size: 13px;
}

.about-list dt {
  color: var(--text-dim);
}

.about-list dd {
  margin: 0;
  font-variant-numeric: tabular-nums;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 18px;
}
</style>

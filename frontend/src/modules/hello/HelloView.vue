<script lang="ts" setup>
import { ref } from 'vue'
import { Greet } from '../../../wailsjs/go/hello/Service'

const name = ref('')
const reply = ref('')

async function greet() {
  reply.value = await Greet(name.value)
}
</script>

<template>
  <div class="module-head">
    <h1 class="module-title">Hello World</h1>
    <p class="module-desc">最小示例：前端调用本模块的 Go 方法，并显示返回值。</p>
  </div>

  <section class="card">
    <h2 class="card-title">打个招呼</h2>
    <div class="field-row">
      <div class="field">
        <label for="hello-name">名字</label>
        <input id="hello-name" v-model.trim="name" placeholder="留空就是「世界」" @keyup.enter="greet" />
      </div>
    </div>
    <div class="actions">
      <button class="primary" @click="greet">调用 Go</button>
      <span class="hint">对应后端 internal/modules/hello/hello.go 的 Service.Greet</span>
    </div>
  </section>

  <div v-if="reply" class="banner ok">{{ reply }}</div>
</template>

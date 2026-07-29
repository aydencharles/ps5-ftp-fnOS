<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { FolderOpen, Send, Settings } from '@lucide/vue'
import logoUrl from '../../../assets/logo.png'
import TaskCenter from './components/TaskCenter.vue'
import { useSystemStore } from './stores/system'
import { useProfilesStore } from './stores/profiles'
import { useLibraryStore } from './stores/library'
import { useTasksStore } from './stores/tasks'

const route = useRoute()
const system = useSystemStore()
const profiles = useProfilesStore()
const library = useLibraryStore()
const tasks = useTasksStore()

const pageInfo = computed(() => {
  const descriptions: Record<string, string> = {
    '/': '从飞牛存储选择文件发送到 PS5',
    '/tasks': '查看传输状态、实时速度和历史结果',
    '/files': '浏览并管理 PS5 上的文件和目录',
    '/settings': '管理 PS5 连接与传输并发',
  }
  return {
    title: String(route.meta.title || 'PS5 FTP Manager'),
    description: descriptions[route.path] || '',
  }
})
onMounted(async () => {
  try {
    const data = await system.bootstrap()
    profiles.hydrate(data.profiles)
    library.hydrate(data.library_roots)
    tasks.hydrate(data.tasks)
    tasks.connect()
  } catch (error) {
    system.error = error instanceof Error ? error.message : String(error)
  }
})
onBeforeUnmount(() => tasks.stream?.close())
</script>

<template>
  <div class="app-shell">
    <header class="app-header">
      <router-link class="brand" to="/" aria-label="返回首页">
        <img class="brand-logo" :src="logoUrl" alt="">
        <span class="brand-name">PS5 FTP Manager</span>
      </router-link>
      <nav class="main-nav" aria-label="主导航">
        <router-link to="/"><Send :size="15" />传输</router-link>
        <router-link to="/files"><FolderOpen :size="15" />PS5 文件</router-link>
        <router-link to="/settings"><Settings :size="15" />设置</router-link>
      </nav>
      <div class="app-state">
        <span :class="['status-dot', system.ready && 'is-ready']" />
        <span>{{ system.ready ? `服务正常 · v${system.version}` : '正在连接服务' }}</span>
      </div>
    </header>

    <main class="page-shell">
      <div class="page-heading">
        <div>
          <h1>{{ pageInfo.title }}</h1>
          <p>{{ pageInfo.description }}</p>
        </div>
        <span class="network-note">仅限可信局域网</span>
      </div>
      <t-alert v-if="system.error" theme="error" :message="system.error" />
      <router-view v-else />
    </main>
    <TaskCenter />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Settings } from '@lucide/vue'
import { MessagePlugin } from 'tdesign-vue-next'
import FnOSIcon from './components/FnOSIcon.vue'
import PlayStationIcon from './components/PlayStationIcon.vue'
import TaskCenter from './components/TaskCenter.vue'
import { useSystemStore } from './stores/system'
import { useProfilesStore } from './stores/profiles'
import { useLibraryStore } from './stores/library'
import { useTasksStore } from './stores/tasks'
import { useExtractionTasksStore } from './stores/extractions'
import { parentPath } from './api'

const route = useRoute()
const system = useSystemStore()
const profiles = useProfilesStore()
const library = useLibraryStore()
const tasks = useTasksStore()
const extractions = useExtractionTasksStore()

async function showError(error: unknown) {
  await MessagePlugin.error(error instanceof Error ? error.message : String(error))
}

watch(() => extractions.items, (current, previous) => {
  const oldStates = new Map((previous || []).map((task) => [task.id, task.state]))
  const affectsCurrentDirectory = current.some((task) => {
    if (task.state !== 'succeeded' || !oldStates.has(task.id) || oldStates.get(task.id) === 'succeeded') return false
    return (task.source.root_id === library.rootId && parentPath(task.source.path) === library.path)
      || (task.destination_parent.root_id === library.rootId && task.destination_parent.path === library.path)
  })
  if (affectsCurrentDirectory) void library.load().catch(showError)
})

const pageInfo = computed(() => {
  const descriptions: Record<string, string> = {
    '/': '从飞牛存储选择文件发送到 PS5',
    '/tasks': '查看传输状态、实时速度和历史结果',
    '/files': '浏览并管理 PS5 上的文件和目录',
    '/settings': '管理连接、传输与界面偏好',
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
    extractions.hydrate(data.extraction_tasks)
    tasks.connect()
    extractions.connect()
  } catch (error) {
    system.error = error instanceof Error ? error.message : String(error)
    await MessagePlugin.error(system.error)
  }
})
onBeforeUnmount(() => { tasks.stream?.close(); extractions.stream?.close() })
</script>

<template>
  <div class="app-shell">
    <header class="app-header">
      <nav class="main-nav" aria-label="主导航">
        <router-link to="/"><FnOSIcon :size="16" />飞牛传输</router-link>
        <router-link to="/files"><PlayStationIcon :size="16" />PS5 文件</router-link>
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
      <router-view />
    </main>
    <TaskCenter />
  </div>
</template>

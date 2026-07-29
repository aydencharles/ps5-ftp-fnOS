<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { ArrowUp, Folder, RefreshCw } from '@lucide/vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { api, parentPath } from '../api'
import { useLibraryStore } from '../stores/library'
import type { Entry, SourceLocator } from '../types'

const props = defineProps<{ initial: SourceLocator }>()
const emit = defineEmits<{ change: [locator: SourceLocator] }>()
const library = useLibraryStore()
const rootId = ref(props.initial.root_id)
const currentPath = ref(props.initial.path)
const entries = ref<Entry[]>([])
const loading = ref(false)

const directories = computed(() => entries.value.filter((entry) => entry.is_dir).sort((left, right) => left.name.localeCompare(right.name, 'zh-CN', { numeric: true })))
const currentRoot = computed(() => library.roots.find((root) => root.id === rootId.value))
const displayPath = computed(() => currentPath.value ? `${currentRoot.value?.label || 'fnOS'} / ${currentPath.value}` : currentRoot.value?.label || 'fnOS')

async function load(path = currentPath.value) {
  if (!rootId.value) return
  loading.value = true
  try {
    const query = new globalThis.URLSearchParams({ root_id: rootId.value, path })
    const data = await api<{ entries: Entry[]; path: string }>(`/api/v1/library/entries?${query}`)
    currentPath.value = data.path || path
    entries.value = data.entries || []
    emit('change', { root_id: rootId.value, path: currentPath.value })
  } catch (reason) {
    entries.value = []
    await MessagePlugin.error(reason instanceof Error ? reason.message : String(reason))
  } finally { loading.value = false }
}

watch(() => props.initial, (value) => {
  if (value.root_id === rootId.value && value.path === currentPath.value) return
  rootId.value = value.root_id
  currentPath.value = value.path
  void load()
}, { deep: true })
watch(rootId, () => { currentPath.value = ''; void load('') })
onMounted(() => { void load() })
</script>

<template>
  <div class="local-directory-picker">
    <div class="local-picker-toolbar">
      <t-select v-model="rootId" :options="library.storageRoots.map(root => ({ label: root.label, value: root.id }))" class="location-select" />
      <t-button variant="text" size="small" :disabled="!currentPath" @click="load(parentPath(currentPath))"><ArrowUp :size="15" />上一级</t-button>
      <t-button variant="text" size="small" @click="load()"><RefreshCw :size="14" />刷新</t-button>
    </div>
    <div class="local-picker-list" :class="{ 'is-loading': loading }">
      <button v-for="entry in directories" :key="entry.path" type="button" @click="load(entry.path)">
        <Folder :size="17" /><span>{{ entry.name }}</span><small>打开</small>
      </button>
      <div v-if="!loading && !directories.length" class="plain-empty compact">当前目录没有子文件夹</div>
    </div>
    <div class="local-picker-target"><span>解压到</span><strong>{{ displayPath }}</strong></div>
  </div>
</template>

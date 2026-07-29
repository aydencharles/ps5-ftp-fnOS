<script setup lang="ts">
import { computed, ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import FileBrowser from '../components/FileBrowser.vue'
import FnOSIcon from '../components/FnOSIcon.vue'
import { notifyError } from '../errorFeedback'
import { useLibraryStore } from '../stores/library'
import { useProfilesStore } from '../stores/profiles'
import { useTasksStore } from '../stores/tasks'
import type { Entry, SourceLocator } from '../types'

type ConflictPolicy = 'smart' | 'overwrite' | 'fail'

const library = useLibraryStore()
const profiles = useProfilesStore()
const tasks = useTasksStore()
const pickerVisible = ref(false)
const submitting = ref(false)
const remoteSources = ref<Entry[]>([])
const localDestination = ref<SourceLocator | null>(null)
const conflict = ref<ConflictPolicy>('smart')

const localDestinationLabel = computed(() => {
  if (!localDestination.value) return '尚未选择飞牛目录'
  const root = library.roots.find((item) => item.id === localDestination.value?.root_id)?.label || 'fnOS'
  return localDestination.value.path ? `${root} / ${localDestination.value.path}` : root
})

function openCopyToFnOS(entries: Entry[]) {
  if (!entries.length) return
  remoteSources.value = entries
  localDestination.value = { root_id: library.rootId, path: library.path }
  pickerVisible.value = true
}

function chooseLocalDestination(locator: SourceLocator) {
  localDestination.value = locator
}

async function copyToFnOS() {
  if (!profiles.selectedId || !remoteSources.value.length || !localDestination.value) return
  submitting.value = true
  try {
    const task = await tasks.createDownload(profiles.selectedId, remoteSources.value.map((entry) => entry.path), localDestination.value, conflict.value)
    pickerVisible.value = false
    await MessagePlugin.success(`下载任务 ${task.id.slice(0, 8)} 已加入队列`)
  } catch (error) {
    await notifyError(error)
  } finally { submitting.value = false }
}
</script>

<template>
  <section class="files-page">
    <FileBrowser mode="manage" @copy-to-fnos="openCopyToFnOS" />
  </section>

  <t-dialog v-model:visible="pickerVisible" dialog-class-name="transfer-picker-dialog" header="复制到飞牛" width="960px" :confirm-btn="{ content: `复制 ${remoteSources.length} 项`, loading: submitting, disabled: !localDestination }" @confirm="copyToFnOS">
    <section class="transfer-destination-picker is-local-destination-picker">
      <header>
        <div><strong>飞牛目标位置</strong><small>当前浏览目录就是保存位置；文件管理操作已禁用。</small></div>
        <t-select v-model="library.rootId" :options="library.storageRoots.map(root => ({ label: root.label, value: root.id }))" placeholder="选择存储空间" class="location-select" />
      </header>
      <FileBrowser mode="source" :picker="true" compact @choose-local-path="chooseLocalDestination" />
      <div class="local-destination-summary"><FnOSIcon :size="15" /><span>复制到</span><strong>{{ localDestinationLabel }}</strong></div>
      <div class="download-conflict-policy">
        <strong>同名文件处理</strong>
        <t-radio-group v-model="conflict">
          <t-radio value="smart">智能处理</t-radio><t-radio value="overwrite">全部覆盖</t-radio><t-radio value="fail">遇到同名文件停止</t-radio>
        </t-radio-group>
      </div>
    </section>
  </t-dialog>
</template>

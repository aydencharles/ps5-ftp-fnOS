<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ArrowRight } from '@lucide/vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { formatBytes } from '../api'
import BrowserDeviceBar from '../components/BrowserDeviceBar.vue'
import FileBrowser from '../components/FileBrowser.vue'
import FnOSIcon from '../components/FnOSIcon.vue'
import PlayStationIcon from '../components/PlayStationIcon.vue'
import { useLibraryStore } from '../stores/library'
import { useProfilesStore } from '../stores/profiles'
import { useTasksStore } from '../stores/tasks'
import type { SourceLocator } from '../types'

const library = useLibraryStore()
const profiles = useProfilesStore()
const tasks = useTasksStore()
type ConflictPolicy = 'smart' | 'overwrite' | 'fail'

const destination = ref('/')
const destinationPickerVisible = ref(false)
const confirmVisible = ref(false)
const submitting = ref(false)
const conflict = ref<ConflictPolicy>('smart')
const selectedSources = ref<SourceLocator[]>([])

const currentRootLabel = computed(() => library.roots.find((root) => root.id === library.rootId)?.label || 'fnOS')
const selectedEntries = computed(() => selectedSources.value.map((locator) => library.entries.find((entry) => entry.path === locator.path)).filter(Boolean))
const selectedSize = computed(() => selectedEntries.value.reduce((sum, entry) => sum + (entry?.is_dir ? 0 : entry?.size || 0), 0))
const selectedLabel = computed(() => {
  const first = selectedSources.value[0]
  if (!first) return ''
  const name = first.path.split('/').filter(Boolean).pop() || currentRootLabel.value
  return selectedSources.value.length === 1 ? name : `${name} 等 ${selectedSources.value.length} 项`
})
const profileLabel = computed(() => profiles.selected?.name || '尚未选择 PS5')
const conflictLabel = computed(() => ({ smart: '智能处理同名文件', overwrite: '全部重新上传', fail: '遇到同名文件停止' })[conflict.value])

watch(() => profiles.selectedId, () => {
  destination.value = profiles.selected?.base_path || '/'
}, { immediate: true })

function openCopyToPS5() {
  if (!selectedSources.value.length) return
  destination.value = profiles.selected?.base_path || '/'
  destinationPickerVisible.value = true
}

function confirmDestination() {
  if (!profiles.selectedId) return
  destinationPickerVisible.value = false
  confirmVisible.value = true
}

async function submit() {
  if (!profiles.selectedId || !selectedSources.value.length) return
  submitting.value = true
  try {
    const task = await tasks.create(profiles.selectedId, selectedSources.value, destination.value, conflict.value)
    selectedSources.value = []
    confirmVisible.value = false
    await MessagePlugin.success(`任务 ${task.id.slice(0, 8)} 已加入队列`)
  } catch (error) {
    await MessagePlugin.error(error instanceof Error ? error.message : String(error))
  } finally { submitting.value = false }
}
</script>

<template>
  <section class="transfer-workbench transfer-workbench-single">
    <div class="browser-pane unified-browser-pane">
      <BrowserDeviceBar title="飞牛存储" subtitle="双击目录进入，勾选要传输的内容">
        <template #icon><FnOSIcon :size="18" /></template>
        <template #control><t-select v-model="library.rootId" :options="library.storageRoots.map(root => ({ label: root.label, value: root.id }))" placeholder="选择存储空间" class="location-select devicebar-select" /></template>
      </BrowserDeviceBar>
      <FileBrowser v-model:selected-sources="selectedSources" mode="source" compact @copy-to-ps5="openCopyToPS5" />
    </div>
  </section>

  <t-dialog v-model:visible="destinationPickerVisible" dialog-class-name="transfer-picker-dialog" header="选择 PS5 目标位置" width="960px" :confirm-btn="{ content: profiles.selectedId ? '下一步' : '请选择 PS5', disabled: !profiles.selectedId }" @confirm="confirmDestination">
    <section class="transfer-destination-picker">
      <header>
        <div><strong>PS5 目的地</strong><small>此处浏览的位置就是接收文件的位置</small></div>
        <t-select v-model="profiles.selectedId" :options="profiles.items.map(profile => ({ label: profile.name, value: profile.id }))" placeholder="选择 PS5" class="location-select" />
      </header>
      <FileBrowser v-model="destination" mode="destination" compact />
    </section>
  </t-dialog>

  <t-dialog v-model:visible="confirmVisible" header="确认传输任务" width="620px" :confirm-btn="{ content: '创建任务', loading: submitting }" @confirm="submit">
    <div class="transfer-confirm">
      <div class="confirm-route-card">
        <div><span class="confirm-route-icon"><FnOSIcon :size="18" /></span><small>来源</small><strong>{{ currentRootLabel }} / {{ selectedLabel }}</strong></div>
        <ArrowRight class="confirm-route-arrow" :size="18" />
        <div><span class="confirm-route-icon"><PlayStationIcon :size="18" /></span><small>目的地</small><strong>{{ profileLabel }} · {{ destination }}</strong></div>
      </div>
      <dl class="confirm-facts">
        <div><dt>已选内容</dt><dd>{{ selectedSources.length }} 项</dd></div>
        <div><dt>当前可统计大小</dt><dd>{{ selectedSize ? formatBytes(selectedSize) : '扫描任务后确定' }}</dd></div>
      </dl>
      <div class="confirm-policy">
        <header><strong>同名文件处理</strong><span>{{ conflictLabel }}</span></header>
        <t-radio-group v-model="conflict">
          <t-radio value="smart">智能处理</t-radio>
          <t-radio value="overwrite">全部覆盖</t-radio>
          <t-radio value="fail">停止任务</t-radio>
        </t-radio-group>
      </div>
    </div>
  </t-dialog>
</template>

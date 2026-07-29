<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ArrowRight } from '@lucide/vue'
import { MessagePlugin } from 'tdesign-vue-next'
import type { FormInstanceFunctions, FormRules } from 'tdesign-vue-next'
import { formatBytes, parentPath } from '../api'
import { notifyError } from '../errorFeedback'
import BrowserDeviceBar from '../components/BrowserDeviceBar.vue'
import FileBrowser from '../components/FileBrowser.vue'
import LocalDirectoryPicker from '../components/LocalDirectoryPicker.vue'
import FnOSIcon from '../components/FnOSIcon.vue'
import PlayStationIcon from '../components/PlayStationIcon.vue'
import { useLibraryStore } from '../stores/library'
import { useProfilesStore } from '../stores/profiles'
import { useTasksStore } from '../stores/tasks'
import { useExtractionTasksStore } from '../stores/extractions'
import type { Entry, SourceLocator } from '../types'
import { optionalPasswordRules } from '../formValidation'

const library = useLibraryStore()
const profiles = useProfilesStore()
const tasks = useTasksStore()
const extractions = useExtractionTasksStore()
type ConflictPolicy = 'smart' | 'overwrite' | 'fail'

const destination = ref('/')
const destinationPickerVisible = ref(false)
const confirmVisible = ref(false)
const submitting = ref(false)
const conflict = ref<ConflictPolicy>('smart')
const selectedSources = ref<SourceLocator[]>([])
const extractVisible = ref(false)
const extractSubmitting = ref(false)
const extractSource = ref<Entry | null>(null)
const extractSourceLocator = ref<SourceLocator | null>(null)
const extractDestination = ref<SourceLocator | null>(null)
const extractPassword = ref('')
const deleteSources = ref(false)
const extractionForm = ref<FormInstanceFunctions>()
const extractionFormData = computed(() => ({ password: extractPassword.value }))
const extractionRules: FormRules = { password: optionalPasswordRules }

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
const extractionFolderName = computed(() => extractSource.value?.name.replace(/\.7z(?:\.001)?$/i, '') || '')

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
    await notifyError(error)
  } finally { submitting.value = false }
}

function openExtraction(entry: Entry) {
  extractSource.value = entry
  extractSourceLocator.value = { root_id: library.rootId, path: entry.path }
  extractDestination.value = { root_id: library.rootId, path: parentPath(entry.path) }
  extractPassword.value = ''
  deleteSources.value = false
  extractVisible.value = true
}

async function submitExtraction() {
  if (!extractSourceLocator.value || !extractDestination.value) return
  if (await extractionForm.value?.validate() !== true) return
  extractSubmitting.value = true
  try {
    const task = await extractions.create(
      extractSourceLocator.value,
      extractDestination.value,
      extractPassword.value,
      deleteSources.value,
    )
    extractPassword.value = ''
    extractVisible.value = false
    await MessagePlugin.success(`解压任务 ${task.id.slice(0, 8)} 已加入队列`)
  } catch (error) {
    await notifyError(error)
  } finally { extractSubmitting.value = false }
}
</script>

<template>
  <section class="transfer-workbench transfer-workbench-single">
    <div class="browser-pane unified-browser-pane">
      <BrowserDeviceBar title="飞牛存储" subtitle="双击目录进入，勾选要传输的内容">
        <template #icon><FnOSIcon :size="18" /></template>
        <template #control><t-select v-model="library.rootId" :options="library.storageRoots.map(root => ({ label: root.label, value: root.id }))" placeholder="选择存储空间" class="location-select devicebar-select" /></template>
      </BrowserDeviceBar>
      <FileBrowser v-model:selected-sources="selectedSources" mode="source" compact @copy-to-ps5="openCopyToPS5" @extract-archive="openExtraction" />
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

  <t-dialog v-model:visible="extractVisible" dialog-class-name="transfer-picker-dialog" header="解压到飞牛" width="760px" :confirm-btn="{ content: '创建解压任务', loading: extractSubmitting, disabled: !extractDestination }" @confirm="submitExtraction" @close="extractPassword = ''">
    <div v-if="extractSource && extractDestination" class="extraction-dialog">
      <t-alert theme="info" :message="`将 ${extractSource.name} 解压为新文件夹 ${extractionFolderName}；目标已存在时不会覆盖。`" />
      <LocalDirectoryPicker :initial="extractDestination" @change="extractDestination = $event" />
      <t-form ref="extractionForm" :data="extractionFormData" :rules="extractionRules" required-mark label-align="top">
        <t-form-item name="password" label="压缩包密码（没有密码可留空）"><t-input v-model="extractPassword" type="password" autocomplete="off" clearable /></t-form-item>
        <t-checkbox v-model="deleteSources">解压成功后删除全部源分卷</t-checkbox>
      </t-form>
    </div>
  </t-dialog>
</template>

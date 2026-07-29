<script setup lang="ts">
import { computed, ref } from 'vue'
import { DialogPlugin, MessagePlugin } from 'tdesign-vue-next'
import type { DropdownOption } from 'tdesign-vue-next'
import { ChevronDown } from '@lucide/vue'
import { useRoute, useRouter } from 'vue-router'
import ExtractionTasksPanel from '../components/ExtractionTasksPanel.vue'
import { formatBytes, formatETA } from '../api'
import { useLibraryStore } from '../stores/library'
import { useProfilesStore } from '../stores/profiles'
import { useTasksStore } from '../stores/tasks'
import type { Task, TaskEvent, TaskItem } from '../types'

const tasks = useTasksStore()
const profiles = useProfilesStore()
const library = useLibraryStore()
const route = useRoute()
const router = useRouter()
const category = computed(() => route.query.tab === 'extraction' ? 'extraction' : 'transfer')
function selectCategory(value: 'transfer' | 'extraction') { void router.replace({ query: { ...route.query, tab: value } }) }
const detailVisible = ref(false)
const detailLoading = ref(false)
const detailTask = ref<Task | null>(null)
const detailItems = ref<TaskItem[]>([])
const detailEvents = ref<TaskEvent[]>([])

const activeTasks = computed(() => tasks.items.filter((task) => ['queued', 'scanning', 'running', 'canceling'].includes(task.state)))
const historyTasks = computed(() => tasks.items.filter((task) => !['queued', 'scanning', 'running', 'canceling'].includes(task.state)))
const recentEvents = computed(() => detailEvents.value.slice(0, 8).reverse())
const percent = (task: Task) => task.total_bytes ? Math.min(100, task.transferred_bytes / task.total_bytes * 100) : 0
const stateLabel: Record<string, string> = { queued: '等待中', scanning: '扫描中', running: '传输中', canceling: '取消中', succeeded: '已完成', failed: '失败', canceled: '已取消', interrupted: '已中断' }
const conflictLabel: Record<string, string> = { smart: '智能处理', overwrite: '全部覆盖', fail: '遇到冲突停止' }
const profileName = (task: Task) => profiles.items.find((profile) => profile.id === task.profile_id)?.name || 'PS5'

function baseName(value: string) {
  return value.split('/').filter(Boolean).pop() || value || '存储空间根目录'
}

function sourceLabel(task: Task) {
  if (task.type === 'delete') return `${profileName(task)} · ${task.destination}`
  const first = task.sources[0]
  if (!first) return '未知来源'
  if (task.type === 'download') {
    const item = baseName(first.path)
    return task.sources.length > 1 ? `${profileName(task)} / ${item} 等 ${task.sources.length} 项` : `${profileName(task)} / ${item}`
  }
  const root = library.roots.find((item) => item.id === first.root_id)?.label || 'fnOS'
  const item = baseName(first.path)
  return task.sources.length > 1 ? `${root} / ${item} 等 ${task.sources.length} 项` : `${root} / ${item}`
}

function destinationLabel(task: Task) {
  if (task.type === 'delete') return '永久删除'
  if (task.type === 'download') {
    const root = library.roots.find((item) => item.id === task.sources[0]?.root_id)?.label || 'fnOS'
    return task.destination ? `${root} / ${task.destination}` : root
  }
  return `${profileName(task)} · ${task.destination}`
}

function taskSubtitle(task: Task) {
  if (task.error) return task.error
  if (task.current_file) return `正在处理：${task.current_file}`
  if (task.state === 'scanning') return '正在统计文件数量和传输大小'
  if (task.state === 'queued') return `任务 #${task.id.slice(0, 8)} · 等待调度`
  return `${task.completed_items}/${task.total_items} 项完成 · 跳过 ${task.skipped_items}`
}

function speedLabel(task: Task) {
  if (task.state === 'scanning') return '统计中'
  if (!task.speed_bytes) return task.state === 'queued' ? '等待开始' : '—'
  return `${formatBytes(task.speed_bytes)}/s`
}

function etaLabel(task: Task) {
  if (task.state === 'queued') return '等待调度'
  if (task.state === 'scanning') return '扫描完成后计算'
  return `预计剩余 ${formatETA(task.eta_seconds)}`
}

function historyDataLabel(task: Task) {
  if (!task.total_bytes) return formatBytes(task.transferred_bytes)
  return `${formatBytes(task.transferred_bytes)} / ${formatBytes(task.total_bytes)}`
}

function activeActionOptions(task: Task): DropdownOption[] {
  return [
    { value: 'detail', content: '查看详情', onClick: () => { void openDetail(task) } },
    { value: 'cancel', content: '取消任务', theme: 'error', disabled: task.state === 'canceling', onClick: () => { void cancel(task.id) } },
  ]
}

function historyActionOptions(task: Task): DropdownOption[] {
  const options: DropdownOption[] = [{ value: 'detail', content: '查看详情', onClick: () => { void openDetail(task) } }]
  if (['failed', 'canceled', 'interrupted'].includes(task.state)) {
    options.push({ value: 'retry', content: '重新提交', onClick: () => { void retry(task) } })
  }
  options.push({ value: 'remove', content: '删除记录', theme: 'error', divider: true, onClick: () => { removeHistory(task) } })
  return options
}

async function cancel(id: string) {
  const dialog = DialogPlugin.confirm({
    header: '取消任务？',
    body: '当前临时文件会被清理，已经完成的正式文件会保留。',
    confirmBtn: { content: '取消任务', theme: 'danger' },
    onConfirm: async () => {
      try { await tasks.cancel(id); dialog.destroy(); await MessagePlugin.success('取消请求已提交') }
      catch (error) { await MessagePlugin.error(error instanceof Error ? error.message : String(error)) }
    },
  })
}

async function retry(task: Task) {
  try {
    await tasks.retry(task.id)
    await MessagePlugin.success('新任务已加入队列，将重新扫描来源')
  } catch (error) {
    await MessagePlugin.error(error instanceof Error ? error.message : String(error))
  }
}

function removeHistory(task: Task) {
  const dialog = DialogPlugin.confirm({
    header: '删除任务记录？',
    body: '只会删除任务、事件和文件明细记录，不会删除 fnOS 或 PS5 上的任何文件。',
    confirmBtn: { content: '删除记录', theme: 'danger' },
    onConfirm: async () => {
      try {
        await tasks.remove(task.id)
        if (detailTask.value?.id === task.id) detailVisible.value = false
        dialog.destroy()
        await MessagePlugin.success('任务记录已删除')
      } catch (error) {
        await MessagePlugin.error(error instanceof Error ? error.message : String(error))
      }
    },
  })
}

async function openDetail(task: Task) {
  detailVisible.value = true
  detailLoading.value = true
  detailTask.value = task
  detailItems.value = []
  detailEvents.value = []
  try {
    const data = await tasks.detail(task.id)
    detailTask.value = data.task
    detailItems.value = data.items || []
    detailEvents.value = data.events || []
  } catch (error) {
    await MessagePlugin.error(error instanceof Error ? error.message : String(error))
  } finally { detailLoading.value = false }
}
</script>

<template>
  <div class="page-task-category-tabs" role="tablist" aria-label="任务类型">
    <button :class="{ 'is-active': category === 'transfer' }" @click="selectCategory('transfer')">传输任务</button>
    <button :class="{ 'is-active': category === 'extraction' }" @click="selectCategory('extraction')">解压任务</button>
  </div>
  <template v-if="category === 'transfer'">
  <section class="data-section">
    <header class="section-header"><div><h2>进行中的任务</h2><p>同一台 PS5 一次只执行一个任务</p></div><span class="section-count">{{ activeTasks.length }}</span></header>
    <div v-if="activeTasks.length" class="task-table">
      <div class="task-table-head" aria-hidden="true"><span>状态</span><span>传输任务</span><span>进度</span><span>速度 / ETA</span><span>操作</span></div>
      <article v-for="task in activeTasks" :key="task.id" class="task-row">
        <div class="task-state"><span :class="['state-mark', `state-${task.state}`]" />{{ stateLabel[task.state] }}</div>
        <div class="task-primary">
          <strong class="task-route"><span><em>从</em>{{ sourceLabel(task) }}</span><i>→</i><span><em>到</em>{{ destinationLabel(task) }}</span></strong>
          <small>{{ taskSubtitle(task) }}</small>
          <div class="thin-progress"><i :style="{ width: `${percent(task)}%` }" /></div>
        </div>
        <div class="task-measure"><strong>{{ Math.round(percent(task)) }}%</strong><small>{{ formatBytes(task.transferred_bytes) }} / {{ formatBytes(task.total_bytes) }}</small></div>
        <div class="task-measure"><strong>{{ speedLabel(task) }}</strong><small>{{ etaLabel(task) }}</small></div>
        <div class="row-actions"><t-dropdown trigger="click" placement="bottom-right" :min-column-width="132" :options="activeActionOptions(task)"><t-button variant="text" size="small">操作<ChevronDown :size="14" /></t-button></t-dropdown></div>
      </article>
    </div>
    <div v-else class="plain-empty compact">当前没有正在执行的任务</div>
  </section>

  <section class="data-section history-section">
    <header class="section-header"><div><h2>历史记录</h2><p>失败或中断的任务可以重新提交</p></div><span class="section-count">{{ historyTasks.length }}</span></header>
    <div v-if="historyTasks.length" class="task-table">
      <div class="task-table-head history-table-head" aria-hidden="true"><span>状态</span><span>传输任务</span><span>传输数据 / 创建时间</span><span>操作</span></div>
      <article v-for="task in historyTasks" :key="task.id" class="task-row history-row">
        <div class="task-state"><span :class="['state-mark', `state-${task.state}`]" />{{ stateLabel[task.state] }}</div>
        <div class="task-primary">
          <strong class="task-route"><span><em>从</em>{{ sourceLabel(task) }}</span><i>→</i><span><em>到</em>{{ destinationLabel(task) }}</span></strong>
          <small>{{ taskSubtitle(task) }}</small>
        </div>
        <div class="task-measure"><strong>{{ historyDataLabel(task) }}</strong><small>{{ new Date(task.created_at).toLocaleString('zh-CN', { hour12: false }) }}</small></div>
        <div class="row-actions"><t-dropdown trigger="click" placement="bottom-right" :min-column-width="132" :options="historyActionOptions(task)"><t-button variant="text" size="small">操作<ChevronDown :size="14" /></t-button></t-dropdown></div>
      </article>
    </div>
    <div v-else class="plain-empty compact">还没有历史任务</div>
  </section>

  <t-dialog v-model:visible="detailVisible" header="任务详情" width="720px" :footer="false">
    <div v-if="detailTask" class="task-detail" :class="{ 'is-loading': detailLoading }">
      <div class="task-detail-route"><span><small>来源</small><strong>{{ sourceLabel(detailTask) }}</strong></span><i>→</i><span><small>{{ detailTask.type === 'delete' ? '操作' : '目的地' }}</small><strong>{{ destinationLabel(detailTask) }}</strong></span></div>
      <dl class="task-detail-grid">
        <div><dt>状态</dt><dd>{{ stateLabel[detailTask.state] }}</dd></div>
        <div><dt>冲突策略</dt><dd>{{ conflictLabel[detailTask.conflict_policy] || '—' }}</dd></div>
        <div><dt>完成项目</dt><dd>{{ detailTask.completed_items }} / {{ detailTask.total_items }}</dd></div>
        <div><dt>传输数据</dt><dd>{{ formatBytes(detailTask.transferred_bytes) }} / {{ formatBytes(detailTask.total_bytes) }}</dd></div>
        <div><dt>创建时间</dt><dd>{{ new Date(detailTask.created_at).toLocaleString('zh-CN', { hour12: false }) }}</dd></div>
        <div><dt>任务编号</dt><dd class="mono">{{ detailTask.id }}</dd></div>
      </dl>
      <t-alert v-if="detailTask.error" theme="error" :message="detailTask.error" />
      <section v-if="recentEvents.length" class="task-events"><h3>最近事件</h3><ul><li v-for="event in recentEvents" :key="event.id"><time>{{ new Date(event.created_at).toLocaleTimeString('zh-CN', { hour12: false }) }}</time><span>{{ event.message }}</span></li></ul></section>
      <section v-if="detailItems.length" class="task-items"><h3>文件明细 <small>显示前 20 项</small></h3><div v-for="item in detailItems.slice(0, 20)" :key="item.id"><span>{{ item.source_path }}</span><small>{{ item.state }} · {{ formatBytes(item.transferred) }} / {{ formatBytes(item.size) }}</small></div></section>
    </div>
  </t-dialog>
  </template>
  <ExtractionTasksPanel v-else />
</template>

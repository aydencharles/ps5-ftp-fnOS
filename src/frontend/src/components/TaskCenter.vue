<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { Ban, CircleCheck, Clock3, Eye, History, Info, ListTodo, RotateCcw, Trash2, X } from '@lucide/vue'
import { DialogPlugin, MessagePlugin } from 'tdesign-vue-next'
import { formatBytes, formatETA } from '../api'
import { useLibraryStore } from '../stores/library'
import { useProfilesStore } from '../stores/profiles'
import { useTasksStore } from '../stores/tasks'
import { useExtractionTasksStore } from '../stores/extractions'
import { useTaskCenterStore } from '../stores/taskCenter'
import type { ExtractionTask, Task, TaskEvent, TaskItem } from '../types'

type TaskTab = 'active' | 'history'

const tasks = useTasksStore()
const extractions = useExtractionTasksStore()
const taskCenter = useTaskCenterStore()
const profiles = useProfilesStore()
const library = useLibraryStore()
const activeTab = computed(() => taskCenter.statusTab)
const category = computed(() => taskCenter.category)
const detailVisible = ref(false)
const detailLoading = ref(false)
const detailTask = ref<Task | null>(null)
const detailItems = ref<TaskItem[]>([])
const detailEvents = ref<TaskEvent[]>([])
const extractionDetailVisible = ref(false)
const extractionDetail = ref<ExtractionTask | null>(null)
const retryExtractionTask = ref<ExtractionTask | null>(null)
const retryPassword = ref('')
const retryVisible = ref(false)
const panelElement = ref<{ offsetWidth: number; offsetHeight: number } | null>(null)
const launcherSize = 52
const launcherMargin = 16
const longPressDelay = 320
const launcherPoint = reactive({ x: 0, y: 0 })
const panelPoint = reactive({ x: 16, y: 16 })
const drag = reactive({ pointerId: null as number | null, startX: 0, startY: 0, originX: 0, originY: 0, active: false })
const suppressClick = ref(false)
let longPressTimer: ReturnType<typeof globalThis.setTimeout> | null = null

const activeStates = new Set(['queued', 'scanning', 'running', 'canceling'])
const activeTasks = computed(() => tasks.items.filter((task) => activeStates.has(task.state)))
const historyTasks = computed(() => tasks.items.filter((task) => !activeStates.has(task.state)))
const activeExtractions = computed(() => extractions.items.filter((task) => activeStates.has(task.state) || task.state === 'extracting' || task.state === 'cleaning'))
const historyExtractions = computed(() => extractions.items.filter((task) => !activeExtractions.value.some((active) => active.id === task.id)))
const visibleTasks = computed(() => activeTab.value === 'active' ? activeTasks.value : historyTasks.value)
const visibleExtractions = computed(() => activeTab.value === 'active' ? activeExtractions.value : historyExtractions.value)
const recentEvents = computed(() => detailEvents.value.slice(0, 8).reverse())
const launcherStyle = computed(() => ({ left: `${launcherPoint.x}px`, top: `${launcherPoint.y}px` }))
const panelStyle = computed(() => ({ left: `${panelPoint.x}px`, top: `${panelPoint.y}px` }))
const totalActive = computed(() => activeTasks.value.length + activeExtractions.value.length)
const launcherLabel = computed(() => totalActive.value
  ? `任务中心，${totalActive.value} 个任务进行中`
  : '任务中心，没有正在进行的任务')

const percent = (task: Task) => task.total_bytes ? Math.min(100, task.transferred_bytes / task.total_bytes * 100) : 0
const extractionPercent = (task: ExtractionTask) => task.total_bytes ? Math.min(100, task.extracted_bytes / task.total_bytes * 100) : 0
const stateLabel: Record<string, string> = { queued: '等待中', scanning: '扫描中', running: '传输中', canceling: '取消中', succeeded: '已完成', failed: '失败', canceled: '已取消', interrupted: '已中断' }
const extractionStateLabel: Record<string, string> = { queued: '等待中', scanning: '扫描中', extracting: '解压中', cleaning: '收尾中', canceling: '取消中', succeeded: '已完成', failed: '失败', canceled: '已取消', interrupted: '已中断' }
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
  return `剩余 ${formatETA(task.eta_seconds)}`
}

function historyDataLabel(task: Task) {
  if (!task.total_bytes) return formatBytes(task.transferred_bytes)
  return `${formatBytes(task.transferred_bytes)} / ${formatBytes(task.total_bytes)}`
}

function formatTime(value: string) {
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

function rootName(rootId: string) {
  return library.roots.find((root) => root.id === rootId)?.label || 'fnOS'
}

function extractionSourceLabel(task: ExtractionTask) {
  return `${rootName(task.source.root_id)} / ${task.source.path}`
}

function extractionDestinationLabel(task: ExtractionTask) {
  return `${rootName(task.destination.root_id)} / ${task.destination.path}`
}

function extractionSubtitle(task: ExtractionTask) {
  if (task.error) return task.error
  if (task.warning) return task.warning
  if (task.current_file) return `正在解压：${task.current_file}`
  if (task.state === 'queued') return `任务 #${task.id.slice(0, 8)} · 等待解压队列`
  if (task.state === 'scanning') return '正在校验分卷和归档内容'
  if (task.state === 'cleaning') return task.delete_sources ? '正在发布目标并清理源分卷' : '正在原子发布目标目录'
  return `${task.completed_items}/${task.total_items} 项完成`
}

function extractionSpeedLabel(task: ExtractionTask) {
  if (task.state === 'scanning') return '校验中'
  if (!task.speed_bytes) return task.state === 'queued' ? '等待开始' : '—'
  return `${formatBytes(task.speed_bytes)}/s`
}

function extractionETALabel(task: ExtractionTask) {
  if (task.state === 'queued') return '等待调度'
  if (task.state === 'scanning') return '扫描完成后计算'
  if (task.state === 'cleaning') return '即将完成'
  return `剩余 ${formatETA(task.eta_seconds)}`
}

function clamp(value: number, minimum: number, maximum: number) {
  return Math.max(minimum, Math.min(maximum, value))
}

function launcherCoordinates() {
  const width = globalThis.innerWidth
  const height = globalThis.innerHeight
  const usableX = Math.max(0, width - launcherSize - launcherMargin * 2)
  const usableY = Math.max(0, height - launcherSize - launcherMargin * 2)
  const { edge, ratio } = taskCenter.launcherPosition
  if (edge === 'left') return { x: launcherMargin, y: launcherMargin + usableY * ratio }
  if (edge === 'right') return { x: width - launcherMargin - launcherSize, y: launcherMargin + usableY * ratio }
  if (edge === 'top') return { x: launcherMargin + usableX * ratio, y: launcherMargin }
  return { x: launcherMargin + usableX * ratio, y: height - launcherMargin - launcherSize }
}

function refreshLauncherPosition() {
  const position = launcherCoordinates()
  launcherPoint.x = position.x
  launcherPoint.y = position.y
}

function updatePanelPosition() {
  if (!taskCenter.visible) return
  const viewportWidth = globalThis.innerWidth
  const viewportHeight = globalThis.innerHeight
  const panelWidth = panelElement.value?.offsetWidth || Math.min(500, viewportWidth - launcherMargin * 2)
  const panelHeight = panelElement.value?.offsetHeight || (viewportHeight - launcherMargin * 2) * .8
  const gap = 12
  const centeredLeft = launcherPoint.x + launcherSize / 2 - panelWidth / 2
  const centeredTop = launcherPoint.y + launcherSize / 2 - panelHeight / 2
  let x = centeredLeft
  let y = centeredTop

  if (taskCenter.launcherPosition.edge === 'left') x = launcherPoint.x + launcherSize + gap
  if (taskCenter.launcherPosition.edge === 'right') x = launcherPoint.x - panelWidth - gap
  if (taskCenter.launcherPosition.edge === 'top') y = launcherPoint.y + launcherSize + gap
  if (taskCenter.launcherPosition.edge === 'bottom') y = launcherPoint.y - panelHeight - gap

  panelPoint.x = clamp(x, launcherMargin, Math.max(launcherMargin, viewportWidth - panelWidth - launcherMargin))
  panelPoint.y = clamp(y, launcherMargin, Math.max(launcherMargin, viewportHeight - panelHeight - launcherMargin))
}

function clearLongPressTimer() {
  if (longPressTimer === null) return
  globalThis.clearTimeout(longPressTimer)
  longPressTimer = null
}

// eslint-disable-next-line no-undef
function handlePointerDown(event: PointerEvent) {
  if (!event.isPrimary || event.button !== 0) return
  drag.pointerId = event.pointerId
  drag.startX = event.clientX
  drag.startY = event.clientY
  drag.originX = launcherPoint.x
  drag.originY = launcherPoint.y
  try { (event.currentTarget as unknown as { setPointerCapture: (pointerId: number) => void }).setPointerCapture(event.pointerId) } catch { /* Pointer capture is an enhancement, not a requirement. */ }
  clearLongPressTimer()
  longPressTimer = globalThis.setTimeout(() => {
    if (drag.pointerId !== event.pointerId) return
    drag.active = true
    taskCenter.close()
  }, longPressDelay)
}

// eslint-disable-next-line no-undef
function handlePointerMove(event: PointerEvent) {
  if (drag.pointerId !== event.pointerId || !drag.active) return
  event.preventDefault()
  launcherPoint.x = clamp(drag.originX + event.clientX - drag.startX, launcherMargin, globalThis.innerWidth - launcherSize - launcherMargin)
  launcherPoint.y = clamp(drag.originY + event.clientY - drag.startY, launcherMargin, globalThis.innerHeight - launcherSize - launcherMargin)
}

function snapLauncherToNearestEdge() {
  const centerX = launcherPoint.x + launcherSize / 2
  const centerY = launcherPoint.y + launcherSize / 2
  const distances = {
    left: centerX,
    right: globalThis.innerWidth - centerX,
    top: centerY,
    bottom: globalThis.innerHeight - centerY,
  }
  const edge = (Object.entries(distances).sort((left, right) => left[1] - right[1])[0]?.[0] || 'right') as 'top' | 'right' | 'bottom' | 'left'
  const usable = edge === 'left' || edge === 'right'
    ? Math.max(1, globalThis.innerHeight - launcherSize - launcherMargin * 2)
    : Math.max(1, globalThis.innerWidth - launcherSize - launcherMargin * 2)
  const coordinate = edge === 'left' || edge === 'right' ? launcherPoint.y : launcherPoint.x
  taskCenter.setLauncherPosition({ edge, ratio: clamp((coordinate - launcherMargin) / usable, 0, 1) })
  drag.active = false
  refreshLauncherPosition()
}

// eslint-disable-next-line no-undef
function finishPointerInteraction(event: PointerEvent) {
  if (drag.pointerId !== event.pointerId) return
  clearLongPressTimer()
  if (drag.active) {
    event.preventDefault()
    snapLauncherToNearestEdge()
    suppressClick.value = true
    globalThis.setTimeout(() => { suppressClick.value = false }, 0)
  }
  try { (event.currentTarget as unknown as { releasePointerCapture: (pointerId: number) => void }).releasePointerCapture(event.pointerId) } catch { /* The pointer may already be released. */ }
  drag.pointerId = null
}

function toggleCenter() {
  if (suppressClick.value) return
  if (taskCenter.visible) taskCenter.close()
  else taskCenter.open(taskCenter.category, totalActive.value ? 'active' : 'history')
}

function closeCenter() {
  taskCenter.close()
}

function switchTab(tab: TaskTab) {
  taskCenter.statusTab = tab
}

// eslint-disable-next-line no-undef
function handleKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && taskCenter.visible && !detailVisible.value) closeCenter()
}

async function cancel(task: Task) {
  const dialog = DialogPlugin.confirm({
    header: '取消任务？',
    body: '当前临时文件会被清理，已经完成的正式文件会保留。',
    confirmBtn: { content: '取消任务', theme: 'danger' },
    onConfirm: async () => {
      try {
        await tasks.cancel(task.id)
        dialog.destroy()
        await MessagePlugin.success('取消请求已提交')
      } catch (error) {
        await MessagePlugin.error(error instanceof Error ? error.message : String(error))
      }
    },
  })
}

async function retry(task: Task) {
  try {
    await tasks.retry(task.id)
    taskCenter.statusTab = 'active'
    await MessagePlugin.success('新任务已加入队列，将重新扫描来源')
  } catch (error) {
    await MessagePlugin.error(error instanceof Error ? error.message : String(error))
  }
}

async function cancelExtraction(task: ExtractionTask) {
  const dialog = DialogPlugin.confirm({
    header: '取消解压任务？',
    body: '临时解压目录会被清理，源分卷不会删除。',
    confirmBtn: { content: '取消任务', theme: 'danger' },
    onConfirm: async () => {
      try { await extractions.cancel(task.id); dialog.destroy(); await MessagePlugin.success('取消请求已提交') }
      catch (error) { await MessagePlugin.error(error instanceof Error ? error.message : String(error)) }
    },
  })
}

function openExtractionRetry(task: ExtractionTask) {
  retryExtractionTask.value = task
  retryPassword.value = ''
  retryVisible.value = true
}

async function submitExtractionRetry() {
  if (!retryExtractionTask.value) return
  try {
    await extractions.retry(retryExtractionTask.value.id, retryPassword.value)
    retryPassword.value = ''
    retryVisible.value = false
    await MessagePlugin.success('解压任务已重新加入队列')
  } catch (error) { await MessagePlugin.error(error instanceof Error ? error.message : String(error)) }
}

function removeExtraction(task: ExtractionTask) {
  const dialog = DialogPlugin.confirm({
    header: '删除解压记录？',
    body: '只删除任务记录，不会删除压缩包或已解压文件。',
    confirmBtn: { content: '删除记录', theme: 'danger' },
    onConfirm: async () => {
      try { await extractions.remove(task.id); dialog.destroy(); await MessagePlugin.success('解压记录已删除') }
      catch (error) { await MessagePlugin.error(error instanceof Error ? error.message : String(error)) }
    },
  })
}

async function openExtractionDetail(task: ExtractionTask) {
  extractionDetailVisible.value = true
  extractionDetail.value = task
  try { extractionDetail.value = (await extractions.detail(task.id)).task }
  catch (error) { await MessagePlugin.error(error instanceof Error ? error.message : String(error)) }
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
  } finally {
    detailLoading.value = false
  }
}

function handleViewportResize() {
  refreshLauncherPosition()
  updatePanelPosition()
}

taskCenter.hydrateLauncherPosition()
refreshLauncherPosition()

watch([() => taskCenter.visible, () => taskCenter.statusTab, () => taskCenter.category, () => visibleTasks.value.length, () => visibleExtractions.value.length], async ([visible]) => {
  if (!visible) return
  await nextTick()
  updatePanelPosition()
})

onMounted(() => {
  globalThis.addEventListener('keydown', handleKeydown)
  globalThis.addEventListener('resize', handleViewportResize)
})
onBeforeUnmount(() => {
  clearLongPressTimer()
  globalThis.removeEventListener('keydown', handleKeydown)
  globalThis.removeEventListener('resize', handleViewportResize)
})
</script>

<template>
  <Teleport to="body">
    <Transition name="task-center-fade"><button v-if="taskCenter.visible" class="task-center-scrim" aria-label="关闭任务中心" @click="closeCenter" /></Transition>
    <Transition name="task-center-panel">
      <aside v-if="taskCenter.visible" id="task-center-panel" ref="panelElement" class="task-center-panel" :class="`is-docked-${taskCenter.launcherPosition.edge}`" :style="panelStyle" role="dialog" aria-label="任务中心">
        <header class="task-center-header">
          <div class="task-center-heading"><span class="task-center-heading-icon"><ListTodo :size="19" /></span><div><h2>任务中心</h2><p v-if="totalActive">{{ totalActive }} 个任务正在处理</p><p v-else>传输与解压记录</p></div></div>
          <button class="task-icon-button task-center-close" aria-label="关闭任务中心" @click="closeCenter"><X :size="18" /></button>
        </header>

        <div class="task-category-tabs" role="tablist" aria-label="任务类型">
          <button :class="{ 'is-active': category === 'transfer' }" @click="taskCenter.category = 'transfer'">传输任务 <em>{{ tasks.items.length }}</em></button>
          <button :class="{ 'is-active': category === 'extraction' }" @click="taskCenter.category = 'extraction'">解压任务 <em>{{ extractions.items.length }}</em></button>
        </div>
        <div class="task-center-tabs" role="tablist" aria-label="任务状态">
          <button id="task-tab-active" role="tab" :aria-selected="activeTab === 'active'" :class="{ 'is-active': activeTab === 'active' }" @click="switchTab('active')"><ListTodo :size="15" /><span>进行中</span><em>{{ category === 'transfer' ? activeTasks.length : activeExtractions.length }}</em></button>
          <button id="task-tab-history" role="tab" :aria-selected="activeTab === 'history'" :class="{ 'is-active': activeTab === 'history' }" @click="switchTab('history')"><History :size="15" /><span>历史任务</span><em>{{ category === 'transfer' ? historyTasks.length : historyExtractions.length }}</em></button>
        </div>

        <div id="task-center-list" class="task-center-list" role="tabpanel">
          <template v-if="category === 'transfer'">
            <article v-for="task in visibleTasks" :key="task.id" class="task-center-item" :class="[`is-${task.state}`, { 'is-active-task': activeTab === 'active' }]">
              <header class="task-item-header"><span class="task-state-chip"><i :class="['state-mark', `state-${task.state}`]" />{{ stateLabel[task.state] || task.state }}</span><span class="task-item-id">#{{ task.id.slice(0, 8) }}</span></header>
              <div class="task-center-route"><span><small>从</small>{{ sourceLabel(task) }}</span><i>→</i><span><small>{{ task.type === 'delete' ? '操作' : '到' }}</small>{{ destinationLabel(task) }}</span></div>
              <template v-if="activeTab === 'active'">
                <p class="task-center-subtitle" :class="{ 'is-error': Boolean(task.error) }">{{ taskSubtitle(task) }}</p>
                <div class="task-center-progress-row"><div class="task-center-progress"><i :style="{ width: `${percent(task)}%` }" /></div><strong>{{ Math.round(percent(task)) }}%</strong></div>
                <div class="task-center-metrics"><span><strong>{{ speedLabel(task) }}</strong><small>{{ etaLabel(task) }}</small></span><span><strong>{{ formatBytes(task.transferred_bytes) }}</strong><small>共 {{ formatBytes(task.total_bytes) }}</small></span></div>
                <footer class="task-center-actions"><button @click="openDetail(task)"><Eye :size="14" />详情</button><button class="is-danger" :disabled="task.state === 'canceling'" @click="cancel(task)"><Ban :size="14" />{{ task.state === 'canceling' ? '取消中' : '取消' }}</button></footer>
              </template>
              <template v-else>
                <p class="task-center-subtitle" :class="{ 'is-error': Boolean(task.error) }">{{ taskSubtitle(task) }}</p>
                <div class="task-history-meta"><span><Clock3 :size="13" />{{ formatTime(task.created_at) }}</span><strong>{{ historyDataLabel(task) }}</strong></div>
                <footer class="task-center-actions"><button @click="openDetail(task)"><Eye :size="14" />详情</button><button v-if="['failed', 'canceled', 'interrupted'].includes(task.state)" @click="retry(task)"><RotateCcw :size="14" />重新提交</button><button class="is-danger is-icon-only" aria-label="删除任务记录" @click="removeHistory(task)"><Trash2 :size="14" /></button></footer>
              </template>
            </article>
          </template>

          <template v-else>
            <article v-for="task in visibleExtractions" :key="task.id" class="task-center-item" :class="[`is-${task.state}`, { 'is-active-task': activeTab === 'active' }]">
              <header class="task-item-header"><span class="task-state-chip"><i :class="['state-mark', `state-${task.state}`]" />{{ extractionStateLabel[task.state] || task.state }}</span><span class="task-item-id">#{{ task.id.slice(0, 8) }}</span></header>
              <div class="task-center-route"><span><small>压缩包</small>{{ extractionSourceLabel(task) }}</span><i>→</i><span><small>目录</small>{{ extractionDestinationLabel(task) }}</span></div>
              <template v-if="activeTab === 'active'">
                <p class="task-center-subtitle" :class="{ 'is-error': Boolean(task.error) }">{{ extractionSubtitle(task) }}</p>
                <div class="task-center-progress-row"><div class="task-center-progress"><i :style="{ width: `${extractionPercent(task)}%` }" /></div><strong>{{ Math.round(extractionPercent(task)) }}%</strong></div>
                <div class="task-center-metrics"><span><strong>{{ extractionSpeedLabel(task) }}</strong><small>{{ extractionETALabel(task) }}</small></span><span><strong>{{ formatBytes(task.extracted_bytes) }}</strong><small>共 {{ formatBytes(task.total_bytes) }}</small></span></div>
                <footer class="task-center-actions"><button @click="openExtractionDetail(task)"><Eye :size="14" />详情</button><button class="is-danger" :disabled="['canceling', 'cleaning'].includes(task.state)" @click="cancelExtraction(task)"><Ban :size="14" />{{ task.state === 'canceling' ? '取消中' : '取消' }}</button></footer>
              </template>
              <template v-else>
                <p class="task-center-subtitle" :class="{ 'is-error': Boolean(task.error) }">{{ extractionSubtitle(task) }}</p>
                <div class="task-history-meta"><span><Clock3 :size="13" />{{ formatTime(task.created_at) }}</span><strong>{{ formatBytes(task.extracted_bytes) }} / {{ formatBytes(task.total_bytes) }}</strong></div>
                <footer class="task-center-actions"><button @click="openExtractionDetail(task)"><Eye :size="14" />详情</button><button v-if="['failed', 'canceled', 'interrupted'].includes(task.state)" @click="openExtractionRetry(task)"><RotateCcw :size="14" />重新提交</button><button class="is-danger is-icon-only" aria-label="删除解压记录" @click="removeExtraction(task)"><Trash2 :size="14" /></button></footer>
              </template>
            </article>
          </template>

          <div v-if="category === 'transfer' ? !visibleTasks.length : !visibleExtractions.length" class="task-center-empty">
            <CircleCheck v-if="activeTab === 'active'" :size="34" /><History v-else :size="34" />
            <strong>{{ activeTab === 'active' ? '当前没有进行中的任务' : '还没有历史任务' }}</strong>
            <span>{{ category === 'transfer' ? '传输与 PS5 文件操作显示在这里' : '飞牛本地解压任务显示在这里' }}</span>
          </div>
        </div>
        <footer class="task-center-note"><Info :size="13" /><span>{{ category === 'transfer' ? '同一台 PS5 一次执行一个任务' : '飞牛本地同时执行一个解压任务' }}</span></footer>
      </aside>
    </Transition>

    <div class="task-launcher-ring" :class="{ 'has-active': totalActive, 'is-dragging': drag.active }" :style="launcherStyle">
      <button class="task-launcher" :aria-label="`${launcherLabel}；长按可移动`" :title="`${launcherLabel}；长按可移动`" aria-controls="task-center-panel" :aria-expanded="taskCenter.visible" @click="toggleCenter" @pointerdown="handlePointerDown" @pointermove="handlePointerMove" @pointerup="finishPointerInteraction" @pointercancel="finishPointerInteraction" @contextmenu.prevent><X v-if="taskCenter.visible" :size="21" /><ListTodo v-else :size="22" /></button>
      <span v-if="totalActive" class="task-launcher-badge" aria-hidden="true">{{ totalActive > 99 ? '99+' : totalActive }}</span>
    </div>
  </Teleport>

  <t-dialog v-model:visible="detailVisible" header="传输任务详情" width="720px" :footer="false">
    <div v-if="detailTask" class="task-detail" :class="{ 'is-loading': detailLoading }">
      <div class="task-detail-route"><span><small>来源</small><strong>{{ sourceLabel(detailTask) }}</strong></span><i>→</i><span><small>目的地</small><strong>{{ destinationLabel(detailTask) }}</strong></span></div>
      <dl class="task-detail-grid"><div><dt>状态</dt><dd>{{ stateLabel[detailTask.state] }}</dd></div><div><dt>冲突策略</dt><dd>{{ detailTask.type === 'delete' ? '—' : conflictLabel[detailTask.conflict_policy] || '—' }}</dd></div><div><dt>完成项目</dt><dd>{{ detailTask.completed_items }} / {{ detailTask.total_items }}</dd></div><div><dt>传输数据</dt><dd>{{ formatBytes(detailTask.transferred_bytes) }} / {{ formatBytes(detailTask.total_bytes) }}</dd></div><div><dt>创建时间</dt><dd>{{ formatTime(detailTask.created_at) }}</dd></div><div><dt>任务编号</dt><dd class="mono">{{ detailTask.id }}</dd></div></dl>
      <t-alert v-if="detailTask.error" theme="error" :message="detailTask.error" />
      <section v-if="recentEvents.length" class="task-events"><h3>最近事件</h3><ul><li v-for="event in recentEvents" :key="event.id"><time>{{ new Date(event.created_at).toLocaleTimeString('zh-CN', { hour12: false }) }}</time><span>{{ event.message }}</span></li></ul></section>
      <section v-if="detailItems.length" class="task-items"><h3>文件明细 <small>显示前 20 项</small></h3><div v-for="item in detailItems.slice(0, 20)" :key="item.id"><span>{{ item.source_path }}</span><small>{{ item.state }} · {{ formatBytes(item.transferred) }} / {{ formatBytes(item.size) }}</small></div></section>
    </div>
  </t-dialog>

  <t-dialog v-model:visible="extractionDetailVisible" header="解压任务详情" width="720px" :footer="false">
    <div v-if="extractionDetail" class="task-detail">
      <div class="task-detail-route"><span><small>压缩包</small><strong>{{ extractionSourceLabel(extractionDetail) }}</strong></span><i>→</i><span><small>目标目录</small><strong>{{ extractionDestinationLabel(extractionDetail) }}</strong></span></div>
      <dl class="task-detail-grid"><div><dt>状态</dt><dd>{{ extractionStateLabel[extractionDetail.state] }}</dd></div><div><dt>当前文件</dt><dd>{{ extractionDetail.current_file || '—' }}</dd></div><div><dt>完成项目</dt><dd>{{ extractionDetail.completed_items }} / {{ extractionDetail.total_items }}</dd></div><div><dt>解压数据</dt><dd>{{ formatBytes(extractionDetail.extracted_bytes) }} / {{ formatBytes(extractionDetail.total_bytes) }}</dd></div><div><dt>速度</dt><dd>{{ extractionDetail.speed_bytes ? `${formatBytes(extractionDetail.speed_bytes)}/s` : '—' }}</dd></div><div><dt>预计剩余</dt><dd>{{ extractionDetail.eta_seconds == null ? '—' : formatETA(extractionDetail.eta_seconds) }}</dd></div><div><dt>删除源分卷</dt><dd>{{ extractionDetail.delete_sources ? '是' : '否' }}</dd></div><div><dt>创建时间</dt><dd>{{ formatTime(extractionDetail.created_at) }}</dd></div><div><dt>任务编号</dt><dd class="mono">{{ extractionDetail.id }}</dd></div></dl>
      <t-alert v-if="extractionDetail.error" theme="error" :message="extractionDetail.error" /><t-alert v-if="extractionDetail.warning" theme="warning" :message="extractionDetail.warning" />
    </div>
  </t-dialog>

  <t-dialog v-model:visible="retryVisible" header="重新提交解压任务" :confirm-btn="{ content: '重新提交' }" @confirm="submitExtractionRetry" @close="retryPassword = ''">
    <t-form label-align="top"><t-form-item label="压缩包密码（没有密码可留空）"><t-input v-model="retryPassword" type="password" autocomplete="off" /></t-form-item></t-form>
  </t-dialog>
</template>

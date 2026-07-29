<script setup lang="ts">
import { computed, ref } from 'vue'
import { ChevronDown } from '@lucide/vue'
import { DialogPlugin, MessagePlugin } from 'tdesign-vue-next'
import type { DropdownOption } from 'tdesign-vue-next'
import { formatBytes, formatETA } from '../api'
import { useExtractionTasksStore } from '../stores/extractions'
import { useLibraryStore } from '../stores/library'
import type { ExtractionTask } from '../types'

const extractions = useExtractionTasksStore()
const library = useLibraryStore()
const detailVisible = ref(false)
const detailTask = ref<ExtractionTask | null>(null)
const retryVisible = ref(false)
const retryTask = ref<ExtractionTask | null>(null)
const retryPassword = ref('')
const activeStates = new Set(['queued', 'scanning', 'extracting', 'cleaning', 'canceling'])
const activeTasks = computed(() => extractions.items.filter((task) => activeStates.has(task.state)))
const historyTasks = computed(() => extractions.items.filter((task) => !activeStates.has(task.state)))
const stateLabel: Record<string, string> = { queued: '等待中', scanning: '扫描中', extracting: '解压中', cleaning: '收尾中', canceling: '取消中', succeeded: '已完成', failed: '失败', canceled: '已取消', interrupted: '已中断' }
const percent = (task: ExtractionTask) => task.total_bytes ? Math.min(100, task.extracted_bytes / task.total_bytes * 100) : 0
const rootName = (id: string) => library.roots.find((root) => root.id === id)?.label || 'fnOS'
const sourceLabel = (task: ExtractionTask) => `${rootName(task.source.root_id)} / ${task.source.path}`
const destinationLabel = (task: ExtractionTask) => `${rootName(task.destination.root_id)} / ${task.destination.path}`
const formatTime = (value: string) => new Date(value).toLocaleString('zh-CN', { hour12: false })

function subtitle(task: ExtractionTask) {
  if (task.error) return task.error
  if (task.warning) return task.warning
  if (task.current_file) return `正在解压：${task.current_file}`
  if (task.state === 'queued') return '等待解压队列'
  if (task.state === 'scanning') return '正在校验分卷和归档内容'
  if (task.state === 'cleaning') return task.delete_sources ? '正在发布目标并清理源分卷' : '正在原子发布目标目录'
  return `${task.completed_items}/${task.total_items} 项完成`
}

function speed(task: ExtractionTask) {
  if (task.state === 'scanning') return '校验中'
  if (!task.speed_bytes) return task.state === 'queued' ? '等待开始' : '—'
  return `${formatBytes(task.speed_bytes)}/s`
}

function eta(task: ExtractionTask) {
  if (task.state === 'queued') return '等待调度'
  if (task.state === 'scanning') return '扫描完成后计算'
  if (task.state === 'cleaning') return '即将完成'
  return `预计剩余 ${formatETA(task.eta_seconds)}`
}

function activeActions(task: ExtractionTask): DropdownOption[] {
  return [
    { value: 'detail', content: '查看详情', onClick: () => { void openDetail(task) } },
    { value: 'cancel', content: '取消任务', theme: 'error', disabled: ['canceling', 'cleaning'].includes(task.state), onClick: () => { void cancel(task) } },
  ]
}

function historyActions(task: ExtractionTask): DropdownOption[] {
  const options: DropdownOption[] = [{ value: 'detail', content: '查看详情', onClick: () => { void openDetail(task) } }]
  if (['failed', 'canceled', 'interrupted'].includes(task.state)) options.push({ value: 'retry', content: '重新提交', onClick: () => openRetry(task) })
  options.push({ value: 'remove', content: '删除记录', theme: 'error', divider: true, onClick: () => remove(task) })
  return options
}

async function cancel(task: ExtractionTask) {
  const dialog = DialogPlugin.confirm({ header: '取消解压任务？', body: '临时目录会被清理，源分卷不会删除。', confirmBtn: { content: '取消任务', theme: 'danger' }, onConfirm: async () => {
    try { await extractions.cancel(task.id); dialog.destroy(); await MessagePlugin.success('取消请求已提交') }
    catch (error) { await MessagePlugin.error(error instanceof Error ? error.message : String(error)) }
  } })
}

function openRetry(task: ExtractionTask) { retryTask.value = task; retryPassword.value = ''; retryVisible.value = true }
async function submitRetry() {
  if (!retryTask.value) return
  try { await extractions.retry(retryTask.value.id, retryPassword.value); retryPassword.value = ''; retryVisible.value = false; await MessagePlugin.success('解压任务已重新加入队列') }
  catch (error) { await MessagePlugin.error(error instanceof Error ? error.message : String(error)) }
}

function remove(task: ExtractionTask) {
  const dialog = DialogPlugin.confirm({ header: '删除解压记录？', body: '不会删除压缩包或已经解压的文件。', confirmBtn: { content: '删除记录', theme: 'danger' }, onConfirm: async () => {
    try { await extractions.remove(task.id); dialog.destroy(); await MessagePlugin.success('解压记录已删除') }
    catch (error) { await MessagePlugin.error(error instanceof Error ? error.message : String(error)) }
  } })
}

async function openDetail(task: ExtractionTask) {
  detailVisible.value = true
  detailTask.value = task
  try { detailTask.value = (await extractions.detail(task.id)).task }
  catch (error) { await MessagePlugin.error(error instanceof Error ? error.message : String(error)) }
}
</script>

<template>
  <section class="data-section">
    <header class="section-header"><div><h2>进行中的解压</h2><p>飞牛本地同时只执行一个解压任务</p></div><span class="section-count">{{ activeTasks.length }}</span></header>
    <div v-if="activeTasks.length" class="task-table">
      <div class="task-table-head" aria-hidden="true"><span>状态</span><span>解压任务</span><span>进度</span><span>速度 / ETA</span><span>操作</span></div>
      <article v-for="task in activeTasks" :key="task.id" class="task-row">
        <div class="task-state"><span :class="['state-mark', `state-${task.state}`]" />{{ stateLabel[task.state] }}</div>
        <div class="task-primary"><strong class="task-route"><span><em>从</em>{{ sourceLabel(task) }}</span><i>→</i><span><em>到</em>{{ destinationLabel(task) }}</span></strong><small>{{ subtitle(task) }}</small><div class="thin-progress"><i :style="{ width: `${percent(task)}%` }" /></div></div>
        <div class="task-measure"><strong>{{ Math.round(percent(task)) }}%</strong><small>{{ formatBytes(task.extracted_bytes) }} / {{ formatBytes(task.total_bytes) }}</small></div>
        <div class="task-measure"><strong>{{ speed(task) }}</strong><small>{{ eta(task) }}</small></div>
        <div class="row-actions"><t-dropdown trigger="click" placement="bottom-right" :options="activeActions(task)"><t-button variant="text" size="small">操作<ChevronDown :size="14" /></t-button></t-dropdown></div>
      </article>
    </div>
    <div v-else class="plain-empty compact">当前没有正在执行的解压任务</div>
  </section>

  <section class="data-section history-section">
    <header class="section-header"><div><h2>解压历史</h2><p>失败或中断的任务可重新输入密码后提交</p></div><span class="section-count">{{ historyTasks.length }}</span></header>
    <div v-if="historyTasks.length" class="task-table">
      <div class="task-table-head history-table-head" aria-hidden="true"><span>状态</span><span>解压任务</span><span>解压数据 / 创建时间</span><span>操作</span></div>
      <article v-for="task in historyTasks" :key="task.id" class="task-row history-row">
        <div class="task-state"><span :class="['state-mark', `state-${task.state}`]" />{{ stateLabel[task.state] }}</div>
        <div class="task-primary"><strong class="task-route"><span><em>从</em>{{ sourceLabel(task) }}</span><i>→</i><span><em>到</em>{{ destinationLabel(task) }}</span></strong><small>{{ subtitle(task) }}</small></div>
        <div class="task-measure"><strong>{{ formatBytes(task.extracted_bytes) }} / {{ formatBytes(task.total_bytes) }}</strong><small>{{ formatTime(task.created_at) }}</small></div>
        <div class="row-actions"><t-dropdown trigger="click" placement="bottom-right" :options="historyActions(task)"><t-button variant="text" size="small">操作<ChevronDown :size="14" /></t-button></t-dropdown></div>
      </article>
    </div>
    <div v-else class="plain-empty compact">还没有解压历史</div>
  </section>

  <t-dialog v-model:visible="detailVisible" header="解压任务详情" width="720px" :footer="false">
    <div v-if="detailTask" class="task-detail"><div class="task-detail-route"><span><small>压缩包</small><strong>{{ sourceLabel(detailTask) }}</strong></span><i>→</i><span><small>目标目录</small><strong>{{ destinationLabel(detailTask) }}</strong></span></div><dl class="task-detail-grid"><div><dt>状态</dt><dd>{{ stateLabel[detailTask.state] }}</dd></div><div><dt>当前文件</dt><dd>{{ detailTask.current_file || '—' }}</dd></div><div><dt>完成项目</dt><dd>{{ detailTask.completed_items }} / {{ detailTask.total_items }}</dd></div><div><dt>解压数据</dt><dd>{{ formatBytes(detailTask.extracted_bytes) }} / {{ formatBytes(detailTask.total_bytes) }}</dd></div><div><dt>速度</dt><dd>{{ detailTask.speed_bytes ? `${formatBytes(detailTask.speed_bytes)}/s` : '—' }}</dd></div><div><dt>预计剩余</dt><dd>{{ detailTask.eta_seconds == null ? '—' : formatETA(detailTask.eta_seconds) }}</dd></div><div><dt>删除源分卷</dt><dd>{{ detailTask.delete_sources ? '是' : '否' }}</dd></div><div><dt>创建时间</dt><dd>{{ formatTime(detailTask.created_at) }}</dd></div><div><dt>任务编号</dt><dd class="mono">{{ detailTask.id }}</dd></div></dl><t-alert v-if="detailTask.error" theme="error" :message="detailTask.error" /><t-alert v-if="detailTask.warning" theme="warning" :message="detailTask.warning" /></div>
  </t-dialog>
  <t-dialog v-model:visible="retryVisible" header="重新提交解压任务" :confirm-btn="{ content: '重新提交' }" @confirm="submitRetry" @close="retryPassword = ''"><t-form label-align="top"><t-form-item label="压缩包密码（没有密码可留空）"><t-input v-model="retryPassword" type="password" autocomplete="off" /></t-form-item></t-form></t-dialog>
</template>

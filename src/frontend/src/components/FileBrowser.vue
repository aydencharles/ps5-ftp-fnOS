<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, shallowRef, watch } from 'vue'
import { DialogPlugin, MessagePlugin } from 'tdesign-vue-next'
import {
  ArrowLeft,
  ArrowRight,
  ArrowUp,
  Download,
  Eye,
  EyeOff,
  File,
  Folder,
  FolderOpen,
  FolderInput,
  FolderPlus,
  Gamepad2,
  Info,
  Pencil,
  RefreshCw,
  Send,
  Trash2,
  X,
} from '@lucide/vue'
import { formatBytes, joinPath, remoteParent } from '../api'
import BrowserDeviceBar from './BrowserDeviceBar.vue'
import PlayStationIcon from './PlayStationIcon.vue'
import { useLibraryStore } from '../stores/library'
import { useProfilesStore } from '../stores/profiles'
import { usePS5FilesStore } from '../stores/ps5Files'
import { useTasksStore } from '../stores/tasks'
import { DesktopFileSelectionController } from '../file-browser/selection'
import type { Entry, SourceLocator } from '../types'
import type { FileSelectionSnapshot, IFileSelectionController, SelectionModifiers } from '../file-browser/selection'

type BrowserMode = 'source' | 'manage' | 'destination'
type SortKey = 'name' | 'size' | 'modified_at'

const props = withDefaults(defineProps<{
  mode?: BrowserMode
  modelValue?: string
  compact?: boolean
  picker?: boolean
  selectedSources?: SourceLocator[]
}>(), {
  mode: 'manage',
  modelValue: '/',
  compact: false,
  picker: false,
  selectedSources: () => [],
})

const emit = defineEmits<{
  'update:modelValue': [path: string]
  'update:selectedSources': [sources: SourceLocator[]]
  'choose-local-path': [locator: SourceLocator]
  'copy-to-ps5': []
  'copy-to-fnos': [entries: Entry[]]
}>()
const library = useLibraryStore()
const profiles = useProfilesStore()
const files = usePS5FilesStore()
const tasks = useTasksStore()
const selectionController: IFileSelectionController = new DesktopFileSelectionController()
const selection = shallowRef<FileSelectionSnapshot>(selectionController.snapshot)
const history = ref<string[]>([])
const historyIndex = ref(-1)
const busy = ref(false)
const sort = reactive<{ key: SortKey; descending: boolean }>({ key: 'name', descending: false })

const editVisible = ref(false)
const edit = reactive<{ mode: 'mkdir' | 'rename'; title: string; name: string }>({ mode: 'mkdir', title: '', name: '' })
const moveVisible = ref(false)
const movePath = ref('/')
const moveEntries = ref<Entry[]>([])
const moveLoading = ref(false)
const deleteVisible = ref(false)
const deleteConfirm = ref('')
const propertiesVisible = ref(false)
const propertiesEntry = ref<Entry | null>(null)
const pickerTargetPath = ref(props.mode === 'source' ? '' : '/')
const pickerFocusedPath = ref<string | null>(null)
const lastEmittedTargetPath = ref<string | null>(null)
const browserRoot = ref<globalThis.HTMLElement | null>(null)
const contextMenuElement = ref<globalThis.HTMLElement | null>(null)
const contextMenu = reactive({ visible: false, x: 0, y: 0, entryPath: '' })

const isSource = computed(() => props.mode === 'source')
const isDestination = computed(() => props.mode === 'destination')
const isLocalPicker = computed(() => isSource.value && props.picker)
const isPicker = computed(() => isDestination.value || isLocalPicker.value)
const basePath = computed(() => isSource.value ? '' : normalizeRemotePath(profiles.selected?.base_path || '/'))
const currentPath = computed(() => isSource.value ? library.path : files.path)
const browserEntries = computed(() => isSource.value ? library.entries : files.entries)
const browserLoading = computed(() => isSource.value ? library.loading : files.loading)
const browserError = computed(() => isSource.value ? library.error : files.error)
const browserReady = computed(() => isSource.value ? Boolean(library.rootId) : Boolean(files.profileId))
const visibleEntries = computed(() => isPicker.value ? browserEntries.value.filter((entry) => entry.is_dir) : browserEntries.value)
const selectedEntries = computed(() => {
  const selected = new Set(selection.value.selectedKeys)
  return browserEntries.value.filter((entry) => selected.has(entry.path))
})
const selectionCount = computed(() => isPicker.value ? 0 : selectedEntries.value.length)
const selectedSize = computed(() => selectedEntries.value.reduce((total, entry) => total + (entry.is_dir ? 0 : entry.size), 0))
const singleSelection = computed(() => selectedEntries.value.length === 1 ? selectedEntries.value[0] : null)
const canDelete = computed(() => selectedEntries.value.length === 1 || (selectedEntries.value.length > 1 && selectedEntries.value.every((entry) => !entry.is_dir)))
const atBase = computed(() => currentPath.value === basePath.value)
const canBack = computed(() => historyIndex.value > 0)
const canForward = computed(() => historyIndex.value >= 0 && historyIndex.value < history.value.length - 1)

const sortedEntries = computed(() => [...visibleEntries.value].sort((left, right) => {
  if (!isPicker.value && left.is_dir !== right.is_dir) return left.is_dir ? -1 : 1
  let result = 0
  if (sort.key === 'name') result = left.name.localeCompare(right.name, 'zh-CN', { numeric: true, sensitivity: 'base' })
  if (sort.key === 'size') result = left.size - right.size
  if (sort.key === 'modified_at') result = (left.modified_at || '').localeCompare(right.modified_at || '')
  return sort.descending ? -result : result
}))
const sortedEntryKeys = computed(() => sortedEntries.value.map((entry) => entry.path))
const headerSelectionState = computed(() => {
  void selection.value
  return selectionController.visibleState(sortedEntryKeys.value)
})

function normalizeRemotePath(value: string) {
  const parts: string[] = []
  for (const part of value.split('/')) {
    if (!part || part === '.') continue
    if (part === '..') parts.pop()
    else parts.push(part)
  }
  return `/${parts.join('/')}`
}

function normalizeLocalPath(value: string) {
  const parts: string[] = []
  for (const part of value.split('/')) {
    if (!part || part === '.') continue
    if (part === '..') parts.pop()
    else parts.push(part)
  }
  return parts.join('/')
}

function isInsideBase(path: string, base: string) {
  return base === '/' || path === base || path.startsWith(`${base}/`)
}

function relativeSegments(current: string, base: string) {
  const rest = base === '/' ? current.slice(1) : current.slice(base.length).replace(/^\//, '')
  const parts = rest.split('/').filter(Boolean)
  let value = base
  return parts.map((part) => {
    value = joinPath(value, part)
    return { label: part, path: value }
  })
}

const breadcrumbs = computed(() => [
  ...(isSource.value ? localBreadcrumbs() : [
    { label: profiles.selected?.name || 'PS5', path: basePath.value },
    ...relativeSegments(files.path, basePath.value),
  ]),
])
const moveBreadcrumbs = computed(() => [
  { label: profiles.selected?.name || 'PS5', path: basePath.value },
  ...relativeSegments(movePath.value, basePath.value),
])

function localBreadcrumbs() {
  const rootLabel = library.roots.find((root) => root.id === library.rootId)?.label || 'fnOS'
  const parts = normalizeLocalPath(library.path).split('/').filter(Boolean)
  const crumbs = [{ label: rootLabel, path: '' }]
  let value = ''
  parts.forEach((part, index) => {
    value = value ? `${value}/${part}` : part
    if (index === 0 && /^\d+$/.test(part)) return
    crumbs.push({ label: part, path: value })
  })
  return crumbs
}

watch(() => profiles.selectedId, (id) => { if (!isSource.value) void resetProfile(id) }, { immediate: true, flush: 'post' })
watch(() => library.rootId, (id) => { if (isSource.value) void resetLibrary(id) }, { immediate: true, flush: 'post' })
watch(() => props.modelValue, (value) => {
  if (!isDestination.value) return
  const path = normalizeRemotePath(value)
  if (!isInsideBase(path, basePath.value)) return
  pickerTargetPath.value = path
  if (lastEmittedTargetPath.value === path) {
    lastEmittedTargetPath.value = null
    return
  }
  if (files.profileId && path !== files.path) void navigate(path)
})
watch(() => props.selectedSources, (sources) => {
  if (!isSource.value || isPicker.value) return
  const keys = sources.filter((source) => source.root_id === library.rootId).map((source) => source.path)
  if (keys.length === selection.value.selectedKeys.length && keys.every((key, index) => key === selection.value.selectedKeys[index])) return
  selection.value = selectionController.replace(keys)
}, { deep: true, immediate: true })

async function resetLibrary(id: string) {
  clearSelection()
  history.value = []
  historyIndex.value = -1
  library.query = ''
  library.path = ''
  if (!id) { library.entries = []; return }
  await navigate('', true)
}

async function resetProfile(id: string) {
  files.profileId = id
  files.query = ''
  clearSelection()
  history.value = []
  historyIndex.value = -1
  if (!id) {
    files.entries = []
    files.error = ''
    files.path = '/'
    return
  }
  const requested = normalizeRemotePath(props.modelValue)
  const initialPath = isDestination.value && isInsideBase(requested, basePath.value) ? requested : basePath.value
  if (isDestination.value && requested !== initialPath) emit('update:modelValue', initialPath)
  await navigate(initialPath, true)
}

async function navigate(path: string, record = true) {
  if (!isPicker.value) clearSelection()
  if (isSource.value) {
    library.path = normalizeLocalPath(path)
  } else {
    const normalized = normalizeRemotePath(path || basePath.value)
    files.path = isInsideBase(normalized, basePath.value) ? normalized : basePath.value
  }
  if (isSource.value) await library.load()
  else await files.load()
  if (isLocalPicker.value) {
    pickDirectoryPath(library.path)
  } else if (isDestination.value) {
    pickDirectoryPath(files.path)
  }
  if (browserError.value || !record) return
  history.value = history.value.slice(0, historyIndex.value + 1)
  if (history.value[history.value.length - 1] !== currentPath.value) history.value.push(currentPath.value)
  historyIndex.value = history.value.length - 1
}

async function goHistory(offset: number) {
  const next = historyIndex.value + offset
  if (next < 0 || next >= history.value.length) return
  historyIndex.value = next
  await navigate(history.value[next], false)
}

function goUp() {
  if (isSource.value) {
    const parts = normalizeLocalPath(library.path).split('/').filter(Boolean)
    parts.pop()
    const parent = parts.length === 1 && /^\d+$/.test(parts[0]) ? '' : parts.join('/')
    void navigate(parent)
    return
  }
  const parent = normalizeRemotePath(remoteParent(files.path))
  void navigate(isInsideBase(parent, basePath.value) ? parent : basePath.value)
}

function open(entry: Entry) {
  if (entry.is_dir) void navigate(entry.path)
  else if (!isPicker.value) showProperties(entry)
}

function applySelection(snapshot: FileSelectionSnapshot, notify = true) {
  selection.value = snapshot
  if (notify && isSource.value && !isPicker.value) {
    emit('update:selectedSources', snapshot.selectedKeys.map((path) => ({ root_id: library.rootId, path })))
  }
}

function selectEntry(entry: Entry, modifiers?: SelectionModifiers) {
  if (isPicker.value) {
    pickDirectoryPath(entry.path)
    return
  }
  applySelection(selectionController.select({ key: entry.path, orderedKeys: sortedEntryKeys.value, modifiers }))
}

function toggleEntry(entry: Entry) {
  if (isPicker.value) return
  applySelection(selectionController.toggle(entry.path))
}

function toggleAll() {
  if (isPicker.value) return
  applySelection(selectionController.toggleVisible(sortedEntryKeys.value))
}

function isEntrySelected(entry: Entry) {
  if (isPicker.value) return false
  void selection.value
  return selectionController.isSelected(entry.path)
}

function clearSelection() {
  if (isPicker.value) return
  applySelection(selectionController.clear())
}

function pickDirectoryPath(path: string) {
  if (!isPicker.value) return
  const target = isSource.value ? normalizeLocalPath(path) : normalizeRemotePath(path)
  pickerTargetPath.value = target
  if (isLocalPicker.value) {
    emit('choose-local-path', { root_id: library.rootId, path: target })
    return
  }
  lastEmittedTargetPath.value = target
  if (props.modelValue !== target) emit('update:modelValue', target)
}

function isPickerTarget(entry: Entry) {
  return isPicker.value && pickerTargetPath.value === entry.path
}

function toggleHidden() {
  library.hidden = !library.hidden
  void library.load()
}

function changeSort(key: SortKey) {
  if (sort.key === key) sort.descending = !sort.descending
  else { sort.key = key; sort.descending = false }
}

function sortMark(key: SortKey) {
  if (sort.key !== key) return ''
  return sort.descending ? '↓' : '↑'
}

function formatModified(value?: string) {
  if (!value) return '—'
  return new Date(value).toLocaleString('zh-CN', { hour12: false })
}

function entryType(entry: Entry) {
  if (entry.is_dir) return '文件夹'
  const extension = entry.name.split('.').pop()?.toUpperCase()
  return extension && extension !== entry.name.toUpperCase() ? `${extension} 文件` : '文件'
}

function showCreate() {
  edit.mode = 'mkdir'
  edit.title = '新建文件夹'
  edit.name = '新建文件夹'
  editVisible.value = true
}

function showRename() {
  if (!singleSelection.value || isPicker.value) return
  edit.mode = 'rename'
  edit.title = '重命名'
  edit.name = singleSelection.value.name
  editVisible.value = true
}

function showProperties(entry: Entry | null = singleSelection.value) {
  if (!entry || isPicker.value) return
  propertiesEntry.value = entry
  propertiesVisible.value = true
  closeContextMenu()
}

function validName(name: string) {
  return Boolean(name) && name !== '.' && name !== '..' && !/[\\/]/.test(name)
}

async function applyEdit() {
  const name = edit.name.trim()
  if (!validName(name)) { await MessagePlugin.warning('名称不能为空，且不能包含 / 或 \\'); return }
  busy.value = true
  try {
    if (edit.mode === 'mkdir') {
      const createdPath = joinPath(files.path, name)
      await files.operation({ action: 'mkdir', path: createdPath }, false)
      if (isDestination.value) {
        await navigate(createdPath)
      } else await files.load()
    } else if (singleSelection.value) {
      await files.operation({ action: 'rename', path: singleSelection.value.path, destination: joinPath(files.path, name) })
    }
    clearSelection()
    editVisible.value = false
    await MessagePlugin.success(edit.mode === 'mkdir' ? '文件夹已创建' : '名称已更新')
  } catch (error) {
    await MessagePlugin.error(error instanceof Error ? error.message : String(error))
  } finally { busy.value = false }
}

async function loadMovePath(path: string) {
  moveLoading.value = true
  try {
    const normalized = normalizeRemotePath(path)
    const result = await files.directories(isInsideBase(normalized, basePath.value) ? normalized : basePath.value)
    movePath.value = result.path
    moveEntries.value = result.entries.filter((entry) => !selectedEntries.value.some((selected) => selected.is_dir && (entry.path === selected.path || entry.path.startsWith(`${selected.path}/`))))
  } catch (error) {
    await MessagePlugin.error(error instanceof Error ? error.message : String(error))
  } finally { moveLoading.value = false }
}

async function showMove() {
  if (!selectedEntries.value.length || isPicker.value) return
  moveVisible.value = true
  await loadMovePath(files.path)
}

async function applyMove() {
  busy.value = true
  try {
    for (const entry of selectedEntries.value) {
      const destination = joinPath(movePath.value, entry.name)
      if (destination !== entry.path) await files.operation({ action: 'move', path: entry.path, destination }, false)
    }
    moveVisible.value = false
    clearSelection()
    await files.load()
    await MessagePlugin.success('已移动到目标文件夹')
  } catch (error) {
    await MessagePlugin.error(error instanceof Error ? error.message : String(error))
  } finally { busy.value = false }
}

function askDelete() {
  if (!canDelete.value || isPicker.value) return
  const entries = [...selectedEntries.value]
  if (entries.length === 1 && entries[0].is_dir) {
    deleteConfirm.value = ''
    deleteVisible.value = true
    return
  }
  const dialog = DialogPlugin.confirm({
    header: `永久删除 ${entries.length} 个文件？`,
    body: entries.length === 1 ? `${entries[0].name} 将从 PS5 永久删除，无法恢复。` : '选中的文件将从 PS5 永久删除，无法恢复。',
    confirmBtn: { content: '永久删除', theme: 'danger' },
    onConfirm: async () => {
      busy.value = true
      try {
        for (const entry of entries) await files.operation({ action: 'delete', path: entry.path, is_dir: false }, false)
        clearSelection()
        await files.load()
        dialog.destroy()
        await MessagePlugin.success('文件已删除')
      } catch (error) {
        await MessagePlugin.error(error instanceof Error ? error.message : String(error))
      } finally { busy.value = false }
    },
  })
}

async function applyDirectoryDelete() {
  const entry = singleSelection.value
  if (!entry || deleteConfirm.value !== entry.name || isPicker.value) return
  busy.value = true
  try {
    const result = await files.operation({ action: 'delete', path: entry.path, is_dir: true, recursive: true, confirm_name: deleteConfirm.value })
    deleteVisible.value = false
    clearSelection()
    if (result.task) {
      tasks.openCenter()
      await MessagePlugin.warning('递归删除已加入任务中心')
    }
  } catch (error) {
    await MessagePlugin.error(error instanceof Error ? error.message : String(error))
  } finally { busy.value = false }
}

function closeContextMenu(event?: { composedPath?: () => unknown[] }) {
  const clickedMenu = event?.composedPath?.().some((node) => {
    const classes = (node as { classList?: { contains: (value: string) => boolean } }).classList
    return classes?.contains('file-context-menu')
  })
  if (clickedMenu) return
  contextMenu.visible = false
}

function selectForContextMenu(entry: Entry) {
  applySelection(selectionController.prepareContextMenu(entry.path))
}

async function openContextMenu(entry: Entry, x: number, y: number) {
  selectForContextMenu(entry)
  contextMenu.entryPath = entry.path
  contextMenu.x = x
  contextMenu.y = y
  contextMenu.visible = true
  await nextTick()
  const menu = contextMenuElement.value
  if (!menu) return
  const rect = menu.getBoundingClientRect()
  contextMenu.x = Math.max(4, Math.min(x, globalThis.innerWidth - rect.width - 4))
  contextMenu.y = Math.max(4, Math.min(y, globalThis.innerHeight - rect.height - 4))
  menu.querySelector<globalThis.HTMLButtonElement>('button:not(:disabled)')?.focus()
}

function showContextMenu(event: { preventDefault: () => void; stopPropagation: () => void; clientX: number; clientY: number }, entry: Entry) {
  if (isPicker.value) return
  event.preventDefault()
  event.stopPropagation()
  void openContextMenu(entry, event.clientX, event.clientY)
}

function copyToPS5() {
  if (!selectionCount.value) return
  closeContextMenu()
  emit('copy-to-ps5')
}

function copyToFnOS() {
  if (!selectedEntries.value.length) return
  const entries = [...selectedEntries.value]
  closeContextMenu()
  emit('copy-to-fnos', entries)
}

function openSelection() {
  if (singleSelection.value) open(singleSelection.value)
  closeContextMenu()
}

function focusRow(path: string | null) {
  if (!path) return
  void nextTick(() => {
    const rows = browserRoot.value?.querySelectorAll<globalThis.HTMLTableRowElement>('tr[data-entry-path]')
    const row = rows ? [...rows].find((candidate) => candidate.dataset.entryPath === path) : undefined
    row?.focus()
  })
}

function rowTabIndex(entry: Entry, index: number) {
  const focusedPath = isPicker.value ? pickerFocusedPath.value : selection.value.focusedKey
  return focusedPath ? (focusedPath === entry.path ? 0 : -1) : (index === 0 ? 0 : -1)
}

function handleRowFocus(entry: Entry) {
  if (isPicker.value) {
    pickerFocusedPath.value = entry.path
    return
  }
  if (!selection.value.focusedKey) selectEntry(entry)
}

function movePickerFocus(direction: -1 | 1 | 'first' | 'last') {
  if (!sortedEntryKeys.value.length) return
  const currentIndex = pickerFocusedPath.value ? sortedEntryKeys.value.indexOf(pickerFocusedPath.value) : -1
  let targetIndex: number
  if (direction === 'first') targetIndex = 0
  else if (direction === 'last') targetIndex = sortedEntryKeys.value.length - 1
  else if (currentIndex < 0) targetIndex = direction > 0 ? 0 : sortedEntryKeys.value.length - 1
  else targetIndex = Math.max(0, Math.min(sortedEntryKeys.value.length - 1, currentIndex + direction))
  pickerFocusedPath.value = sortedEntryKeys.value[targetIndex]
  focusRow(pickerFocusedPath.value)
}

function handleRowKeydown(event: globalThis.KeyboardEvent, entry: Entry) {
  if ((event.shiftKey && event.key === 'F10') || event.key === 'ContextMenu') {
    if (isPicker.value) return
    event.preventDefault()
    const rect = (event.currentTarget as globalThis.HTMLElement).getBoundingClientRect()
    void openContextMenu(entry, rect.left + 24, rect.top + 24)
    return
  }
  if (event.key === 'Enter') {
    event.preventDefault()
    open(entry)
    return
  }
  if (event.key === ' ') {
    event.preventDefault()
    if (isPicker.value) pickDirectoryPath(entry.path)
    else toggleEntry(entry)
    return
  }
  if (event.key === 'Escape') {
    if (!isPicker.value) clearSelection()
    return
  }
  if (!isPicker.value && (event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'a') {
    event.preventDefault()
    applySelection(selectionController.replace(sortedEntryKeys.value))
    return
  }
  const direction = event.key === 'ArrowDown' ? 1 : event.key === 'ArrowUp' ? -1 : event.key === 'Home' ? 'first' : event.key === 'End' ? 'last' : null
  if (direction === null) return
  event.preventDefault()
  if (isPicker.value) {
    movePickerFocus(direction)
    return
  }
  const snapshot = selectionController.moveFocus({
    orderedKeys: sortedEntryKeys.value,
    direction,
    modifiers: { shiftKey: event.shiftKey },
  })
  applySelection(snapshot)
  focusRow(snapshot.focusedKey)
}

function handleTableBackgroundClick(event: globalThis.MouseEvent) {
  if (isPicker.value || browserLoading.value) return
  const target = event.target
  if (!(target instanceof globalThis.Element)) return
  if (target.closest('tr[data-entry-path]') || target.closest('thead')) return
  clearSelection()
}

function handleGlobalKeydown(event: globalThis.KeyboardEvent) {
  if (event.key === 'Escape' && contextMenu.visible) closeContextMenu()
}

onMounted(() => {
  globalThis.addEventListener('pointerdown', closeContextMenu)
  globalThis.addEventListener('keydown', handleGlobalKeydown)
  // Bootstrap assigns the first Library Root after the initial route has
  // mounted. The watcher above covers the normal case; this closes the
  // remaining mount-timing gap without reloading an already active browser.
  if (isSource.value && library.rootId && !library.loading && !library.entries.length) {
    void resetLibrary(library.rootId)
  }
})
onBeforeUnmount(() => {
  globalThis.removeEventListener('pointerdown', closeContextMenu)
  globalThis.removeEventListener('keydown', handleGlobalKeydown)
})
</script>

<template>
  <section ref="browserRoot" :class="['file-station', { 'is-compact': compact, 'is-destination': isDestination, 'is-source': isSource }]">
    <BrowserDeviceBar
      v-if="!compact"
      :title="profiles.selected?.name || '尚未选择 PS5'"
      :subtitle="profiles.selected ? `${profiles.selected.host}:${profiles.selected.port} · ${profiles.selected.preset}` : '请先在设置中添加连接'"
    >
      <template #icon><PlayStationIcon :size="18" /></template>
      <template #control><t-select v-model="profiles.selectedId" :options="profiles.items.map(profile => ({ label: profile.name, value: profile.id }))" placeholder="选择 PS5" class="station-profile-select devicebar-select" /></template>
    </BrowserDeviceBar>

    <div class="station-navigation">
      <div class="navigation-buttons">
        <t-button variant="text" size="small" title="后退" aria-label="后退" :disabled="!canBack" @click="goHistory(-1)"><ArrowLeft :size="16" /></t-button>
        <t-button variant="text" size="small" title="前进" aria-label="前进" :disabled="!canForward" @click="goHistory(1)"><ArrowRight :size="16" /></t-button>
        <t-button variant="text" size="small" title="返回上级" aria-label="返回上级" :disabled="atBase" @click="goUp"><ArrowUp :size="16" /></t-button>
        <t-button variant="text" size="small" title="刷新" aria-label="刷新" :disabled="!browserReady" @click="isSource ? library.load() : files.load()"><RefreshCw :size="15" /></t-button>
      </div>
      <nav class="station-breadcrumb" :aria-label="isSource ? 'fnOS 当前路径' : 'PS5 当前路径'" data-testid="ps5-breadcrumb">
        <template v-for="(crumb, index) in breadcrumbs" :key="crumb.path">
          <span v-if="index" aria-hidden="true">/</span>
          <button :class="{ 'is-current': index === breadcrumbs.length - 1 }" @click="navigate(crumb.path)">{{ crumb.label }}</button>
        </template>
      </nav>
      <t-input v-if="isSource" v-model="library.query" clearable placeholder="搜索当前文件夹" class="station-search" @enter="library.load()" @clear="library.load()" />
      <t-input v-else v-model="files.query" clearable placeholder="搜索当前文件夹" class="station-search" @enter="clearSelection(); files.load()" @clear="files.load()" />
    </div>

    <div v-if="!isPicker" class="station-actions">
      <template v-if="isSource && !isLocalPicker">
        <t-button variant="text" size="small" @click="toggleHidden"><component :is="library.hidden ? EyeOff : Eye" :size="15" />{{ library.hidden ? '隐藏文件已显示' : '显示隐藏文件' }}</t-button>
        <t-button v-if="selectionCount" variant="text" size="small" @click="clearSelection"><X :size="15" />清空选择</t-button>
        <t-button theme="primary" size="small" :disabled="!selectionCount" @click="copyToPS5"><Send :size="15" />复制到 PS5</t-button>
        <span class="selection-note">已选择 {{ selectionCount }} 项</span>
      </template>
      <template v-else>
        <t-button theme="primary" size="small" :disabled="!files.profileId" @click="showCreate"><FolderPlus :size="15" />新建文件夹</t-button>
        <span class="action-separator" />
        <t-button variant="text" size="small" :disabled="!selectedEntries.length" @click="copyToFnOS"><Download :size="15" />复制到飞牛</t-button>
        <t-button variant="text" size="small" :disabled="!singleSelection" @click="showRename"><Pencil :size="14" />重命名</t-button>
        <t-button variant="text" size="small" :disabled="!selectedEntries.length" @click="showMove"><FolderInput :size="15" />移动到</t-button>
        <t-button theme="danger" variant="text" size="small" :disabled="!canDelete" @click="askDelete"><Trash2 :size="15" />删除</t-button>
        <span v-if="selectedEntries.length" class="selection-note">已选择 {{ selectedEntries.length }} 项</span>
        <span v-if="selectedEntries.length > 1 && !canDelete" class="selection-warning">包含文件夹时请逐个删除</span>
      </template>
    </div>

    <t-alert v-if="browserError" theme="error" :message="browserError" class="station-error" />

    <div class="station-table-wrap" @click="handleTableBackgroundClick">
      <table :class="['station-table', { 'is-destination-table': isPicker }]">
        <thead>
          <tr v-if="isPicker">
            <th><button @click="changeSort('name')">目录名称 <span>{{ sortMark('name') }}</span></button></th>
            <th class="time-column"><button @click="changeSort('modified_at')">修改时间 <span>{{ sortMark('modified_at') }}</span></button></th>
          </tr>
          <tr v-else>
            <th class="check-column" @click.stop @dblclick.stop @keydown.stop @mousedown.stop><t-checkbox :checked="headerSelectionState === 'all'" :indeterminate="headerSelectionState === 'partial'" @change="toggleAll" /></th>
            <th><button @click="changeSort('name')">名称 <span>{{ sortMark('name') }}</span></button></th>
            <th class="type-column">类型</th>
            <th class="size-column"><button @click="changeSort('size')">大小 <span>{{ sortMark('size') }}</span></button></th>
            <th class="time-column"><button @click="changeSort('modified_at')">修改时间 <span>{{ sortMark('modified_at') }}</span></button></th>
          </tr>
        </thead>
        <tbody :class="{ 'is-loading': browserLoading }">
          <template v-if="isPicker">
            <tr v-for="(entry, index) in sortedEntries" :key="entry.path" :data-entry-path="entry.path" :class="{ 'is-target': isPickerTarget(entry) }" :tabindex="rowTabIndex(entry, index)" @click="selectEntry(entry)" @dblclick="open(entry)" @focus="handleRowFocus(entry)" @keydown="handleRowKeydown($event, entry)">
              <td><span class="station-file-name" @dblclick.stop="open(entry)"><Folder class="file-type-icon is-folder" :size="17" /><span>{{ entry.name }}</span></span></td>
              <td class="time-column">{{ formatModified(entry.modified_at) }}</td>
            </tr>
          </template>
          <template v-else>
            <tr v-for="(entry, index) in sortedEntries" :key="entry.path" :data-entry-path="entry.path" :class="{ 'is-selected': isEntrySelected(entry) }" :tabindex="rowTabIndex(entry, index)" @click="selectEntry(entry, $event)" @contextmenu="showContextMenu($event, entry)" @dblclick="open(entry)" @focus="handleRowFocus(entry)" @keydown="handleRowKeydown($event, entry)">
              <td class="check-column" @click.stop @dblclick.stop @keydown.stop @mousedown.stop><t-checkbox :checked="isEntrySelected(entry)" @change="toggleEntry(entry)" /></td>
              <td><span class="station-file-name" @dblclick.stop="open(entry)"><Gamepad2 v-if="entry.game_kind" class="file-type-icon is-game" :size="17" /><Folder v-else-if="entry.is_dir" class="file-type-icon is-folder" :size="17" /><File v-else class="file-type-icon" :size="16" /><span class="file-name-copy"><span>{{ entry.name }}</span><small v-if="entry.game_kind">{{ entry.game_kind === 'game-directory' ? 'PS5 游戏目录' : '游戏镜像' }}</small></span></span></td>
              <td class="type-column">{{ entryType(entry) }}</td>
              <td class="size-column">{{ entry.is_dir ? '—' : formatBytes(entry.size) }}</td>
              <td class="time-column">{{ formatModified(entry.modified_at) }}</td>
            </tr>
          </template>
          <tr v-if="browserLoading" class="station-empty-row"><td :colspan="isPicker ? 2 : 5"><div class="station-empty">正在读取文件…</div></td></tr>
          <tr v-else-if="!browserReady" class="station-empty-row"><td :colspan="isPicker ? 2 : 5"><div class="station-empty"><strong>{{ isSource ? '没有可用的存储空间' : '还没有 PS5 连接' }}</strong><span>{{ isSource ? '请检查 fnOS 存储卷是否已挂载' : '请先在设置中添加并测试 FTP 连接' }}</span><t-button v-if="!isSource" variant="outline" size="small" @click="$router.push('/settings')">前往设置</t-button></div></td></tr>
          <tr v-else-if="!sortedEntries.length" class="station-empty-row"><td :colspan="isPicker ? 2 : 5"><div class="station-empty"><strong>{{ (isSource ? library.query : files.query) ? '没有匹配的项目' : isPicker ? '当前目录没有子文件夹' : '这个文件夹是空的' }}</strong><span>{{ (isSource ? library.query : files.query) ? '请尝试其他搜索词' : isSource ? '可以返回上级选择其他位置' : '可以在这里新建文件夹' }}</span></div></td></tr>
        </tbody>
      </table>
    </div>

    <footer class="station-statusbar">
      <template v-if="isSource && !isLocalPicker">
        <span>{{ browserEntries.length }} 个项目</span><strong v-if="selectionCount">已选择 {{ selectionCount }} 项</strong>
      </template>
      <template v-else-if="isPicker">
        <span>目标目录</span><strong class="destination-path">{{ pickerTargetPath || '根目录' }}</strong>
      </template>
      <template v-else>
        <span>{{ files.entries.length }} 个项目</span>
        <span v-if="selectedEntries.length">已选择 {{ selectedEntries.length }} 项<span v-if="selectedSize">，{{ formatBytes(selectedSize) }}</span></span>
      </template>
      <span class="status-path">当前：{{ currentPath || '根目录' }}</span>
    </footer>
  </section>

  <Teleport to="body">
    <div v-if="contextMenu.visible" ref="contextMenuElement" class="file-context-menu" role="menu" :style="{ left: `${contextMenu.x}px`, top: `${contextMenu.y}px` }" @click.stop @contextmenu.prevent @keydown.esc.stop="closeContextMenu()">
      <button role="menuitem" :disabled="!singleSelection" @click="openSelection"><FolderOpen :size="15" />打开</button>
      <template v-if="isSource">
        <button role="menuitem" :disabled="!selectionCount" @click="copyToPS5"><Send :size="15" />复制到 PS5</button>
      </template>
      <template v-else>
        <button role="menuitem" :disabled="!selectedEntries.length" @click="copyToFnOS"><Download :size="15" />复制到飞牛</button>
        <button role="menuitem" :disabled="!singleSelection" @click="showRename(); closeContextMenu()"><Pencil :size="15" />重命名</button>
        <button role="menuitem" :disabled="!selectedEntries.length" @click="showMove(); closeContextMenu()"><FolderInput :size="15" />移动到</button>
        <button role="menuitem" class="is-danger" :disabled="!canDelete" @click="askDelete(); closeContextMenu()"><Trash2 :size="15" />删除</button>
      </template>
      <button role="menuitem" :disabled="!singleSelection" @click="showProperties()"><Info :size="15" />属性</button>
    </div>
  </Teleport>

  <t-dialog v-model:visible="editVisible" :header="edit.title" :confirm-btn="{ content: '确认', loading: busy }" @confirm="applyEdit">
    <t-form label-align="top">
      <t-form-item label="名称"><t-input v-model="edit.name" autofocus @enter="applyEdit" /></t-form-item>
      <p class="dialog-path-hint">位置：{{ files.path }}</p>
    </t-form>
  </t-dialog>

  <t-dialog v-model:visible="propertiesVisible" header="属性" :confirm-btn="{ content: '关闭' }" @confirm="propertiesVisible = false">
    <dl v-if="propertiesEntry" class="file-properties">
      <div><dt>名称</dt><dd>{{ propertiesEntry.name }}</dd></div>
      <div><dt>类型</dt><dd>{{ entryType(propertiesEntry) }}</dd></div>
      <div><dt>大小</dt><dd>{{ propertiesEntry.is_dir ? '—' : formatBytes(propertiesEntry.size) }}</dd></div>
      <div><dt>修改时间</dt><dd>{{ formatModified(propertiesEntry.modified_at) }}</dd></div>
      <div v-if="!isSource"><dt>路径</dt><dd>{{ propertiesEntry.path }}</dd></div>
    </dl>
  </t-dialog>

  <template v-if="!isDestination && !isSource">
    <t-dialog v-model:visible="moveVisible" header="移动到" width="640px" :confirm-btn="{ content: `移动 ${selectedEntries.length} 项`, loading: busy }" @confirm="applyMove">
      <div class="folder-picker">
        <div class="folder-picker-nav">
          <t-button variant="text" size="small" :disabled="movePath === basePath" @click="loadMovePath(remoteParent(movePath))"><ArrowUp :size="14" />返回上级</t-button>
          <nav class="station-breadcrumb">
            <template v-for="(crumb, index) in moveBreadcrumbs" :key="crumb.path">
              <span v-if="index">/</span><button :class="{ 'is-current': index === moveBreadcrumbs.length - 1 }" @click="loadMovePath(crumb.path)">{{ crumb.label }}</button>
            </template>
          </nav>
        </div>
        <div class="folder-picker-list" :class="{ 'is-loading': moveLoading }">
          <button v-for="entry in moveEntries" :key="entry.path" @dblclick="loadMovePath(entry.path)"><Folder class="file-type-icon is-folder" :size="17" /><span>{{ entry.name }}</span><small>双击打开</small></button>
          <div v-if="!moveLoading && !moveEntries.length" class="station-empty">当前目录没有子文件夹</div>
        </div>
        <div class="folder-picker-target"><span>移动到</span><strong>{{ movePath }}</strong></div>
      </div>
    </t-dialog>

    <t-dialog v-model:visible="deleteVisible" header="永久删除文件夹" :confirm-btn="{ content: '永久删除', theme: 'danger', loading: busy, disabled: deleteConfirm !== singleSelection?.name }" @confirm="applyDirectoryDelete">
      <t-alert theme="error" message="文件夹及其中的所有内容将永久删除。请输入文件夹名称进行确认。" />
      <p class="dialog-hint">{{ singleSelection?.name }}</p>
      <t-input v-model="deleteConfirm" placeholder="输入完整文件夹名称" />
    </t-dialog>
  </template>
</template>

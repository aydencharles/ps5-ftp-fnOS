<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
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
  FolderInput,
  FolderPlus,
  Gamepad2,
  Pencil,
  RefreshCw,
  Send,
  Trash2,
  X,
} from '@lucide/vue'
import { formatBytes, joinPath, remoteParent } from '../api'
import { useLibraryStore } from '../stores/library'
import { useProfilesStore } from '../stores/profiles'
import { usePS5FilesStore } from '../stores/ps5Files'
import type { Entry, SourceLocator } from '../types'

type BrowserMode = 'source' | 'manage' | 'destination'
type SortKey = 'name' | 'size' | 'modified_at'

const props = withDefaults(defineProps<{
  mode?: BrowserMode
  modelValue?: string
  compact?: boolean
  picker?: boolean
}>(), {
  mode: 'manage',
  modelValue: '/',
  compact: false,
  picker: false,
})

const emit = defineEmits<{
  'update:modelValue': [path: string]
  'choose-local-path': [locator: SourceLocator]
  'copy-to-ps5': []
  'copy-to-fnos': [entries: Entry[]]
}>()
const library = useLibraryStore()
const profiles = useProfilesStore()
const files = usePS5FilesStore()
const selectedPaths = ref<string[]>([])
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
const contextMenu = reactive({ visible: false, x: 0, y: 0 })

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
  if (isSource.value) return library.selected.map((locator) => library.entries.find((entry) => entry.path === locator.path)).filter((entry): entry is Entry => Boolean(entry))
  return selectedPaths.value.map((path) => files.entries.find((entry) => entry.path === path)).filter((entry): entry is Entry => Boolean(entry))
})
const selectionCount = computed(() => isLocalPicker.value ? 0 : isSource.value ? library.selected.length : selectedEntries.value.length)
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
  if (!isDestination.value || !files.profileId) return
  const path = normalizeRemotePath(value)
  if (path !== files.path && isInsideBase(path, basePath.value)) void navigate(path)
})

async function resetLibrary(id: string) {
  selectedPaths.value = []
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
  selectedPaths.value = []
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
  if (isSource.value) {
    library.path = normalizeLocalPath(path)
  } else {
    const normalized = normalizeRemotePath(path || basePath.value)
    files.path = isInsideBase(normalized, basePath.value) ? normalized : basePath.value
  }
  selectedPaths.value = []
  if (isSource.value) await library.load()
  else await files.load()
  if (isLocalPicker.value) {
    emit('choose-local-path', { root_id: library.rootId, path: library.path })
  } else if (isDestination.value && props.modelValue !== files.path) {
    emit('update:modelValue', files.path)
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
  else if (isSource.value && !isLocalPicker.value) library.toggle(entry)
}

function selectEntry(entry: Entry, event?: { ctrlKey: boolean; metaKey: boolean }) {
  if (isPicker.value) return
  if (isSource.value) {
    if (event?.ctrlKey || event?.metaKey || !library.isSelected(entry)) library.toggle(entry)
    return
  }
  if (event?.ctrlKey || event?.metaKey) {
    selectedPaths.value = selectedPaths.value.includes(entry.path)
      ? selectedPaths.value.filter((path) => path !== entry.path)
      : [...selectedPaths.value, entry.path]
    return
  }
  selectedPaths.value = [entry.path]
}

function toggleEntry(entry: Entry) {
  if (isSource.value) { library.toggle(entry); return }
  selectedPaths.value = selectedPaths.value.includes(entry.path)
    ? selectedPaths.value.filter((path) => path !== entry.path)
    : [...selectedPaths.value, entry.path]
}

function toggleAll() {
  if (isPicker.value) return
  if (isSource.value) {
    const allSelected = sortedEntries.value.length > 0 && sortedEntries.value.every((entry) => library.isSelected(entry))
    if (allSelected) {
      const shown = new Set(sortedEntries.value.map((entry) => entry.path))
      library.selected = library.selected.filter((locator) => locator.root_id !== library.rootId || !shown.has(locator.path))
    } else {
      sortedEntries.value.forEach((entry) => { if (!library.isSelected(entry)) library.toggle(entry) })
    }
    return
  }
  selectedPaths.value = selectedPaths.value.length === sortedEntries.value.length ? [] : sortedEntries.value.map((entry) => entry.path)
}

function isEntrySelected(entry: Entry) {
  if (isPicker.value) return false
  return isSource.value ? library.isSelected(entry) : selectedPaths.value.includes(entry.path)
}

function clearSelection() {
  if (isPicker.value) return
  if (isSource.value) library.selected = []
  else selectedPaths.value = []
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
    selectedPaths.value = []
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
    selectedPaths.value = []
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
        selectedPaths.value = []
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
    selectedPaths.value = []
    if (result.task) await MessagePlugin.warning('递归删除已加入任务中心')
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
  if (isSource.value) {
    if (!library.isSelected(entry)) library.selected = [{ root_id: library.rootId, path: entry.path }]
    return
  }
  if (!selectedPaths.value.includes(entry.path)) selectedPaths.value = [entry.path]
}

function showContextMenu(event: { preventDefault: () => void; stopPropagation: () => void; clientX: number; clientY: number }, entry: Entry) {
  if (isPicker.value) return
  event.preventDefault()
  event.stopPropagation()
  selectForContextMenu(entry)
  contextMenu.x = event.clientX
  contextMenu.y = event.clientY
  contextMenu.visible = true
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

onMounted(() => {
  globalThis.addEventListener('pointerdown', closeContextMenu)
  // Bootstrap assigns the first Library Root after the initial route has
  // mounted. The watcher above covers the normal case; this closes the
  // remaining mount-timing gap without reloading an already active browser.
  if (isSource.value && library.rootId && !library.loading && !library.entries.length) {
    void resetLibrary(library.rootId)
  }
})
onBeforeUnmount(() => globalThis.removeEventListener('pointerdown', closeContextMenu))
</script>

<template>
  <section :class="['file-station', { 'is-compact': compact, 'is-destination': isDestination, 'is-source': isSource }]">
    <header v-if="!compact" class="station-devicebar">
      <div class="station-device">
        <span class="device-mark"><Gamepad2 :size="17" /></span>
        <div><strong>{{ profiles.selected?.name || '尚未选择 PS5' }}</strong><small v-if="profiles.selected">{{ profiles.selected.host }}:{{ profiles.selected.port }} · {{ profiles.selected.preset }}</small><small v-else>请先在设置中添加连接</small></div>
      </div>
      <t-select v-model="profiles.selectedId" :options="profiles.items.map(profile => ({ label: profile.name, value: profile.id }))" placeholder="选择 PS5" class="station-profile-select" />
    </header>

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
      <t-input v-else v-model="files.query" clearable placeholder="搜索当前文件夹" class="station-search" @enter="selectedPaths = []; files.load()" @clear="files.load()" />
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

    <div class="station-table-wrap">
      <table :class="['station-table', { 'is-destination-table': isPicker }]">
        <thead>
          <tr v-if="isPicker">
            <th><button @click="changeSort('name')">目录名称 <span>{{ sortMark('name') }}</span></button></th>
            <th class="time-column"><button @click="changeSort('modified_at')">修改时间 <span>{{ sortMark('modified_at') }}</span></button></th>
          </tr>
          <tr v-else>
            <th class="check-column" @click.stop @dblclick.stop @keydown.stop @mousedown.stop><t-checkbox :checked="Boolean(sortedEntries.length) && sortedEntries.every(entry => isEntrySelected(entry))" @change="toggleAll" /></th>
            <th><button @click="changeSort('name')">名称 <span>{{ sortMark('name') }}</span></button></th>
            <th class="type-column">类型</th>
            <th class="size-column"><button @click="changeSort('size')">大小 <span>{{ sortMark('size') }}</span></button></th>
            <th class="time-column"><button @click="changeSort('modified_at')">修改时间 <span>{{ sortMark('modified_at') }}</span></button></th>
          </tr>
        </thead>
        <tbody :class="{ 'is-loading': browserLoading }">
          <template v-if="isPicker">
            <tr v-for="entry in sortedEntries" :key="entry.path" :data-entry-path="entry.path" tabindex="0" @dblclick="open(entry)" @keydown.enter="open(entry)">
              <td><button class="station-file-name" @dblclick.stop="open(entry)"><Folder class="file-type-icon is-folder" :size="17" /><span>{{ entry.name }}</span></button></td>
              <td class="time-column">{{ formatModified(entry.modified_at) }}</td>
            </tr>
          </template>
          <template v-else>
            <tr v-for="entry in sortedEntries" :key="entry.path" :data-entry-path="entry.path" :class="{ 'is-selected': isEntrySelected(entry) }" tabindex="0" @click="selectEntry(entry, $event)" @contextmenu="showContextMenu($event, entry)" @dblclick="open(entry)" @keydown.enter="open(entry)">
              <td class="check-column" @click.stop @dblclick.stop @keydown.stop @mousedown.stop><t-checkbox :checked="isEntrySelected(entry)" @change="toggleEntry(entry)" /></td>
              <td><button class="station-file-name" @dblclick.stop="open(entry)"><Gamepad2 v-if="entry.game_kind" class="file-type-icon is-game" :size="17" /><Folder v-else-if="entry.is_dir" class="file-type-icon is-folder" :size="17" /><File v-else class="file-type-icon" :size="16" /><span class="file-name-copy"><span>{{ entry.name }}</span><small v-if="entry.game_kind">{{ entry.game_kind === 'game-directory' ? 'PS5 游戏目录' : '游戏镜像' }}</small></span></button></td>
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
        <span>当前选择</span><strong class="destination-path">{{ currentPath || '根目录' }}</strong>
      </template>
      <template v-else>
        <span>{{ files.entries.length }} 个项目</span>
        <span v-if="selectedEntries.length">已选择 {{ selectedEntries.length }} 项<span v-if="selectedSize">，{{ formatBytes(selectedSize) }}</span></span>
      </template>
      <span class="status-path">当前：{{ currentPath || '根目录' }}</span>
    </footer>
  </section>

  <Teleport to="body">
    <div v-if="contextMenu.visible" class="file-context-menu" :style="{ left: `${contextMenu.x}px`, top: `${contextMenu.y}px` }" @click.stop @contextmenu.prevent>
      <template v-if="isSource">
        <button :disabled="!selectionCount" @click="copyToPS5"><Send :size="15" />复制到 PS5</button>
        <button :disabled="!selectionCount" @click="clearSelection(); closeContextMenu()"><X :size="15" />取消选择</button>
      </template>
      <template v-else>
        <button :disabled="!selectedEntries.length" @click="copyToFnOS"><Download :size="15" />复制到飞牛</button>
        <button :disabled="!singleSelection" @click="showRename(); closeContextMenu()"><Pencil :size="15" />重命名</button>
        <button :disabled="!selectedEntries.length" @click="showMove(); closeContextMenu()"><FolderInput :size="15" />移动到</button>
        <button class="is-danger" :disabled="!canDelete" @click="askDelete(); closeContextMenu()"><Trash2 :size="15" />删除</button>
      </template>
    </div>
  </Teleport>

  <t-dialog v-model:visible="editVisible" :header="edit.title" :confirm-btn="{ content: '确认', loading: busy }" @confirm="applyEdit">
    <t-form label-align="top">
      <t-form-item label="名称"><t-input v-model="edit.name" autofocus @enter="applyEdit" /></t-form-item>
      <p class="dialog-path-hint">位置：{{ files.path }}</p>
    </t-form>
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

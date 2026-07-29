<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { CheckCircle2, CircleMinus, LoaderCircle, Server, XCircle } from '@lucide/vue'
import { useProfilesStore } from '../stores/profiles'
import type { ConnectionCheck, ConnectionTestResult, Profile } from '../types'

const visible = defineModel<boolean>('visible', { default: false })
const props = defineProps<{ profile: Profile | null }>()
const profiles = useProfilesStore()
const result = ref<ConnectionTestResult | null>(null)
const requestError = ref('')
const running = ref(false)
let controller: globalThis.AbortController | null = null

const checkLabels: Record<ConnectionCheck['id'], string> = {
  resolve: '解析主机地址',
  connect: '连接 FTP 服务',
  authenticate: '验证登录信息',
  directory: '读取基础目录',
}

const target = computed(() => props.profile ? `${props.profile.host}:${props.profile.port}` : '')
const confirmButton = computed(() => ({
  content: result.value || requestError.value ? '重新检测' : '正在检测',
  loading: running.value,
  disabled: running.value || !props.profile,
}))

function formatDuration(milliseconds: number) {
  if (milliseconds < 1) return '不足 1 ms'
  if (milliseconds < 1000) return `${milliseconds} ms`
  return `${(milliseconds / 1000).toFixed(1)} s`
}

async function run() {
  const profile = props.profile
  if (!profile || running.value) return
  controller?.abort()
  const activeController = new globalThis.AbortController()
  controller = activeController
  running.value = true
  result.value = null
  requestError.value = ''
  try {
    const response = await profiles.test(profile.id, activeController.signal)
    if (controller === activeController) result.value = response
  } catch (error) {
    const isAbortError = error instanceof Error && error.name === 'AbortError'
    if (controller === activeController && !isAbortError) {
      requestError.value = error instanceof Error ? error.message : String(error)
    }
  } finally {
    if (controller === activeController) {
      running.value = false
      controller = null
    }
  }
}

watch(visible, (isVisible) => {
  if (isVisible) void run()
  else {
    controller?.abort()
    controller = null
    running.value = false
  }
}, { immediate: true })

onBeforeUnmount(() => controller?.abort())
</script>

<template>
  <t-dialog
    v-model:visible="visible"
    dialog-class-name="connection-test-dialog"
    header="测试 PS5 连接"
    width="620px"
    :confirm-btn="confirmButton"
    :cancel-btn="{ content: '关闭' }"
    @confirm="run"
  >
    <div v-if="profile" class="connection-test" aria-live="polite">
      <div class="connection-target">
        <span class="connection-target-icon"><Server :size="17" /></span>
        <div><strong>{{ profile.name }}</strong><small>{{ target }} · {{ profile.preset }} · {{ profile.base_path }}</small></div>
      </div>

      <div v-if="running" class="connection-testing-state">
        <LoaderCircle :size="24" class="spin" />
        <strong>正在检测连接</strong>
        <span>依次验证地址、FTP 服务、登录信息和基础目录，最长等待 15 秒。</span>
      </div>

      <template v-else-if="result">
        <div class="connection-result" :class="result.ok ? 'is-success' : 'is-failed'">
          <CheckCircle2 v-if="result.ok" :size="22" />
          <XCircle v-else :size="22" />
          <div>
            <strong>{{ result.ok ? '连接成功' : '连接检测未通过' }}</strong>
            <span>{{ result.ok ? 'PS5 FTP 已可用于文件浏览和传输。' : '已定位失败阶段，请按下方建议检查。' }}</span>
          </div>
          <small>{{ formatDuration(result.duration_ms) }}</small>
        </div>

        <ol class="connection-checks">
          <li v-for="check in result.checks" :key="check.id" :class="`is-${check.status}`">
            <CheckCircle2 v-if="check.status === 'passed'" :size="17" />
            <XCircle v-else-if="check.status === 'failed'" :size="17" />
            <CircleMinus v-else :size="17" />
            <div><strong>{{ checkLabels[check.id] }}</strong><span>{{ check.detail || (check.status === 'skipped' ? '因前序检测失败而跳过' : '等待检测') }}</span></div>
            <small v-if="check.status === 'passed' || check.status === 'failed'">{{ formatDuration(check.duration_ms) }}</small>
          </li>
        </ol>

        <section v-if="result.failure" class="connection-failure">
          <header><strong>{{ result.failure.title }}</strong><span class="failure-code">{{ result.failure.code }}</span></header>
          <p>{{ result.failure.message }}</p>
          <h4>建议检查</h4>
          <ul><li v-for="suggestion in result.failure.suggestions" :key="suggestion">{{ suggestion }}</li></ul>
          <details v-if="result.failure.detail">
            <summary>技术详情</summary>
            <code>{{ result.failure.detail }}</code>
          </details>
        </section>
      </template>

      <section v-else-if="requestError" class="connection-failure">
        <header><strong>无法启动连接检测</strong><span class="failure-code">request_failed</span></header>
        <p>应用未能取得检测结果，请确认服务运行正常后重试。</p>
        <details open><summary>技术详情</summary><code>{{ requestError }}</code></details>
      </section>
    </div>
  </t-dialog>
</template>

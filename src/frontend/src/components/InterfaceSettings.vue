<script setup lang="ts">
import { Type } from '@lucide/vue'
import { useUiPreferencesStore, type UiButtonSize, type UiFontSize } from '../stores/uiPreferences'

const ui = useUiPreferencesStore()

const fontOptions: { value: UiFontSize; label: string; detail: string }[] = [
  { value: 'small', label: '小号', detail: '更紧凑' },
  { value: 'standard', label: '标准', detail: '默认' },
  { value: 'large', label: '大号', detail: '更易读' },
]

const buttonOptions: { value: UiButtonSize; label: string; detail: string }[] = [
  { value: 'compact', label: '紧凑', detail: '28 px' },
  { value: 'standard', label: '标准', detail: '32 px' },
  { value: 'large', label: '宽松', detail: '36 px' },
]
</script>

<template>
  <section class="settings-section interface-settings">
    <header>
      <h2>界面设置</h2>
      <p>统一调整全局文字层级和标准操作按钮</p>
    </header>
    <div class="settings-body interface-settings-body">
      <div class="interface-setting-row">
        <div class="interface-setting-copy">
          <span class="interface-setting-icon"><Type :size="17" /></span>
          <div><strong>字体大小</strong><small>页面标题、正文、辅助文字和组件文字会按比例调整</small></div>
        </div>
        <div class="interface-options" role="group" aria-label="字体大小">
          <button v-for="option in fontOptions" :key="option.value" type="button" :class="['interface-option', { 'is-selected': ui.fontSize === option.value }]" :aria-pressed="ui.fontSize === option.value" @click="ui.setFontSize(option.value)">
            <strong>{{ option.label }}</strong><small>{{ option.detail }}</small>
          </button>
        </div>
      </div>

      <div class="interface-setting-row">
        <div class="interface-setting-copy">
          <span class="interface-setting-icon interface-button-icon"><i /><i /></span>
          <div><strong>按钮大小</strong><small>统一标准操作按钮的高度和横向留白</small></div>
        </div>
        <div class="interface-options" role="group" aria-label="按钮大小">
          <button v-for="option in buttonOptions" :key="option.value" type="button" :class="['interface-option', { 'is-selected': ui.buttonSize === option.value }]" :aria-pressed="ui.buttonSize === option.value" @click="ui.setButtonSize(option.value)">
            <strong>{{ option.label }}</strong><small>{{ option.detail }}</small>
          </button>
        </div>
      </div>

      <p class="interface-settings-note">修改会立即应用，并保存在当前浏览器中。</p>
    </div>
  </section>
</template>

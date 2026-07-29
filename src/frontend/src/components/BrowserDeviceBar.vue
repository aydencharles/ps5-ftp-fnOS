<script setup lang="ts">
defineProps<{
  title: string
  subtitle: string
}>()
</script>

<template>
  <header class="browser-devicebar">
    <div class="browser-devicebar-context">
      <span class="browser-devicebar-icon"><slot name="icon" /></span>
      <div class="browser-devicebar-copy">
        <strong>{{ title }}</strong>
        <small>{{ subtitle }}</small>
      </div>
    </div>
    <div class="browser-devicebar-control"><slot name="control" /></div>
  </header>
</template>

<template>
  <iframe
    :srcdoc="htmlContent"
    sandbox="allow-scripts allow-same-origin"
    class="html-preview"
  />
</template>

<script setup lang="ts">
defineProps<{
  htmlContent: string
}>()
</script>

<style scoped>
.html-preview {
  flex: 1;
  border: none;
  width: 100%;
  height: 100%;
  background: #fff;
}
</style>

<template>
  <div class="binary-preview">
    <n-icon :size="64" depth="3"><DocumentOutline /></n-icon>
    <n-text style="font-size: 16px; margin-top: 12px">{{ fileName }}</n-text>
    <n-text depth="3" style="margin-top: 4px">{{ formatBytes(fileSize) }}</n-text>
    <n-button style="margin-top: 16px" @click="$emit('download')">
      <template #icon><n-icon><DownloadOutline /></n-icon></template>
      Download File
    </n-button>
  </div>
</template>

<script setup lang="ts">
import { NIcon, NText, NButton } from 'naive-ui'
import { DocumentOutline, DownloadOutline } from '@vicons/ionicons5'

defineProps<{
  fileName: string
  fileSize: number
}>()

defineEmits<{
  download: []
}>()

const formatBytes = (bytes: number): string => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}
</script>

<style scoped>
.binary-preview {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
}
</style>

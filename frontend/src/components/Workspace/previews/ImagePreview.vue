<template>
  <div class="image-preview">
    <img :src="blobUrl" :alt="fileName" />
  </div>
</template>

<script setup lang="ts">
defineProps<{
  blobUrl: string
  fileName: string
}>()
</script>

<style scoped>
.image-preview {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  overflow: auto;
}

.image-preview img {
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
  border-radius: 4px;
}
</style>

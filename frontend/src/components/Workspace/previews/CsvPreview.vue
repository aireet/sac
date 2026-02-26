<template>
  <div class="csv-preview">
    <n-data-table
      :columns="columns"
      :data="data"
      :scroll-x="columns.length * 150"
      :max-height="tableHeight"
      virtual-scroll
      size="small"
      striped
      bordered
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { NDataTable } from 'naive-ui'

defineProps<{
  columns: Array<{ title: string; key: string }>
  data: Array<Record<string, string | number>>
}>()

const tableHeight = computed(() => window.innerHeight - 140)
</script>

<style scoped>
.csv-preview {
  flex: 1;
  overflow: auto;
  padding: 8px;
}
</style>

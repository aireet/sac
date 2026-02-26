<template>
  <div class="editor-overlay">
    <div v-if="!hideHeader" class="editor-header">
      <n-breadcrumb separator="/">
        <n-breadcrumb-item @click="$emit('close')">
          <n-icon :size="14"><ArrowBackOutline /></n-icon>
          <span style="margin-left: 4px">Back</span>
        </n-breadcrumb-item>
        <n-breadcrumb-item v-for="(seg, i) in pathSegments" :key="i">
          {{ seg }}
        </n-breadcrumb-item>
      </n-breadcrumb>
      <n-space :size="8" align="center">
        <n-tag v-if="dirty" size="small" type="warning">Unsaved</n-tag>
        <n-button
          v-if="category === 'text' && canSave"
          size="small"
          type="primary"
          :disabled="!dirty"
          :loading="saving"
          @click="$emit('save')"
        >
          Save
        </n-button>
        <n-button size="small" quaternary @click="$emit('download')">
          <template #icon><n-icon :size="14"><DownloadOutline /></n-icon></template>
          Download
        </n-button>
        <n-button size="small" quaternary @click="$emit('close')">Close</n-button>
      </n-space>
    </div>
    <div class="editor-body">
      <n-spin :show="loading" style="height: 100%">
        <TextPreview
          v-if="category === 'text'"
          :content="content"
          :can-save="canSave"
          @update:content="$emit('update:content', $event)"
        />
        <CsvPreview
          v-else-if="category === 'csv'"
          :columns="csvColumns"
          :data="csvData"
        />
        <HtmlPreview
          v-else-if="category === 'html'"
          :html-content="content"
        />
        <ImagePreview
          v-else-if="category === 'image'"
          :blob-url="blobUrl"
          :file-name="file.name"
        />
        <BinaryPreview
          v-else
          :file-name="file.name"
          :file-size="file.size"
          @download="$emit('download')"
        />
      </n-spin>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { NBreadcrumb, NBreadcrumbItem, NSpace, NTag, NButton, NIcon, NSpin } from 'naive-ui'
import { ArrowBackOutline, DownloadOutline } from '@vicons/ionicons5'
import type { FileCategory } from '../../utils/fileTypes'
import type { WorkspaceFile } from '../../services/workspaceAPI'
import TextPreview from './previews/TextPreview.vue'
import CsvPreview from './previews/CsvPreview.vue'
import HtmlPreview from './previews/HtmlPreview.vue'
import ImagePreview from './previews/ImagePreview.vue'
import BinaryPreview from './previews/BinaryPreview.vue'

const props = defineProps<{
  file: WorkspaceFile
  category: FileCategory
  content: string
  blobUrl: string
  loading: boolean
  saving: boolean
  dirty: boolean
  canSave: boolean
  csvColumns: Array<{ title: string; key: string }>
  csvData: Array<Record<string, string | number>>
  hideHeader?: boolean
}>()

defineEmits<{
  close: []
  download: []
  save: []
  'update:content': [value: string]
}>()

const pathSegments = computed(() => props.file.path.replace(/^\//, '').split('/'))
</script>

<style scoped>
.editor-overlay {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}

.editor-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 16px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  flex-shrink: 0;
}

.editor-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.editor-body :deep(.n-spin-container),
.editor-body :deep(.n-spin-content) {
  height: 100%;
  display: flex;
  flex-direction: column;
}
</style>

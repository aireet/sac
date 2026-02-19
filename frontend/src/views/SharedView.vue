<template>
  <n-config-provider :theme="darkTheme">
    <n-message-provider>
      <div class="shared-page">
        <!-- Loading -->
        <div v-if="loading" class="shared-center">
          <n-spin size="large" />
          <n-text depth="3" style="margin-top: 16px">Loading shared file...</n-text>
        </div>

        <!-- Error -->
        <div v-else-if="error" class="shared-center">
          <n-result status="404" title="Link not found" description="This link may have been deleted or never existed." />
        </div>

        <!-- File Preview -->
        <template v-else>
          <div class="shared-header">
            <n-text strong style="font-size: 16px">{{ meta?.file_name }}</n-text>
            <n-space :size="8" align="center">
              <n-text depth="3" style="font-size: 12px">{{ formatBytes(meta?.size_bytes ?? 0) }}</n-text>
              <n-button size="small" @click="handleDownload">
                <template #icon><n-icon :size="14"><DownloadOutline /></n-icon></template>
                Download
              </n-button>
            </n-space>
          </div>
          <div class="shared-body">
            <FilePreview
              :file="previewFile"
              :category="category"
              :content="content"
              :blob-url="blobUrl"
              :loading="false"
              :saving="false"
              :dirty="false"
              :can-save="false"
              :csv-columns="csvColumns"
              :csv-data="csvData"
              :hide-header="true"
              @download="handleDownload"
              @close="handleDownload"
            />
          </div>
        </template>
      </div>
    </n-message-provider>
  </n-config-provider>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { NConfigProvider, NMessageProvider, NSpin, NText, NResult, NButton, NIcon, NSpace, darkTheme } from 'naive-ui'
import { DownloadOutline } from '@vicons/ionicons5'
import FilePreview from '../components/Workspace/FilePreview.vue'
import { getSharedFile, fetchSharedFileBlob, type SharedFileMeta } from '../services/workspaceAPI'
import type { WorkspaceFile } from '../services/workspaceAPI'
import { getFileCategory, type FileCategory, MAX_TEXT_PREVIEW_BYTES, MAX_CSV_PREVIEW_BYTES, MAX_CSV_PREVIEW_ROWS, MAX_IMAGE_PREVIEW_BYTES } from '../utils/fileTypes'

const route = useRoute()
const code = route.params.code as string

const loading = ref(true)
const error = ref(false)
const meta = ref<SharedFileMeta | null>(null)
const category = ref<FileCategory>('binary')
const content = ref('')
const blobUrl = ref('')
const csvColumns = ref<Array<{ title: string; key: string }>>([])
const csvData = ref<Array<Record<string, string>>>([])

const previewFile = ref<WorkspaceFile>({
  name: '',
  path: '',
  size: 0,
  is_directory: false,
})

const formatBytes = (bytes: number): string => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

const parseCsv = (text: string, isTsv = false) => {
  const sep = isTsv ? '\t' : ','
  const lines = text.split('\n').filter(l => l.trim())
  if (lines.length === 0) return { columns: [], data: [] }
  const headers = lines[0]!.split(sep).map(h => h.trim().replace(/^"|"$/g, ''))
  const columns = headers.map((h, i) => ({
    title: h || `col_${i}`,
    key: `c${i}`,
  }))
  const data = lines.slice(1).map((line, rowIdx) => {
    const cells = line.split(sep).map(c => c.trim().replace(/^"|"$/g, ''))
    const row: Record<string, string | number> = { _key: rowIdx }
    headers.forEach((_, i) => { row[`c${i}`] = cells[i] ?? '' })
    return row
  })
  return { columns, data }
}

const handleDownload = () => {
  if (!meta.value) return
  const baseUrl = import.meta.env.VITE_API_URL
    || (window.location.hostname === 'localhost' || window.location.hostname === '127.0.0.1'
      ? `${window.location.protocol}//${window.location.hostname}:8080/api`
      : `${window.location.protocol}//${window.location.host}/api`)
  const url = `${baseUrl}/s/${code}/raw`
  const a = document.createElement('a')
  a.href = url
  a.download = meta.value.file_name
  document.body.appendChild(a)
  a.click()
  a.remove()
}

onMounted(async () => {
  try {
    meta.value = await getSharedFile(code)
    previewFile.value = {
      name: meta.value.file_name,
      path: meta.value.file_name,
      size: meta.value.size_bytes,
      is_directory: false,
    }
    category.value = getFileCategory(meta.value.file_name)

    const blob = await fetchSharedFileBlob(code)

    if (category.value === 'text') {
      if (meta.value.size_bytes > MAX_TEXT_PREVIEW_BYTES) {
        category.value = 'binary'
      } else {
        content.value = await blob.text()
      }
    } else if (category.value === 'csv') {
      if (meta.value.size_bytes > MAX_CSV_PREVIEW_BYTES) {
        category.value = 'binary'
      } else {
        const text = await blob.text()
        content.value = text
        const parsed = parseCsv(text, meta.value.file_name.endsWith('.tsv'))
        csvColumns.value = parsed.columns
        csvData.value = parsed.data.length > MAX_CSV_PREVIEW_ROWS
          ? parsed.data.slice(0, MAX_CSV_PREVIEW_ROWS)
          : parsed.data
      }
    } else if (category.value === 'html') {
      content.value = await blob.text()
    } else if (category.value === 'image') {
      if (meta.value.size_bytes > MAX_IMAGE_PREVIEW_BYTES) {
        category.value = 'binary'
      } else {
        blobUrl.value = URL.createObjectURL(blob)
      }
    }
  } catch {
    error.value = true
  } finally {
    loading.value = false
  }
})

onUnmounted(() => {
  if (blobUrl.value) {
    URL.revokeObjectURL(blobUrl.value)
  }
})
</script>

<style scoped>
.shared-page {
  height: 100vh;
  display: flex;
  flex-direction: column;
  background: #1e1e2e;
  color: #e0e0e0;
}

.shared-center {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
}

.shared-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 24px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  flex-shrink: 0;
}

.shared-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}
</style>

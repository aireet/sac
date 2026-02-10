<template>
  <div class="workspace-panel">
    <!-- Tab: Private / Public -->
    <n-tabs v-model:value="spaceTab" type="line" size="small" style="margin-bottom: 8px">
      <n-tab-pane name="private" tab="Private">
        <template #tab>
          <n-space :size="4" align="center">
            <n-icon size="14"><LockClosedOutline /></n-icon>
            <span>Private</span>
          </n-space>
        </template>
      </n-tab-pane>
      <n-tab-pane v-if="isAdmin" name="public" tab="Public">
        <template #tab>
          <n-space :size="4" align="center">
            <n-icon size="14"><GlobeOutline /></n-icon>
            <span>Public</span>
          </n-space>
        </template>
      </n-tab-pane>
    </n-tabs>

    <!-- Quota bar (private only) -->
    <div v-if="spaceTab === 'private' && quota" class="quota-bar">
      <n-progress
        type="line"
        :percentage="quotaPercent"
        :status="quotaPercent > 90 ? 'error' : quotaPercent > 70 ? 'warning' : 'success'"
        :show-indicator="false"
        :height="4"
      />
      <n-text depth="3" style="font-size: 11px">
        {{ formatBytes(quota.used_bytes) }} / {{ formatBytes(quota.max_bytes) }}
        ({{ quota.file_count }} / {{ quota.max_file_count }} files)
      </n-text>
    </div>

    <!-- Path hint -->
    <n-text depth="3" style="font-size: 11px; display: block; margin-bottom: 4px">
      {{ spaceTab === 'private' ? 'Agent: /workspace/private/' : 'Agent: /workspace/public/ (read-only)' }}
    </n-text>

    <!-- Breadcrumb navigation -->
    <n-breadcrumb style="margin: 8px 0" separator="/">
      <n-breadcrumb-item @click="navigateTo('')">
        <n-icon><HomeOutline /></n-icon>
      </n-breadcrumb-item>
      <n-breadcrumb-item
        v-for="(crumb, idx) in breadcrumbs"
        :key="idx"
        @click="navigateTo(crumb.path)"
      >
        {{ crumb.name }}
      </n-breadcrumb-item>
    </n-breadcrumb>

    <!-- Actions toolbar -->
    <n-space style="margin-bottom: 8px" :size="8">
      <n-upload
        :show-file-list="false"
        :custom-request="handleUpload"
        :multiple="true"
        :disabled="spaceTab === 'public' && !isAdmin"
      >
        <n-button size="small" type="primary">
          <template #icon><n-icon><CloudUploadOutline /></n-icon></template>
          Upload
        </n-button>
      </n-upload>
      <n-button size="small" @click="showNewFolder = true" :disabled="spaceTab === 'public' && !isAdmin">
        <template #icon><n-icon><FolderOpenOutline /></n-icon></template>
        New Folder
      </n-button>
      <n-button size="small" quaternary @click="loadFiles">
        <template #icon><n-icon><RefreshOutline /></n-icon></template>
      </n-button>
    </n-space>

    <!-- File list -->
    <n-spin :show="loading">
      <div class="file-list">
        <n-empty v-if="!loading && files.length === 0" description="No files" size="small" />
        <div
          v-for="file in files"
          :key="file.path"
          class="file-item"
          @click="handleFileClick(file)"
        >
          <n-icon size="18" class="file-icon">
            <FolderOutline v-if="file.is_directory" />
            <DocumentOutline v-else />
          </n-icon>
          <div class="file-info">
            <n-text class="file-name">{{ file.name }}</n-text>
            <n-text depth="3" style="font-size: 11px" v-if="!file.is_directory">
              {{ formatBytes(file.size) }}
            </n-text>
          </div>
          <n-space :size="4" class="file-actions" @click.stop>
            <n-button
              v-if="!file.is_directory"
              size="tiny"
              quaternary
              circle
              @click="handleDownload(file)"
              title="Download"
            >
              <template #icon><n-icon><DownloadOutline /></n-icon></template>
            </n-button>
            <n-popconfirm
              @positive-click="handleDelete(file)"
              :positive-text="'Delete'"
              :negative-text="'Cancel'"
              v-if="spaceTab === 'private' || isAdmin"
            >
              <template #trigger>
                <n-button size="tiny" quaternary circle title="Delete">
                  <template #icon><n-icon><TrashOutline /></n-icon></template>
                </n-button>
              </template>
              Delete "{{ file.name }}"?
            </n-popconfirm>
          </n-space>
        </div>
      </div>
    </n-spin>

    <!-- New folder dialog -->
    <n-modal v-model:show="showNewFolder" preset="dialog" title="New Folder" :positive-text="'Create'" :negative-text="'Cancel'" @positive-click="handleCreateFolder">
      <n-input v-model:value="newFolderName" placeholder="Folder name" @keyup.enter="handleCreateFolder" />
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import {
  NTabs, NTabPane, NSpace, NIcon, NText, NButton, NUpload, NSpin, NEmpty,
  NBreadcrumb, NBreadcrumbItem, NProgress, NPopconfirm, NModal, NInput,
  useMessage, type UploadCustomRequestOptions,
} from 'naive-ui'
import {
  LockClosedOutline, GlobeOutline, HomeOutline, CloudUploadOutline,
  FolderOpenOutline, RefreshOutline, FolderOutline, DocumentOutline,
  DownloadOutline, TrashOutline,
} from '@vicons/ionicons5'
import {
  listFiles, uploadFile, downloadFile, deleteFile, createDirectory, getQuota,
  listPublicFiles, uploadPublicFile, downloadPublicFile, deletePublicFile, createPublicDirectory,
  type WorkspaceFile, type WorkspaceQuota,
} from '../../services/workspaceAPI'
import { useAuthStore } from '../../stores/auth'

const props = defineProps<{
  agentId: number
}>()

const message = useMessage()
const authStore = useAuthStore()

const spaceTab = ref<'private' | 'public'>('private')
const currentPath = ref('')
const files = ref<WorkspaceFile[]>([])
const loading = ref(false)
const quota = ref<WorkspaceQuota | null>(null)

const showNewFolder = ref(false)
const newFolderName = ref('')

const isAdmin = computed(() => authStore.isAdmin)

const quotaPercent = computed(() => {
  if (!quota.value || quota.value.max_bytes === 0) return 0
  return Math.round((quota.value.used_bytes / quota.value.max_bytes) * 100)
})

const breadcrumbs = computed(() => {
  if (!currentPath.value) return []
  const parts = currentPath.value.replace(/\/$/, '').split('/')
  return parts.map((name, idx) => ({
    name,
    path: parts.slice(0, idx + 1).join('/') + '/',
  }))
})

const navigateTo = (path: string) => {
  currentPath.value = path
}

const loadFiles = async () => {
  loading.value = true
  try {
    const result = spaceTab.value === 'private'
      ? await listFiles(props.agentId, currentPath.value || '/')
      : await listPublicFiles(currentPath.value || '/')
    files.value = result.files || []
  } catch (err) {
    console.error('Failed to load files:', err)
    message.error('Failed to load files')
    files.value = []
  } finally {
    loading.value = false
  }
}

const loadQuota = async () => {
  try {
    quota.value = await getQuota(props.agentId)
  } catch {
    // Ignore quota fetch errors
  }
}

const handleFileClick = (file: WorkspaceFile) => {
  if (file.is_directory) {
    currentPath.value = file.path
  }
}

const handleUpload = async ({ file, onFinish, onError }: UploadCustomRequestOptions) => {
  if (!file.file) return
  try {
    if (spaceTab.value === 'private') {
      await uploadFile(props.agentId, file.file, currentPath.value || '/')
    } else {
      await uploadPublicFile(file.file, currentPath.value || '/')
    }
    message.success(`Uploaded ${file.name}`)
    onFinish()
    loadFiles()
    if (spaceTab.value === 'private') loadQuota()
  } catch (err) {
    console.error('Upload failed:', err)
    message.error(`Failed to upload ${file.name}`)
    onError()
  }
}

const handleDownload = (file: WorkspaceFile) => {
  if (spaceTab.value === 'private') {
    downloadFile(props.agentId, file.path)
  } else {
    downloadPublicFile(file.path)
  }
}

const handleDelete = async (file: WorkspaceFile) => {
  try {
    const deletePath = file.is_directory ? file.path + '/' : file.path
    if (spaceTab.value === 'private') {
      await deleteFile(props.agentId, deletePath)
    } else {
      await deletePublicFile(deletePath)
    }
    message.success(`Deleted ${file.name}`)
    loadFiles()
    if (spaceTab.value === 'private') loadQuota()
  } catch (err) {
    console.error('Delete failed:', err)
    message.error(`Failed to delete ${file.name}`)
  }
}

const handleCreateFolder = async () => {
  if (!newFolderName.value.trim()) return
  const folderPath = (currentPath.value || '') + newFolderName.value.trim()
  try {
    if (spaceTab.value === 'private') {
      await createDirectory(props.agentId, folderPath)
    } else {
      await createPublicDirectory(folderPath)
    }
    message.success(`Created folder "${newFolderName.value}"`)
    newFolderName.value = ''
    showNewFolder.value = false
    loadFiles()
  } catch (err) {
    console.error('Create folder failed:', err)
    message.error('Failed to create folder')
  }
}

const formatBytes = (bytes: number): string => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

// Reload when tab or path changes
watch([spaceTab, currentPath], () => {
  loadFiles()
})

// Reload when agentId changes
watch(() => props.agentId, () => {
  currentPath.value = ''
  loadFiles()
  loadQuota()
})

onMounted(() => {
  loadFiles()
  loadQuota()
})
</script>

<style scoped>
.workspace-panel {
  padding: 0 12px 12px;
}

.quota-bar {
  margin-bottom: 8px;
}

.file-list {
  min-height: 200px;
}

.file-item {
  display: flex;
  align-items: center;
  padding: 8px;
  border-radius: 6px;
  cursor: pointer;
  gap: 8px;
  transition: background-color 0.15s;
}

.file-item:hover {
  background-color: rgba(255, 255, 255, 0.06);
}

.file-icon {
  flex-shrink: 0;
  color: #888;
}

.file-info {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.file-name {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  font-size: 13px;
}

.file-actions {
  flex-shrink: 0;
  opacity: 0;
  transition: opacity 0.15s;
}

.file-item:hover .file-actions {
  opacity: 1;
}
</style>

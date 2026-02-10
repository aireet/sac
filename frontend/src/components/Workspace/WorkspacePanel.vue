<template>
  <div class="workspace-panel">
    <!-- Header: Tabs + Quota -->
    <div class="ws-header">
      <n-tabs v-model:value="spaceTab" type="line" size="small">
        <n-tab-pane name="private">
          <template #tab>
            <n-space :size="4" align="center">
              <n-icon size="14"><LockClosedOutline /></n-icon>
              <span>Private</span>
            </n-space>
          </template>
        </n-tab-pane>
        <n-tab-pane v-if="isAdmin" name="public">
          <template #tab>
            <n-space :size="4" align="center">
              <n-icon size="14"><GlobeOutline /></n-icon>
              <span>Public</span>
            </n-space>
          </template>
        </n-tab-pane>
      </n-tabs>
      <div v-if="spaceTab === 'private' && quota" class="ws-quota">
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
    </div>

    <!-- Toolbar -->
    <div class="ws-toolbar">
      <n-input v-model:value="searchQuery" placeholder="Search files..." size="small" clearable style="flex: 1" />
      <n-space :size="4" style="flex-shrink: 0">
        <n-upload
          :show-file-list="false"
          :custom-request="handleUpload"
          :multiple="true"
          :disabled="spaceTab === 'public' && !isAdmin"
        >
          <n-button size="small" type="primary">Upload</n-button>
        </n-upload>
        <n-button size="small" @click="showNewFile = true" :disabled="spaceTab === 'public' && !isAdmin">
          New File
        </n-button>
        <n-button size="small" @click="showNewFolder = true" :disabled="spaceTab === 'public' && !isAdmin">
          New Folder
        </n-button>
        <n-button size="small" quaternary @click="refreshTree">
          <template #icon><n-icon><RefreshOutline /></n-icon></template>
        </n-button>
      </n-space>
    </div>

    <!-- Active directory indicator -->
    <div v-if="activeDir !== '/'" class="ws-active-dir">
      <n-icon :size="12" style="flex-shrink: 0"><FolderOutline /></n-icon>
      <n-text depth="3" style="font-size: 11px; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap">
        {{ activeDir }}
      </n-text>
      <n-button size="tiny" quaternary @click="activeDir = '/'">
        <n-icon :size="12"><CloseOutline /></n-icon>
      </n-button>
    </div>

    <!-- Batch bar -->
    <div v-if="checkedKeys.length > 0" class="ws-batch-bar">
      <n-text depth="3" style="font-size: 12px">{{ checkedKeys.length }} selected</n-text>
      <n-button size="tiny" @click="handleBatchDownload">Download</n-button>
      <n-popconfirm @positive-click="handleBatchDelete">
        <template #trigger>
          <n-button size="tiny" type="error">Delete</n-button>
        </template>
        Delete {{ checkedKeys.length }} items?
      </n-popconfirm>
      <n-button size="tiny" quaternary @click="checkedKeys = []">Clear</n-button>
    </div>

    <!-- Body: Tree -->
    <div class="ws-body">
      <div
        class="ws-content"
        :class="{ 'drag-over': dragOver }"
        @dragover.prevent
        @dragenter.prevent="onDragEnter"
        @dragleave="onDragLeave"
        @drop.prevent="handleDrop"
      >
        <n-spin :show="rootLoading">
          <n-tree
            v-if="treeData.length > 0 || rootLoading"
            block-line
            :data="treeData"
            :pattern="searchQuery"
            :filter="treeFilter"
            :selectable="false"
            :checkable="true"
            :cascade="false"
            :checked-keys="checkedKeys"
            :expanded-keys="expandedKeys"
            :on-load="handleTreeLoad"
            :render-prefix="renderPrefix"
            :render-suffix="renderSuffix"
            :node-props="getNodeProps"
            @update:checked-keys="handleCheckedKeysUpdate"
            @update:expanded-keys="handleExpandedKeysUpdate"
          />
          <n-empty v-else-if="!rootLoading" description="No files" style="margin-top: 40px" />
        </n-spin>
      </div>
    </div>

    <!-- New folder dialog -->
    <n-modal
      v-model:show="showNewFolder"
      preset="dialog"
      title="New Folder"
      positive-text="Create"
      negative-text="Cancel"
      @positive-click="handleCreateFolder"
    >
      <n-input v-model:value="newFolderName" placeholder="Folder name" @keyup.enter="handleCreateFolder" />
    </n-modal>

    <!-- New file dialog -->
    <n-modal
      v-model:show="showNewFile"
      preset="dialog"
      title="New File"
      positive-text="Create"
      negative-text="Cancel"
      @positive-click="handleCreateFile"
    >
      <n-input v-model:value="newFileName" placeholder="File name (e.g. notes.txt)" @keyup.enter="handleCreateFile" />
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, h, type Component } from 'vue'
import {
  NTabs, NTabPane, NSpace, NIcon, NText, NButton, NUpload, NSpin,
  NEmpty, NPopconfirm, NModal, NInput, NProgress, NTree,
  useMessage, useDialog,
  type UploadCustomRequestOptions, type TreeOption,
} from 'naive-ui'
import {
  LockClosedOutline, GlobeOutline, RefreshOutline, DocumentOutline,
  DownloadOutline, TrashOutline, FolderOutline, CodeSlashOutline,
  DocumentTextOutline, ImageOutline, SettingsOutline, CloseOutline,
} from '@vicons/ionicons5'
import {
  listFiles, uploadFile, downloadFile, deleteFile, createDirectory, getQuota,
  listPublicFiles, uploadPublicFile, downloadPublicFile, deletePublicFile,
  createPublicDirectory,
  type WorkspaceFile, type WorkspaceQuota,
} from '../../services/workspaceAPI'
import { getFileIcon } from '../../utils/fileTypes'
import { useAuthStore } from '../../stores/auth'

const props = defineProps<{
  agentId: number
}>()

const emit = defineEmits<{
  openFile: [file: WorkspaceFile, spaceTab: 'private' | 'public']
}>()

const message = useMessage()
const dialog = useDialog()
const authStore = useAuthStore()

// --- State ---
const spaceTab = ref<'private' | 'public'>('private')
const treeData = ref<TreeOption[]>([])
const expandedKeys = ref<string[]>([])
const checkedKeys = ref<string[]>([])
const activeDir = ref('/')
const rootLoading = ref(false)
const quota = ref<WorkspaceQuota | null>(null)
const searchQuery = ref('')
const dragOver = ref(false)
const dragCounter = ref(0)
const showNewFolder = ref(false)
const newFolderName = ref('')
const showNewFile = ref(false)
const newFileName = ref('')

// --- Computed ---
const isAdmin = computed(() => authStore.isAdmin)
const canEdit = computed(() => spaceTab.value === 'private' || isAdmin.value)

const quotaPercent = computed(() => {
  if (!quota.value || quota.value.max_bytes === 0) return 0
  return Math.round((quota.value.used_bytes / quota.value.max_bytes) * 100)
})

// --- Icon mapping ---
const iconComponents: Record<string, Component> = {
  'code': CodeSlashOutline,
  'settings': SettingsOutline,
  'document-text': DocumentTextOutline,
  'image': ImageOutline,
  'document': DocumentOutline,
}

// --- Tree helpers ---
const buildNodes = (files: WorkspaceFile[]): TreeOption[] => {
  return files
    .sort((a, b) => {
      if (a.is_directory !== b.is_directory) return a.is_directory ? -1 : 1
      return a.name.localeCompare(b.name)
    })
    .map(f => ({
      key: f.path,
      label: f.name,
      isLeaf: !f.is_directory,
      file: f,
    }))
}

const findNode = (nodes: TreeOption[], key: string): TreeOption | null => {
  for (const node of nodes) {
    if (node.key === key) return node
    if (node.children) {
      const found = findNode(node.children, key)
      if (found) return found
    }
  }
  return null
}

const treeFilter = (pattern: string, node: TreeOption) => {
  return (node.label || '').toLowerCase().includes(pattern.toLowerCase())
}

// --- Tree render functions ---
const renderPrefix = ({ option }: { option: TreeOption }) => {
  const file = (option as any).file as WorkspaceFile
  if (file.is_directory) {
    return h(NIcon, { size: 18, color: '#e2b86b' }, () => h(FolderOutline))
  }
  const iconType = getFileIcon(file.name)
  const IconComp = iconComponents[iconType] || DocumentOutline
  return h(NIcon, { size: 18 }, () => h(IconComp))
}

const renderSuffix = ({ option }: { option: TreeOption }) => {
  const file = (option as any).file as WorkspaceFile
  const items: any[] = []

  if (!file.is_directory) {
    items.push(h('span', { class: 'tree-file-size' }, formatBytes(file.size)))
    items.push(
      h(NButton, {
        size: 'tiny', quaternary: true, circle: true, title: 'Download',
        onClick: (e: Event) => { e.stopPropagation(); handleDownloadFile(file) },
      }, { icon: () => h(NIcon, { size: 14 }, () => h(DownloadOutline)) })
    )
  }

  if (canEdit.value) {
    items.push(
      h(NButton, {
        size: 'tiny', quaternary: true, circle: true, type: 'error', title: 'Delete',
        onClick: (e: Event) => {
          e.stopPropagation()
          dialog.warning({
            title: 'Confirm',
            content: `Delete "${file.name}"?`,
            positiveText: 'Delete',
            negativeText: 'Cancel',
            onPositiveClick: () => handleDeleteFile(file),
          })
        },
      }, { icon: () => h(NIcon, { size: 14 }, () => h(TrashOutline)) })
    )
  }

  return h('div', { class: 'tree-suffix', onClick: (e: Event) => e.stopPropagation() }, items)
}

const getNodeProps = ({ option }: { option: TreeOption }) => {
  const file = (option as any).file as WorkspaceFile
  return {
    onClick: (e: MouseEvent) => {
      // If clicked on the expand arrow, let native handle it
      const target = e.target as HTMLElement
      if (target.closest('.n-tree-node-switcher')) return
      // If clicked on checkbox area, let native handle it
      if (target.closest('.n-tree-node-checkbox')) return

      if (file.is_directory) {
        activeDir.value = file.path
        // Toggle expand
        const key = file.path
        const idx = expandedKeys.value.indexOf(key)
        if (idx >= 0) {
          expandedKeys.value = expandedKeys.value.filter(k => k !== key)
        } else {
          expandedKeys.value = [...expandedKeys.value, key]
        }
      } else {
        emit('openFile', file, spaceTab.value)
      }
    },
  }
}

const handleExpandedKeysUpdate = (keys: string[]) => {
  // Track newly expanded dirs to set activeDir
  const newDirs = keys.filter(k => !expandedKeys.value.includes(k))
  expandedKeys.value = keys
  if (newDirs.length > 0) {
    activeDir.value = newDirs[newDirs.length - 1]
  }
}

const handleCheckedKeysUpdate = (keys: Array<string | number>) => {
  checkedKeys.value = keys as string[]
}

// --- Data loading ---
const loadRootFiles = async () => {
  rootLoading.value = true
  try {
    const result = spaceTab.value === 'private'
      ? await listFiles(props.agentId, '/')
      : await listPublicFiles('/')
    treeData.value = buildNodes(result.files || [])
  } catch (err) {
    console.error('Failed to load files:', err)
    message.error('Failed to load files')
    treeData.value = []
  } finally {
    rootLoading.value = false
  }
}

const handleTreeLoad = async (node: TreeOption): Promise<void> => {
  const path = node.key as string
  try {
    const result = spaceTab.value === 'private'
      ? await listFiles(props.agentId, path)
      : await listPublicFiles(path)
    node.children = buildNodes(result.files || [])
  } catch (err) {
    console.error('Failed to load directory:', err)
    node.children = []
  }
}

const reloadDir = async (dirPath: string) => {
  try {
    const result = spaceTab.value === 'private'
      ? await listFiles(props.agentId, dirPath)
      : await listPublicFiles(dirPath)
    const newNodes = buildNodes(result.files || [])

    if (dirPath === '/') {
      // Preserve children of previously expanded subdirs
      for (const newNode of newNodes) {
        const existing = treeData.value.find(n => n.key === newNode.key)
        if (existing?.children) {
          newNode.children = existing.children
        }
      }
      treeData.value = newNodes
    } else {
      const parentNode = findNode(treeData.value, dirPath)
      if (parentNode) {
        const oldChildren = parentNode.children || []
        for (const newNode of newNodes) {
          const existing = oldChildren.find(n => n.key === newNode.key)
          if (existing?.children) {
            newNode.children = existing.children
          }
        }
        parentNode.children = newNodes
        // Force reactivity by creating new root reference
        treeData.value = [...treeData.value]
      }
    }
  } catch (err) {
    console.error('Failed to reload dir:', err)
  }
}

const refreshTree = () => {
  expandedKeys.value = []
  checkedKeys.value = []
  activeDir.value = '/'
  searchQuery.value = ''
  loadRootFiles()
}

const loadQuota = async () => {
  try {
    quota.value = await getQuota(props.agentId)
  } catch {
    // ignore
  }
}

// --- File operations ---
const handleUpload = async ({ file, onFinish, onError }: UploadCustomRequestOptions) => {
  if (!file.file) return
  try {
    if (spaceTab.value === 'private') {
      await uploadFile(props.agentId, file.file, activeDir.value)
    } else {
      await uploadPublicFile(file.file, activeDir.value)
    }
    message.success(`Uploaded ${file.name}`)
    onFinish()
    await reloadDir(activeDir.value)
    if (spaceTab.value === 'private') loadQuota()
  } catch (err) {
    console.error('Upload failed:', err)
    message.error(`Failed to upload ${file.name}`)
    onError()
  }
}

const handleDownloadFile = (file: WorkspaceFile) => {
  if (spaceTab.value === 'private') {
    downloadFile(props.agentId, file.path)
  } else {
    downloadPublicFile(file.path)
  }
}

const handleDeleteFile = async (file: WorkspaceFile) => {
  try {
    const deletePath = file.is_directory ? file.path + '/' : file.path
    if (spaceTab.value === 'private') {
      await deleteFile(props.agentId, deletePath)
    } else {
      await deletePublicFile(deletePath)
    }
    message.success(`Deleted ${file.name}`)
    // Reload parent directory
    const parts = file.path.replace(/\/$/, '').split('/')
    parts.pop()
    const parentDir = parts.join('/') || '/'
    await reloadDir(parentDir)
    if (spaceTab.value === 'private') loadQuota()
  } catch (err) {
    console.error('Delete failed:', err)
    message.error(`Failed to delete ${file.name}`)
  }
}

const handleCreateFolder = async () => {
  if (!newFolderName.value.trim()) return
  const folderPath = activeDir.value === '/'
    ? newFolderName.value.trim()
    : activeDir.value.replace(/\/$/, '') + '/' + newFolderName.value.trim()
  try {
    if (spaceTab.value === 'private') {
      await createDirectory(props.agentId, folderPath)
    } else {
      await createPublicDirectory(folderPath)
    }
    message.success(`Created folder "${newFolderName.value}"`)
    newFolderName.value = ''
    showNewFolder.value = false
    await reloadDir(activeDir.value)
  } catch (err) {
    console.error('Create folder failed:', err)
    message.error('Failed to create folder')
  }
}

const handleCreateFile = async () => {
  const name = newFileName.value.trim()
  if (!name) return
  const emptyFile = new File([''], name, { type: 'text/plain' })
  try {
    if (spaceTab.value === 'private') {
      await uploadFile(props.agentId, emptyFile, activeDir.value)
    } else {
      await uploadPublicFile(emptyFile, activeDir.value)
    }
    message.success(`Created file "${name}"`)
    newFileName.value = ''
    showNewFile.value = false
    await reloadDir(activeDir.value)
    if (spaceTab.value === 'private') loadQuota()
    // Open the newly created file in editor
    // Path format must match API response: no leading slash, e.g. "notes.txt" or "subdir/notes.txt"
    const filePath = activeDir.value === '/'
      ? name
      : activeDir.value.replace(/\/$/, '') + '/' + name
    const newWsFile: WorkspaceFile = { name, path: filePath, size: 0, is_directory: false }
    emit('openFile', newWsFile, spaceTab.value)
  } catch (err) {
    console.error('Create file failed:', err)
    message.error('Failed to create file')
  }
}

// --- Drag & Drop ---
const onDragEnter = (e: DragEvent) => {
  e.preventDefault()
  dragCounter.value++
  dragOver.value = true
}

const onDragLeave = () => {
  dragCounter.value--
  if (dragCounter.value <= 0) {
    dragCounter.value = 0
    dragOver.value = false
  }
}

const handleDrop = async (e: DragEvent) => {
  dragCounter.value = 0
  dragOver.value = false
  const droppedFiles = e.dataTransfer?.files
  if (!droppedFiles?.length) return
  for (const f of droppedFiles) {
    try {
      if (spaceTab.value === 'private') {
        await uploadFile(props.agentId, f, activeDir.value)
      } else {
        await uploadPublicFile(f, activeDir.value)
      }
      message.success(`Uploaded ${f.name}`)
    } catch {
      message.error(`Failed to upload ${f.name}`)
    }
  }
  await reloadDir(activeDir.value)
  if (spaceTab.value === 'private') loadQuota()
}

// --- Batch operations ---
const handleBatchDownload = () => {
  for (const key of checkedKeys.value) {
    const node = findNode(treeData.value, key)
    const file = node ? (node as any).file as WorkspaceFile : null
    if (file && !file.is_directory) {
      handleDownloadFile(file)
    }
  }
  checkedKeys.value = []
}

const handleBatchDelete = async () => {
  for (const key of checkedKeys.value) {
    const node = findNode(treeData.value, key)
    const file = node ? (node as any).file as WorkspaceFile : null
    if (file) {
      await handleDeleteFile(file)
    }
  }
  checkedKeys.value = []
}

// --- Utilities ---
const formatBytes = (bytes: number): string => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

// --- Watchers ---
watch(spaceTab, () => {
  expandedKeys.value = []
  checkedKeys.value = []
  activeDir.value = '/'
  searchQuery.value = ''
  loadRootFiles()
})

watch(() => props.agentId, () => {
  expandedKeys.value = []
  checkedKeys.value = []
  activeDir.value = '/'
  searchQuery.value = ''
  loadRootFiles()
  loadQuota()
})

onMounted(() => {
  loadRootFiles()
  loadQuota()
})
</script>

<style scoped>
.workspace-panel {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.ws-header {
  padding: 12px 12px 0;
  flex-shrink: 0;
}

.ws-quota {
  margin-top: 4px;
}

.ws-toolbar {
  padding: 8px 12px;
  flex-shrink: 0;
  display: flex;
  gap: 8px;
  align-items: center;
}

.ws-active-dir {
  padding: 2px 12px 4px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 4px;
}

.ws-batch-bar {
  padding: 4px 12px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 8px;
  background: rgba(14, 165, 233, 0.08);
}

.ws-body {
  flex: 1;
  overflow-y: auto;
  padding: 0 8px 12px;
  min-height: 0;
}

.ws-content {
  min-height: 100%;
  border-radius: 8px;
  transition: border 0.2s, background 0.2s;
  border: 2px solid transparent;
}

.ws-content.drag-over {
  border: 2px dashed #0ea5e9;
  background: rgba(14, 165, 233, 0.04);
}

/* Tree node styles */
:deep(.n-tree-node) {
  border-radius: 4px !important;
}

:deep(.n-tree-node-content) {
  padding: 2px 4px !important;
}

:deep(.tree-suffix) {
  display: flex;
  align-items: center;
  gap: 2px;
  opacity: 0;
  transition: opacity 0.15s;
}

:deep(.n-tree-node:hover .tree-suffix) {
  opacity: 1;
}

:deep(.tree-file-size) {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.35);
  margin-right: 4px;
  white-space: nowrap;
}
</style>

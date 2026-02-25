<template>
  <div
    class="sidebar"
    :class="{ 'drop-active': dragOver }"
    @dragover.prevent="handleDragOver"
    @dragenter.prevent="handleDragEnter"
    @dragleave="handleDragLeave"
    @drop.prevent="handleDrop"
  >
    <div class="sidebar-header">
      <n-text depth="3" style="font-size: 11px; text-transform: uppercase; letter-spacing: 0.5px">Explorer</n-text>
    </div>

    <!-- Toolbar -->
    <div class="sidebar-toolbar">
      <n-input v-model:value="searchQuery" placeholder="Search files..." size="small" clearable style="flex: 1" />
    </div>

    <!-- Active directory indicator -->
    <div v-if="activeDir !== '/'" class="sidebar-active-dir">
      <n-icon :size="12" style="flex-shrink: 0"><FolderOutline /></n-icon>
      <n-text depth="3" style="font-size: 11px; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap">
        {{ activeDir }}
      </n-text>
      <n-button size="tiny" quaternary @click="activeDir = '/'">
        <n-icon :size="12"><CloseOutline /></n-icon>
      </n-button>
    </div>

    <!-- Tree body -->
    <div class="sidebar-body">
      <n-tree
        block-line
        :data="treeData"
        :pattern="searchQuery"
        :filter="treeFilter"
        :selectable="false"
        :expanded-keys="expandedKeys"
        :render-prefix="renderPrefix"
        :render-suffix="renderSuffix"
        :node-props="getNodeProps"
        @update:expanded-keys="handleExpandedKeysUpdate"
      />
    </div>

    <!-- Actions -->
    <div v-if="!readonly" class="sidebar-actions">
      <div class="action-row">
        <n-tooltip :disabled="!!skillId" trigger="hover">
          <template #trigger>
            <n-button size="small" :disabled="!skillId" style="flex: 1" @click="showNewFile = true">
              New File
            </n-button>
          </template>
          Save skill first
        </n-tooltip>
        <n-tooltip :disabled="!!skillId" trigger="hover">
          <template #trigger>
            <n-button size="small" :disabled="!skillId" style="flex: 1" @click="showNewFolder = true">
              New Folder
            </n-button>
          </template>
          Save skill first
        </n-tooltip>
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

    <!-- Drop overlay -->
    <div v-if="dragOver" class="drop-overlay">
      <n-text depth="3" style="font-size: 12px">Drop files or folders here</n-text>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, h, type Component } from 'vue'
import {
  NText, NButton, NTooltip, NInput, NIcon, NTree, NModal,
  type TreeOption,
} from 'naive-ui'
import {
  FolderOutline, DocumentOutline, CodeSlashOutline, SettingsOutline,
  DocumentTextOutline, ImageOutline, CloseOutline, TrashOutline,
} from '@vicons/ionicons5'
import type { SkillFile } from '../../services/skillAPI'

const props = defineProps<{
  files: SkillFile[]
  activeFile: string
  dirtyFiles: Set<string>
  skillId: number | null
  uploading: boolean
  commandName: string
  readonly?: boolean
}>()

const emit = defineEmits<{
  select: [filepath: string]
  deleteFile: [filepath: string]
  newFile: [filepath: string]
  upload: [file: File]
  uploadIn: [file: File, dirPath: string]
  uploadFolder: [files: { file: File; filepath: string }[]]
}>()

// --- State ---
const searchQuery = ref('')
const expandedKeys = ref<string[]>([])
const activeDir = ref('/')
const showNewFolder = ref(false)
const newFolderName = ref('')
const showNewFile = ref(false)
const newFileName = ref('')
const pendingDirs = ref(new Set<string>())
const collapsedDirs = ref(new Set<string>())

// --- Icon mapping ---
const iconComponents: Record<string, Component> = {
  'code': CodeSlashOutline,
  'settings': SettingsOutline,
  'document-text': DocumentTextOutline,
  'image': ImageOutline,
  'document': DocumentOutline,
}

function getFileIcon(name: string): string {
  const ext = name.split('.').pop()?.toLowerCase() ?? ''
  const codeExts = ['js', 'ts', 'jsx', 'tsx', 'vue', 'py', 'go', 'rs', 'java', 'rb', 'c', 'cpp', 'h', 'sh', 'bash', 'sql', 'lua', 'php', 'pl', 'kt']
  const configExts = ['json', 'yaml', 'yml', 'toml', 'ini', 'cfg', 'conf', 'env', 'xml']
  const docExts = ['md', 'txt', 'csv', 'log', 'diff', 'patch']
  const imgExts = ['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp', 'ico', 'svg']
  if (codeExts.includes(ext)) return 'code'
  if (configExts.includes(ext)) return 'settings'
  if (docExts.includes(ext)) return 'document-text'
  if (imgExts.includes(ext)) return 'image'
  return 'document'
}

// --- Build tree from flat file list ---
const treeData = computed(() => {
  const root: TreeOption[] = []
  const dirMap = new Map<string, TreeOption>()

  // SKILL.md is always first (virtual)
  root.push({
    key: 'SKILL.md',
    label: 'SKILL.md',
    isLeaf: true,
    _isVirtual: true,
  })

  // Ensure a directory node exists, creating parents as needed
  function ensureDir(dirPath: string): TreeOption {
    if (dirMap.has(dirPath)) return dirMap.get(dirPath)!
    const parts = dirPath.split('/')
    const name = parts[parts.length - 1]!
    const node: TreeOption = {
      key: '__dir__' + dirPath,
      label: name,
      isLeaf: false,
      children: [],
      _dirPath: dirPath,
    }
    dirMap.set(dirPath, node)

    if (parts.length === 1) {
      root.push(node)
    } else {
      const parentPath = parts.slice(0, -1).join('/')
      const parent = ensureDir(parentPath)
      parent.children!.push(node)
    }

    // Auto-expand new dirs (unless user manually collapsed them)
    if (!collapsedDirs.value.has(node.key as string) && !expandedKeys.value.includes(node.key as string)) {
      expandedKeys.value = [...expandedKeys.value, node.key as string]
    }
    return node
  }

  for (const f of props.files) {
    const parts = f.filepath.split('/')
    const fileName = parts[parts.length - 1]!
    const fileNode: TreeOption = {
      key: f.filepath,
      label: fileName,
      isLeaf: true,
      _file: f,
    }

    if (parts.length === 1) {
      root.push(fileNode)
    } else {
      const dirPath = parts.slice(0, -1).join('/')
      const dir = ensureDir(dirPath)
      dir.children!.push(fileNode)
    }
  }

  // Add pending (empty) directories that don't already exist from real files
  for (const dirPath of pendingDirs.value) {
    if (!dirMap.has(dirPath)) {
      ensureDir(dirPath)
    }
  }

  // Sort: dirs first, then files, alphabetically (skip SKILL.md which stays first)
  function sortNodes(nodes: TreeOption[]) {
    // Separate SKILL.md from the rest
    const skillMd = nodes.filter(n => n.key === 'SKILL.md')
    const rest = nodes.filter(n => n.key !== 'SKILL.md')
    rest.sort((a, b) => {
      if (a.isLeaf !== b.isLeaf) return a.isLeaf ? 1 : -1
      return (a.label || '').localeCompare(b.label || '')
    })
    for (const n of rest) {
      if (n.children) sortNodes(n.children)
    }
    nodes.length = 0
    nodes.push(...skillMd, ...rest)
  }
  sortNodes(root)
  return root
})

// --- Tree render functions ---
const renderPrefix = ({ option }: { option: TreeOption }) => {
  if (!option.isLeaf) {
    return h(NIcon, { size: 18, color: '#e2b86b' }, () => h(FolderOutline))
  }
  const iconType = getFileIcon(option.label || '')
  const IconComp = iconComponents[iconType] || DocumentOutline
  return h(NIcon, { size: 18 }, () => h(IconComp))
}

const renderSuffix = ({ option }: { option: TreeOption }) => {
  if (props.readonly) return null
  const items: any[] = []

  if (option.isLeaf) {
    // Dirty indicator
    const filepath = option.key as string
    if (props.dirtyFiles.has(filepath)) {
      items.push(h('span', { class: 'dirty-dot' }))
    }

    // Delete button (not for SKILL.md)
    if (filepath !== 'SKILL.md' && props.skillId) {
      items.push(
        h(NButton, {
          size: 'tiny', quaternary: true, circle: true, type: 'error', title: 'Delete',
          onClick: (e: Event) => { e.stopPropagation(); emit('deleteFile', filepath) },
        }, { icon: () => h(NIcon, { size: 14 }, () => h(TrashOutline)) })
      )
    }
  } else {
    // Folder actions: new file in dir, delete empty pending dir
    const dirPath = (option as any)._dirPath as string
    items.push(
      h(NButton, {
        size: 'tiny', quaternary: true, circle: true, title: 'New file in ' + option.label,
        onClick: (e: Event) => {
          e.stopPropagation()
          activeDir.value = dirPath
          showNewFile.value = true
        },
      }, { default: () => '+' }),
    )
    // Show delete button for empty directories (pending or no children with real files)
    if (!option.children?.length || option.children.every((c: TreeOption) => !c.isLeaf && !c.children?.length)) {
      items.push(
        h(NButton, {
          size: 'tiny', quaternary: true, circle: true, type: 'error', title: 'Delete folder',
          onClick: (e: Event) => {
            e.stopPropagation()
            pendingDirs.value.delete(dirPath)
            pendingDirs.value = new Set(pendingDirs.value)
            if (activeDir.value === dirPath) activeDir.value = '/'
          },
        }, { icon: () => h(NIcon, { size: 14 }, () => h(TrashOutline)) })
      )
    }
  }

  return h('div', { class: 'tree-suffix', onClick: (e: Event) => e.stopPropagation() }, items)
}

const getNodeProps = ({ option }: { option: TreeOption }) => {
  const isActive = option.isLeaf && option.key === props.activeFile
  return {
    class: isActive ? 'skill-tree-active' : undefined,
    onClick: (e: MouseEvent) => {
      const target = e.target as HTMLElement
      if (target.closest('.n-tree-node-switcher')) return

      if (option.isLeaf) {
        emit('select', option.key as string)
      } else {
        const dirPath = (option as any)._dirPath as string
        activeDir.value = dirPath
        // Toggle expand
        const key = option.key as string
        const idx = expandedKeys.value.indexOf(key)
        if (idx >= 0) {
          expandedKeys.value = expandedKeys.value.filter(k => k !== key)
          collapsedDirs.value.add(key)
          collapsedDirs.value = new Set(collapsedDirs.value)
        } else {
          expandedKeys.value = [...expandedKeys.value, key]
          collapsedDirs.value.delete(key)
          collapsedDirs.value = new Set(collapsedDirs.value)
        }
      }
    },
  }
}

const handleExpandedKeysUpdate = (keys: string[]) => {
  // Track dirs that were collapsed
  const removed = expandedKeys.value.filter(k => !keys.includes(k))
  for (const k of removed) collapsedDirs.value.add(k)
  const added = keys.filter(k => !expandedKeys.value.includes(k))
  for (const k of added) collapsedDirs.value.delete(k)
  collapsedDirs.value = new Set(collapsedDirs.value)

  expandedKeys.value = keys
  if (added.length > 0) {
    const lastKey = added[added.length - 1]!
    if (lastKey.startsWith('__dir__')) {
      activeDir.value = lastKey.substring(7)
    }
  }
}

const treeFilter = (pattern: string, node: TreeOption) => {
  return (node.label || '').toLowerCase().includes(pattern.toLowerCase())
}

// --- Actions ---
function handleCreateFolder() {
  const name = newFolderName.value.trim()
  if (!name) return
  const dirPath = activeDir.value === '/'
    ? name
    : activeDir.value.replace(/\/$/, '') + '/' + name
  pendingDirs.value = new Set([...pendingDirs.value, dirPath])
  activeDir.value = dirPath
  newFolderName.value = ''
  showNewFolder.value = false
}

function handleCreateFile() {
  const name = newFileName.value.trim()
  if (!name) return
  const filepath = activeDir.value === '/'
    ? name
    : activeDir.value.replace(/\/$/, '') + '/' + name
  emit('newFile', filepath)
  newFileName.value = ''
  showNewFile.value = false
}

// --- Drag & drop ---
const dragOver = ref(false)
let dragCounter = 0

function handleDragEnter() {
  dragCounter++
  dragOver.value = true
}

function handleDragOver() {
  // just needs to exist to allow drop (prevent default is on the template)
}

function handleDragLeave() {
  dragCounter--
  if (dragCounter <= 0) {
    dragCounter = 0
    dragOver.value = false
  }
}

async function handleDrop(e: DragEvent) {
  dragCounter = 0
  dragOver.value = false
  if (props.readonly || !props.skillId || !e.dataTransfer) return

  const items = e.dataTransfer.items
  if (!items?.length) return

  const files: { file: File; filepath: string }[] = []
  const singleFiles: File[] = []

  // Use webkitGetAsEntry to detect folders vs files
  const entries: FileSystemEntry[] = []
  for (const item of items) {
    const entry = item.webkitGetAsEntry?.()
    if (entry) entries.push(entry)
  }

  if (entries.length > 0) {
    for (const entry of entries) {
      if (entry.isFile) {
        const file = await entryToFile(entry as FileSystemFileEntry)
        if (file) singleFiles.push(file)
      } else if (entry.isDirectory) {
        await readDirectoryEntry(entry as FileSystemDirectoryEntry, entry.name, files)
      }
    }
  }

  // Emit single files as individual uploads, folder contents as batch
  for (const f of singleFiles) {
    if (activeDir.value === '/') {
      emit('upload', f)
    } else {
      emit('uploadIn', f, activeDir.value)
    }
  }
  if (files.length > 0) {
    emit('uploadFolder', files)
  }
}

function entryToFile(entry: FileSystemFileEntry): Promise<File | null> {
  return new Promise(resolve => {
    entry.file(f => resolve(f), () => resolve(null))
  })
}

async function readDirectoryEntry(
  dirEntry: FileSystemDirectoryEntry,
  basePath: string,
  out: { file: File; filepath: string }[],
) {
  const reader = dirEntry.createReader()
  let entries: FileSystemEntry[] = []
  // readEntries may return partial results, must call repeatedly
  let batch: FileSystemEntry[]
  do {
    batch = await new Promise<FileSystemEntry[]>((resolve, reject) => {
      reader.readEntries(resolve, reject)
    })
    entries = entries.concat(batch)
  } while (batch.length > 0)

  for (const entry of entries) {
    const path = basePath ? basePath + '/' + entry.name : entry.name
    if (entry.isFile) {
      const file = await entryToFile(entry as FileSystemFileEntry)
      if (file) out.push({ file, filepath: path })
    } else if (entry.isDirectory) {
      await readDirectoryEntry(entry as FileSystemDirectoryEntry, path, out)
    }
  }
}
</script>

<style scoped>
.sidebar {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  position: relative;
}

.sidebar.drop-active {
  background: rgba(99, 180, 255, 0.06);
}

.drop-overlay {
  position: absolute;
  inset: 0;
  z-index: 10;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 2px dashed rgba(99, 180, 255, 0.4);
  border-radius: 6px;
  pointer-events: none;
  background: rgba(99, 180, 255, 0.04);
}

.sidebar-header {
  padding: 10px 12px 6px;
  flex-shrink: 0;
}

.sidebar-toolbar {
  padding: 4px 8px 4px;
  flex-shrink: 0;
  display: flex;
  gap: 4px;
  align-items: center;
}

.sidebar-active-dir {
  padding: 2px 12px 4px;
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 4px;
}

.sidebar-body {
  flex: 1;
  overflow-y: auto;
  padding: 0 4px 8px;
  min-height: 0;
}

.sidebar-actions {
  padding: 8px;
  display: flex;
  flex-direction: column;
  gap: 4px;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
  flex-shrink: 0;
}

.action-row {
  display: flex;
  gap: 4px;
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

:deep(.skill-tree-active) {
  background: rgba(255, 255, 255, 0.1) !important;
}

.dirty-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #e2b340;
  flex-shrink: 0;
}
</style>

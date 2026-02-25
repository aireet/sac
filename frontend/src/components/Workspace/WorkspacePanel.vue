<template>
  <div class="workspace-panel">
    <n-tabs v-model:value="panelTab" type="segment" size="small" style="padding: 8px 12px 0">
      <n-tab-pane name="output">
        <template #tab>
          <n-space :size="4" align="center">
            <span>Output</span>
            <span v-if="outputBadgeCount > 0" class="output-badge">{{ outputBadgeCount }}</span>
          </n-space>
        </template>

        <!-- Toolbar -->
        <div class="ws-toolbar">
          <n-input v-model:value="searchQuery" placeholder="Search files..." size="small" clearable style="flex: 1" />
          <n-space :size="4" style="flex-shrink: 0">
            <n-tooltip trigger="hover">
              <template #trigger>
                <n-button size="small" quaternary @click="refreshTree">
                  <template #icon><n-icon><RefreshOutline /></n-icon></template>
                </n-button>
              </template>
              Refresh file list
            </n-tooltip>
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
          <n-popconfirm v-if="canEdit" @positive-click="handleBatchDelete">
            <template #trigger>
              <n-button size="tiny" type="error">Delete</n-button>
            </template>
            Delete {{ checkedKeys.length }} items?
          </n-popconfirm>
          <n-button size="tiny" quaternary @click="checkedKeys = []">Clear</n-button>
        </div>

        <!-- Body: Tree -->
        <div class="ws-body">
          <div class="ws-content">
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
      </n-tab-pane>
      <n-tab-pane name="skills" tab="Skills">
        <!-- Skill actions bar -->
        <div class="skills-toolbar">
          <n-button size="tiny" quaternary @click="emit('openMarketplace')">+ Install</n-button>
          <n-button v-if="outdatedCount > 0" size="tiny" type="warning" :loading="syncing" @click="handleSyncSkills">
            Sync {{ outdatedCount }}
          </n-button>
        </div>

        <!-- Global sync progress bar -->
        <div v-if="activeSyncCount > 0" class="sync-status-bar">
          <n-spin :size="12" />
          <n-text depth="3" style="font-size: 12px; margin-left: 6px">
            {{ [...(syncProgress?.values() ?? [])].find(e => e.action === 'progress')?.message || 'Syncing...' }}
          </n-text>
        </div>

        <div class="skills-list" v-if="installedSkills && installedSkills.length > 0">
          <div
            v-for="as in installedSkills"
            :key="as.id"
            class="skill-item"
            @click="as.skill && emit('executeCommand', '/' + as.skill.command_name)"
          >
            <span class="skill-icon">{{ as.skill?.icon || '🔧' }}</span>
            <div class="skill-info">
              <n-space align="center" :size="4">
                <span class="skill-name">{{ as.skill?.name || 'Unknown' }}</span>
                <n-tag v-if="as.skill && isOutdated(as)" type="warning" size="tiny" round :bordered="false" style="font-size: 10px; padding: 0 5px; height: 16px">NEW</n-tag>
              </n-space>
              <div class="skill-cmd">/{{ as.skill?.command_name }}</div>
              <div v-if="as.skill && getSyncEvent(as.skill.id)" class="skill-sync-progress">
                <n-text :type="getSyncEvent(as.skill.id)!.action === 'error' ? 'error' : getSyncEvent(as.skill.id)!.action === 'complete' ? 'success' : 'info'" style="font-size: 11px">
                  {{ getSyncEvent(as.skill.id)!.message }}
                </n-text>
              </div>
            </div>
            <n-tag v-if="as.skill?.is_official" size="tiny" type="info" :bordered="false">Official</n-tag>
            <n-tag v-else-if="as.skill?.group_id" size="tiny" type="warning" :bordered="false">Group</n-tag>
            <n-tag v-else-if="as.skill?.is_public" size="tiny" type="success" :bordered="false">Public</n-tag>
            <n-tag v-else size="tiny" :bordered="false">Private</n-tag>
            <div class="skill-actions" @click.stop>
              <n-tooltip trigger="hover" v-if="as.skill">
                <template #trigger>
                  <n-button size="tiny" quaternary circle @click="openDetail(as.skill!)">
                    <template #icon><n-icon :size="14"><EyeOutline /></n-icon></template>
                  </n-button>
                </template>
                View skill
              </n-tooltip>
              <n-popconfirm @positive-click="as.skill && handleUninstall(as.skill.id)">
                <template #trigger>
                  <n-button size="tiny" quaternary circle type="error" title="Uninstall">
                    <template #icon><n-icon :size="14"><CloseOutline /></n-icon></template>
                  </n-button>
                </template>
                Uninstall this skill?
              </n-popconfirm>
            </div>
          </div>
        </div>
        <n-empty v-else description="No skills installed" size="small" style="margin-top: 40px">
          <template #extra>
            <n-button size="small" @click="emit('openMarketplace')">Browse Marketplace</n-button>
          </template>
        </n-empty>
      </n-tab-pane>
    </n-tabs>

    <!-- Skill preview (readonly editor) -->
    <SkillEditor
      v-if="showSkillDetail && detailSkill"
      :skill="detailSkill"
      :readonly="true"
      @close="showSkillDetail = false"
    />

    <!-- Share link modal -->
    <n-modal v-model:show="showShareModal">
      <n-card title="Share Link" style="width: 480px" :bordered="false" size="small" closable @close="showShareModal = false">
        <n-space vertical :size="12">
          <n-input-group>
            <n-input id="share-url-input" :value="shareUrl" readonly style="flex: 1" />
            <n-button type="primary" @click="handleCopyShareUrl">Copy</n-button>
          </n-input-group>
          <n-text depth="3" style="font-size: 12px">Anyone with this link can view the file without logging in.</n-text>
        </n-space>
        <template #footer>
          <n-space :size="8" justify="end">
            <n-button size="small" type="error" @click="handleDeleteShare">Delete Link</n-button>
          </n-space>
        </template>
      </n-card>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, h, type Component } from 'vue'
import {
  NSpace, NIcon, NText, NButton, NSpin, NTabs, NTabPane, NTag,
  NEmpty, NPopconfirm, NModal, NInput, NInputGroup, NCard, NTree, NTooltip,
  useMessage, useDialog,
  type TreeOption,
} from 'naive-ui'
import {
  RefreshOutline, DocumentOutline,
  DownloadOutline, TrashOutline, FolderOutline, CodeSlashOutline,
  DocumentTextOutline, ImageOutline, SettingsOutline, CloseOutline,
  ShareSocialOutline, EyeOutline,
} from '@vicons/ionicons5'
import type { AgentSkill } from '../../generated/sac/v1/agent'
import type { Skill } from '../../generated/sac/v1/skill'
import type { SkillSyncEvent } from '../../services/skillSyncWS'
import SkillEditor from '../SkillMarketplace/SkillEditor.vue'
import { syncAgentSkills } from '../../services/skillAPI'
import { uninstallSkill } from '../../services/agentAPI'
import { extractApiError } from '../../utils/error'
import {
  listOutputFiles, downloadOutputFile, deleteOutputFile, shareOutputFile, deleteShare,
  watchOutputFiles,
  type WorkspaceFile, type SpaceTab,
} from '../../services/workspaceAPI'
import { getFileIcon } from '../../utils/fileTypes'

const props = defineProps<{
  agentId: number
  installedSkills?: AgentSkill[]
  syncProgress?: Map<number, SkillSyncEvent>
}>()

const emit = defineEmits<{
  openFile: [file: WorkspaceFile, spaceTab: SpaceTab]
  executeCommand: [command: string]
  skillsChanged: []
  openMarketplace: []
}>()

const message = useMessage()
const dialog = useDialog()

// --- State ---
const panelTab = ref('output')
const spaceTab = ref<SpaceTab>('output')
const treeData = ref<TreeOption[]>([])
const expandedKeys = ref<string[]>([])
const checkedKeys = ref<string[]>([])
const activeDir = ref('/')
const rootLoading = ref(false)
const searchQuery = ref('')
const outputBadgeCount = ref(0)

// Share state
const showShareModal = ref(false)
const shareUrl = ref('')
const shareCode = ref('')
const sharingFile = ref(false)

// Skill management state
const showSkillDetail = ref(false)
const detailSkill = ref<Skill | null>(null)
const syncing = ref(false)

// --- Skill helpers ---
const activeSyncCount = computed(() => props.syncProgress?.size ?? 0)

function getSyncEvent(skillId: number): SkillSyncEvent | undefined {
  return props.syncProgress?.get(skillId)
}

const outdatedCount = computed(() => {
  if (!props.installedSkills) return 0
  return props.installedSkills.filter(as => isOutdated(as)).length
})

function isOutdated(as: AgentSkill): boolean {
  if (!as.skill) return false
  return as.synced_version < as.skill.version
}

function openDetail(skill: Skill) {
  detailSkill.value = skill
  showSkillDetail.value = true
}

async function handleUninstall(skillId: number) {
  try {
    await uninstallSkill(props.agentId, skillId)
    message.success('Skill uninstalled')
    emit('skillsChanged')
  } catch (error) {
    message.error(extractApiError(error, 'Failed to uninstall'))
  }
}

async function handleSyncSkills() {
  syncing.value = true
  try {
    await syncAgentSkills(props.agentId)
    message.success('Skills synced')
    emit('skillsChanged')
  } catch (error) {
    message.error(extractApiError(error, 'Sync failed'))
  } finally {
    syncing.value = false
  }
}

// --- Computed ---
const canEdit = computed(() => true) // allow delete on output

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

    // Share button (output tab only)
    if (spaceTab.value === 'output') {
      items.push(
        h(NButton, {
          size: 'tiny', quaternary: true, circle: true, title: 'Share',
          onClick: (e: Event) => { e.stopPropagation(); handleShareFile(file) },
        }, { icon: () => h(NIcon, { size: 14 }, () => h(ShareSocialOutline)) })
      )
    }

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
  const hlAction = highlightedFiles.value.get(option.key as string)
  return {
    class: hlAction === 'upload' ? 'ws-flash-green' : hlAction === 'delete' ? 'ws-flash-red' : undefined,
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
    activeDir.value = newDirs[newDirs.length - 1]!
  }
}

const handleCheckedKeysUpdate = (keys: Array<string | number>) => {
  checkedKeys.value = keys as string[]
}

// --- Data loading ---
const listFilesForTab = async (path: string) => {
  return listOutputFiles(props.agentId, path)
}

const loadRootFiles = async () => {
  rootLoading.value = true
  try {
    const result = await listFilesForTab('/')
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
    const result = await listFilesForTab(path)
    node.children = buildNodes(result.files || [])
  } catch (err) {
    console.error('Failed to load directory:', err)
    node.children = []
  }
}

const reloadDir = async (dirPath: string) => {
  try {
    const result = await listFilesForTab(dirPath)
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

// --- File operations ---
const handleDownloadFile = (file: WorkspaceFile) => {
  downloadOutputFile(props.agentId, file.path)
}

const handleDeleteFile = async (file: WorkspaceFile) => {
  try {
    const deletePath = file.is_directory ? file.path + '/' : file.path
    await deleteOutputFile(props.agentId, deletePath)
    message.success(`Deleted ${file.name}`)
    // Reload parent directory
    const parts = file.path.replace(/\/$/, '').split('/')
    parts.pop()
    const parentDir = parts.join('/') || '/'
    await reloadDir(parentDir)
  } catch (err) {
    console.error('Delete failed:', err)
    message.error(`Failed to delete ${file.name}`)
  }
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

// --- Share ---
const handleShareFile = async (file: WorkspaceFile) => {
  sharingFile.value = true
  try {
    const result = await shareOutputFile(props.agentId, file.path)
    shareCode.value = result.short_code
    shareUrl.value = `${window.location.origin}/s/${result.short_code}`
    showShareModal.value = true
  } catch (err) {
    console.error('Share failed:', err)
    message.error('Failed to create share link')
  } finally {
    sharingFile.value = false
  }
}

const handleCopyShareUrl = async () => {
  const text = shareUrl.value
  // Try modern clipboard API first
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text)
      message.success('Link copied to clipboard')
      return
    } catch { /* fall through */ }
  }
  // Fallback: select the input element directly
  const input = document.querySelector('#share-url-input input') as HTMLInputElement | null
  if (input) {
    input.select()
    input.setSelectionRange(0, text.length)
    document.execCommand('copy')
    message.success('Link copied to clipboard')
    return
  }
  // Last resort: textarea fallback
  const ta = document.createElement('textarea')
  ta.value = text
  ta.style.cssText = 'position:fixed;left:-9999px'
  document.body.appendChild(ta)
  ta.focus()
  ta.select()
  document.execCommand('copy')
  document.body.removeChild(ta)
  message.success('Link copied to clipboard')
}

const handleDeleteShare = async () => {
  try {
    await deleteShare(shareCode.value)
    message.success('Share link deleted')
    showShareModal.value = false
  } catch (err) {
    console.error('Delete share failed:', err)
    message.error('Failed to delete share link')
  }
}

// --- Watchers ---
watch(() => props.agentId, () => {
  expandedKeys.value = []
  checkedKeys.value = []
  activeDir.value = '/'
  searchQuery.value = ''
  outputBadgeCount.value = 0
  loadRootFiles()
  startOutputWatch()
})

// --- Output file highlight flash ---
const highlightedFiles = ref<Map<string, 'upload' | 'delete'>>(new Map())

const flashHighlight = (path: string, action: 'upload' | 'delete') => {
  // Normalize: strip trailing slash, use the filename portion as tree key
  const key = path.replace(/^\/+/, '')
  highlightedFiles.value.set(key, action)
  // Force reactivity
  highlightedFiles.value = new Map(highlightedFiles.value)
  setTimeout(() => {
    highlightedFiles.value.delete(key)
    highlightedFiles.value = new Map(highlightedFiles.value)
  }, 2000)
}

// --- Output WebSocket watch (always active, auto-switch on event) ---
let outputWatchAbort: (() => void) | null = null

const startOutputWatch = () => {
  stopOutputWatch()
  outputWatchAbort = watchOutputFiles(props.agentId, (event) => {
    // Show notification
    const action = event.action === 'upload' ? 'New file' : 'File removed'
    message.info(`${action}: ${event.name}`, { duration: 3000 })

    // Flash highlight on the affected file
    flashHighlight(event.path, event.action as 'upload' | 'delete')

    // Refresh file list
    if (!rootLoading.value) loadRootFiles()
  }, () => {
    // onReconnect: refresh file list to catch events missed during disconnect
    if (!rootLoading.value) {
      loadRootFiles()
    }
  })
}

const stopOutputWatch = () => {
  if (outputWatchAbort) {
    outputWatchAbort()
    outputWatchAbort = null
  }
}

onMounted(() => {
  loadRootFiles()
  startOutputWatch()
})

onUnmounted(() => {
  stopOutputWatch()
})
</script>

<style scoped>
.workspace-panel {
  height: 100%;
  display: flex;
  flex-direction: column;
  position: relative;
}

.workspace-panel :deep(.n-tabs) {
  height: 100%;
  display: flex;
  flex-direction: column;
}

.workspace-panel :deep(.n-tabs .n-tab-pane) {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}

.workspace-panel :deep(.n-tabs-pane-wrapper) {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.skills-list {
  padding: 8px 4px;
  overflow-y: auto;
  flex: 1;
}

.skill-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border-radius: 6px;
  cursor: pointer;
  transition: background 0.15s;
}

.skill-item:hover {
  background: rgba(255, 255, 255, 0.06);
}

.skill-icon {
  font-size: 20px;
  flex-shrink: 0;
  width: 28px;
  text-align: center;
}

.skill-info {
  flex: 1;
  min-width: 0;
  overflow: hidden;
}

.skill-name {
  font-size: 13px;
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.skill-cmd {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.4);
  font-family: monospace;
}

.sync-status-bar {
  display: flex;
  align-items: center;
  padding: 6px 12px;
  background: rgba(14, 165, 233, 0.06);
  border-bottom: 1px solid rgba(14, 165, 233, 0.1);
  flex-shrink: 0;
}

.skill-sync-progress {
  margin-top: 2px;
}

.skills-toolbar {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
  padding: 4px 8px;
  flex-shrink: 0;
}

.skill-actions {
  display: flex;
  align-items: center;
  gap: 2px;
  opacity: 0;
  transition: opacity 0.15s;
  flex-shrink: 0;
}

.skill-item:hover .skill-actions {
  opacity: 1;
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

.output-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 16px;
  height: 16px;
  padding: 0 4px;
  border-radius: 8px;
  background: #f0a020;
  color: #fff;
  font-size: 10px;
  font-weight: 600;
  line-height: 1;
}

:deep(.ws-flash-green) {
  animation: flash-green 2s ease-out;
}

:deep(.ws-flash-red) {
  animation: flash-red 2s ease-out;
}

@keyframes flash-green {
  0% { background: rgba(99, 226, 183, 0.4); }
  100% { background: transparent; }
}

@keyframes flash-red {
  0% { background: rgba(232, 88, 88, 0.4); }
  100% { background: transparent; }
}
</style>

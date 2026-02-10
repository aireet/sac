<template>
  <n-config-provider :theme="darkTheme">
    <n-layout style="height: 100vh">
      <!-- Top Header Bar -->
      <n-layout-header bordered style="height: 60px; padding: 0 24px; display: flex; align-items: center; justify-content: space-between;">
        <div class="header-content">
          <img :src="sacLogo" alt="SAC" class="logo" />
          <span class="subtitle">Sandbox Agent Cluster</span>
        </div>

        <!-- Agent Quick Switcher + User Actions -->
        <div class="agent-switcher">
          <n-space align="center" :size="12">
            <n-text depth="3" style="font-size: 13px">Current Agent:</n-text>
            <n-select
              :value="selectedAgentId"
              :options="agentOptions"
              :loading="loadingAgents"
              placeholder="Select an agent"
              style="min-width: 220px"
              :consistent-menu-width="false"
              @update:value="handleAgentSelect"
            >
              <template #empty>
                <n-empty description="No agents yet" size="small">
                  <template #extra>
                    <n-button size="small" @click="showAgentCreator = true">
                      Create Agent
                    </n-button>
                  </template>
                </n-empty>
              </template>
            </n-select>
            <n-button
              size="small"
              quaternary
              circle
              @click="showAgentCreator = true"
              title="Create new agent"
            >
              <template #icon>
                <n-icon><Add /></n-icon>
              </template>
            </n-button>
            <n-divider vertical />
            <n-button
              size="small"
              :type="viewMode === 'marketplace' ? 'primary' : 'default'"
              :secondary="viewMode === 'marketplace'"
              @click="viewMode = viewMode === 'marketplace' ? 'terminal' : 'marketplace'"
            >
              <template #icon>
                <n-icon><StorefrontOutline /></n-icon>
              </template>
              Marketplace
            </n-button>
            <n-divider vertical />
            <n-button-group size="small">
              <n-tooltip trigger="hover">
                <template #trigger>
                  <n-button
                    :type="inputMode === 'chat' ? 'primary' : 'default'"
                    :secondary="inputMode === 'chat'"
                    @click="inputMode = 'chat'"
                  >
                    <template #icon>
                      <n-icon><ChatbubblesOutline /></n-icon>
                    </template>
                  </n-button>
                </template>
                Chat Mode
              </n-tooltip>
              <n-tooltip trigger="hover">
                <template #trigger>
                  <n-button
                    :type="inputMode === 'terminal' ? 'primary' : 'default'"
                    :secondary="inputMode === 'terminal'"
                    @click="inputMode = 'terminal'"
                  >
                    <template #icon>
                      <n-icon><TerminalOutline /></n-icon>
                    </template>
                  </n-button>
                </template>
                Terminal Mode
              </n-tooltip>
            </n-button-group>
            <n-divider vertical />
            <n-button
              v-if="authStore.isAdmin"
              size="small"
              quaternary
              @click="$router.push('/admin')"
            >
              <template #icon>
                <n-icon><SettingsOutline /></n-icon>
              </template>
              Admin
            </n-button>
            <n-text depth="3" style="font-size: 13px">{{ authStore.user?.username }}</n-text>
            <n-button
              size="small"
              quaternary
              @click="handleLogout"
            >
              <template #icon>
                <n-icon><LogOutOutline /></n-icon>
              </template>
            </n-button>
          </n-space>
        </div>
      </n-layout-header>

      <!-- Main Content Area -->
      <n-layout has-sider class="main-body">
        <!-- Left Sider -->
        <n-layout-sider
          bordered
          :width="380"
          :collapsed-width="0"
          :show-trigger="'bar'"
          collapse-mode="width"
        >
          <n-tabs v-model:value="activeTab" type="segment" animated style="padding: 12px">
            <n-tab-pane name="agents" tab="My Agents">
              <AgentSelector
                ref="agentSelectorRef"
                :selected-agent-id="selectedAgentId"
                :pod-statuses="podStatuses"
                @select="handleAgentSelect"
                @create="showAgentCreator = true"
                @edit="handleEditAgent"
              />
            </n-tab-pane>
            <n-tab-pane name="skills" tab="Agent Dashboard" :disabled="!selectedAgentId">
              <SkillPanel
                v-if="selectedAgent"
                :agent-id="selectedAgentId"
                :agent="selectedAgent"
                :pod-status="podStatuses.get(selectedAgentId) || null"
                :installed-skills="selectedAgent.installed_skills"
                @execute-command="handleExecuteCommand"
                @restart="handleRestartFromPanel"
                @skills-changed="handleSkillsChanged"
                @open-marketplace="viewMode = 'marketplace'"
              />
            </n-tab-pane>
          </n-tabs>
        </n-layout-sider>

        <!-- Center Content -->
        <div class="center-content">
          <template v-if="viewMode === 'terminal'">
            <!-- File Editor Overlay (over terminal) -->
            <div v-if="editorFile" class="editor-overlay">
              <div class="editor-header">
                <n-breadcrumb separator="/">
                  <n-breadcrumb-item @click="closeEditor">
                    <n-icon :size="14"><ArrowBackOutline /></n-icon>
                    <span style="margin-left: 4px">Back</span>
                  </n-breadcrumb-item>
                  <n-breadcrumb-item v-for="(seg, i) in editorPathSegments" :key="i">
                    {{ seg }}
                  </n-breadcrumb-item>
                </n-breadcrumb>
                <n-space :size="8" align="center">
                  <n-tag v-if="editorDirty" size="small" type="warning">Unsaved</n-tag>
                  <n-button
                    v-if="editorCategory === 'text'"
                    size="small"
                    type="primary"
                    :disabled="!editorDirty"
                    :loading="editorSaving"
                    @click="handleEditorSave"
                  >
                    Save
                  </n-button>
                  <n-button size="small" quaternary @click="handleEditorDownload">
                    <template #icon><n-icon :size="14"><DownloadOutline /></n-icon></template>
                    Download
                  </n-button>
                  <n-button size="small" quaternary @click="closeEditor">Close</n-button>
                </n-space>
              </div>
              <div class="editor-body">
                <n-spin :show="editorLoading" style="height: 100%">
                  <!-- Text editor -->
                  <textarea
                    v-if="editorCategory === 'text'"
                    v-model="editorContent"
                    class="editor-textarea"
                    spellcheck="false"
                  />
                  <!-- Image preview -->
                  <div v-else-if="editorCategory === 'image'" class="image-preview">
                    <img :src="editorBlobUrl" :alt="editorFile.name" />
                  </div>
                  <!-- Binary fallback -->
                  <div v-else class="binary-preview">
                    <n-icon :size="64" depth="3"><DocumentOutline /></n-icon>
                    <n-text style="font-size: 16px; margin-top: 12px">{{ editorFile.name }}</n-text>
                    <n-text depth="3" style="margin-top: 4px">{{ formatBytes(editorFile.size) }}</n-text>
                    <n-button style="margin-top: 16px" @click="handleEditorDownload">
                      <template #icon><n-icon><DownloadOutline /></n-icon></template>
                      Download File
                    </n-button>
                  </div>
                </n-spin>
              </div>
            </div>

            <!-- Terminal (hidden when editor is open) -->
            <div v-show="!editorFile" class="terminal-area">
              <Terminal
                ref="terminalRef"
                :session-id="sessionId"
                :ws-url="wsUrl"
                :agent-id="selectedAgentId"
                :interactive-mode="inputMode === 'terminal'"
              />
            </div>
            <ChatInput
              v-if="sessionId && inputMode === 'chat' && !editorFile"
              @send="handleSendMessage"
              @interrupt="handleInterrupt"
            />
          </template>
          <SkillMarketplace
            v-else
            :agent-id="selectedAgentId"
            :agent="selectedAgent"
            :installed-skills="selectedAgent?.installed_skills"
            @skills-changed="handleSkillsChanged"
            @close="viewMode = 'terminal'"
          />
        </div>

        <!-- Right Workspace Panel -->
        <template v-if="selectedAgentId && viewMode === 'terminal'">
          <div
            class="resize-handle"
            :class="{ active: resizing }"
            @mousedown="startResize"
          />
          <div class="right-panel" :style="{ width: rightPanelWidth + 'px' }">
            <WorkspacePanel :agent-id="selectedAgentId" @open-file="handleOpenFile" />
          </div>
        </template>
      </n-layout>
    </n-layout>

    <!-- Agent Creator Modal -->
    <AgentCreator
      v-model:show="showAgentCreator"
      :agent="editingAgent"
      @success="handleAgentCreated"
    />
  </n-config-provider>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import {
  NConfigProvider,
  NLayout,
  NLayoutHeader,
  NLayoutSider,
  NTabs,
  NTabPane,
  NSpace,
  NText,
  NSelect,
  NButton,
  NButtonGroup,
  NIcon,
  NDivider,
  NEmpty,
  NTooltip,
  NTag,
  NBreadcrumb,
  NBreadcrumbItem,
  NSpin,
  darkTheme,
  useMessage,
} from 'naive-ui'
import {
  Add, StorefrontOutline, TerminalOutline, ChatbubblesOutline, SettingsOutline, LogOutOutline,
  ArrowBackOutline, DownloadOutline, DocumentOutline,
} from '@vicons/ionicons5'
import Terminal from '../components/Terminal/Terminal.vue'
import ChatInput from '../components/ChatInput/ChatInput.vue'
import SkillPanel from '../components/SkillPanel/SkillPanel.vue'
import SkillMarketplace from '../components/SkillMarketplace/SkillMarketplace.vue'
import AgentSelector from '../components/Agent/AgentSelector.vue'
import AgentCreator from '../components/Agent/AgentCreator.vue'
import WorkspacePanel from '../components/Workspace/WorkspacePanel.vue'
import { getAgent, getAgents, getAgentStatuses, type Agent, type AgentStatus } from '../services/agentAPI'
import { createSession, deleteSession, waitForSessionReady } from '../services/sessionAPI'
import {
  fetchFileBlob, fetchPublicFileBlob, downloadFile, downloadPublicFile,
  uploadFile, uploadPublicFile,
  type WorkspaceFile,
} from '../services/workspaceAPI'
import { getFileCategory, MAX_TEXT_PREVIEW_BYTES, MAX_IMAGE_PREVIEW_BYTES } from '../utils/fileTypes'
import { extractApiError } from '../utils/error'
import { useAuthStore } from '../stores/auth'
import { getWsBaseUrl } from '../services/api'
import sacLogo from '../assets/sac-logo.svg'

const router = useRouter()
const authStore = useAuthStore()
const sessionId = ref('')
const wsUrl = ref(getWsBaseUrl())
const message = useMessage()

const terminalRef = ref()
const agentSelectorRef = ref()

const activeTab = ref('agents')
const selectedAgentId = ref<number>(0)
const selectedAgent = ref<Agent | null>(null)
const showAgentCreator = ref(false)
const editingAgent = ref<Agent | null>(null)
const viewMode = ref<'terminal' | 'marketplace'>('terminal')
const inputMode = ref<'chat' | 'terminal'>('terminal')

// Right panel resize
const rightPanelWidth = ref(480)
const resizing = ref(false)

const startResize = (e: MouseEvent) => {
  e.preventDefault()
  resizing.value = true
  const startX = e.clientX
  const startWidth = rightPanelWidth.value

  const onMouseMove = (ev: MouseEvent) => {
    const delta = startX - ev.clientX
    rightPanelWidth.value = Math.min(700, Math.max(300, startWidth + delta))
  }
  const onMouseUp = () => {
    resizing.value = false
    document.removeEventListener('mousemove', onMouseMove)
    document.removeEventListener('mouseup', onMouseUp)
  }
  document.addEventListener('mousemove', onMouseMove)
  document.addEventListener('mouseup', onMouseUp)
}

// --- File Editor (center overlay) ---
const editorFile = ref<WorkspaceFile | null>(null)
const editorSpaceTab = ref<'private' | 'public'>('private')
const editorCategory = ref<'text' | 'image' | 'binary'>('binary')
const editorContent = ref('')
const editorOriginalContent = ref('')
const editorBlobUrl = ref('')
const editorLoading = ref(false)
const editorSaving = ref(false)

const editorDirty = computed(() => editorCategory.value === 'text' && editorContent.value !== editorOriginalContent.value)

const editorPathSegments = computed(() => {
  if (!editorFile.value) return []
  return editorFile.value.path.replace(/^\//, '').split('/')
})

const handleOpenFile = async (file: WorkspaceFile, spaceTab: 'private' | 'public') => {
  editorFile.value = file
  editorSpaceTab.value = spaceTab
  editorCategory.value = getFileCategory(file.name)
  editorContent.value = ''
  editorOriginalContent.value = ''
  editorLoading.value = true

  // Clean up previous blob URL
  if (editorBlobUrl.value) {
    URL.revokeObjectURL(editorBlobUrl.value)
    editorBlobUrl.value = ''
  }

  try {
    const blob = spaceTab === 'private'
      ? await fetchFileBlob(selectedAgentId.value, file.path)
      : await fetchPublicFileBlob(file.path)

    if (editorCategory.value === 'text') {
      if (file.size > MAX_TEXT_PREVIEW_BYTES) {
        editorCategory.value = 'binary'
        message.warning('File too large to edit in browser')
      } else {
        const text = await blob.text()
        editorContent.value = text
        editorOriginalContent.value = text
      }
    } else if (editorCategory.value === 'image') {
      if (file.size > MAX_IMAGE_PREVIEW_BYTES) {
        editorCategory.value = 'binary'
        message.warning('Image too large to preview')
      } else {
        editorBlobUrl.value = URL.createObjectURL(blob)
      }
    }
  } catch (err) {
    console.error('Failed to load file:', err)
    message.error('Failed to load file')
    editorFile.value = null
  } finally {
    editorLoading.value = false
  }
}

const handleEditorSave = async () => {
  if (!editorFile.value || !editorDirty.value) return
  editorSaving.value = true
  try {
    const blob = new Blob([editorContent.value], { type: 'text/plain' })
    const f = new File([blob], editorFile.value.name)
    // Extract the directory path (parent of the file)
    const parts = editorFile.value.path.split('/')
    parts.pop() // remove filename
    let dirPath = parts.join('/')
    // Ensure trailing slash for non-root dirs (backend concatenates path + filename)
    if (dirPath && !dirPath.endsWith('/')) dirPath += '/'
    if (!dirPath) dirPath = '/'

    if (editorSpaceTab.value === 'private') {
      await uploadFile(selectedAgentId.value, f, dirPath)
    } else {
      await uploadPublicFile(f, dirPath)
    }
    editorOriginalContent.value = editorContent.value
    message.success('File saved')
  } catch (err) {
    console.error('Save failed:', err)
    message.error('Failed to save file')
  } finally {
    editorSaving.value = false
  }
}

const handleEditorDownload = () => {
  if (!editorFile.value) return
  if (editorSpaceTab.value === 'private') {
    downloadFile(selectedAgentId.value, editorFile.value.path)
  } else {
    downloadPublicFile(editorFile.value.path)
  }
}

const closeEditor = () => {
  editorFile.value = null
  editorContent.value = ''
  editorOriginalContent.value = ''
  if (editorBlobUrl.value) {
    URL.revokeObjectURL(editorBlobUrl.value)
    editorBlobUrl.value = ''
  }
}

const formatBytes = (bytes: number): string => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

// Agent list for header dropdown
const agents = ref<Agent[]>([])
const loadingAgents = ref(false)

// Pod statuses polling (shared with AgentSelector and SkillPanel)
const podStatuses = ref<Map<number, AgentStatus>>(new Map())
let pollTimer: ReturnType<typeof setInterval> | null = null

const pollStatuses = async () => {
  try {
    const statuses = await getAgentStatuses()
    const map = new Map<number, AgentStatus>()
    for (const s of statuses) {
      map.set(s.agent_id, s)
    }
    podStatuses.value = map
  } catch {
    // silently ignore polling errors
  }
}

const agentOptions = computed(() => {
  return agents.value.map(agent => ({
    label: `${agent.icon || '🤖'} ${agent.name}`,
    value: agent.id,
    disabled: false,
  }))
})

const switchingAgent = ref(false)

const handleAgentSelect = async (agentId: number) => {
  if (agentId === selectedAgentId.value) return
  if (switchingAgent.value) return

  selectedAgentId.value = agentId
  closeEditor()

  if (agentId > 0) {
    switchingAgent.value = true
    viewMode.value = 'terminal'
    try {
      selectedAgent.value = await getAgent(agentId)
      activeTab.value = 'skills'

      // Clean up old connection and session
      terminalRef.value?.cleanup()
      if (sessionId.value) {
        try {
          await deleteSession(sessionId.value)
        } catch (err) {
          console.warn('Failed to delete old session:', err)
        }
        sessionId.value = ''
      }

      // Create new session for the selected agent
      await createSessionForAgent(agentId)
    } catch (error) {
      console.error('Failed to switch agent:', error)
      selectedAgent.value = null
    } finally {
      switchingAgent.value = false
    }
  } else {
    selectedAgent.value = null
  }
}

const createSessionForAgent = async (agentId: number) => {
  const loadingMsg = message.loading('Creating session...', { duration: 0 })

  try {
    // Create session
    const response = await createSession(agentId)
    console.log('Session created:', response)

    // Wait for session to be ready
    loadingMsg.content = 'Waiting for container to start...'
    const session = await waitForSessionReady(response.session_id)

    sessionId.value = session.session_id
    console.log('Session ready:', session)

    // Show agent switch banner in terminal
    if (selectedAgent.value) {
      terminalRef.value?.writeBanner(selectedAgent.value.name)
    }

    loadingMsg.type = 'success'
    loadingMsg.content = 'Session ready!'
    setTimeout(() => loadingMsg.destroy(), 2000)
  } catch (error) {
    console.error('Failed to create session:', error)
    loadingMsg.type = 'error'
    loadingMsg.content = extractApiError(error, 'Failed to create session')
    setTimeout(() => loadingMsg.destroy(), 3000)
  }
}

const handleEditAgent = (agent: Agent) => {
  editingAgent.value = agent
  showAgentCreator.value = true
}

const handleAgentCreated = async () => {
  editingAgent.value = null
  await agentSelectorRef.value?.reload()

  // If we were editing the selected agent, reload it
  if (selectedAgentId.value > 0) {
    await handleAgentSelect(selectedAgentId.value)
  }
}

const handleSendMessage = (text: string) => {
  console.log('[MainView] handleSendMessage:', text, 'terminalRef:', !!terminalRef.value)
  if (terminalRef.value) {
    terminalRef.value.sendMessage(text)
  } else {
    console.warn('[MainView] Terminal not available, cannot send message')
    message.warning('Terminal not connected. Please wait for the session to be ready.')
  }
}

const handleInterrupt = () => {
  if (terminalRef.value) {
    terminalRef.value.sendInterrupt()
  }
}

const handleExecuteCommand = (command: string) => {
  console.log('[MainView] handleExecuteCommand:', command)
  handleSendMessage(command)
}

const handleSkillsChanged = async () => {
  // Reload agent to refresh installed_skills
  if (selectedAgentId.value > 0) {
    try {
      selectedAgent.value = await getAgent(selectedAgentId.value)
    } catch (error) {
      console.error('Failed to reload agent:', error)
    }
  }
}

const handleRestartFromPanel = () => {
  // Fast poll for 30s to catch the transition back to Running
  let fastPolls = 15
  const fastTimer = setInterval(async () => {
    await pollStatuses()
    fastPolls--
    if (fastPolls <= 0) {
      clearInterval(fastTimer)
    }
  }, 2000)
}

const handleLogout = () => {
  authStore.logout()
  router.push('/login')
}

// Load agents list for header dropdown
const loadAgents = async () => {
  loadingAgents.value = true
  try {
    agents.value = (await getAgents()) || []
  } catch (error) {
    console.error('Failed to load agents:', error)
  } finally {
    loadingAgents.value = false
  }
}

// Watch for agent changes to reload dropdown list
watch(showAgentCreator, (newVal) => {
  if (!newVal) {
    // Reload agents when creator modal is closed
    loadAgents()
  }
})

onMounted(() => {
  loadAgents()
  pollStatuses()
  pollTimer = setInterval(pollStatuses, 5000)
})

onUnmounted(() => {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
})
</script>

<style scoped>
.header-content {
  display: flex;
  align-items: center;
  gap: 12px;
}

.logo {
  height: 32px;
}

.subtitle {
  font-size: 14px;
  color: #888;
  font-weight: 400;
}

.agent-switcher {
  display: flex;
  align-items: center;
  gap: 12px;
}

:deep(.n-base-selection) {
  border-radius: 6px;
}

:deep(.n-base-selection-input) {
  font-size: 14px;
}

.main-body {
  height: calc(100vh - 60px) !important;
}

.center-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  overflow: hidden;
}

.terminal-area {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.resize-handle {
  width: 6px;
  cursor: col-resize;
  background: transparent;
  transition: background 0.2s;
  flex-shrink: 0;
}

.resize-handle:hover,
.resize-handle.active {
  background: rgba(14, 165, 233, 0.3);
}

.right-panel {
  flex-shrink: 0;
  border-left: 1px solid rgba(255, 255, 255, 0.08);
  overflow: hidden;
}

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

.editor-textarea {
  width: 100%;
  flex: 1;
  min-height: 0;
  resize: none;
  border: none;
  outline: none;
  background: #1a1a1a;
  color: rgba(255, 255, 255, 0.87);
  font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', monospace;
  font-size: 13px;
  line-height: 1.6;
  padding: 16px;
  tab-size: 2;
}

.editor-textarea:focus {
  outline: none;
}

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

.binary-preview {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 4px;
}
</style>

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
            <template v-if="userGroups.length > 0">
              <n-tag v-for="g in userGroups" :key="g.id" size="small" type="info" :bordered="false">
                {{ g.name }}
              </n-tag>
            </template>
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
                @agent-updated="handleAgentUpdated"
                @edit-claude-m-d="openClaudeMDEditor"
              />
            </n-tab-pane>
          </n-tabs>
        </n-layout-sider>

        <!-- Center Content -->
        <div class="center-content">
          <template v-if="viewMode === 'terminal'">
            <!-- File Preview Overlay (over terminal) -->
            <FilePreview
              v-if="editorFile"
              :file="editorFile"
              :category="editorCategory"
              :content="editorContent"
              :blob-url="editorBlobUrl"
              :loading="editorLoading"
              :saving="editorSaving"
              :dirty="editorDirty"
              :can-save="editorCanSave"
              :csv-columns="csvColumns"
              :csv-data="csvData"
              @update:content="editorContent = $event"
              @save="handleEditorSave"
              @download="handleEditorDownload"
              @close="closeEditor"
            />

            <!-- CLAUDE.md Full Editor (over terminal) -->
            <div v-else-if="showClaudeMDEditor" class="claudemd-editor-overlay">
              <div class="claudemd-editor-header">
                <n-text strong style="font-size: 16px">CLAUDE.md — {{ selectedAgent?.name }}</n-text>
                <n-space :size="8">
                  <n-button
                    size="small"
                    type="primary"
                    :loading="savingClaudeMD"
                    @click="handleSaveClaudeMD"
                  >
                    Save &amp; Restart
                  </n-button>
                  <n-button size="small" quaternary @click="showClaudeMDEditor = false">
                    Close
                  </n-button>
                </n-space>
              </div>
              <div class="claudemd-editor-body">
                <div v-if="claudeMDReadonly" class="claudemd-readonly-section">
                  <div class="claudemd-section-label">System &amp; Group Instructions (read-only)</div>
                  <pre class="claudemd-readonly-content">{{ claudeMDReadonly }}</pre>
                </div>
                <div v-if="claudeMDReadonly" class="claudemd-divider">
                  <span>--- Agent Instructions (editable) ---</span>
                </div>
                <div v-else class="claudemd-section-label" style="padding: 12px 16px 0;">Agent Instructions</div>
                <textarea
                  v-model="claudeMDText"
                  class="claudemd-raw-textarea"
                  placeholder="Enter agent instructions..."
                  spellcheck="false"
                />
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
  darkTheme,
  useMessage,
} from 'naive-ui'
import {
  Add, StorefrontOutline, TerminalOutline, ChatbubblesOutline, SettingsOutline, LogOutOutline,
} from '@vicons/ionicons5'
import Terminal from '../components/Terminal/Terminal.vue'
import ChatInput from '../components/ChatInput/ChatInput.vue'
import SkillPanel from '../components/SkillPanel/SkillPanel.vue'
import SkillMarketplace from '../components/SkillMarketplace/SkillMarketplace.vue'
import AgentSelector from '../components/Agent/AgentSelector.vue'
import AgentCreator from '../components/Agent/AgentCreator.vue'
import WorkspacePanel from '../components/Workspace/WorkspacePanel.vue'
import FilePreview from '../components/Workspace/FilePreview.vue'
import { getAgent, getAgents, getAgentStatuses, updateAgent, restartAgent, previewClaudeMD, type Agent, type AgentStatus } from '../services/agentAPI'
import { createSession, waitForSessionReady } from '../services/sessionAPI'
import {
  fetchFileBlob, fetchPublicFileBlob, fetchGroupFileBlob, fetchOutputFileBlob,
  downloadFile, downloadPublicFile, downloadGroupFile, downloadOutputFile,
  uploadFile, uploadPublicFile, uploadGroupFile,
  syncWorkspaceToPodStream,
  type WorkspaceFile, type SpaceTab,
} from '../services/workspaceAPI'
import { getFileCategory, type FileCategory, MAX_TEXT_PREVIEW_BYTES, MAX_CSV_PREVIEW_BYTES, MAX_CSV_PREVIEW_ROWS, MAX_IMAGE_PREVIEW_BYTES } from '../utils/fileTypes'
import { listGroups, type Group } from '../services/groupAPI'
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
const userGroups = ref<Group[]>([])

// CLAUDE.md editor
const showClaudeMDEditor = ref(false)
const claudeMDReadonly = ref('')
const claudeMDText = ref('')
const savingClaudeMD = ref(false)
const loadingClaudeMD = ref(false)

async function openClaudeMDEditor() {
  if (!selectedAgentId.value) return
  showClaudeMDEditor.value = true
  loadingClaudeMD.value = true
  try {
    const data = await previewClaudeMD(selectedAgentId.value)
    claudeMDReadonly.value = data.readonly
    claudeMDText.value = data.instructions
  } catch (error) {
    claudeMDReadonly.value = ''
    claudeMDText.value = selectedAgent.value?.instructions || ''
    message.error(extractApiError(error, 'Failed to load CLAUDE.md'))
  } finally {
    loadingClaudeMD.value = false
  }
}

async function handleSaveClaudeMD() {
  if (!selectedAgentId.value) return
  savingClaudeMD.value = true
  try {
    await updateAgent(selectedAgentId.value, { instructions: claudeMDText.value })
    await restartAgent(selectedAgentId.value)
    message.success('Instructions saved, agent restarting...')
    showClaudeMDEditor.value = false
    // Refresh agent data
    selectedAgent.value = await getAgent(selectedAgentId.value)
  } catch (error) {
    message.error(extractApiError(error, 'Failed to save instructions'))
  } finally {
    savingClaudeMD.value = false
  }
}

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
const editorSpaceTab = ref<SpaceTab>('private')
const editorGroupId = ref<number | undefined>(undefined)
const editorCategory = ref<FileCategory>('binary')
const editorContent = ref('')
const editorOriginalContent = ref('')
const editorBlobUrl = ref('')
const editorLoading = ref(false)
const editorSaving = ref(false)
const csvColumns = ref<Array<{ title: string; key: string }>>([])
const csvData = ref<Array<Record<string, string>>>([])
const editorDirty = computed(() => editorCategory.value === 'text' && editorContent.value !== editorOriginalContent.value)
const editorCanSave = computed(() => {
  if (editorSpaceTab.value === 'output') return false
  if (editorSpaceTab.value === 'public' && !authStore.isAdmin) return false
  return true
})

const handleOpenFile = async (file: WorkspaceFile, spaceTab: SpaceTab, groupId?: number) => {
  editorFile.value = file
  editorSpaceTab.value = spaceTab
  editorGroupId.value = groupId
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
    let blob: Blob
    switch (spaceTab) {
      case 'private':
        blob = await fetchFileBlob(selectedAgentId.value, file.path)
        break
      case 'public':
        blob = await fetchPublicFileBlob(file.path)
        break
      case 'group':
        blob = await fetchGroupFileBlob(groupId!, file.path)
        break
      case 'output':
        blob = await fetchOutputFileBlob(selectedAgentId.value, file.path)
        break
    }

    if (editorCategory.value === 'text') {
      if (file.size > MAX_TEXT_PREVIEW_BYTES) {
        editorCategory.value = 'binary'
        message.warning('File too large to edit in browser')
      } else {
        const text = await blob.text()
        editorContent.value = text
        editorOriginalContent.value = text
      }
    } else if (editorCategory.value === 'csv') {
      if (file.size > MAX_CSV_PREVIEW_BYTES) {
        editorCategory.value = 'binary'
        message.warning('CSV too large to preview')
      } else {
        const text = await blob.text()
        editorContent.value = text
        editorOriginalContent.value = text
        const parsed = parseCsv(text, file.name.endsWith('.tsv'))
        csvColumns.value = parsed.columns
        if (parsed.data.length > MAX_CSV_PREVIEW_ROWS) {
          csvData.value = parsed.data.slice(0, MAX_CSV_PREVIEW_ROWS)
          message.info(`Showing first ${MAX_CSV_PREVIEW_ROWS} of ${parsed.data.length} rows`)
        } else {
          csvData.value = parsed.data
        }
      }
    } else if (editorCategory.value === 'html') {
      const text = await blob.text()
      editorContent.value = text
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

const parseCsv = (text: string, isTsv = false) => {
  const sep = isTsv ? '\t' : ','
  const lines = text.split('\n').filter(l => l.trim())
  if (lines.length === 0) return { columns: [], data: [] }
  const headers = lines[0]!.split(sep).map(h => h.trim().replace(/^"|"$/g, ''))
  const columns = headers.map((h, i) => ({
    title: h || `col_${i}`,
    key: `c${i}`,
    ellipsis: { tooltip: true },
    resizable: true,
    minWidth: 80,
  }))
  const data = lines.slice(1).map((line, rowIdx) => {
    const cells = line.split(sep).map(c => c.trim().replace(/^"|"$/g, ''))
    const row: Record<string, string | number> = { _key: rowIdx }
    headers.forEach((_, i) => { row[`c${i}`] = cells[i] ?? '' })
    return row
  })
  return { columns, data }
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

    switch (editorSpaceTab.value) {
      case 'private':
        await uploadFile(selectedAgentId.value, f, dirPath)
        break
      case 'public':
        await uploadPublicFile(f, dirPath)
        break
      case 'group':
        await uploadGroupFile(editorGroupId.value!, f, dirPath)
        break
      case 'output':
        // output is read-only, save should not be reachable
        break
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
  switch (editorSpaceTab.value) {
    case 'private':
      downloadFile(selectedAgentId.value, editorFile.value.path)
      break
    case 'public':
      downloadPublicFile(editorFile.value.path)
      break
    case 'group':
      if (editorGroupId.value) downloadGroupFile(editorGroupId.value, editorFile.value.path)
      break
    case 'output':
      downloadOutputFile(selectedAgentId.value, editorFile.value.path)
      break
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

      // Clean up old WS connection (backend manages session lifecycle)
      terminalRef.value?.cleanup()
      sessionId.value = ''

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
    // Create session — backend returns status 'running' once pod is ready
    const response = await createSession(agentId)
    console.log('Session created:', response)

    // If already running (existing pod reuse or pod ready), skip polling
    if (response.status !== 'running') {
      loadingMsg.content = 'Waiting for container to start...'
      await waitForSessionReady(response.session_id)
    }

    // New StatefulSet — sync workspace files with progress
    if (response.is_new) {
      loadingMsg.content = 'Syncing workspace files...'
      await syncWorkspaceToPodStream(agentId, (e) => {
        if (e.error) return
        if (e.total === 0) {
          loadingMsg.content = 'No files to sync'
        } else {
          const pct = Math.round((e.synced / e.total) * 100)
          loadingMsg.content = `Syncing files ${e.synced}/${e.total} (${pct}%)${e.file ? ': ' + e.file : ''}`
        }
      })
    }

    sessionId.value = response.session_id
    console.log('Session ready:', response.session_id)

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

const handleAgentUpdated = async () => {
  if (selectedAgentId.value > 0) {
    try {
      selectedAgent.value = await getAgent(selectedAgentId.value)
    } catch (error) {
      console.error('Failed to reload agent:', error)
    }
  }
}

const handleRestartFromPanel = async () => {
  // Immediately disconnect the dead terminal so the user sees a clear state
  terminalRef.value?.cleanup()
  sessionId.value = ''

  const loadingMsg = message.loading('Restarting agent, waiting for pod to come back...', { duration: 0 })

  // Wait a moment for K8s to process the force-delete, then poll until Running
  await new Promise(r => setTimeout(r, 1500))

  let retries = 30 // 60s total (30 × 2s)
  const timer = setInterval(async () => {
    await pollStatuses()
    retries--

    const status = podStatuses.value.get(selectedAgentId.value)
    const phase = status?.status

    if (phase === 'Running') {
      clearInterval(timer)
      loadingMsg.content = 'Pod is back, reconnecting...'
      try {
        await createSessionForAgent(selectedAgentId.value)
        loadingMsg.destroy()
      } catch {
        loadingMsg.type = 'error'
        loadingMsg.content = 'Failed to reconnect after restart'
        setTimeout(() => loadingMsg.destroy(), 3000)
      }
      return
    }

    if (retries <= 0) {
      clearInterval(timer)
      loadingMsg.type = 'warning'
      loadingMsg.content = 'Pod did not come back within 60s. Try selecting the agent again.'
      setTimeout(() => loadingMsg.destroy(), 5000)
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
  listGroups().then(g => { userGroups.value = g }).catch(() => {})
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
  position: relative;
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

.claudemd-editor-overlay {
  position: absolute;
  inset: 0;
  z-index: 10;
  display: flex;
  flex-direction: column;
  background: #1e1e2e;
}

.claudemd-editor-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  flex-shrink: 0;
}

.claudemd-editor-body {
  flex: 1;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.claudemd-readonly-section {
  flex-shrink: 0;
  max-height: 40%;
  overflow-y: auto;
  border-bottom: none;
}

.claudemd-section-label {
  padding: 8px 16px 4px;
  font-size: 12px;
  color: rgba(255, 255, 255, 0.35);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  flex-shrink: 0;
}

.claudemd-readonly-content {
  margin: 0;
  padding: 8px 16px 12px;
  font-family: monospace;
  font-size: 13px;
  line-height: 1.5;
  color: rgba(255, 255, 255, 0.5);
  white-space: pre-wrap;
  word-wrap: break-word;
}

.claudemd-divider {
  flex-shrink: 0;
  text-align: center;
  padding: 8px 0;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
}

.claudemd-divider span {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.35);
}

.claudemd-raw-textarea {
  width: 100%;
  height: 100%;
  background: transparent;
  color: #e0e0e0;
  border: none;
  outline: none;
  resize: none;
  padding: 16px;
  font-family: monospace;
  font-size: 14px;
  line-height: 1.6;
  box-sizing: border-box;
}
</style>

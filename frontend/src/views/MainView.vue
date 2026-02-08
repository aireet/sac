<template>
  <n-config-provider :theme="darkTheme">
    <n-layout style="height: 100vh">
      <!-- Top Header Bar -->
      <n-layout-header bordered style="height: 60px; padding: 0 24px; display: flex; align-items: center; justify-content: space-between;">
        <div class="header-content">
          <h1 class="logo">SAC</h1>
          <span class="subtitle">Sandbox Agent Cluster</span>
        </div>

        <!-- Agent Quick Switcher -->
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
          </n-space>
        </div>
      </n-layout-header>

      <!-- Main Content Area -->
      <n-layout has-sider style="height: calc(100vh - 60px)">
        <n-layout-sider
          bordered
          :width="400"
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

        <n-layout-content content-class="main-content">
          <template v-if="viewMode === 'terminal'">
            <div class="terminal-area">
              <Terminal
                ref="terminalRef"
                :user-id="userId"
                :session-id="sessionId"
                :ws-url="wsUrl"
                :agent-id="selectedAgentId"
                :interactive-mode="inputMode === 'terminal'"
              />
            </div>
            <ChatInput
              v-if="sessionId && inputMode === 'chat'"
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
        </n-layout-content>
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
import {
  NConfigProvider,
  NLayout,
  NLayoutHeader,
  NLayoutSider,
  NLayoutContent,
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
  darkTheme,
  useMessage,
} from 'naive-ui'
import { Add, StorefrontOutline, TerminalOutline, ChatbubblesOutline } from '@vicons/ionicons5'
import Terminal from '../components/Terminal/Terminal.vue'
import ChatInput from '../components/ChatInput/ChatInput.vue'
import SkillPanel from '../components/SkillPanel/SkillPanel.vue'
import SkillMarketplace from '../components/SkillMarketplace/SkillMarketplace.vue'
import AgentSelector from '../components/Agent/AgentSelector.vue'
import AgentCreator from '../components/Agent/AgentCreator.vue'
import { getAgent, getAgents, getAgentStatuses, type Agent, type AgentStatus } from '../services/agentAPI'
import { createSession, deleteSession, waitForSessionReady } from '../services/sessionAPI'

// Configuration - these should come from environment or auth context
const userId = ref('1')
const sessionId = ref('')
const wsUrl = ref(import.meta.env.VITE_WS_URL || `${window.location.protocol === 'https:' ? 'wss:' : 'ws:'}//${window.location.hostname}:8081`)
const message = useMessage()

const terminalRef = ref()
const agentSelectorRef = ref()

const activeTab = ref('agents')
const selectedAgentId = ref<number>(0)
const selectedAgent = ref<Agent | null>(null)
const showAgentCreator = ref(false)
const editingAgent = ref<Agent | null>(null)
const viewMode = ref<'terminal' | 'marketplace'>('terminal')
const inputMode = ref<'chat' | 'terminal'>('chat')

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

  if (agentId > 0) {
    switchingAgent.value = true
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
    loadingMsg.content = `Failed to create session: ${error}`
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
  align-items: baseline;
  gap: 12px;
}

.logo {
  font-size: 28px;
  font-weight: 700;
  margin: 0;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
  letter-spacing: 2px;
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

:deep(.main-content) {
  display: flex;
  flex-direction: column;
  height: 100%;
  overflow: hidden;
}

.terminal-area {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}
</style>

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
              v-model:value="selectedAgentId"
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
                @select="handleAgentSelect"
                @create="showAgentCreator = true"
                @edit="handleEditAgent"
              />
            </n-tab-pane>
            <n-tab-pane name="skills" tab="Installed Skills" :disabled="!selectedAgentId">
              <SkillPanel
                v-if="selectedAgent"
                :agent-id="selectedAgentId"
                :installed-skills="selectedAgent.installed_skills"
                @execute-command="handleExecuteCommand"
              />
            </n-tab-pane>
            <n-tab-pane name="marketplace" tab="Skill Marketplace">
              <SkillEditor />
            </n-tab-pane>
          </n-tabs>
        </n-layout-sider>

        <n-layout-content>
          <Terminal
            ref="terminalRef"
            :user-id="userId"
            :session-id="sessionId"
            :ws-url="wsUrl"
            :agent-id="selectedAgentId"
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
import { ref, computed, watch, onMounted } from 'vue'
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
  NTag,
  NSelect,
  NButton,
  NIcon,
  NEmpty,
  darkTheme,
  useMessage,
} from 'naive-ui'
import { Add } from '@vicons/ionicons5'
import Terminal from '../components/Terminal/Terminal.vue'
import SkillPanel from '../components/SkillPanel/SkillPanel.vue'
import SkillEditor from '../components/SkillRegister/SkillEditor.vue'
import AgentSelector from '../components/Agent/AgentSelector.vue'
import AgentCreator from '../components/Agent/AgentCreator.vue'
import { getAgent, getAgents, type Agent } from '../services/agentAPI'
import { createSession, waitForSessionReady } from '../services/sessionAPI'

// Configuration - these should come from environment or auth context
const userId = ref('1')
const sessionId = ref('')
const wsUrl = ref(import.meta.env.VITE_WS_URL || 'ws://localhost:8081')
const message = useMessage()

const terminalRef = ref()
const agentSelectorRef = ref()

const activeTab = ref('agents')
const selectedAgentId = ref<number>(0)
const selectedAgent = ref<Agent | null>(null)
const showAgentCreator = ref(false)
const editingAgent = ref<Agent | null>(null)

// Agent list for header dropdown
const agents = ref<Agent[]>([])
const loadingAgents = ref(false)

const agentOptions = computed(() => {
  return agents.value.map(agent => ({
    label: `${agent.icon || '🤖'} ${agent.name}`,
    value: agent.id,
    disabled: false,
  }))
})

const handleAgentSelect = async (agentId: number) => {
  selectedAgentId.value = agentId

  if (agentId > 0) {
    try {
      selectedAgent.value = await getAgent(agentId)
      activeTab.value = 'skills'

      // Create a new session for this agent if we don't have one
      if (!sessionId.value) {
        await createSessionForAgent(agentId)
      } else {
        // TODO: Check if we need to restart the session with new agent config
        message.warning('Session already running. Reconnect to apply new agent.')
      }
    } catch (error) {
      console.error('Failed to load agent:', error)
      selectedAgent.value = null
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

const handleExecuteCommand = (command: string) => {
  if (terminalRef.value) {
    terminalRef.value.sendCommand(command)
  }
}

// Load agents list for header dropdown
const loadAgents = async () => {
  loadingAgents.value = true
  try {
    agents.value = await getAgents()
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
</style>

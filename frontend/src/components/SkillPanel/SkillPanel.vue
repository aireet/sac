<template>
  <div class="skill-panel">
    <!-- Upper Section: Agent Info & Pod Status -->
    <n-card :bordered="false" size="small" class="agent-info-card">
      <n-space align="center" :size="12" style="margin-bottom: 12px">
        <span class="agent-icon-large">{{ agent?.icon || '🤖' }}</span>
        <div>
          <n-text strong style="font-size: 16px">{{ agent?.name }}</n-text>
          <br />
          <n-text depth="3" style="font-size: 13px">{{ agent?.description || 'No description' }}</n-text>
        </div>
      </n-space>

      <n-descriptions label-placement="left" :column="1" bordered size="small" :label-style="{ width: '60px', whiteSpace: 'nowrap' }">
        <n-descriptions-item label="Status">
          <n-tag :type="statusTagType" size="small" round>
            {{ statusLabel }}
          </n-tag>
          <n-text depth="3" style="margin-left: 8px; font-size: 12px">
            {{ podStatus?.restart_count ?? 0 }} restarts
          </n-text>
        </n-descriptions-item>
        <n-descriptions-item label="CPU">
          {{ podStatus?.cpu_request || '-' }} / {{ podStatus?.cpu_limit || '-' }}
        </n-descriptions-item>
        <n-descriptions-item label="Mem">
          {{ podStatus?.memory_request || '-' }} / {{ podStatus?.memory_limit || '-' }}
        </n-descriptions-item>
        <n-descriptions-item label="LLM">
          <n-text code style="font-size: 12px; word-break: break-all">
            {{ agent?.config?.anthropic_base_url || 'Anthropic (default)' }}
          </n-text>
        </n-descriptions-item>
      </n-descriptions>

      <n-space style="margin-top: 12px" :size="8">
        <n-popconfirm @positive-click="handleRestart">
          <template #trigger>
            <n-button size="small" :loading="restarting">
              <template #icon>
                <n-icon><RefreshOutline /></n-icon>
              </template>
              Restart Pod
            </n-button>
          </template>
          Restarting will terminate the current session and all unsaved conversation will be lost. Continue?
        </n-popconfirm>
        <n-button
          size="small"
          @click="openHistory"
        >
          <template #icon>
            <n-icon><ChatbubblesOutline /></n-icon>
          </template>
          View History
        </n-button>
      </n-space>
    </n-card>

    <n-divider style="margin: 12px 0" />

    <!-- Lower Section: Installed Skills -->
    <n-card :bordered="false" size="small">
      <template #header>
        <n-space align="center" justify="space-between" style="width: 100%">
          <n-text strong>Skills</n-text>
          <n-button size="tiny" quaternary @click="$emit('openMarketplace')">
            + Install
          </n-button>
        </n-space>
      </template>

      <n-empty v-if="installedSkillList.length === 0" description="No skills installed" size="small">
        <template #extra>
          <n-button size="small" @click="$emit('openMarketplace')">
            Browse Marketplace
          </n-button>
        </template>
      </n-empty>

      <n-space vertical v-else>
        <n-card
          v-for="skill in installedSkillList"
          :key="skill.id"
          size="small"
          hoverable
          class="skill-card"
          @click="executeSkill(skill)"
        >
          <n-space align="center" justify="space-between">
            <n-space align="center" :size="8" style="cursor: pointer; flex: 1">
              <span class="skill-icon">{{ skill.icon }}</span>
              <div>
                <n-text strong style="font-size: 13px">{{ skill.name }}</n-text>
                <br />
                <n-text depth="3" style="font-size: 11px; font-family: monospace">
                  /{{ skill.command_name }}
                </n-text>
              </div>
            </n-space>
            <div @click.stop>
              <n-popconfirm @positive-click="handleUninstall(skill.id)">
                <template #trigger>
                  <n-button size="tiny" quaternary type="error" title="Uninstall">
                    <template #icon>
                      <n-icon><CloseOutline /></n-icon>
                    </template>
                  </n-button>
                </template>
                Uninstall this skill?
              </n-popconfirm>
            </div>
          </n-space>
        </n-card>
      </n-space>
    </n-card>

    <!-- Conversation History Modal -->
    <n-modal
      v-model:show="showHistoryModal"
      preset="card"
      title="Conversation History"
      style="width: 800px; max-height: 80vh"
      :segmented="{ content: true, footer: true }"
    >
      <template #header-extra>
        <n-button size="small" quaternary @click="handleExportCSV" :loading="exporting">
          Export CSV
        </n-button>
      </template>

      <n-space :size="8" align="center" style="margin-bottom: 12px">
        <n-select
          v-model:value="historySessionFilter"
          placeholder="All Sessions"
          clearable
          size="small"
          style="width: 320px"
          :options="sessionOptions"
          @update:value="onSessionFilterChange"
        />
      </n-space>

      <div class="history-list">
        <n-spin :show="historyLoading">
          <n-empty v-if="!historyLoading && historyMessages.length === 0" description="No conversations yet" />
          <div v-else class="message-list">
            <div
              v-for="msg in historyMessages"
              :key="msg.id"
              class="message-item"
              :class="msg.role"
            >
              <div class="message-header">
                <n-tag :type="msg.role === 'user' ? 'info' : 'success'" size="small" round>
                  {{ msg.role }}
                </n-tag>
                <n-text depth="3" style="font-size: 11px; margin-left: 8px">
                  {{ formatTime(msg.timestamp) }}
                </n-text>
                <n-text v-if="!historySessionFilter" depth="3" style="font-size: 11px; margin-left: 8px; font-family: monospace">
                  {{ msg.session_id.substring(0, 8) }}
                </n-text>
              </div>
              <div class="message-content">
                <n-text style="white-space: pre-wrap; word-break: break-word; font-size: 13px">{{ msg.content }}</n-text>
              </div>
            </div>
          </div>
        </n-spin>
      </div>

      <template #footer>
        <n-space justify="space-between" align="center">
          <n-text depth="3" style="font-size: 12px">
            {{ historyMessages.length }} messages
          </n-text>
          <n-space :size="8">
            <n-button
              size="small"
              :disabled="!canGoPrev"
              :loading="historyLoading"
              @click="goPrev"
            >
              Newer
            </n-button>
            <n-button
              size="small"
              :disabled="!canGoNext"
              :loading="historyLoading"
              @click="goNext"
            >
              Older
            </n-button>
          </n-space>
        </n-space>
      </template>
    </n-modal>

    <!-- Parameter Input Modal -->
    <n-modal
      v-model:show="showParameterModal"
      preset="card"
      title="Skill Parameters"
      style="width: 600px"
    >
      <n-form
        ref="formRef"
        :model="parameterValues"
        label-placement="left"
        label-width="120"
      >
        <n-form-item
          v-for="param in currentSkill?.parameters"
          :key="param.name"
          :label="param.label"
          :required="param.required"
        >
          <n-input
            v-if="param.type === 'text'"
            v-model:value="parameterValues[param.name]"
            :placeholder="param.default_value"
          />
          <n-input-number
            v-else-if="param.type === 'number'"
            v-model:value="parameterValues[param.name]"
            :placeholder="param.default_value"
            style="width: 100%"
          />
          <n-date-picker
            v-else-if="param.type === 'date'"
            v-model:value="parameterValues[param.name]"
            type="date"
            style="width: 100%"
          />
          <n-select
            v-else-if="param.type === 'select'"
            v-model:value="parameterValues[param.name]"
            :options="
              param.options?.map((opt) => ({ label: opt, value: opt }))
            "
          />
        </n-form-item>
      </n-form>
      <template #footer>
        <n-space justify="end">
          <n-button @click="showParameterModal = false">Cancel</n-button>
          <n-button type="primary" @click="executeWithParameters">
            Execute
          </n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import {
  NCard,
  NSpace,
  NInput,
  NTag,
  NModal,
  NForm,
  NFormItem,
  NInputNumber,
  NDatePicker,
  NSelect,
  NButton,
  NIcon,
  NText,
  NDescriptions,
  NDescriptionsItem,
  NDivider,
  NEmpty,
  NPopconfirm,
  NSpin,
  useMessage,
} from 'naive-ui'
import { RefreshOutline, CloseOutline, ChatbubblesOutline } from '@vicons/ionicons5'
import { type Skill } from '../../services/skillAPI'
import {
  restartAgent,
  uninstallSkill,
  getConversations,
  getConversationSessions,
  exportConversationsCSV,
  type Agent,
  type AgentStatus,
  type ConversationMessage,
  type SessionInfo,
} from '../../services/agentAPI'
import { extractApiError } from '../../utils/error'

const props = defineProps<{
  agentId: number
  agent: Agent
  podStatus: AgentStatus | null
  installedSkills?: any[]
}>()

const emit = defineEmits<{
  executeCommand: [command: string]
  restart: []
  skillsChanged: []
  openMarketplace: []
}>()

const message = useMessage()

// --- Agent Info Section ---
const restarting = ref(false)

const statusTagType = computed((): 'success' | 'warning' | 'error' | 'default' => {
  const status = props.podStatus?.status
  switch (status) {
    case 'Running': return 'success'
    case 'Pending': return 'warning'
    case 'Failed':
    case 'Error': return 'error'
    default: return 'default'
  }
})

const statusLabel = computed((): string => {
  const status = props.podStatus?.status
  switch (status) {
    case 'Running': return 'Running'
    case 'Pending': return 'Pending'
    case 'Failed': return 'Failed'
    case 'Error': return 'Error'
    case 'NotDeployed': return 'Not Deployed'
    default: return status || 'Unknown'
  }
})

const handleRestart = async () => {
  restarting.value = true
  try {
    await restartAgent(props.agentId)
    message.success('Agent pod is restarting')
    emit('restart')
  } catch (error) {
    message.error(extractApiError(error, 'Failed to restart agent pod'))
    console.error(error)
  } finally {
    restarting.value = false
  }
}

// --- Conversation History Section ---
const PAGE_SIZE = 20
const showHistoryModal = ref(false)
const historyMessages = ref<ConversationMessage[]>([])
const historyLoading = ref(false)
const historySessionFilter = ref<string | null>(null)
const exporting = ref(false)
const sessions = ref<SessionInfo[]>([])
// pagination cursors
const canGoNext = ref(false) // has older messages
const canGoPrev = ref(false) // has newer messages
const isFirstLoad = ref(true)

const sessionOptions = computed(() =>
  sessions.value.map(s => ({
    label: `${s.session_id.substring(0, 10)}... (${s.count} msgs)`,
    value: s.session_id,
  }))
)

const formatTime = (ts: string) => new Date(ts).toLocaleString()

const openHistory = async () => {
  showHistoryModal.value = true
  historyMessages.value = []
  historySessionFilter.value = null
  isFirstLoad.value = true
  canGoPrev.value = false
  // load sessions list + first page in parallel
  await Promise.all([loadSessions(), loadPage({})])
}

const loadSessions = async () => {
  try {
    sessions.value = await getConversationSessions(props.agentId)
  } catch (error) {
    console.error('Failed to load sessions:', error)
  }
}

const onSessionFilterChange = () => {
  isFirstLoad.value = true
  canGoPrev.value = false
  loadPage({})
}

const loadPage = async (cursor: { before?: string; after?: string }) => {
  historyLoading.value = true
  try {
    const params: any = { agent_id: props.agentId, limit: PAGE_SIZE }
    if (historySessionFilter.value) {
      params.session_id = historySessionFilter.value
    }
    if (cursor.before) params.before = cursor.before
    if (cursor.after) params.after = cursor.after

    const data = await getConversations(params)
    const conversations = data.conversations || []

    historyMessages.value = conversations
    canGoNext.value = data.has_more && !cursor.after
    // When navigating with "after", has_more means there are newer messages
    if (cursor.after) {
      canGoPrev.value = data.has_more
      canGoNext.value = true // we came from an older page, so there must be older messages
    } else if (cursor.before) {
      canGoPrev.value = true // we came from a newer page
      canGoNext.value = data.has_more
    } else {
      // initial load: newest page
      canGoPrev.value = false
      canGoNext.value = data.has_more
    }
    isFirstLoad.value = false
  } catch (error) {
    message.error(extractApiError(error, 'Failed to load conversation history'))
    console.error(error)
  } finally {
    historyLoading.value = false
  }
}

const goNext = () => {
  // Older messages: use the earliest timestamp on current page as "before" cursor
  if (historyMessages.value.length > 0) {
    const oldest = historyMessages.value[0]!.timestamp
    loadPage({ before: oldest })
  }
}

const goPrev = () => {
  // Newer messages: use the latest timestamp on current page as "after" cursor
  if (historyMessages.value.length > 0) {
    const newest = historyMessages.value[historyMessages.value.length - 1]!.timestamp
    loadPage({ after: newest })
  }
}

const handleExportCSV = async () => {
  exporting.value = true
  try {
    await exportConversationsCSV(props.agentId, historySessionFilter.value || undefined)
    message.success('CSV exported')
  } catch (error) {
    message.error(extractApiError(error, 'Failed to export CSV'))
    console.error(error)
  } finally {
    exporting.value = false
  }
}

// --- Skills Section ---
const showParameterModal = ref(false)
const currentSkill = ref<Skill | null>(null)
const parameterValues = ref<Record<string, any>>({})

// Installed skills derived from agent data
const installedSkillList = computed((): Skill[] => {
  if (!props.installedSkills) return []
  return props.installedSkills
    .map((as: any) => as.skill)
    .filter((s: any) => s != null) as Skill[]
})

const handleUninstall = async (skillId: number) => {
  try {
    await uninstallSkill(props.agentId, skillId)
    message.success('Skill uninstalled')
    emit('skillsChanged')
  } catch (error) {
    message.error(extractApiError(error, 'Failed to uninstall skill'))
    console.error(error)
  }
}

const executeSkill = (skill: Skill) => {
  console.log('[SkillPanel] executeSkill called:', skill.name, '/', skill.command_name)
  currentSkill.value = skill

  if (skill.parameters && skill.parameters.length > 0) {
    parameterValues.value = {}
    skill.parameters.forEach((param) => {
      if (param.default_value) {
        parameterValues.value[param.name] = param.default_value
      }
    })
    showParameterModal.value = true
  } else {
    sendCommand(`/${skill.command_name}`)
  }
}

const executeWithParameters = () => {
  if (!currentSkill.value) return

  const args = currentSkill.value.parameters
    ?.map((p) => parameterValues.value[p.name])
    .filter((v) => v !== undefined && v !== null && v !== '')
    .map(String)
    .join(' ')

  const command = `/${currentSkill.value.command_name}${args ? ' ' + args : ''}`

  showParameterModal.value = false
  sendCommand(command)
}

const sendCommand = (command: string) => {
  console.log('[SkillPanel] sendCommand:', command)
  emit('executeCommand', command)
  message.success(`Sending: ${command}`)
}
</script>

<style scoped>
.skill-panel {
  height: 100%;
  overflow-y: auto;
}

.agent-info-card {
  background: transparent;
}

.agent-icon-large {
  font-size: 36px;
  line-height: 1;
}

.skill-card {
  cursor: pointer;
  transition: transform 0.2s;
}

.skill-card:hover {
  transform: translateY(-2px);
}

.skill-icon {
  font-size: 24px;
}

.history-list {
  max-height: 55vh;
  overflow-y: auto;
}

.message-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.message-item {
  padding: 8px 12px;
  border-radius: 6px;
  border-left: 3px solid transparent;
}

.message-item.user {
  border-left-color: #2080f0;
  background: rgba(32, 128, 240, 0.06);
}

.message-item.assistant {
  border-left-color: #18a058;
  background: rgba(24, 160, 88, 0.06);
}

.message-header {
  display: flex;
  align-items: center;
  margin-bottom: 4px;
}

.message-content {
  padding-left: 4px;
}
</style>

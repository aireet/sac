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
        <n-descriptions-item label="LLM">
          <n-text code style="font-size: 12px; word-break: break-all">
            {{ agent?.config?.anthropic_base_url || 'Anthropic (default)' }}
          </n-text>
        </n-descriptions-item>
        <n-descriptions-item label="Pod">
          <n-text code style="font-size: 11px; word-break: break-all">
            {{ podStatus?.pod_name || '-' }}
          </n-text>
        </n-descriptions-item>
      </n-descriptions>

      <!-- Resource Usage -->
      <div v-if="podStatus?.status === 'Running'" class="resource-usage">
        <div class="usage-item">
          <div class="usage-header">
            <n-text depth="3" style="font-size: 12px">CPU</n-text>
            <n-text style="font-size: 12px">
              {{ podStatus?.cpu_usage || '-' }} / {{ podStatus?.cpu_limit || '-' }}
            </n-text>
          </div>
          <n-progress
            type="line"
            :percentage="Math.min(cpuPercent, 100)"
            :color="usageColor(cpuPercent)"
            :rail-color="'rgba(255,255,255,0.08)'"
            :height="8"
            :border-radius="4"
            :show-indicator="false"
          />
        </div>
        <div class="usage-item">
          <div class="usage-header">
            <n-text depth="3" style="font-size: 12px">Memory</n-text>
            <n-text style="font-size: 12px">
              {{ podStatus?.memory_usage || '-' }} / {{ podStatus?.memory_limit || '-' }}
            </n-text>
          </div>
          <n-progress
            type="line"
            :percentage="Math.min(memPercent, 100)"
            :color="usageColor(memPercent)"
            :rail-color="'rgba(255,255,255,0.08)'"
            :height="8"
            :border-radius="4"
            :show-indicator="false"
          />
        </div>
      </div>

      <n-space style="margin-top: 12px" :size="8">
        <n-popconfirm @positive-click="handleRestart">
          <template #trigger>
            <n-button size="small" :loading="restarting">
              <template #icon>
                <n-icon><RefreshOutline /></n-icon>
              </template>
              Restart Agent
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

    <!-- CLAUDE.md Editor Section -->
    <n-button
      block
      style="margin: 8px 0"
      @click="emit('editClaudeMD')"
    >
      CLAUDE.md
    </n-button>

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
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import {
  NCard,
  NSpace,
  NTag,
  NModal,
  NSelect,
  NButton,
  NIcon,
  NText,
  NDescriptions,
  NDescriptionsItem,
  NEmpty,
  NPopconfirm,
  NSpin,
  NProgress,
  useMessage,
} from 'naive-ui'
import { RefreshOutline, ChatbubblesOutline } from '@vicons/ionicons5'
import {
  restartAgent,
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
}>()

const emit = defineEmits<{
  restart: []
  agentUpdated: []
  editClaudeMD: []
}>()

const message = useMessage()

// --- Agent Info Section ---
const restarting = ref(false)

const cpuPercent = computed(() => Math.round(props.podStatus?.cpu_usage_percent ?? 0))
const memPercent = computed(() => Math.round(props.podStatus?.memory_usage_percent ?? 0))

const usageColor = (pct: number): string => {
  if (pct >= 90) return '#e88080'
  if (pct >= 70) return '#f0a020'
  return '#63e2b7'
}

const statusTagType = computed((): 'success' | 'warning' | 'error' | 'default' => {
  const status = props.podStatus?.status
  switch (status) {
    case 'Running': return 'success'
    case 'Pending':
    case 'Terminating': return 'warning'
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
    case 'Terminating': return 'Terminating'
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
const canGoNext = ref(false)
const canGoPrev = ref(false)
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
    if (cursor.after) {
      canGoPrev.value = data.has_more
      canGoNext.value = true
    } else if (cursor.before) {
      canGoPrev.value = true
      canGoNext.value = data.has_more
    } else {
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
  if (historyMessages.value.length > 0) {
    const oldest = historyMessages.value[0]!.timestamp
    loadPage({ before: oldest })
  }
}

const goPrev = () => {
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

.resource-usage {
  margin-top: 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.usage-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.usage-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
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

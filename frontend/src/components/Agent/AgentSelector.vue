<template>
  <div class="agent-selector">
    <n-space vertical size="medium">
      <n-space justify="space-between" align="center">
        <n-text strong style="font-size: 16px">My Agents</n-text>
        <n-button
          type="primary"
          size="small"
          :disabled="agents.length >= 3"
          @click="$emit('create')"
        >
          <template #icon>
            <n-icon><Add /></n-icon>
          </template>
          {{ agents.length >= 3 ? 'Max 3 agents' : 'New Agent' }}
        </n-button>
      </n-space>

      <n-spin :show="loading">
        <n-empty
          v-if="!loading && agents.length === 0"
          description="No agents yet. Create your first agent to get started!"
        />

        <n-space vertical v-else>
          <n-card
            v-for="agent in agents"
            :key="agent.id"
            :class="['agent-card', { selected: selectedAgentId === agent.id }]"
            hoverable
            @click="selectAgent(agent.id)"
          >
            <template #header>
              <n-space align="center">
                <span class="agent-icon">{{ agent.icon || '🤖' }}</span>
                <n-text strong>{{ agent.name }}</n-text>
                <n-tag
                  v-if="selectedAgentId === agent.id"
                  type="success"
                  size="small"
                  round
                >
                  Active
                </n-tag>
              </n-space>
            </template>

            <template #header-extra>
              <n-space>
                <n-button
                  text
                  size="small"
                  @click.stop="$emit('edit', agent)"
                >
                  <template #icon>
                    <n-icon><CreateOutline /></n-icon>
                  </template>
                </n-button>
                <n-popconfirm
                  @positive-click="handleDelete(agent.id)"
                >
                  <template #trigger>
                    <n-button
                      text
                      size="small"
                      type="error"
                      @click.stop
                    >
                      <template #icon>
                        <n-icon><TrashOutline /></n-icon>
                      </template>
                    </n-button>
                  </template>
                  Delete this agent permanently?
                </n-popconfirm>
              </n-space>
            </template>

            <n-space vertical size="small">
              <n-text depth="3" style="font-size: 13px">
                {{ agent.description || 'No description' }}
              </n-text>
              <n-text depth="3" style="font-size: 12px">
                {{ agent.installed_skills?.length || 0 }} skills installed
              </n-text>
            </n-space>
          </n-card>
        </n-space>
      </n-spin>
    </n-space>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import {
  NSpace,
  NButton,
  NText,
  NCard,
  NTag,
  NIcon,
  NSpin,
  NEmpty,
  NPopconfirm,
  useMessage,
} from 'naive-ui'
import { Add, CreateOutline, TrashOutline } from '@vicons/ionicons5'
import { getAgents, deleteAgent, type Agent } from '../../services/agentAPI'

const props = defineProps<{
  selectedAgentId?: number
}>()

const emit = defineEmits<{
  select: [agentId: number]
  create: []
  edit: [agent: Agent]
}>()

const message = useMessage()

const agents = ref<Agent[]>([])
const loading = ref(false)

const loadAgents = async () => {
  loading.value = true
  try {
    agents.value = await getAgents()
  } catch (error) {
    message.error('Failed to load agents')
    console.error(error)
  } finally {
    loading.value = false
  }
}

const selectAgent = (agentId: number) => {
  emit('select', agentId)
}

const handleDelete = async (id: number) => {
  try {
    await deleteAgent(id)
    message.success('Agent deleted successfully')
    await loadAgents()

    // If deleted agent was selected, clear selection
    if (props.selectedAgentId === id) {
      emit('select', 0)
    }
  } catch (error) {
    message.error('Failed to delete agent')
    console.error(error)
  }
}

onMounted(() => {
  loadAgents()
})

// Expose reload method for parent component
defineExpose({
  reload: loadAgents,
})
</script>

<style scoped>
.agent-selector {
  height: 100%;
  overflow-y: auto;
}

.agent-card {
  cursor: pointer;
  transition: all 0.2s;
  border: 2px solid transparent;
}

.agent-card:hover {
  border-color: rgba(99, 226, 183, 0.3);
}

.agent-card.selected {
  border-color: rgba(99, 226, 183, 0.8);
  background: rgba(99, 226, 183, 0.05);
}

.agent-icon {
  font-size: 24px;
  line-height: 1;
}
</style>

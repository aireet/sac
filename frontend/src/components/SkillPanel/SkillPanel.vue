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

      <n-descriptions label-placement="left" :column="2" bordered size="small">
        <n-descriptions-item label="Status">
          <n-tag :type="statusTagType" size="small" round>
            {{ statusLabel }}
          </n-tag>
        </n-descriptions-item>
        <n-descriptions-item label="Restarts">
          {{ podStatus?.restart_count ?? '-' }}
        </n-descriptions-item>
        <n-descriptions-item label="CPU Request">
          {{ podStatus?.cpu_request || '-' }}
        </n-descriptions-item>
        <n-descriptions-item label="CPU Limit">
          {{ podStatus?.cpu_limit || '-' }}
        </n-descriptions-item>
        <n-descriptions-item label="Mem Request">
          {{ podStatus?.memory_request || '-' }}
        </n-descriptions-item>
        <n-descriptions-item label="Mem Limit">
          {{ podStatus?.memory_limit || '-' }}
        </n-descriptions-item>
      </n-descriptions>

      <n-space style="margin-top: 12px" :size="8">
        <n-button
          size="small"
          :loading="restarting"
          @click="handleRestart"
        >
          <template #icon>
            <n-icon><RefreshOutline /></n-icon>
          </template>
          Restart Pod
        </n-button>
      </n-space>
    </n-card>

    <n-divider style="margin: 12px 0" />

    <!-- Lower Section: Skills -->
    <n-card title="Skills" :bordered="false" size="small">
      <n-space vertical>
        <n-input
          v-model:value="searchQuery"
          placeholder="Search skills..."
          clearable
        >
          <template #prefix>
            <n-icon :component="SearchIcon" />
          </template>
        </n-input>

        <n-tabs v-model:value="activeCategory" type="line" animated>
          <n-tab-pane
            v-for="category in categories"
            :key="category"
            :name="category"
            :tab="category"
          >
            <n-grid :x-gap="12" :y-gap="12" :cols="2">
              <n-grid-item
                v-for="skill in filteredSkillsByCategory(category)"
                :key="skill.id"
              >
                <n-card
                  :title="skill.name"
                  size="small"
                  hoverable
                  @click="executeSkill(skill)"
                  class="skill-card"
                >
                  <template #header-extra>
                    <span class="skill-icon">{{ skill.icon }}</span>
                  </template>
                  <n-text depth="3" style="font-size: 11px; font-family: monospace; display: block; margin-bottom: 4px">
                    /{{ skill.command_name }}
                  </n-text>
                  <n-ellipsis :line-clamp="2">
                    {{ skill.description }}
                  </n-ellipsis>
                  <template #footer>
                    <n-tag
                      v-if="skill.is_official"
                      type="success"
                      size="small"
                    >
                      Official
                    </n-tag>
                    <n-tag v-if="skill.is_public" type="info" size="small">
                      Public
                    </n-tag>
                  </template>
                </n-card>
              </n-grid-item>
            </n-grid>
          </n-tab-pane>
        </n-tabs>
      </n-space>
    </n-card>

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
import { ref, computed, onMounted, watch } from 'vue'
import {
  NCard,
  NSpace,
  NInput,
  NTabs,
  NTabPane,
  NGrid,
  NGridItem,
  NTag,
  NEllipsis,
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
  useMessage,
} from 'naive-ui'
import { Search as SearchIcon, RefreshOutline } from '@vicons/ionicons5'
import { getSkills, type Skill } from '../../services/skillAPI'
import { restartAgent, type Agent, type AgentStatus } from '../../services/agentAPI'

const props = defineProps<{
  agentId: number
  agent: Agent
  podStatus: AgentStatus | null
  installedSkills?: any[]
}>()

const emit = defineEmits<{
  executeCommand: [command: string]
  restart: []
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
    message.error('Failed to restart agent pod')
    console.error(error)
  } finally {
    restarting.value = false
  }
}

// --- Skills Section ---
const skills = ref<Skill[]>([])
const searchQuery = ref('')
const activeCategory = ref<string>('')
const showParameterModal = ref(false)
const currentSkill = ref<Skill | null>(null)
const parameterValues = ref<Record<string, any>>({})

const categories = computed(() => {
  const cats = new Set(skills.value.map((s) => s.category))
  return Array.from(cats)
})

const filteredSkillsByCategory = (category: string) => {
  return skills.value.filter((skill) => {
    const matchesCategory = skill.category === category
    const matchesSearch =
      searchQuery.value === '' ||
      skill.name.toLowerCase().includes(searchQuery.value.toLowerCase()) ||
      skill.description.toLowerCase().includes(searchQuery.value.toLowerCase())
    return matchesCategory && matchesSearch
  })
}

const executeSkill = (skill: Skill) => {
  currentSkill.value = skill

  if (skill.parameters && skill.parameters.length > 0) {
    // Show parameter input modal
    parameterValues.value = {}
    skill.parameters.forEach((param) => {
      if (param.default_value) {
        parameterValues.value[param.name] = param.default_value
      }
    })
    showParameterModal.value = true
  } else {
    // Execute as slash command
    sendCommand(`/${skill.command_name}`)
  }
}

const executeWithParameters = () => {
  if (!currentSkill.value) return

  // Build slash command with arguments
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
  emit('executeCommand', command)
  message.success('Command sent to terminal')
}

const loadSkills = async () => {
  try {
    skills.value = await getSkills()
  } catch (error) {
    message.error('Failed to load skills')
    console.error(error)
  }
}

// Auto-select first category when categories change
watch(categories, (newCategories) => {
  if (newCategories.length > 0 && !activeCategory.value) {
    activeCategory.value = newCategories[0]
  }
}, { immediate: true })

onMounted(() => {
  loadSkills()
})
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
</style>

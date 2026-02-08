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
  useMessage,
} from 'naive-ui'
import { RefreshOutline, CloseOutline } from '@vicons/ionicons5'
import { type Skill } from '../../services/skillAPI'
import { restartAgent, uninstallSkill, type Agent, type AgentStatus } from '../../services/agentAPI'

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
    message.error('Failed to restart agent pod')
    console.error(error)
  } finally {
    restarting.value = false
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
    message.error('Failed to uninstall skill')
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
</style>

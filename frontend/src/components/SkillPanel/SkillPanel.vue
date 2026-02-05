<template>
  <div class="skill-panel">
    <n-card title="Skills" :bordered="false">
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

        <n-tabs type="line" animated>
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
import { ref, computed, onMounted } from 'vue'
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
  useMessage,
} from 'naive-ui'
import { Search as SearchIcon } from '@vicons/ionicons5'
import { getSkills, type Skill } from '../../services/skillAPI'

const emit = defineEmits<{
  executeCommand: [command: string]
}>()

const message = useMessage()

const skills = ref<Skill[]>([])
const searchQuery = ref('')
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
    // Execute directly
    sendCommand(skill.prompt)
  }
}

const executeWithParameters = () => {
  if (!currentSkill.value) return

  let prompt = currentSkill.value.prompt

  // Replace parameter placeholders
  currentSkill.value.parameters?.forEach((param) => {
    const value = parameterValues.value[param.name]
    if (value !== undefined && value !== null) {
      const placeholder = `{{${param.name}}}`
      prompt = prompt.replace(new RegExp(placeholder, 'g'), String(value))
    }
  })

  showParameterModal.value = false
  sendCommand(prompt)
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

onMounted(() => {
  loadSkills()
})
</script>

<style scoped>
.skill-panel {
  height: 100%;
  overflow-y: auto;
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

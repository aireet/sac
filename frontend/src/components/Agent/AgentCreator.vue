<template>
  <n-modal
    v-model:show="showModal"
    preset="card"
    :title="isEditing ? 'Edit Agent' : 'Create New Agent'"
    style="width: 700px"
    :segmented="{
      content: 'soft',
      footer: 'soft',
    }"
    @after-leave="resetForm"
  >
    <n-form
      ref="formRef"
      :model="formData"
      :rules="rules"
      label-placement="left"
      label-width="140"
    >
      <n-form-item label="Name" path="name">
        <n-input
          v-model:value="formData.name"
          placeholder="e.g., Data Analyst Assistant"
        />
      </n-form-item>

      <n-form-item label="Description" path="description">
        <n-input
          v-model:value="formData.description"
          type="textarea"
          placeholder="Describe what this agent does"
          :rows="2"
        />
      </n-form-item>

      <n-form-item label="Icon" path="icon">
        <n-select
          v-model:value="formData.icon"
          :options="iconOptions"
          placeholder="Select an icon"
          :render-label="renderIconLabel"
        />
      </n-form-item>

      <n-divider>Anthropic Configuration</n-divider>

      <n-form-item label="Auth Token" path="anthropic_auth_token">
        <n-input
          v-model:value="formData.anthropic_auth_token"
          type="password"
          show-password-on="click"
          placeholder="sk-or-v1-..."
        />
      </n-form-item>

      <n-form-item label="Base URL" path="anthropic_base_url">
        <n-input
          v-model:value="formData.anthropic_base_url"
          placeholder="https://openrouter.ai/api"
        />
      </n-form-item>

      <n-form-item label="Haiku Model" path="anthropic_haiku_model">
        <n-input
          v-model:value="formData.anthropic_haiku_model"
          placeholder="claude-haiku-4.5"
        />
      </n-form-item>

      <n-form-item label="Opus Model" path="anthropic_opus_model">
        <n-input
          v-model:value="formData.anthropic_opus_model"
          placeholder="claude-opus-4.5"
        />
      </n-form-item>

      <n-form-item label="Sonnet Model" path="anthropic_sonnet_model">
        <n-input
          v-model:value="formData.anthropic_sonnet_model"
          placeholder="claude-sonnet-4.5"
        />
      </n-form-item>

    </n-form>

    <template #footer>
      <n-space justify="end">
        <n-button @click="showModal = false">Cancel</n-button>
        <n-button type="primary" @click="handleSubmit" :loading="submitting">
          {{ isEditing ? 'Update' : 'Create' }}
        </n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<script setup lang="ts">
import { ref, h, watch } from 'vue'
import {
  NModal,
  NForm,
  NFormItem,
  NInput,
  NSelect,
  NButton,
  NSpace,
  NText,
  useMessage,
} from 'naive-ui'
import { createAgent, updateAgent, type Agent } from '../../services/agentAPI'

const props = defineProps<{
  show: boolean
  agent?: Agent | null
}>()

const emit = defineEmits<{
  'update:show': [value: boolean]
  success: []
}>()

const message = useMessage()

const showModal = ref(props.show)
const isEditing = ref(false)
const submitting = ref(false)

const formRef = ref()
const formData = ref({
  name: '',
  description: '',
  icon: '🤖',
  anthropic_auth_token: '',
  anthropic_base_url: 'https://openrouter.ai/api',
  anthropic_haiku_model: 'claude-haiku-4.5',
  anthropic_opus_model: 'claude-opus-4.5',
  anthropic_sonnet_model: 'claude-sonnet-4.5',
})

const iconOptions = [
  { label: '🤖 Robot', value: '🤖' },
  { label: '🧠 Brain', value: '🧠' },
  { label: '⚡ Lightning', value: '⚡' },
  { label: '🎯 Target', value: '🎯' },
  { label: '💡 Bulb', value: '💡' },
  { label: '📊 Chart', value: '📊' },
  { label: '📈 Trending', value: '📈' },
  { label: '🔍 Search', value: '🔍' },
  { label: '💬 Chat', value: '💬' },
  { label: '🎨 Palette', value: '🎨' },
  { label: '🛠️ Tools', value: '🛠️' },
  { label: '🚀 Rocket', value: '🚀' },
  { label: '⭐ Star', value: '⭐' },
  { label: '🎬 Movie', value: '🎬' },
  { label: '📚 Books', value: '📚' },
  { label: '🔮 Crystal Ball', value: '🔮' },
  { label: '👨‍💻 Developer', value: '👨‍💻' },
  { label: '📝 Note', value: '📝' },
  { label: '💼 Briefcase', value: '💼' },
  { label: '🎓 Graduate', value: '🎓' },
]

const rules = {
  name: { required: true, message: 'Please input agent name', trigger: 'blur' },
  anthropic_auth_token: { required: true, message: 'Please input auth token', trigger: 'blur' },
}

const renderIconLabel = (option: any) => {
  return h('div', { style: 'display: flex; align-items: center; gap: 8px; font-size: 16px;' }, [
    h('span', { style: 'font-size: 20px;' }, option.value),
    h('span', option.label.substring(3)), // Remove emoji from label text
  ])
}

const resetForm = () => {
  formData.value = {
    name: '',
    description: '',
    icon: '🤖',
    anthropic_auth_token: '',
    anthropic_base_url: 'https://openrouter.ai/api',
    anthropic_haiku_model: 'claude-haiku-4.5',
    anthropic_opus_model: 'claude-opus-4.5',
    anthropic_sonnet_model: 'claude-sonnet-4.5',
  }
  isEditing.value = false
}

const handleSubmit = async () => {
  await formRef.value?.validate()

  submitting.value = true
  try {
    if (isEditing.value && props.agent) {
      await updateAgent(props.agent.id, formData.value)
      message.success('Agent updated successfully')
    } else {
      await createAgent(formData.value)
      message.success('Agent created successfully')
    }

    showModal.value = false
    emit('success')
  } catch (error: any) {
    const errorMsg = error.response?.data?.message || error.response?.data?.error || 'Failed to save agent'
    message.error(errorMsg)
    console.error(error)
  } finally {
    submitting.value = false
  }
}

// Watch props changes
watch(() => props.show, (val) => {
  showModal.value = val
})

watch(() => props.agent, (agent) => {
  if (agent) {
    isEditing.value = true
    formData.value = {
      name: agent.name,
      description: agent.description,
      icon: agent.icon,
      anthropic_auth_token: agent.config?.anthropic_auth_token || '',
      anthropic_base_url: agent.config?.anthropic_base_url || 'https://openrouter.ai/api',
      anthropic_haiku_model: agent.config?.anthropic_haiku_model || 'claude-haiku-4.5',
      anthropic_opus_model: agent.config?.anthropic_opus_model || 'claude-opus-4.5',
      anthropic_sonnet_model: agent.config?.anthropic_sonnet_model || 'claude-sonnet-4.5',
    }
  } else {
    resetForm()
  }
}, { immediate: true })

watch(showModal, (val) => {
  emit('update:show', val)
})
</script>

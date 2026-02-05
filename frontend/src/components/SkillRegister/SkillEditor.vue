<template>
  <div class="skill-editor">
    <n-card title="Skill Management">
      <n-space vertical size="large">
        <!-- Create New Skill Button -->
        <n-button type="primary" @click="openCreateModal">
          Create New Skill
        </n-button>

        <!-- Skills List -->
        <n-data-table
          :columns="columns"
          :data="mySkills"
          :pagination="pagination"
          :loading="loading"
        />
      </n-space>
    </n-card>

    <!-- Create/Edit Modal -->
    <n-modal
      v-model:show="showEditorModal"
      preset="card"
      :title="isEditing ? 'Edit Skill' : 'Create New Skill'"
      style="width: 800px"
      :segmented="{
        content: 'soft',
        footer: 'soft',
      }"
    >
      <n-form
        ref="formRef"
        :model="formData"
        :rules="rules"
        label-placement="left"
        label-width="120"
      >
        <n-form-item label="Name" path="name">
          <n-input v-model:value="formData.name" placeholder="Skill name" />
        </n-form-item>

        <n-form-item label="Description" path="description">
          <n-input
            v-model:value="formData.description"
            type="textarea"
            placeholder="Describe what this skill does"
            :rows="3"
          />
        </n-form-item>

        <n-form-item label="Icon" path="icon">
          <n-input
            v-model:value="formData.icon"
            placeholder="Emoji icon (e.g., 💰)"
            maxlength="2"
          />
        </n-form-item>

        <n-form-item label="Category" path="category">
          <n-select
            v-model:value="formData.category"
            :options="categoryOptions"
            placeholder="Select category"
          />
        </n-form-item>

        <n-form-item label="Prompt" path="prompt">
          <n-input
            v-model:value="formData.prompt"
            type="textarea"
            placeholder="Enter the prompt for Claude. Use {{paramName}} for parameters."
            :rows="10"
          />
        </n-form-item>

        <n-form-item label="Parameters">
          <n-space vertical style="width: 100%">
            <n-dynamic-input
              v-model:value="formData.parameters"
              :on-create="createParameter"
            >
              <template #default="{ value }">
                <n-space>
                  <n-input
                    v-model:value="value.name"
                    placeholder="Parameter name"
                    style="width: 120px"
                  />
                  <n-input
                    v-model:value="value.label"
                    placeholder="Display label"
                    style="width: 150px"
                  />
                  <n-select
                    v-model:value="value.type"
                    :options="parameterTypeOptions"
                    style="width: 100px"
                  />
                  <n-checkbox v-model:checked="value.required">
                    Required
                  </n-checkbox>
                </n-space>
              </template>
            </n-dynamic-input>
          </n-space>
        </n-form-item>

        <n-form-item label="Public">
          <n-switch v-model:value="formData.is_public">
            <template #checked>Public</template>
            <template #unchecked>Private</template>
          </n-switch>
          <n-text depth="3" style="margin-left: 12px">
            Public skills can be viewed and forked by others
          </n-text>
        </n-form-item>
      </n-form>

      <template #footer>
        <n-space justify="end">
          <n-button @click="showEditorModal = false">Cancel</n-button>
          <n-button type="primary" @click="handleSubmit">
            {{ isEditing ? 'Update' : 'Create' }}
          </n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, h, onMounted, computed } from 'vue'
import {
  NCard,
  NSpace,
  NButton,
  NDataTable,
  NModal,
  NForm,
  NFormItem,
  NInput,
  NSelect,
  NSwitch,
  NText,
  NDynamicInput,
  NCheckbox,
  NTag,
  NPopconfirm,
  useMessage,
  type DataTableColumns,
} from 'naive-ui'
import {
  getSkills,
  createSkill,
  updateSkill,
  deleteSkill,
  type Skill,
  type SkillParameter,
} from '../../services/skillAPI'

const message = useMessage()

const mySkills = ref<Skill[]>([])
const loading = ref(false)
const showEditorModal = ref(false)
const isEditing = ref(false)
const currentSkillId = ref<number | null>(null)

const formRef = ref()
const formData = ref({
  name: '',
  description: '',
  icon: '📝',
  category: '',
  prompt: '',
  parameters: [] as SkillParameter[],
  is_public: false,
})

const categoryOptions = [
  { label: '数据查询', value: '数据查询' },
  { label: '数据分析', value: '数据分析' },
  { label: '报表生成', value: '报表生成' },
  { label: '数据处理', value: '数据处理' },
  { label: '其他', value: '其他' },
]

const parameterTypeOptions = [
  { label: 'Text', value: 'text' },
  { label: 'Number', value: 'number' },
  { label: 'Date', value: 'date' },
  { label: 'Select', value: 'select' },
]

const rules = {
  name: { required: true, message: 'Please input skill name', trigger: 'blur' },
  category: { required: true, message: 'Please select category', trigger: 'change' },
  prompt: { required: true, message: 'Please input prompt', trigger: 'blur' },
}

const pagination = {
  pageSize: 10,
}

const createParameter = (): SkillParameter => ({
  name: '',
  label: '',
  type: 'text',
  required: false,
})

const columns: DataTableColumns<Skill> = [
  {
    title: 'Icon',
    key: 'icon',
    width: 60,
  },
  {
    title: 'Name',
    key: 'name',
  },
  {
    title: 'Category',
    key: 'category',
    width: 120,
  },
  {
    title: 'Status',
    key: 'status',
    width: 100,
    render(row) {
      return h(
        NSpace,
        {},
        {
          default: () => [
            row.is_official && h(NTag, { type: 'success', size: 'small' }, { default: () => 'Official' }),
            row.is_public && h(NTag, { type: 'info', size: 'small' }, { default: () => 'Public' }),
          ],
        }
      )
    },
  },
  {
    title: 'Actions',
    key: 'actions',
    width: 200,
    render(row) {
      return h(
        NSpace,
        {},
        {
          default: () => [
            h(
              NButton,
              {
                size: 'small',
                onClick: () => editSkill(row),
              },
              { default: () => 'Edit' }
            ),
            h(
              NPopconfirm,
              {
                onPositiveClick: () => handleDelete(row.id),
              },
              {
                trigger: () =>
                  h(
                    NButton,
                    {
                      size: 'small',
                      type: 'error',
                    },
                    { default: () => 'Delete' }
                  ),
                default: () => 'Are you sure to delete this skill?',
              }
            ),
          ],
        }
      )
    },
  },
]

const openCreateModal = () => {
  isEditing.value = false
  currentSkillId.value = null
  formData.value = {
    name: '',
    description: '',
    icon: '📝',
    category: '',
    prompt: '',
    parameters: [],
    is_public: false,
  }
  showEditorModal.value = true
}

const editSkill = (skill: Skill) => {
  isEditing.value = true
  currentSkillId.value = skill.id
  formData.value = {
    name: skill.name,
    description: skill.description,
    icon: skill.icon,
    category: skill.category,
    prompt: skill.prompt,
    parameters: skill.parameters || [],
    is_public: skill.is_public,
  }
  showEditorModal.value = true
}

const handleSubmit = async () => {
  await formRef.value?.validate()

  try {
    if (isEditing.value && currentSkillId.value) {
      await updateSkill(currentSkillId.value, formData.value)
      message.success('Skill updated successfully')
    } else {
      await createSkill(formData.value)
      message.success('Skill created successfully')
    }

    showEditorModal.value = false
    loadSkills()
  } catch (error) {
    message.error('Failed to save skill')
    console.error(error)
  }
}

const handleDelete = async (id: number) => {
  try {
    await deleteSkill(id)
    message.success('Skill deleted successfully')
    loadSkills()
  } catch (error) {
    message.error('Failed to delete skill')
    console.error(error)
  }
}

const loadSkills = async () => {
  loading.value = true
  try {
    const allSkills = await getSkills()
    // Filter to show only user's own skills (not official or others' public skills)
    mySkills.value = allSkills.filter((s) => !s.is_official && s.created_by === 1) // TODO: Use actual user ID
  } catch (error) {
    message.error('Failed to load skills')
    console.error(error)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadSkills()
})
</script>

<style scoped>
.skill-editor {
  height: 100%;
  overflow-y: auto;
}
</style>

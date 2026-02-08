<template>
  <div class="marketplace">
    <!-- Header -->
    <div class="marketplace-header">
      <n-space align="center" :size="12">
        <n-button quaternary circle @click="$emit('close')">
          <template #icon>
            <n-icon><ArrowBack /></n-icon>
          </template>
        </n-button>
        <n-text strong style="font-size: 20px">Skill Marketplace</n-text>
      </n-space>
      <n-button quaternary circle @click="$emit('close')">
        <template #icon>
          <n-icon><CloseOutline /></n-icon>
        </template>
      </n-button>
    </div>

    <!-- Search + Filter -->
    <div class="marketplace-filters">
      <n-input
        v-model:value="searchQuery"
        placeholder="Search skills..."
        clearable
        size="large"
        class="search-input"
      >
        <template #prefix>
          <n-icon :component="SearchIcon" />
        </template>
      </n-input>

      <n-space :size="8" style="margin-top: 12px" wrap>
        <n-button
          v-for="cat in allCategories"
          :key="cat"
          size="small"
          :type="selectedCategory === cat ? 'primary' : 'default'"
          :secondary="selectedCategory === cat"
          round
          @click="selectedCategory = selectedCategory === cat ? '' : cat"
        >
          {{ cat }}
        </n-button>
      </n-space>
    </div>

    <!-- Tabs -->
    <n-tabs v-model:value="activeTab" type="line" class="marketplace-tabs">
      <n-tab-pane name="browse" tab="Browse Marketplace">
        <n-spin :show="loading">
          <n-empty v-if="filteredBrowseSkills.length === 0 && !loading" description="No skills found" />
          <div v-else class="skill-grid">
            <SkillCard
              v-for="skill in filteredBrowseSkills"
              :key="skill.id"
              :skill="skill"
              :installed="isInstalled(skill.id)"
              :installing="installingId === skill.id"
              :can-install="!!agentId"
              :owned="isOwned(skill)"
              @install="handleInstall"
              @view="openDetail"
              @edit="openEditor(skill)"
              @delete="handleDelete(skill)"
              @fork="handleFork(skill)"
            />
          </div>
        </n-spin>
      </n-tab-pane>

      <n-tab-pane name="my" tab="My Skills">
        <div style="margin-bottom: 16px">
          <n-button type="primary" @click="openEditor()">
            + Create Skill
          </n-button>
        </div>
        <n-spin :show="loading">
          <n-empty v-if="filteredMySkills.length === 0 && !loading" description="You haven't created any skills yet" />
          <div v-else class="skill-grid">
            <SkillCard
              v-for="skill in filteredMySkills"
              :key="skill.id"
              :skill="skill"
              :installed="isInstalled(skill.id)"
              :installing="installingId === skill.id"
              :can-install="!!agentId"
              :owned="true"
              @install="handleInstall"
              @view="openDetail"
              @edit="openEditor(skill)"
              @delete="handleDelete(skill)"
              @fork="handleFork(skill)"
            />
          </div>
        </n-spin>
      </n-tab-pane>
    </n-tabs>

    <!-- Detail Drawer -->
    <n-drawer v-model:show="showDetail" :width="480" placement="right">
      <n-drawer-content v-if="detailSkill" :title="detailSkill.name" closable>
        <n-space vertical :size="16">
          <n-space align="center" :size="12">
            <span style="font-size: 48px">{{ detailSkill.icon }}</span>
            <div>
              <n-text strong style="font-size: 18px">{{ detailSkill.name }}</n-text>
              <br />
              <n-text depth="3" code style="font-size: 13px">/{{ detailSkill.command_name }}</n-text>
            </div>
          </n-space>

          <n-space :size="6">
            <n-tag v-if="detailSkill.is_official" type="success" size="small" round>Official</n-tag>
            <n-tag v-if="detailSkill.is_public" type="info" size="small" round>Public</n-tag>
            <n-tag v-if="detailSkill.category" size="small" :bordered="false">{{ detailSkill.category }}</n-tag>
          </n-space>

          <div>
            <n-text strong style="display: block; margin-bottom: 8px">Description</n-text>
            <n-text depth="2">{{ detailSkill.description || 'No description' }}</n-text>
          </div>

          <div>
            <n-text strong style="display: block; margin-bottom: 8px">Prompt Preview</n-text>
            <n-scrollbar style="max-height: 300px">
              <pre class="prompt-preview">{{ detailSkill.prompt }}</pre>
            </n-scrollbar>
          </div>

          <div v-if="detailSkill.parameters && detailSkill.parameters.length > 0">
            <n-text strong style="display: block; margin-bottom: 8px">Parameters</n-text>
            <n-descriptions bordered :column="1" size="small">
              <n-descriptions-item
                v-for="param in detailSkill.parameters"
                :key="param.name"
                :label="param.label || param.name"
              >
                <n-space :size="6">
                  <n-tag size="tiny">{{ param.type }}</n-tag>
                  <n-tag v-if="param.required" size="tiny" type="warning">required</n-tag>
                  <n-text v-if="param.default_value" depth="3">default: {{ param.default_value }}</n-text>
                </n-space>
              </n-descriptions-item>
            </n-descriptions>
          </div>
        </n-space>

        <template #footer>
          <n-space>
            <n-tooltip v-if="!agentId" trigger="hover">
              <template #trigger>
                <n-button disabled>Install</n-button>
              </template>
              Select an agent first
            </n-tooltip>
            <n-button
              v-else-if="isInstalled(detailSkill.id)"
              disabled
            >
              Installed
            </n-button>
            <n-button
              v-else
              type="primary"
              :loading="installingId === detailSkill.id"
              @click="handleInstall(detailSkill)"
            >
              Install to Agent
            </n-button>

            <n-button
              v-if="!isOwned(detailSkill) && !detailSkill.is_official"
              @click="handleFork(detailSkill)"
            >
              Fork
            </n-button>
          </n-space>
        </template>
      </n-drawer-content>
    </n-drawer>

    <!-- Create / Edit Modal -->
    <n-modal
      v-model:show="showEditor"
      preset="card"
      :title="editingSkill ? 'Edit Skill' : 'Create New Skill'"
      style="width: 800px; max-width: 90vw"
      :segmented="{ content: 'soft', footer: 'soft' }"
    >
      <n-form
        ref="formRef"
        :model="formData"
        :rules="formRules"
        label-placement="left"
        label-width="120"
      >
        <n-form-item label="Name" path="name">
          <n-input v-model:value="formData.name" placeholder="Skill name" />
        </n-form-item>

        <n-form-item label="Command" path="command_name">
          <n-space vertical style="width: 100%">
            <n-input
              v-model:value="formData.command_name"
              placeholder="e.g. sac-query (auto-generated if empty)"
            >
              <template #prefix>/</template>
            </n-input>
            <n-text depth="3" style="font-size: 12px">
              The slash command name. Use lowercase letters, numbers, and hyphens only.
            </n-text>
          </n-space>
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
          <n-select
            v-model:value="formData.icon"
            :options="iconOptions"
            placeholder="Select an emoji icon"
            :render-label="renderIconLabel"
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
          <n-space vertical style="width: 100%">
            <n-input
              v-model:value="formData.prompt"
              type="textarea"
              placeholder="Write your skill prompt in Markdown format.&#10;&#10;Example:&#10;# Task&#10;Help the user analyze data and create visualizations."
              :autosize="{ minRows: 12, maxRows: 30 }"
            />
            <n-text depth="3" style="font-size: 12px">
              Write the prompt in Markdown. This becomes a Claude Code slash command. Use $ARGUMENTS for parameter values.
            </n-text>
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
          <n-button @click="showEditor = false">Cancel</n-button>
          <n-button type="primary" :loading="saving" @click="handleSubmit">
            {{ editingSkill ? 'Update' : 'Create' }}
          </n-button>
        </n-space>
      </template>
    </n-modal>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, h } from 'vue'
import {
  NSpace,
  NText,
  NTag,
  NIcon,
  NInput,
  NButton,
  NTabs,
  NTabPane,
  NDrawer,
  NDrawerContent,
  NModal,
  NForm,
  NFormItem,
  NSelect,
  NSwitch,
  NEmpty,
  NSpin,
  NScrollbar,
  NDescriptions,
  NDescriptionsItem,
  NTooltip,
  useMessage,
} from 'naive-ui'
import { Search as SearchIcon, ArrowBack, CloseOutline } from '@vicons/ionicons5'
import SkillCard from './SkillCard.vue'
import {
  getSkills,
  createSkill,
  updateSkill,
  deleteSkill,
  forkSkill,
  type Skill,
  type SkillParameter,
} from '../../services/skillAPI'
import { installSkill, type Agent } from '../../services/agentAPI'

const props = defineProps<{
  agentId: number
  agent: Agent | null
  installedSkills?: any[]
}>()

const emit = defineEmits<{
  skillsChanged: []
  close: []
}>()

const message = useMessage()

// --- State ---
const loading = ref(false)
const allSkills = ref<Skill[]>([])
const searchQuery = ref('')
const selectedCategory = ref('')
const activeTab = ref('browse')
const installingId = ref<number | null>(null)

// Detail drawer
const showDetail = ref(false)
const detailSkill = ref<Skill | null>(null)

// Editor modal
const showEditor = ref(false)
const editingSkill = ref<Skill | null>(null)
const saving = ref(false)
const formRef = ref()
const formData = ref(emptyForm())

// --- Computed ---
const installedSkillIds = computed(() => {
  if (!props.installedSkills) return new Set<number>()
  return new Set(
    props.installedSkills
      .map((as: any) => as.skill?.id ?? as.skill_id)
      .filter((id: any) => id != null)
  )
})

const allCategories = computed(() => {
  const cats = new Set<string>()
  allSkills.value.forEach(s => {
    if (s.category) cats.add(s.category)
  })
  return ['All', ...Array.from(cats)]
})

const browseSkills = computed(() =>
  allSkills.value.filter(s => s.is_official || s.is_public || isOwned(s))
)

const mySkills = computed(() =>
  allSkills.value.filter(s => isOwned(s))
)

const filteredBrowseSkills = computed(() => filterSkills(browseSkills.value))
const filteredMySkills = computed(() => filterSkills(mySkills.value))

// --- Helpers ---
function filterSkills(skills: Skill[]) {
  let result = skills
  if (selectedCategory.value && selectedCategory.value !== 'All') {
    result = result.filter(s => s.category === selectedCategory.value)
  }
  if (searchQuery.value) {
    const q = searchQuery.value.toLowerCase()
    result = result.filter(
      s => s.name.toLowerCase().includes(q) ||
           s.description.toLowerCase().includes(q) ||
           s.command_name.toLowerCase().includes(q)
    )
  }
  return result
}

function isInstalled(skillId: number) {
  return installedSkillIds.value.has(skillId)
}

function isOwned(skill: Skill) {
  return !skill.is_official && skill.created_by === 1 // TODO: Use actual user ID
}

function emptyForm() {
  return {
    name: '',
    description: '',
    icon: '📝',
    category: '',
    prompt: '',
    command_name: '',
    parameters: [] as SkillParameter[],
    is_public: false,
  }
}

// --- Actions ---
async function loadSkills() {
  loading.value = true
  try {
    allSkills.value = await getSkills()
  } catch (error) {
    message.error('Failed to load skills')
    console.error(error)
  } finally {
    loading.value = false
  }
}

async function handleInstall(skill: Skill) {
  if (!props.agentId) return
  installingId.value = skill.id
  try {
    await installSkill(props.agentId, skill.id)
    message.success('Skill installed')
    emit('skillsChanged')
  } catch (error) {
    message.error('Failed to install skill')
    console.error(error)
  } finally {
    installingId.value = null
  }
}

async function handleDelete(skill: Skill) {
  try {
    await deleteSkill(skill.id)
    message.success('Skill deleted')
    loadSkills()
  } catch (error) {
    message.error('Failed to delete skill')
    console.error(error)
  }
}

async function handleFork(skill: Skill) {
  try {
    await forkSkill(skill.id)
    message.success('Skill forked to My Skills')
    loadSkills()
  } catch (error: any) {
    const msg = error?.response?.data?.error || 'Failed to fork skill'
    message.error(msg)
    console.error(error)
  }
}

function openDetail(skill: Skill) {
  detailSkill.value = skill
  showDetail.value = true
}

function openEditor(skill?: Skill) {
  if (skill) {
    editingSkill.value = skill
    formData.value = {
      name: skill.name,
      description: skill.description,
      icon: skill.icon,
      category: skill.category,
      prompt: skill.prompt,
      command_name: skill.command_name || '',
      parameters: skill.parameters || [],
      is_public: skill.is_public,
    }
  } else {
    editingSkill.value = null
    formData.value = emptyForm()
  }
  showEditor.value = true
}

async function handleSubmit() {
  await formRef.value?.validate()
  saving.value = true
  try {
    if (editingSkill.value) {
      await updateSkill(editingSkill.value.id, formData.value)
      message.success('Skill updated')
    } else {
      await createSkill(formData.value)
      message.success('Skill created')
    }
    showEditor.value = false
    loadSkills()
  } catch (error: any) {
    const msg = error?.response?.data?.error || 'Failed to save skill'
    message.error(msg)
    console.error(error)
  } finally {
    saving.value = false
  }
}

// --- Form options ---
const categoryOptions = [
  { label: '数据查询', value: '数据查询' },
  { label: '数据分析', value: '数据分析' },
  { label: '报表生成', value: '报表生成' },
  { label: '数据处理', value: '数据处理' },
  { label: '其他', value: '其他' },
]

const iconOptions = [
  { label: '💰 Money', value: '💰' },
  { label: '📈 Chart', value: '📈' },
  { label: '📊 Bar Chart', value: '📊' },
  { label: '📦 Package', value: '📦' },
  { label: '🎯 Target', value: '🎯' },
  { label: '📅 Calendar', value: '📅' },
  { label: '📝 Note', value: '📝' },
  { label: '🔍 Search', value: '🔍' },
  { label: '⚙️ Settings', value: '⚙️' },
  { label: '📧 Email', value: '📧' },
  { label: '📱 Phone', value: '📱' },
  { label: '💡 Bulb', value: '💡' },
  { label: '🚀 Rocket', value: '🚀' },
  { label: '⭐ Star', value: '⭐' },
  { label: '✅ Check', value: '✅' },
  { label: '❌ Cross', value: '❌' },
  { label: '📂 Folder', value: '📂' },
  { label: '📄 Document', value: '📄' },
  { label: '🔔 Bell', value: '🔔' },
  { label: '🎨 Palette', value: '🎨' },
  { label: '🛠️ Tools', value: '🛠️' },
  { label: '📌 Pin', value: '📌' },
  { label: '🔗 Link', value: '🔗' },
  { label: '💬 Chat', value: '💬' },
  { label: '📸 Camera', value: '📸' },
  { label: '🎬 Movie', value: '🎬' },
  { label: '🌟 Sparkle', value: '🌟' },
  { label: '🔥 Fire', value: '🔥' },
  { label: '⚡ Lightning', value: '⚡' },
  { label: '🎁 Gift', value: '🎁' },
]

const formRules = {
  name: { required: true, message: 'Please input skill name', trigger: 'blur' },
  command_name: {
    trigger: 'blur',
    validator(_rule: any, value: string) {
      if (value && !/^[a-z0-9]+(-[a-z0-9]+)*$/.test(value)) {
        return new Error('Only lowercase letters, numbers, and hyphens allowed')
      }
      return true
    },
  },
  category: { required: true, message: 'Please select category', trigger: 'change' },
  prompt: { required: true, message: 'Please input prompt', trigger: 'blur' },
}

const renderIconLabel = (option: any) => {
  return h('div', { style: 'display: flex; align-items: center; gap: 8px; font-size: 16px;' }, [
    h('span', { style: 'font-size: 20px;' }, option.value),
    h('span', option.label.substring(3)),
  ])
}

onMounted(() => {
  loadSkills()
})
</script>

<style scoped>
.marketplace {
  height: 100%;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.marketplace-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 16px 24px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.08);
  flex-shrink: 0;
}

.marketplace-filters {
  padding: 16px 24px 8px;
  flex-shrink: 0;
}

.search-input {
  max-width: 600px;
}

.marketplace-tabs {
  flex: 1;
  overflow: hidden;
  display: flex;
  flex-direction: column;
  padding: 0 24px;
}

.marketplace-tabs :deep(.n-tabs-nav) {
  flex-shrink: 0;
}

.marketplace-tabs :deep(.n-tab-pane) {
  flex: 1;
  overflow-y: auto;
  padding-bottom: 24px;
}

.skill-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 16px;
  padding: 4px 0;
}

.prompt-preview {
  font-family: monospace;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
  margin: 0;
  padding: 12px;
  background: rgba(0, 0, 0, 0.2);
  border-radius: 6px;
  color: rgba(255, 255, 255, 0.7);
}
</style>

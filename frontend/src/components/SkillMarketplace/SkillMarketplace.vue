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

    <!-- Skill Preview (readonly editor) -->
    <SkillEditor
      v-if="showDetail && detailSkill"
      :skill="detailSkill"
      :readonly="true"
      @close="showDetail = false"
    />

    <!-- Skill Editor Overlay -->
    <SkillEditor
      v-if="showEditor"
      :skill="editingSkill"
      @close="showEditor = false"
      @saved="handleEditorSaved"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import {
  NSpace,
  NText,
  NIcon,
  NInput,
  NButton,
  NTabs,
  NTabPane,
  NEmpty,
  NSpin,
  useMessage,
} from 'naive-ui'
import { Search as SearchIcon, ArrowBack, CloseOutline } from '@vicons/ionicons5'
import SkillCard from './SkillCard.vue'
import SkillEditor from './SkillEditor.vue'
import {
  getSkills,
  deleteSkill,
  forkSkill,
  type Skill,
} from '../../services/skillAPI'
import { installSkill, type Agent } from '../../services/agentAPI'
import { extractApiError } from '../../utils/error'
import { useAuthStore } from '../../stores/auth'

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
const authStore = useAuthStore()

// --- State ---
const loading = ref(false)
const allSkills = ref<Skill[]>([])
const searchQuery = ref('')
const activeTab = ref('browse')
const installingId = ref<number | null>(null)

// Detail drawer
const showDetail = ref(false)
const detailSkill = ref<Skill | null>(null)

// Editor
const showEditor = ref(false)
const editingSkill = ref<Skill | null>(null)

// --- Computed ---
const installedSkillIds = computed(() => {
  if (!props.installedSkills) return new Set<number>()
  return new Set(
    props.installedSkills
      .map((as: any) => as.skill?.id ?? as.skill_id)
      .filter((id: any) => id != null)
  )
})

const browseSkills = computed(() =>
  allSkills.value.filter(s => s.is_official || s.is_public || s.group_id || isOwned(s))
)

const mySkills = computed(() =>
  allSkills.value.filter(s => isOwned(s))
)

const filteredBrowseSkills = computed(() => filterSkills(browseSkills.value))
const filteredMySkills = computed(() => filterSkills(mySkills.value))

// --- Helpers ---
function filterSkills(skills: Skill[]) {
  if (!searchQuery.value) return skills
  const q = searchQuery.value.toLowerCase()
  return skills.filter(
    s => s.name.toLowerCase().includes(q) ||
         s.description.toLowerCase().includes(q) ||
         s.command_name.toLowerCase().includes(q)
  )
}

function isInstalled(skillId: number) {
  return installedSkillIds.value.has(skillId)
}

function isOwned(skill: Skill) {
  return !skill.is_official && Number(skill.created_by) === Number(authStore.userId)
}

// --- Actions ---
async function loadSkills() {
  loading.value = true
  try {
    allSkills.value = await getSkills()
  } catch (error) {
    message.error(extractApiError(error, 'Failed to load skills'))
    console.error(error)
  } finally {
    loading.value = false
  }
}

async function handleInstall(skill: Skill) {
  if (!props.agentId) {
    message.warning('Please select an agent first')
    return
  }
  if (isInstalled(skill.id)) {
    message.info('This skill is already installed on the current agent')
    return
  }
  installingId.value = skill.id
  try {
    await installSkill(props.agentId, skill.id)
    message.success('Skill installed successfully')
    emit('skillsChanged')
  } catch (error) {
    message.error(extractApiError(error, 'Failed to install skill'))
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
    message.error(extractApiError(error, 'Failed to delete skill'))
    console.error(error)
  }
}

async function handleFork(skill: Skill) {
  try {
    const forked = await forkSkill(skill.id)
    message.success('Skill forked to My Skills')
    if (props.agentId && forked?.id) {
      try {
        await installSkill(props.agentId, forked.id)
        message.success('Forked skill installed to agent')
        emit('skillsChanged')
      } catch (installErr) {
        console.error('Auto-install after fork failed:', installErr)
      }
    }
    loadSkills()
  } catch (error) {
    message.error(extractApiError(error, 'Failed to fork skill'))
    console.error(error)
  }
}

function openDetail(skill: Skill) {
  detailSkill.value = skill
  showDetail.value = true
}

function openEditor(skill?: Skill) {
  editingSkill.value = skill ?? null
  showEditor.value = true
}

function handleEditorSaved() {
  loadSkills()
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
</style>

<template>
  <n-config-provider :theme="darkTheme">
    <n-layout style="height: 100vh">
      <n-layout-header bordered style="height: 60px; padding: 0 24px; display: flex; align-items: center; justify-content: space-between;">
        <div style="display: flex; align-items: center; gap: 12px">
          <router-link to="/" style="text-decoration: none; display: flex; align-items: center;">
            <img :src="sacLogo" alt="SAC" class="logo" />
          </router-link>
          <span class="subtitle">Admin Panel</span>
        </div>
        <n-button quaternary @click="$router.push('/')">Back to Dashboard</n-button>
      </n-layout-header>

      <n-layout-content style="padding: 24px; height: calc(100vh - 60px); overflow-y: auto;">
        <n-tabs v-model:value="activeTab" type="line" size="large">
          <!-- System Settings Tab -->
          <n-tab-pane name="settings" tab="System Settings">
            <n-spin :show="loadingSettings">
              <n-card title="Default Resource Limits" style="margin-bottom: 16px">
                <n-table :bordered="false" :single-line="false">
                  <thead>
                    <tr>
                      <th>Setting</th>
                      <th>Value</th>
                      <th>Description</th>
                      <th style="width: 100px">Action</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="setting in settings" :key="setting.key">
                      <td><n-text code>{{ setting.key }}</n-text></td>
                      <td>
                        <n-input
                          v-model:value="settingEdits[setting.key]"
                          size="small"
                          style="max-width: 200px"
                        />
                      </td>
                      <td><n-text depth="3">{{ setting.description }}</n-text></td>
                      <td>
                        <n-button
                          size="small"
                          type="primary"
                          :disabled="settingEdits[setting.key] === formatValue(setting.value)"
                          :loading="savingSetting === setting.key"
                          @click="saveSetting(setting.key)"
                        >
                          Save
                        </n-button>
                      </td>
                    </tr>
                  </tbody>
                </n-table>
              </n-card>
            </n-spin>
          </n-tab-pane>

          <!-- User Management Tab -->
          <n-tab-pane name="users" tab="User Management">
            <n-spin :show="loadingUsers">
              <n-data-table
                :columns="userColumns"
                :data="users"
                :bordered="false"
                :single-line="false"
              />
            </n-spin>

            <!-- User Settings Modal -->
            <n-modal
              v-model:show="showUserSettings"
              preset="card"
              :title="`Resource Settings: ${selectedUser?.username || ''}`"
              style="width: 600px; max-width: 90vw"
            >
              <n-spin :show="loadingUserSettings">
                <n-table :bordered="false" :single-line="false" v-if="userSettingsList.length > 0">
                  <thead>
                    <tr>
                      <th>Setting</th>
                      <th>Override Value</th>
                      <th style="width: 140px">Action</th>
                    </tr>
                  </thead>
                  <tbody>
                    <tr v-for="us in userSettingsList" :key="us.key">
                      <td><n-text code>{{ us.key }}</n-text></td>
                      <td>
                        <n-input v-model:value="userSettingEdits[us.key]" size="small" style="max-width: 200px" />
                      </td>
                      <td>
                        <n-space :size="4">
                          <n-button
                            size="small"
                            type="primary"
                            :disabled="userSettingEdits[us.key] === formatValue(us.value)"
                            @click="saveUserSettingValue(us.key)"
                          >
                            Save
                          </n-button>
                          <n-popconfirm @positive-click="removeUserSetting(us.key)">
                            <template #trigger>
                              <n-button size="small" type="error" quaternary>Reset</n-button>
                            </template>
                            Reset to system default?
                          </n-popconfirm>
                        </n-space>
                      </td>
                    </tr>
                  </tbody>
                </n-table>
                <n-empty v-else description="No custom overrides. Using system defaults." />

                <n-divider />
                <n-text strong>Add Override</n-text>
                <n-space style="margin-top: 12px" :size="8">
                  <n-select
                    v-model:value="newOverrideKey"
                    :options="availableOverrideKeys"
                    placeholder="Select setting"
                    style="width: 220px"
                    size="small"
                  />
                  <n-input v-model:value="newOverrideValue" placeholder="Value" size="small" style="width: 160px" />
                  <n-button size="small" type="primary" :disabled="!newOverrideKey || !newOverrideValue" @click="addUserOverride">
                    Add
                  </n-button>
                </n-space>
              </n-spin>
            </n-modal>

            <!-- User Agents Modal -->
            <n-modal
              v-model:show="showUserAgents"
              preset="card"
              :title="`Agents: ${selectedAgentUser?.username || ''}`"
              style="width: 1000px; max-width: 95vw"
            >
              <template #header-extra>
                <n-button size="small" type="warning" @click="showBatchImageUpdate = true; batchImageForm = ''">
                  Batch Update Image
                </n-button>
              </template>
              <n-spin :show="loadingUserAgents">
                <n-data-table
                  :columns="agentColumns"
                  :data="userAgents"
                  :bordered="false"
                  :single-line="false"
                  :scroll-x="1060"
                  v-if="userAgents.length > 0"
                />
                <n-empty v-else description="No agents found for this user." />
              </n-spin>
            </n-modal>

            <!-- Agent Resource Editor Modal -->
            <n-modal
              v-model:show="showResourceEditor"
              preset="card"
              :title="`Resources: ${selectedResourceAgent?.name || ''}`"
              style="width: 480px; max-width: 90vw"
            >
              <n-space vertical :size="16">
                <div>
                  <n-text depth="3" style="display: block; margin-bottom: 4px">CPU Request</n-text>
                  <n-input v-model:value="resourceForm.cpu_request" placeholder="e.g. 1 (use default)" />
                </div>
                <div>
                  <n-text depth="3" style="display: block; margin-bottom: 4px">CPU Limit</n-text>
                  <n-input v-model:value="resourceForm.cpu_limit" placeholder="e.g. 2 (use default)" />
                </div>
                <div>
                  <n-text depth="3" style="display: block; margin-bottom: 4px">Memory Request</n-text>
                  <n-input v-model:value="resourceForm.memory_request" placeholder="e.g. 2Gi (use default)" />
                </div>
                <div>
                  <n-text depth="3" style="display: block; margin-bottom: 4px">Memory Limit</n-text>
                  <n-input v-model:value="resourceForm.memory_limit" placeholder="e.g. 4Gi (use default)" />
                </div>
                <n-text depth="3" style="font-size: 12px">
                  Leave empty to use user/system defaults. Changes take effect after restarting the agent.
                </n-text>
                <n-button type="primary" block :loading="savingResources" @click="saveAgentResources">
                  Save
                </n-button>
              </n-space>
            </n-modal>
            <!-- Agent Image Editor Modal -->
            <n-modal
              v-model:show="showImageEditor"
              preset="card"
              :title="`Image: ${selectedImageAgent?.name || ''}`"
              style="width: 520px; max-width: 90vw"
            >
              <n-space vertical :size="16">
                <div>
                  <n-text depth="3" style="display: block; margin-bottom: 4px">Docker Image (full path with tag)</n-text>
                  <n-input v-model:value="imageForm" placeholder="e.g. registry/repo:tag" />
                </div>
                <n-text depth="3" style="font-size: 12px">
                  Updating the image will trigger a rolling update of the pod. Active sessions will be disconnected.
                </n-text>
                <n-button type="primary" block :loading="savingImage" @click="saveAgentImage">
                  Update Image
                </n-button>
              </n-space>
            </n-modal>

            <!-- Batch Update Image Modal -->
            <n-modal
              v-model:show="showBatchImageUpdate"
              preset="card"
              title="Batch Update All Agent Images"
              style="width: 520px; max-width: 90vw"
            >
              <n-space vertical :size="16">
                <div>
                  <n-text depth="3" style="display: block; margin-bottom: 4px">New Docker Image for ALL agents</n-text>
                  <n-input v-model:value="batchImageForm" placeholder="e.g. registry/repo:tag" />
                </div>
                <n-text depth="3" style="font-size: 12px">
                  This will update the image for ALL deployed StatefulSets. Pods will be rolling-updated and all active sessions disconnected.
                </n-text>
                <n-button type="warning" block :loading="savingBatchImage" @click="doBatchUpdateImage">
                  Update All
                </n-button>
              </n-space>
            </n-modal>
          </n-tab-pane>

          <!-- Conversations Tab -->
          <n-tab-pane name="conversations" tab="Conversations">
            <n-card style="margin-bottom: 16px">
              <n-space :size="12" align="center" :wrap="true">
                <n-select
                  v-model:value="convFilterUser"
                  :options="userOptions"
                  placeholder="All Users"
                  clearable
                  style="width: 180px"
                  size="small"
                  @update:value="onUserFilterChange"
                />
                <n-select
                  v-model:value="convFilterAgent"
                  :options="agentOptions"
                  placeholder="All Agents"
                  clearable
                  style="width: 180px"
                  size="small"
                />
                <n-input
                  v-model:value="convFilterSession"
                  placeholder="Session ID"
                  clearable
                  size="small"
                  style="width: 200px"
                />
                <n-date-picker
                  v-model:value="convFilterTimeRange"
                  type="datetimerange"
                  clearable
                  size="small"
                  style="width: 360px"
                  start-placeholder="Start Time"
                  end-placeholder="End Time"
                />
                <n-button type="primary" size="small" @click="searchConversations">
                  Search
                </n-button>
                <n-button size="small" @click="exportCSV" :loading="exportingCSV">
                  Export CSV
                </n-button>
              </n-space>
            </n-card>

            <n-spin :show="loadingConversations">
              <n-data-table
                :columns="conversationColumns"
                :data="conversations"
                :bordered="false"
                :single-line="false"
                :scroll-x="900"
              />
              <n-space justify="center" style="margin-top: 16px" v-if="conversations.length > 0 && hasMoreConversations">
                <n-button @click="loadMoreConversations" :loading="loadingConversations">
                  Load More
                </n-button>
              </n-space>
              <n-empty v-if="!loadingConversations && conversations.length === 0" description="No conversations found." />
            </n-spin>
          </n-tab-pane>
        </n-tabs>
      </n-layout-content>
    </n-layout>
  </n-config-provider>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, h } from 'vue'
import {
  NConfigProvider,
  NLayout,
  NLayoutHeader,
  NLayoutContent,
  NTabs,
  NTabPane,
  NCard,
  NTable,
  NDataTable,
  NInput,
  NButton,
  NText,
  NSpace,
  NSelect,
  NSpin,
  NEmpty,
  NModal,
  NDivider,
  NTag,
  NPopconfirm,
  NDatePicker,
  NTooltip,
  darkTheme,
  useMessage,
  type DataTableColumns,
} from 'naive-ui'
import {
  getSystemSettings,
  updateSystemSetting,
  getUsers,
  updateUserRole,
  getUserSettings,
  updateUserSetting,
  deleteUserSetting,
  getUserAgents,
  deleteUserAgent,
  restartUserAgent,
  updateAgentResources,
  updateAgentImage,
  batchUpdateImage,
  getConversations,
  exportConversationsCSV,
  type SystemSetting,
  type AdminUser,
  type UserSetting,
  type AdminAgent,
  type ConversationRecord,
} from '../services/adminAPI'
import { extractApiError } from '../utils/error'
import sacLogo from '../assets/sac-logo.svg'

const message = useMessage()
const activeTab = ref('settings')

// --- System Settings ---
const settings = ref<SystemSetting[]>([])
const settingEdits = ref<Record<string, string>>({})
const loadingSettings = ref(false)
const savingSetting = ref<string | null>(null)

function formatValue(val: any): string {
  if (typeof val === 'object' && val !== null) return JSON.stringify(val)
  return String(val ?? '')
}

async function loadSettings() {
  loadingSettings.value = true
  try {
    settings.value = await getSystemSettings()
    settingEdits.value = {}
    for (const s of settings.value) {
      settingEdits.value[s.key] = formatValue(s.value)
    }
  } catch (error) {
    message.error(extractApiError(error, 'Failed to load settings'))
  } finally {
    loadingSettings.value = false
  }
}

async function saveSetting(key: string) {
  savingSetting.value = key
  try {
    let value: any = settingEdits.value[key]
    // Try to parse as number
    const num = Number(value)
    if (!isNaN(num) && value.trim() !== '') {
      value = num
    }
    await updateSystemSetting(key, value)
    message.success(`Setting "${key}" updated`)
    await loadSettings()
  } catch (error) {
    message.error(extractApiError(error, 'Failed to save setting'))
  } finally {
    savingSetting.value = null
  }
}

// --- Users ---
const users = ref<AdminUser[]>([])
const loadingUsers = ref(false)

const userColumns = computed<DataTableColumns<AdminUser>>(() => [
  { title: 'Username', key: 'username' },
  { title: 'Email', key: 'email' },
  {
    title: 'Role',
    key: 'role',
    render(row) {
      return h(NTag, {
        type: row.role === 'admin' ? 'warning' : 'default',
        size: 'small',
      }, { default: () => row.role })
    },
  },
  { title: 'Agents', key: 'agent_count' },
  {
    title: 'Created',
    key: 'created_at',
    render(row) {
      return new Date(row.created_at).toLocaleDateString()
    },
  },
  {
    title: 'Actions',
    key: 'actions',
    width: 280,
    render(row) {
      return h(NSpace, { size: 4 }, {
        default: () => [
          h(NButton, {
            size: 'small',
            onClick: () => toggleRole(row),
          }, { default: () => row.role === 'admin' ? 'Demote' : 'Promote' }),
          h(NButton, {
            size: 'small',
            type: 'info',
            onClick: () => openUserSettings(row),
          }, { default: () => 'Settings' }),
          h(NButton, {
            size: 'small',
            type: 'warning',
            onClick: () => openUserAgents(row),
          }, { default: () => 'Agents' }),
        ]
      })
    },
  },
])

async function loadUsers() {
  loadingUsers.value = true
  try {
    users.value = await getUsers()
  } catch (error) {
    message.error(extractApiError(error, 'Failed to load users'))
  } finally {
    loadingUsers.value = false
  }
}

async function toggleRole(user: AdminUser) {
  const newRole = user.role === 'admin' ? 'user' : 'admin'
  try {
    await updateUserRole(user.id, newRole)
    message.success(`User "${user.username}" is now ${newRole}`)
    await loadUsers()
  } catch (error) {
    message.error(extractApiError(error, 'Failed to update role'))
  }
}

// --- User Settings ---
const showUserSettings = ref(false)
const selectedUser = ref<AdminUser | null>(null)
const userSettingsList = ref<UserSetting[]>([])
const userSettingEdits = ref<Record<string, string>>({})
const loadingUserSettings = ref(false)
const newOverrideKey = ref<string | null>(null)
const newOverrideValue = ref('')

const allSettingKeys = [
  'max_agents_per_user',
  'default_cpu_request',
  'default_cpu_limit',
  'default_memory_request',
  'default_memory_limit',
]

const availableOverrideKeys = computed(() => {
  const existing = new Set(userSettingsList.value.map(s => s.key))
  return allSettingKeys
    .filter(k => !existing.has(k))
    .map(k => ({ label: k, value: k }))
})

async function openUserSettings(user: AdminUser) {
  selectedUser.value = user
  showUserSettings.value = true
  loadingUserSettings.value = true
  try {
    userSettingsList.value = (await getUserSettings(user.id)) || []
    userSettingEdits.value = {}
    for (const s of userSettingsList.value) {
      userSettingEdits.value[s.key] = formatValue(s.value)
    }
  } catch (error) {
    message.error(extractApiError(error, 'Failed to load user settings'))
  } finally {
    loadingUserSettings.value = false
  }
}

async function saveUserSettingValue(key: string) {
  if (!selectedUser.value) return
  try {
    let value: any = userSettingEdits.value[key]
    const num = Number(value)
    if (!isNaN(num) && value.trim() !== '') {
      value = num
    }
    await updateUserSetting(selectedUser.value.id, key, value)
    message.success(`User setting "${key}" updated`)
    await openUserSettings(selectedUser.value)
  } catch (error) {
    message.error(extractApiError(error, 'Failed to save user setting'))
  }
}

async function removeUserSetting(key: string) {
  if (!selectedUser.value) return
  try {
    await deleteUserSetting(selectedUser.value.id, key)
    message.success(`User override "${key}" removed`)
    await openUserSettings(selectedUser.value)
  } catch (error) {
    message.error(extractApiError(error, 'Failed to remove setting'))
  }
}

async function addUserOverride() {
  if (!selectedUser.value || !newOverrideKey.value || !newOverrideValue.value) return
  try {
    let value: any = newOverrideValue.value
    const num = Number(value)
    if (!isNaN(num) && value.trim() !== '') {
      value = num
    }
    await updateUserSetting(selectedUser.value.id, newOverrideKey.value, value)
    message.success('Override added')
    newOverrideKey.value = null
    newOverrideValue.value = ''
    await openUserSettings(selectedUser.value)
  } catch (error) {
    message.error(extractApiError(error, 'Failed to add override'))
  }
}

// --- User Agents ---
const showUserAgents = ref(false)
const selectedAgentUser = ref<AdminUser | null>(null)
const userAgents = ref<AdminAgent[]>([])
const loadingUserAgents = ref(false)

const statusColorMap: Record<string, 'success' | 'error' | 'warning' | 'default' | 'info'> = {
  Running: 'success',
  Error: 'error',
  CrashLoopBackOff: 'error',
  ImagePullBackOff: 'error',
  Pending: 'warning',
  NotDeployed: 'default',
  Unknown: 'default',
}

const agentColumns = computed<DataTableColumns<AdminAgent>>(() => [
  { title: 'Name', key: 'name', width: 120 },
  {
    title: 'Description',
    key: 'description',
    ellipsis: { tooltip: true },
  },
  {
    title: 'Status',
    key: 'pod_status',
    width: 100,
    render(row) {
      return h(NTag, {
        type: statusColorMap[row.pod_status] || 'default',
        size: 'small',
      }, { default: () => row.pod_status || 'Unknown' })
    },
  },
  {
    title: 'Image',
    key: 'image',
    width: 100,
    render(row) {
      if (!row.image) return '-'
      const tag = row.image.split(':').pop() || row.image
      return h(NTooltip, {}, {
        trigger: () => h(NText, { code: true, style: 'font-size: 12px' }, { default: () => tag }),
        default: () => row.image,
      })
    },
  },
  {
    title: 'Resources',
    key: 'resources',
    width: 160,
    render(row) {
      if (!row.cpu_request && !row.memory_request) return '-'
      return h('div', { style: 'font-size: 12px; line-height: 1.4' }, [
        h('div', `CPU: ${row.cpu_request || '-'} / ${row.cpu_limit || '-'}`),
        h('div', `Mem: ${row.memory_request || '-'} / ${row.memory_limit || '-'}`),
      ])
    },
  },
  { title: 'Restarts', key: 'restart_count', width: 70 },
  {
    title: 'Skills',
    key: 'skills',
    width: 60,
    render(row) {
      return row.installed_skills ? row.installed_skills.length : 0
    },
  },
  {
    title: 'Actions',
    key: 'actions',
    width: 280,
    render(row) {
      return h(NSpace, { size: 4 }, {
        default: () => [
          h(NButton, {
            size: 'small',
            type: 'info',
            onClick: () => openResourceEditor(row),
          }, { default: () => 'Resources' }),
          h(NButton, {
            size: 'small',
            type: 'warning',
            disabled: row.pod_status === 'NotDeployed',
            onClick: () => openImageEditor(row),
          }, { default: () => 'Image' }),
          h(NButton, {
            size: 'small',
            disabled: row.pod_status === 'NotDeployed',
            onClick: () => handleRestartAgent(row),
          }, { default: () => 'Restart' }),
          h(NPopconfirm, {
            onPositiveClick: () => handleDeleteAgent(row),
          }, {
            trigger: () => h(NButton, {
              size: 'small',
              type: 'error',
              quaternary: true,
            }, { default: () => 'Delete' }),
            default: () => `Delete agent "${row.name}"?`,
          }),
        ]
      })
    },
  },
])

async function openUserAgents(user: AdminUser) {
  selectedAgentUser.value = user
  showUserAgents.value = true
  await loadUserAgents()
}

async function loadUserAgents() {
  if (!selectedAgentUser.value) return
  loadingUserAgents.value = true
  try {
    userAgents.value = await getUserAgents(selectedAgentUser.value.id)
  } catch (error) {
    message.error(extractApiError(error, 'Failed to load agents'))
  } finally {
    loadingUserAgents.value = false
  }
}

async function handleDeleteAgent(agent: AdminAgent) {
  if (!selectedAgentUser.value) return
  try {
    await deleteUserAgent(selectedAgentUser.value.id, agent.id)
    message.success(`Agent "${agent.name}" deleted`)
    await loadUserAgents()
    await loadUsers()
  } catch (error) {
    message.error(extractApiError(error, 'Failed to delete agent'))
  }
}

async function handleRestartAgent(agent: AdminAgent) {
  if (!selectedAgentUser.value) return
  try {
    await restartUserAgent(selectedAgentUser.value.id, agent.id)
    message.success(`Agent "${agent.name}" is restarting`)
    await loadUserAgents()
  } catch (error) {
    message.error(extractApiError(error, 'Failed to restart agent'))
  }
}

// --- Agent Resource Editor ---
const showResourceEditor = ref(false)
const selectedResourceAgent = ref<AdminAgent | null>(null)
const resourceForm = ref({
  cpu_request: '',
  cpu_limit: '',
  memory_request: '',
  memory_limit: '',
})
const savingResources = ref(false)

function openResourceEditor(agent: AdminAgent) {
  selectedResourceAgent.value = agent
  resourceForm.value = {
    cpu_request: agent.cpu_request || '',
    cpu_limit: agent.cpu_limit || '',
    memory_request: agent.memory_request || '',
    memory_limit: agent.memory_limit || '',
  }
  showResourceEditor.value = true
}

async function saveAgentResources() {
  if (!selectedAgentUser.value || !selectedResourceAgent.value) return
  savingResources.value = true
  try {
    const agent = selectedResourceAgent.value
    // Send empty string to clear (backend sets NULL), non-empty to override
    await updateAgentResources(selectedAgentUser.value.id, agent.id, {
      cpu_request: resourceForm.value.cpu_request,
      cpu_limit: resourceForm.value.cpu_limit,
      memory_request: resourceForm.value.memory_request,
      memory_limit: resourceForm.value.memory_limit,
    })
    message.success('Resources saved. Restart agent to apply.')
    showResourceEditor.value = false
    await loadUserAgents()
  } catch (error) {
    message.error(extractApiError(error, 'Failed to save resources'))
  } finally {
    savingResources.value = false
  }
}

// --- Agent Image Editor ---
const showImageEditor = ref(false)
const selectedImageAgent = ref<AdminAgent | null>(null)
const imageForm = ref('')
const savingImage = ref(false)

function openImageEditor(agent: AdminAgent) {
  selectedImageAgent.value = agent
  imageForm.value = agent.image || ''
  showImageEditor.value = true
}

async function saveAgentImage() {
  if (!selectedAgentUser.value || !selectedImageAgent.value || !imageForm.value.trim()) return
  savingImage.value = true
  try {
    await updateAgentImage(selectedAgentUser.value.id, selectedImageAgent.value.id, imageForm.value.trim())
    message.success('Agent image updated. Pod is restarting.')
    showImageEditor.value = false
    await loadUserAgents()
  } catch (error) {
    message.error(extractApiError(error, 'Failed to update image'))
  } finally {
    savingImage.value = false
  }
}

// --- Batch Update Image ---
const showBatchImageUpdate = ref(false)
const batchImageForm = ref('')
const savingBatchImage = ref(false)

async function doBatchUpdateImage() {
  if (!batchImageForm.value.trim()) return
  savingBatchImage.value = true
  try {
    const result = await batchUpdateImage(batchImageForm.value.trim())
    message.success(`Batch update: ${result.updated} updated, ${result.failed} failed out of ${result.total}`)
    showBatchImageUpdate.value = false
    await loadUserAgents()
  } catch (error) {
    message.error(extractApiError(error, 'Failed to batch update images'))
  } finally {
    savingBatchImage.value = false
  }
}

// --- Conversations ---
const conversations = ref<ConversationRecord[]>([])
const loadingConversations = ref(false)
const hasMoreConversations = ref(false)
const exportingCSV = ref(false)
const convFilterUser = ref<number | null>(null)
const convFilterAgent = ref<number | null>(null)
const convFilterSession = ref('')
const convFilterTimeRange = ref<[number, number] | null>(null)

const userOptions = computed(() =>
  users.value.map(u => ({ label: u.username, value: u.id }))
)

const agentOptions = ref<{ label: string; value: number }[]>([])

async function onUserFilterChange(userId: number | null) {
  convFilterAgent.value = null
  if (userId) {
    try {
      const agents = await getUserAgents(userId)
      agentOptions.value = agents.map(a => ({ label: a.name, value: a.id }))
    } catch {
      agentOptions.value = []
    }
  } else {
    agentOptions.value = []
  }
}

const PAGE_SIZE = 50

function buildConvFilterParams(): Record<string, any> {
  const params: any = {}
  if (convFilterUser.value) params.user_id = convFilterUser.value
  if (convFilterAgent.value) params.agent_id = convFilterAgent.value
  if (convFilterSession.value.trim()) params.session_id = convFilterSession.value.trim()
  if (convFilterTimeRange.value) {
    params.start = new Date(convFilterTimeRange.value[0]).toISOString()
    params.end = new Date(convFilterTimeRange.value[1]).toISOString()
  }
  return params
}

async function searchConversations() {
  loadingConversations.value = true
  conversations.value = []
  try {
    const params = { ...buildConvFilterParams(), limit: PAGE_SIZE }
    const result = await getConversations(params)
    conversations.value = result.conversations
    hasMoreConversations.value = result.count >= PAGE_SIZE
  } catch (error) {
    message.error(extractApiError(error, 'Failed to load conversations'))
  } finally {
    loadingConversations.value = false
  }
}

async function loadMoreConversations() {
  if (conversations.value.length === 0) return
  loadingConversations.value = true
  try {
    const last = conversations.value[conversations.value.length - 1]!
    const params = { ...buildConvFilterParams(), limit: PAGE_SIZE, before: last.timestamp }
    const result = await getConversations(params)
    conversations.value.push(...result.conversations)
    hasMoreConversations.value = result.count >= PAGE_SIZE
  } catch (error) {
    message.error(extractApiError(error, 'Failed to load more'))
  } finally {
    loadingConversations.value = false
  }
}

async function exportCSV() {
  exportingCSV.value = true
  try {
    await exportConversationsCSV(buildConvFilterParams())
    message.success('CSV exported')
  } catch (error) {
    message.error(extractApiError(error, 'Failed to export CSV'))
  } finally {
    exportingCSV.value = false
  }
}

function truncateContent(text: string, maxLen = 120): string {
  if (!text || text.length <= maxLen) return text
  return text.slice(0, maxLen) + '...'
}

const conversationColumns = computed<DataTableColumns<ConversationRecord>>(() => [
  {
    title: 'Time',
    key: 'timestamp',
    width: 170,
    render(row) {
      return new Date(row.timestamp).toLocaleString()
    },
  },
  { title: 'User', key: 'username', width: 100 },
  { title: 'Agent', key: 'agent_name', width: 100 },
  {
    title: 'Session',
    key: 'session_id',
    width: 120,
    ellipsis: { tooltip: true },
    render(row) {
      return row.session_id.slice(0, 12) + '...'
    },
  },
  {
    title: 'Role',
    key: 'role',
    width: 80,
    render(row) {
      return h(NTag, {
        type: row.role === 'user' ? 'info' : row.role === 'assistant' ? 'success' : 'default',
        size: 'small',
      }, { default: () => row.role })
    },
  },
  {
    title: 'Content',
    key: 'content',
    ellipsis: { tooltip: true },
    render(row) {
      return truncateContent(row.content)
    },
  },
])

onMounted(() => {
  loadSettings()
  loadUsers()
})
</script>

<style scoped>
.logo {
  height: 32px;
}

.subtitle {
  font-size: 14px;
  color: #888;
  font-weight: 400;
}
</style>

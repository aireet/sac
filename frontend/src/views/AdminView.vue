<template>
  <n-config-provider :theme="darkTheme">
    <n-layout style="height: 100vh">
      <n-layout-header bordered style="height: 60px; padding: 0 24px; display: flex; align-items: center; justify-content: space-between;">
        <div style="display: flex; align-items: baseline; gap: 12px">
          <router-link to="/" style="text-decoration: none">
            <h1 class="logo">SAC</h1>
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
              style="width: 900px; max-width: 95vw"
            >
              <n-spin :show="loadingUserAgents">
                <n-data-table
                  :columns="agentColumns"
                  :data="userAgents"
                  :bordered="false"
                  :single-line="false"
                  v-if="userAgents.length > 0"
                />
                <n-empty v-else description="No agents found for this user." />
              </n-spin>
            </n-modal>
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
  type SystemSetting,
  type AdminUser,
  type UserSetting,
  type AdminAgent,
} from '../services/adminAPI'
import { extractApiError } from '../utils/error'

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
  { title: 'Name', key: 'name', width: 150 },
  {
    title: 'Description',
    key: 'description',
    width: 200,
    ellipsis: { tooltip: true },
  },
  {
    title: 'Status',
    key: 'pod_status',
    width: 120,
    render(row) {
      return h(NTag, {
        type: statusColorMap[row.pod_status] || 'default',
        size: 'small',
      }, { default: () => row.pod_status || 'Unknown' })
    },
  },
  {
    title: 'Resources',
    key: 'resources',
    width: 180,
    render(row) {
      if (!row.cpu_request && !row.memory_request) return '-'
      return h('div', { style: 'font-size: 12px; line-height: 1.4' }, [
        h('div', `CPU: ${row.cpu_request || '-'} / ${row.cpu_limit || '-'}`),
        h('div', `Mem: ${row.memory_request || '-'} / ${row.memory_limit || '-'}`),
      ])
    },
  },
  { title: 'Restarts', key: 'restart_count', width: 80 },
  {
    title: 'Skills',
    key: 'skills',
    width: 70,
    render(row) {
      return row.installed_skills ? row.installed_skills.length : 0
    },
  },
  {
    title: 'Actions',
    key: 'actions',
    width: 160,
    render(row) {
      return h(NSpace, { size: 4 }, {
        default: () => [
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

onMounted(() => {
  loadSettings()
  loadUsers()
})
</script>

<style scoped>
.logo {
  font-size: 28px;
  font-weight: 700;
  margin: 0;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
  letter-spacing: 2px;
}

.subtitle {
  font-size: 14px;
  color: #888;
  font-weight: 400;
}
</style>

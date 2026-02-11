<template>
  <n-modal
    :show="show"
    preset="card"
    title="Manage Groups"
    style="max-width: 600px"
    :mask-closable="true"
    @update:show="$emit('update:show', $event)"
  >
    <!-- Create group form -->
    <div class="create-group">
      <n-input
        v-model:value="newGroupName"
        placeholder="New group name"
        size="small"
        style="flex: 1"
        @keyup.enter="handleCreateGroup"
      />
      <n-button size="small" type="primary" :loading="creating" @click="handleCreateGroup">
        Create
      </n-button>
    </div>

    <n-divider style="margin: 12px 0" />

    <!-- Groups list -->
    <n-spin :show="loading">
      <n-empty v-if="!loading && groups.length === 0" description="No groups yet" />
      <div v-else class="group-list">
        <div v-for="group in groups" :key="group.id" class="group-item">
          <div class="group-info" @click="toggleExpand(group.id)">
            <n-icon :size="16" style="flex-shrink: 0">
              <ChevronForwardOutline v-if="!expandedGroupId || expandedGroupId !== group.id" />
              <ChevronDownOutline v-else />
            </n-icon>
            <div class="group-meta">
              <n-text strong>{{ group.name }}</n-text>
              <n-text v-if="group.description" depth="3" style="font-size: 12px">
                {{ group.description }}
              </n-text>
            </div>
            <n-text depth="3" style="font-size: 12px; flex-shrink: 0">
              {{ group.member_count ?? '?' }} members
            </n-text>
            <n-popconfirm @positive-click="handleDeleteGroup(group.id)">
              <template #trigger>
                <n-button size="tiny" quaternary type="error" @click.stop>
                  <template #icon><n-icon :size="14"><TrashOutline /></n-icon></template>
                </n-button>
              </template>
              Delete group "{{ group.name }}"?
            </n-popconfirm>
          </div>

          <!-- Expanded: member management -->
          <div v-if="expandedGroupId === group.id" class="group-members">
            <div class="member-add">
              <n-input
                v-model:value="addMemberUsername"
                placeholder="Username to add"
                size="small"
                style="flex: 1"
                @keyup.enter="handleAddMember(group.id)"
              />
              <n-select
                v-model:value="addMemberRole"
                :options="roleOptions"
                size="small"
                style="width: 110px"
              />
              <n-button size="small" :loading="addingMember" @click="handleAddMember(group.id)">
                Add
              </n-button>
            </div>

            <n-spin :show="membersLoading">
              <div v-if="members.length === 0 && !membersLoading" style="padding: 8px; text-align: center">
                <n-text depth="3" style="font-size: 12px">No members</n-text>
              </div>
              <div v-for="member in members" :key="member.id" class="member-row">
                <n-text style="flex: 1">
                  {{ member.user?.display_name || member.user?.username || `User #${member.user_id}` }}
                </n-text>
                <n-select
                  :value="member.role"
                  :options="roleOptions"
                  size="tiny"
                  style="width: 100px"
                  @update:value="(val: string) => handleUpdateRole(group.id, member.user_id, val)"
                />
                <n-popconfirm @positive-click="handleRemoveMember(group.id, member.user_id)">
                  <template #trigger>
                    <n-button size="tiny" quaternary type="error">
                      <template #icon><n-icon :size="14"><CloseOutline /></n-icon></template>
                    </n-button>
                  </template>
                  Remove this member?
                </n-popconfirm>
              </div>
            </n-spin>
          </div>
        </div>
      </div>
    </n-spin>
  </n-modal>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import {
  NModal, NInput, NButton, NIcon, NText, NEmpty, NSpin, NDivider,
  NSelect, NPopconfirm,
  useMessage,
} from 'naive-ui'
import {
  ChevronForwardOutline, ChevronDownOutline, TrashOutline, CloseOutline,
} from '@vicons/ionicons5'
import {
  listGroups, createGroup, deleteGroup,
  listMembers, addMember, removeMember, updateMemberRole,
  type Group, type GroupMember,
} from '../../services/groupAPI'
import { findUserByUsername } from '../../services/userAPI'

const props = defineProps<{
  show: boolean
}>()

const emit = defineEmits<{
  'update:show': [value: boolean]
  'groups-changed': []
}>()

const message = useMessage()

// State
const groups = ref<Group[]>([])
const loading = ref(false)
const creating = ref(false)
const newGroupName = ref('')
const expandedGroupId = ref<number | null>(null)
const members = ref<GroupMember[]>([])
const membersLoading = ref(false)
const addMemberUsername = ref('')
const addMemberRole = ref('member')
const addingMember = ref(false)

const roleOptions = [
  { label: 'Member', value: 'member' },
  { label: 'Admin', value: 'admin' },
]

// Load groups when modal opens
watch(() => props.show, async (val) => {
  if (val) {
    await loadGroups()
  } else {
    expandedGroupId.value = null
    members.value = []
  }
})

const loadGroups = async () => {
  loading.value = true
  try {
    groups.value = (await listGroups()) || []
  } catch {
    groups.value = []
  } finally {
    loading.value = false
  }
}

const handleCreateGroup = async () => {
  const name = newGroupName.value.trim()
  if (!name) return
  creating.value = true
  try {
    await createGroup(name)
    newGroupName.value = ''
    message.success(`Group "${name}" created`)
    await loadGroups()
    emit('groups-changed')
  } catch (err) {
    console.error('Create group failed:', err)
    message.error('Failed to create group')
  } finally {
    creating.value = false
  }
}

const handleDeleteGroup = async (groupId: number) => {
  try {
    await deleteGroup(groupId)
    message.success('Group deleted')
    if (expandedGroupId.value === groupId) {
      expandedGroupId.value = null
      members.value = []
    }
    await loadGroups()
    emit('groups-changed')
  } catch (err) {
    console.error('Delete group failed:', err)
    message.error('Failed to delete group')
  }
}

const toggleExpand = async (groupId: number) => {
  if (expandedGroupId.value === groupId) {
    expandedGroupId.value = null
    members.value = []
  } else {
    expandedGroupId.value = groupId
    await loadMembers(groupId)
  }
}

const loadMembers = async (groupId: number) => {
  membersLoading.value = true
  try {
    members.value = (await listMembers(groupId)) || []
  } catch {
    members.value = []
  } finally {
    membersLoading.value = false
  }
}

const handleAddMember = async (groupId: number) => {
  const username = addMemberUsername.value.trim()
  if (!username) return
  addingMember.value = true
  try {
    const user = await findUserByUsername(username)
    if (!user) {
      message.error(`User "${username}" not found`)
      return
    }
    await addMember(groupId, user.id, addMemberRole.value)
    addMemberUsername.value = ''
    message.success(`Added ${username} to group`)
    await loadMembers(groupId)
    await loadGroups()
    emit('groups-changed')
  } catch (err) {
    console.error('Add member failed:', err)
    message.error('Failed to add member')
  } finally {
    addingMember.value = false
  }
}

const handleRemoveMember = async (groupId: number, userId: number) => {
  try {
    await removeMember(groupId, userId)
    message.success('Member removed')
    await loadMembers(groupId)
    await loadGroups()
    emit('groups-changed')
  } catch (err) {
    console.error('Remove member failed:', err)
    message.error('Failed to remove member')
  }
}

const handleUpdateRole = async (groupId: number, userId: number, role: string) => {
  try {
    await updateMemberRole(groupId, userId, role)
    await loadMembers(groupId)
  } catch (err) {
    console.error('Update role failed:', err)
    message.error('Failed to update role')
  }
}
</script>

<style scoped>
.create-group {
  display: flex;
  gap: 8px;
  align-items: center;
}

.group-list {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.group-item {
  border: 1px solid rgba(255, 255, 255, 0.08);
  border-radius: 6px;
  overflow: hidden;
}

.group-info {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  cursor: pointer;
  transition: background 0.15s;
}

.group-info:hover {
  background: rgba(255, 255, 255, 0.04);
}

.group-meta {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.group-members {
  padding: 8px 12px 12px;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
  background: rgba(0, 0, 0, 0.15);
}

.member-add {
  display: flex;
  gap: 4px;
  margin-bottom: 8px;
}

.member-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 4px 0;
}
</style>

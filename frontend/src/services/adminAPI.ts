import api from './api'
import type {
  SystemSetting,
  UserSetting,
  AdminUser,
  AdminGroupBrief,
  AgentWithStatus,
  BatchUpdateImageResponse,
  AdminConversation,
  AdminConversationListResponse,
  AdminUserListResponse,
  SystemSettingListResponse,
  UserSettingListResponse,
  AgentWithStatusListResponse,
} from '../generated/sac/v1/admin'
import type { GroupWithMemberCount, GroupMember, GroupListResponse, GroupMemberListResponse } from '../generated/sac/v1/group'
import { normalizeInt64, normalizeInt64Array } from '../utils/proto'

export type { SystemSetting, UserSetting, AdminUser, AdminConversation }
export type AdminUserGroup = AdminGroupBrief
export interface AdminAgent {
  id: number
  name: string
  description: string
  icon: string
  instructions: string
  config?: { [key: string]: any }
  created_by: number
  cpu_request: string
  cpu_limit: string
  memory_request: string
  memory_limit: string
  created_at?: string
  updated_at?: string
  installed_skills: { id: number; agent_id: number; skill_id: number; order: number; synced_version: number; created_at?: string; skill?: any }[]
  pod_status: string
  restart_count: number
  image: string
}
export interface AdminGroup {
  id: number
  name: string
  description: string
  owner_id: number
  claude_md_template: string
  created_at?: string
  updated_at?: string
  owner?: { id: number; username: string; display_name: string }
  member_count: number
}
export type AdminGroupMember = GroupMember

function normalizeAdminUser(u: AdminUser): AdminUser {
  normalizeInt64(u, ['id'])
  if (u.groups) normalizeInt64Array(u.groups, ['id'])
  return u
}

function normalizeAgentWithStatus(aws: AgentWithStatus): AgentWithStatus {
  if (aws.agent) {
    normalizeInt64(aws.agent, ['id', 'created_by'])
    if (aws.agent.installed_skills) {
      for (const s of aws.agent.installed_skills) {
        normalizeInt64(s, ['id', 'agent_id', 'skill_id'])
        if (s.skill) normalizeInt64(s.skill, ['id', 'created_by', 'forked_from'])
      }
    }
  }
  return aws
}

function normalizeGroupMember(m: GroupMember): GroupMember {
  normalizeInt64(m, ['id', 'group_id', 'user_id'])
  if (m.user) normalizeInt64(m.user, ['id'])
  return m
}

function normalizeGroupWithMemberCount(g: GroupWithMemberCount): GroupWithMemberCount {
  if (g.group) {
    normalizeInt64(g.group, ['id', 'owner_id'])
    if (g.group.owner) normalizeInt64(g.group.owner, ['id'])
  }
  return g
}

// System settings
export async function getSystemSettings(): Promise<SystemSetting[]> {
  const response = await api.get<SystemSettingListResponse>('/admin/settings')
  return normalizeInt64Array(response.data.settings ?? [], ['id'])
}

export async function updateSystemSetting(key: string, value: any): Promise<SystemSetting> {
  const response = await api.put(`/admin/settings/${key}`, { value })
  return normalizeInt64(response.data, ['id'])
}

// Users
export async function getUsers(): Promise<AdminUser[]> {
  const response = await api.get<AdminUserListResponse>('/admin/users')
  return (response.data.users ?? []).map(normalizeAdminUser)
}

export async function updateUserRole(userId: number, role: string): Promise<void> {
  await api.put(`/admin/users/${userId}/role`, { role })
}

// User settings overrides
export async function getUserSettings(userId: number): Promise<UserSetting[]> {
  const response = await api.get<UserSettingListResponse>(`/admin/users/${userId}/settings`)
  return normalizeInt64Array(response.data.settings ?? [], ['id', 'user_id'])
}

export async function updateUserSetting(userId: number, key: string, value: any): Promise<UserSetting> {
  const response = await api.put(`/admin/users/${userId}/settings/${key}`, { value })
  return normalizeInt64(response.data, ['id', 'user_id'])
}

export async function deleteUserSetting(userId: number, key: string): Promise<void> {
  await api.delete(`/admin/users/${userId}/settings/${key}`)
}

// User agents
export async function getUserAgents(userId: number): Promise<AdminAgent[]> {
  const response = await api.get<AgentWithStatusListResponse>(`/admin/users/${userId}/agents`)
  return (response.data.agents ?? []).map(aws => {
    normalizeAgentWithStatus(aws)
    const a = aws.agent
    return {
      id: a?.id ?? 0,
      name: a?.name ?? '',
      description: a?.description ?? '',
      icon: a?.icon ?? '',
      instructions: a?.instructions ?? '',
      config: a?.config,
      created_by: a?.created_by ?? 0,
      cpu_request: aws.cpu_request ?? a?.cpu_request ?? '',
      cpu_limit: aws.cpu_limit ?? a?.cpu_limit ?? '',
      memory_request: aws.memory_request ?? a?.memory_request ?? '',
      memory_limit: aws.memory_limit ?? a?.memory_limit ?? '',
      created_at: a?.created_at,
      updated_at: a?.updated_at,
      installed_skills: a?.installed_skills ?? [],
      pod_status: aws.pod_status,
      restart_count: aws.restart_count,
      image: aws.image,
    }
  })
}

export async function deleteUserAgent(userId: number, agentId: number): Promise<void> {
  await api.delete(`/admin/users/${userId}/agents/${agentId}`)
}

export async function restartUserAgent(userId: number, agentId: number): Promise<void> {
  await api.post(`/admin/users/${userId}/agents/${agentId}/restart`)
}

export async function updateAgentResources(userId: number, agentId: number, resources: {
  cpu_request?: string
  cpu_limit?: string
  memory_request?: string
  memory_limit?: string
}): Promise<void> {
  await api.put(`/admin/users/${userId}/agents/${agentId}/resources`, resources)
}

// Agent image management
export async function updateAgentImage(userId: number, agentId: number, image: string): Promise<void> {
  await api.put(`/admin/users/${userId}/agents/${agentId}/image`, { image })
}

export async function batchUpdateImage(image: string): Promise<BatchUpdateImageResponse> {
  const response = await api.post('/admin/agents/batch-update-image', { image })
  return response.data
}

// Conversations
export type ConversationRecord = AdminConversation

export interface ConversationParams {
  user_id?: number
  agent_id?: number
  session_id?: string
  limit?: number
  before?: string
  start?: string
  end?: string
}

export async function getConversations(params: ConversationParams): Promise<AdminConversationListResponse> {
  const response = await api.get('/admin/conversations', { params })
  const data = response.data as AdminConversationListResponse
  if (data.conversations) {
    normalizeInt64Array(data.conversations, ['id', 'user_id', 'agent_id'])
  }
  return data
}

export async function exportConversationsCSV(params: { user_id?: number; agent_id?: number; session_id?: string; start?: string; end?: string }): Promise<void> {
  const response = await api.get('/admin/conversations/export', {
    params,
    responseType: 'blob',
  })
  const url = window.URL.createObjectURL(new Blob([response.data]))
  const link = document.createElement('a')
  link.href = url
  const disposition = response.headers['content-disposition']
  const filename = disposition?.match(/filename=(.+)/)?.[1] || 'conversations.csv'
  link.setAttribute('download', filename)
  document.body.appendChild(link)
  link.click()
  link.remove()
  window.URL.revokeObjectURL(url)
}

// Admin group management
export async function getAdminGroups(): Promise<AdminGroup[]> {
  const response = await api.get<GroupListResponse>('/admin/groups')
  return (response.data.groups ?? []).map(g => {
    normalizeGroupWithMemberCount(g)
    return {
      id: g.group?.id ?? 0,
      name: g.group?.name ?? '',
      description: g.group?.description ?? '',
      owner_id: g.group?.owner_id ?? 0,
      claude_md_template: g.group?.claude_md_template ?? '',
      created_at: g.group?.created_at,
      updated_at: g.group?.updated_at,
      owner: g.group?.owner,
      member_count: g.member_count,
    }
  })
}

export async function createAdminGroup(data: { name: string; description?: string; owner_id?: number }): Promise<AdminGroup> {
  const response = await api.post('/admin/groups', data)
  const g = normalizeGroupWithMemberCount(response.data)
  return {
    id: g.group?.id ?? 0,
    name: g.group?.name ?? '',
    description: g.group?.description ?? '',
    owner_id: g.group?.owner_id ?? 0,
    claude_md_template: g.group?.claude_md_template ?? '',
    created_at: g.group?.created_at,
    updated_at: g.group?.updated_at,
    owner: g.group?.owner,
    member_count: g.member_count,
  }
}

export async function updateAdminGroup(id: number, data: { name?: string; description?: string }): Promise<void> {
  await api.put(`/admin/groups/${id}`, data)
}

export async function deleteAdminGroup(id: number): Promise<void> {
  await api.delete(`/admin/groups/${id}`)
}

export async function getAdminGroupMembers(groupId: number): Promise<GroupMember[]> {
  const response = await api.get<GroupMemberListResponse>(`/admin/groups/${groupId}/members`)
  return (response.data.members ?? []).map(normalizeGroupMember)
}

export async function addAdminGroupMember(groupId: number, userId: number, role: string = 'member'): Promise<void> {
  await api.post(`/admin/groups/${groupId}/members`, { user_id: userId, role })
}

export async function removeAdminGroupMember(groupId: number, userId: number): Promise<void> {
  await api.delete(`/admin/groups/${groupId}/members/${userId}`)
}

export async function updateAdminGroupMemberRole(groupId: number, userId: number, role: string): Promise<void> {
  await api.put(`/admin/groups/${groupId}/members/${userId}`, { role })
}

export async function updateAdminGroupTemplate(groupId: number, template: string): Promise<void> {
  await api.put(`/admin/groups/${groupId}/template`, { claude_md_template: template })
}

// Admin reset user password
export async function resetUserPassword(userId: number, newPassword: string): Promise<void> {
  await api.put(`/admin/users/${userId}/password`, { new_password: newPassword })
}

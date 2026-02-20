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

export type { SystemSetting, UserSetting, AdminUser, AgentWithStatus, AdminConversation }
export type AdminUserGroup = AdminGroupBrief
export type AdminAgent = AgentWithStatus
export type AdminGroup = GroupWithMemberCount
export type AdminGroupMember = GroupMember

// System settings
export async function getSystemSettings(): Promise<SystemSetting[]> {
  const response = await api.get<SystemSettingListResponse>('/admin/settings')
  return response.data.settings ?? []
}

export async function updateSystemSetting(key: string, value: any): Promise<SystemSetting> {
  const response = await api.put(`/admin/settings/${key}`, { value })
  return response.data
}

// Users
export async function getUsers(): Promise<AdminUser[]> {
  const response = await api.get<AdminUserListResponse>('/admin/users')
  return response.data.users ?? []
}

export async function updateUserRole(userId: number, role: string): Promise<void> {
  await api.put(`/admin/users/${userId}/role`, { role })
}

// User settings overrides
export async function getUserSettings(userId: number): Promise<UserSetting[]> {
  const response = await api.get<UserSettingListResponse>(`/admin/users/${userId}/settings`)
  return response.data.settings ?? []
}

export async function updateUserSetting(userId: number, key: string, value: any): Promise<UserSetting> {
  const response = await api.put(`/admin/users/${userId}/settings/${key}`, { value })
  return response.data
}

export async function deleteUserSetting(userId: number, key: string): Promise<void> {
  await api.delete(`/admin/users/${userId}/settings/${key}`)
}

// User agents
export async function getUserAgents(userId: number): Promise<AgentWithStatus[]> {
  const response = await api.get<AgentWithStatusListResponse>(`/admin/users/${userId}/agents`)
  return response.data.agents ?? []
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
  return response.data
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
export async function getAdminGroups(): Promise<GroupWithMemberCount[]> {
  const response = await api.get<GroupListResponse>('/admin/groups')
  return response.data.groups ?? []
}

export async function createAdminGroup(data: { name: string; description?: string; owner_id?: number }): Promise<GroupWithMemberCount> {
  const response = await api.post('/admin/groups', data)
  return response.data
}

export async function updateAdminGroup(id: number, data: { name?: string; description?: string }): Promise<void> {
  await api.put(`/admin/groups/${id}`, data)
}

export async function deleteAdminGroup(id: number): Promise<void> {
  await api.delete(`/admin/groups/${id}`)
}

export async function getAdminGroupMembers(groupId: number): Promise<GroupMember[]> {
  const response = await api.get<GroupMemberListResponse>(`/admin/groups/${groupId}/members`)
  return response.data.members ?? []
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

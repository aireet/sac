import api from './api'

export interface SystemSetting {
  id: number
  key: string
  value: any
  description: string
  created_at: string
  updated_at: string
}

export interface UserSetting {
  id: number
  user_id: number
  key: string
  value: any
  created_at: string
  updated_at: string
}

export interface AdminUser {
  id: number
  username: string
  email: string
  display_name: string
  role: string
  agent_count: number
  created_at: string
  updated_at: string
}

// System settings
export async function getSystemSettings(): Promise<SystemSetting[]> {
  const response = await api.get('/admin/settings')
  return response.data
}

export async function updateSystemSetting(key: string, value: any): Promise<SystemSetting> {
  const response = await api.put(`/admin/settings/${key}`, { value })
  return response.data
}

// Users
export async function getUsers(): Promise<AdminUser[]> {
  const response = await api.get('/admin/users')
  return response.data
}

export async function updateUserRole(userId: number, role: string): Promise<void> {
  await api.put(`/admin/users/${userId}/role`, { role })
}

// User settings overrides
export async function getUserSettings(userId: number): Promise<UserSetting[]> {
  const response = await api.get(`/admin/users/${userId}/settings`)
  return response.data
}

export async function updateUserSetting(userId: number, key: string, value: any): Promise<UserSetting> {
  const response = await api.put(`/admin/users/${userId}/settings/${key}`, { value })
  return response.data
}

export async function deleteUserSetting(userId: number, key: string): Promise<void> {
  await api.delete(`/admin/users/${userId}/settings/${key}`)
}

// User agents
export interface AdminAgent {
  id: number
  name: string
  description: string
  icon: string
  config: Record<string, any>
  created_by: number
  created_at: string
  updated_at: string
  installed_skills: any[]
  pod_status: string
  restart_count: number
  cpu_request: string
  cpu_limit: string
  memory_request: string
  memory_limit: string
  image: string
}

export async function getUserAgents(userId: number): Promise<AdminAgent[]> {
  const response = await api.get(`/admin/users/${userId}/agents`)
  return response.data
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

export async function batchUpdateImage(image: string): Promise<{ total: number; updated: number; failed: number; errors: any[] }> {
  const response = await api.post('/admin/agents/batch-update-image', { image })
  return response.data
}

// Conversations
export interface ConversationRecord {
  id: number
  user_id: number
  agent_id: number
  session_id: string
  role: string
  content: string
  timestamp: string
  username: string
  agent_name: string
}

export interface ConversationParams {
  user_id?: number
  agent_id?: number
  session_id?: string
  limit?: number
  before?: string
  start?: string
  end?: string
}

export async function getConversations(params: ConversationParams): Promise<{ conversations: ConversationRecord[]; count: number }> {
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

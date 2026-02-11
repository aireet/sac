import api from './api'

export interface Agent {
  id: number
  name: string
  description: string
  icon: string
  config?: Record<string, any>
  created_by: number
  created_at: string
  updated_at: string
  installed_skills?: AgentSkill[]
}

export interface AgentSkill {
  id: number
  agent_id: number
  skill_id: number
  order: number
  synced_version: number
  created_at: string
  skill?: {
    id: number
    name: string
    description: string
    icon: string
    category: string
    version: number
  }
}

export interface CreateAgentRequest {
  name: string
  description?: string
  icon?: string
  config?: Record<string, any>
}

export interface UpdateAgentRequest {
  name?: string
  description?: string
  icon?: string
  config?: Record<string, any>
}

// Get all agents for current user
export const getAgents = async (): Promise<Agent[]> => {
  const response = await api.get<Agent[]>('/agents')
  return response.data
}

// Get a specific agent by ID
export const getAgent = async (id: number): Promise<Agent> => {
  const response = await api.get<Agent>(`/agents/${id}`)
  return response.data
}

// Create a new agent
export const createAgent = async (data: CreateAgentRequest): Promise<Agent> => {
  const response = await api.post<Agent>('/agents', data)
  return response.data
}

// Update an existing agent
export const updateAgent = async (id: number, data: UpdateAgentRequest): Promise<Agent> => {
  const response = await api.put<Agent>(`/agents/${id}`, data)
  return response.data
}

// Delete an agent
export const deleteAgent = async (id: number): Promise<void> => {
  await api.delete(`/agents/${id}`)
}

// Restart an agent (delete pod, K8s will recreate it)
export const restartAgent = async (id: number): Promise<void> => {
  await api.post(`/agents/${id}/restart`)
}

// Install a skill to an agent
export const installSkill = async (agentId: number, skillId: number): Promise<void> => {
  await api.post(`/agents/${agentId}/skills`, { skill_id: skillId })
}

// Uninstall a skill from an agent
export const uninstallSkill = async (agentId: number, skillId: number): Promise<void> => {
  await api.delete(`/agents/${agentId}/skills/${skillId}`)
}

// Agent pod status
export interface AgentStatus {
  agent_id: number
  pod_name: string
  status: string
  restart_count: number
  cpu_request: string
  cpu_limit: string
  memory_request: string
  memory_limit: string
}

// Get pod statuses for all agents
export const getAgentStatuses = async (): Promise<AgentStatus[]> => {
  const response = await api.get<AgentStatus[]>('/agent-statuses')
  return response.data
}

// Conversation history
export interface ConversationMessage {
  id: number
  user_id: number
  agent_id: number
  session_id: string
  role: string
  content: string
  message_uuid: string
  timestamp: string
}

export interface ConversationQuery {
  agent_id: number
  session_id?: string
  limit?: number
  before?: string
  after?: string
}

export interface SessionInfo {
  session_id: string
  first_at: string
  last_at: string
  count: number
}

export const getConversations = async (params: ConversationQuery): Promise<{ conversations: ConversationMessage[]; count: number; has_more: boolean }> => {
  const response = await api.get('/conversations', { params })
  return response.data
}

export const getConversationSessions = async (agentId: number): Promise<SessionInfo[]> => {
  const response = await api.get('/conversations/sessions', { params: { agent_id: agentId } })
  return response.data.sessions
}

export const exportConversationsCSV = async (agentId: number, sessionId?: string): Promise<void> => {
  const params: Record<string, any> = { agent_id: agentId }
  if (sessionId) params.session_id = sessionId
  const response = await api.get('/conversations/export', {
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

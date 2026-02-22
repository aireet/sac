import api from './api'
import type {
  Agent,
  AgentSkill,
  CreateAgentRequest,
  UpdateAgentRequest,
  AgentStatus,
  ClaudeMDPreview,
  AgentListResponse,
  AgentStatusListResponse,
} from '../generated/sac/v1/agent'
import type {
  ConversationMessage,
  SessionSummary,
  ConversationListResponse,
} from '../generated/sac/v1/history'
import { normalizeInt64, normalizeInt64Array } from '../utils/proto'

export type { Agent, AgentSkill, CreateAgentRequest, UpdateAgentRequest, AgentStatus, ConversationMessage }

const AGENT_I64 = ['id', 'created_by'] as const
const AGENT_SKILL_I64 = ['id', 'agent_id', 'skill_id'] as const
const AGENT_STATUS_I64 = ['agent_id'] as const

function normalizeAgent(a: Agent): Agent {
  normalizeInt64(a, [...AGENT_I64])
  if (a.installed_skills) {
    for (const s of a.installed_skills) {
      normalizeInt64(s, [...AGENT_SKILL_I64])
      if (s.skill) normalizeInt64(s.skill, ['id', 'created_by', 'forked_from'])
    }
  }
  return a
}

export interface ConversationQuery {
  agent_id: number
  session_id?: string
  limit?: number
  before?: string
  after?: string
}

export type SessionInfo = SessionSummary

// Get all agents for current user
export const getAgents = async (): Promise<Agent[]> => {
  const response = await api.get<AgentListResponse>('/agents')
  return (response.data.agents ?? []).map(normalizeAgent)
}

// Get a specific agent by ID
export const getAgent = async (id: number): Promise<Agent> => {
  const response = await api.get<Agent>(`/agents/${id}`)
  return normalizeAgent(response.data)
}

// Create a new agent
export const createAgent = async (data: CreateAgentRequest): Promise<Agent> => {
  const response = await api.post<Agent>('/agents', data)
  return normalizeAgent(response.data)
}

// Update an existing agent
export const updateAgent = async (id: number, data: UpdateAgentRequest): Promise<Agent> => {
  const response = await api.put<Agent>(`/agents/${id}`, data)
  return normalizeAgent(response.data)
}

// Delete an agent
export const deleteAgent = async (id: number): Promise<void> => {
  await api.delete(`/agents/${id}`)
}

// Restart an agent (delete pod, K8s will recreate it)
export const restartAgent = async (id: number): Promise<void> => {
  await api.post(`/agents/${id}/restart`)
}

export const previewClaudeMD = async (id: number): Promise<ClaudeMDPreview> => {
  const response = await api.get<ClaudeMDPreview>(`/agents/${id}/claude-md-preview`)
  return response.data
}

// Install a skill to an agent
export const installSkill = async (agentId: number, skillId: number): Promise<void> => {
  await api.post(`/agents/${agentId}/skills`, { skill_id: skillId })
}

// Uninstall a skill from an agent
export const uninstallSkill = async (agentId: number, skillId: number): Promise<void> => {
  await api.delete(`/agents/${agentId}/skills/${skillId}`)
}

// Get pod statuses for all agents
export const getAgentStatuses = async (): Promise<AgentStatus[]> => {
  const response = await api.get<AgentStatusListResponse>('/agent-statuses')
  return normalizeInt64Array(response.data.statuses ?? [], [...AGENT_STATUS_I64])
}

export const getConversations = async (params: ConversationQuery): Promise<ConversationListResponse> => {
  const response = await api.get<ConversationListResponse>('/conversations', { params })
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

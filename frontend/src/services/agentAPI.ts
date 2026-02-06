import axios from 'axios'

const getApiBaseUrl = () => {
  if (import.meta.env.VITE_API_URL) {
    return import.meta.env.VITE_API_URL
  }
  const protocol = window.location.protocol === 'https:' ? 'https:' : 'http:'
  const host = window.location.hostname
  return `${protocol}//${host}:8080/api`
}

const api = axios.create({
  baseURL: getApiBaseUrl(),
  headers: {
    'Content-Type': 'application/json',
  },
})

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
  created_at: string
  skill?: {
    id: number
    name: string
    description: string
    icon: string
    category: string
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

// Install a skill to an agent
export const installSkill = async (agentId: number, skillId: number): Promise<void> => {
  await api.post(`/agents/${agentId}/skills`, { skill_id: skillId })
}

// Uninstall a skill from an agent
export const uninstallSkill = async (agentId: number, skillId: number): Promise<void> => {
  await api.delete(`/agents/${agentId}/skills/${skillId}`)
}

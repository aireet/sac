import axios from 'axios'

// Auto-detect API URL based on current host
const getApiBaseUrl = () => {
  if (import.meta.env.VITE_API_URL) {
    return import.meta.env.VITE_API_URL
  }
  // Use current host with port 8080
  const protocol = window.location.protocol === 'https:' ? 'https:' : 'http:'
  const host = window.location.hostname
  return `${protocol}//${host}:8080/api`
}

const API_BASE_URL = getApiBaseUrl()

const apiClient = axios.create({
  baseURL: API_BASE_URL,
  headers: {
    'Content-Type': 'application/json',
  },
})

export interface SkillParameter {
  name: string
  label: string
  type: 'text' | 'select' | 'date' | 'number'
  required: boolean
  default_value?: string
  options?: string[]
}

export interface Skill {
  id: number
  name: string
  description: string
  icon: string
  category: string
  prompt: string
  command_name: string
  parameters?: SkillParameter[]
  is_official: boolean
  created_by: number
  is_public: boolean
  forked_from?: number
  created_at: string
  updated_at: string
}

export interface CreateSkillRequest {
  name: string
  description: string
  icon: string
  category: string
  prompt: string
  parameters?: SkillParameter[]
  is_public: boolean
}

export async function getSkills(): Promise<Skill[]> {
  const response = await apiClient.get('/skills')
  return response.data
}

export async function getSkill(id: number): Promise<Skill> {
  const response = await apiClient.get(`/skills/${id}`)
  return response.data
}

export async function createSkill(skill: CreateSkillRequest): Promise<Skill> {
  const response = await apiClient.post('/skills', skill)
  return response.data
}

export async function updateSkill(id: number, skill: Partial<CreateSkillRequest>): Promise<Skill> {
  const response = await apiClient.put(`/skills/${id}`, skill)
  return response.data
}

export async function deleteSkill(id: number): Promise<void> {
  await apiClient.delete(`/skills/${id}`)
}

export async function forkSkill(id: number): Promise<Skill> {
  const response = await apiClient.post(`/skills/${id}/fork`)
  return response.data
}

export async function getPublicSkills(): Promise<Skill[]> {
  const response = await apiClient.get('/skills/public')
  return response.data
}

export async function syncAgentSkills(agentId: number): Promise<void> {
  await apiClient.post(`/agents/${agentId}/sync-skills`)
}

import api from './api'

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
  version: number
  created_at: string
  updated_at: string
}

export interface CreateSkillRequest {
  name: string
  description: string
  icon: string
  category: string
  prompt: string
  command_name?: string
  parameters?: SkillParameter[]
  is_public: boolean
}

export async function getSkills(): Promise<Skill[]> {
  const response = await api.get('/skills')
  return response.data
}

export async function getSkill(id: number): Promise<Skill> {
  const response = await api.get(`/skills/${id}`)
  return response.data
}

export async function createSkill(skill: CreateSkillRequest): Promise<Skill> {
  const response = await api.post('/skills', skill)
  return response.data
}

export async function updateSkill(id: number, skill: Partial<CreateSkillRequest>): Promise<Skill> {
  const response = await api.put(`/skills/${id}`, skill)
  return response.data
}

export async function deleteSkill(id: number): Promise<void> {
  await api.delete(`/skills/${id}`)
}

export async function forkSkill(id: number): Promise<Skill> {
  const response = await api.post(`/skills/${id}/fork`)
  return response.data
}

export async function getPublicSkills(): Promise<Skill[]> {
  const response = await api.get('/skills/public')
  return response.data
}

export async function syncAgentSkills(agentId: number): Promise<void> {
  await api.post(`/agents/${agentId}/sync-skills`)
}

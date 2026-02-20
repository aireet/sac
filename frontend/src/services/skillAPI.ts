import api from './api'
import type { Skill, SkillParameter, CreateSkillRequest, SkillListResponse } from '../generated/sac/v1/skill'

export type { Skill, SkillParameter, CreateSkillRequest }

export async function getSkills(): Promise<Skill[]> {
  const response = await api.get<SkillListResponse>('/skills')
  return response.data.skills ?? []
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
  const response = await api.get<SkillListResponse>('/skills/public')
  return response.data.skills ?? []
}

export async function syncAgentSkills(agentId: number): Promise<void> {
  await api.post(`/agents/${agentId}/sync-skills`)
}

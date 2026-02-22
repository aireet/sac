import api from './api'
import type { Skill, SkillParameter, CreateSkillRequest, SkillListResponse } from '../generated/sac/v1/skill'
import { normalizeInt64 } from '../utils/proto'

export type { Skill, SkillParameter, CreateSkillRequest }

const SKILL_I64 = ['id', 'created_by', 'forked_from'] as const

function normalizeSkill(s: Skill): Skill {
  return normalizeInt64(s, [...SKILL_I64])
}

export async function getSkills(): Promise<Skill[]> {
  const response = await api.get<SkillListResponse>('/skills')
  return (response.data.skills ?? []).map(normalizeSkill)
}

export async function getSkill(id: number): Promise<Skill> {
  const response = await api.get(`/skills/${id}`)
  return normalizeSkill(response.data)
}

export async function createSkill(skill: CreateSkillRequest): Promise<Skill> {
  const response = await api.post('/skills', skill)
  return normalizeSkill(response.data)
}

export async function updateSkill(id: number, skill: Partial<CreateSkillRequest>): Promise<Skill> {
  const response = await api.put(`/skills/${id}`, skill)
  return normalizeSkill(response.data)
}

export async function deleteSkill(id: number): Promise<void> {
  await api.delete(`/skills/${id}`)
}

export async function forkSkill(id: number): Promise<Skill> {
  const response = await api.post(`/skills/${id}/fork`)
  return normalizeSkill(response.data)
}

export async function getPublicSkills(): Promise<Skill[]> {
  const response = await api.get<SkillListResponse>('/skills/public')
  return (response.data.skills ?? []).map(normalizeSkill)
}

export async function syncAgentSkills(agentId: number): Promise<void> {
  await api.post(`/agents/${agentId}/sync-skills`)
}

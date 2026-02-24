import api from './api'
import type { Skill, SkillParameter, SkillFrontmatter, SkillFile, SkillFileContentResponse, CreateSkillRequest, SkillListResponse, SkillFileListResponse } from '../generated/sac/v1/skill'
import { normalizeInt64 } from '../utils/proto'

export type { Skill, SkillParameter, SkillFrontmatter, SkillFile, SkillFileContentResponse, CreateSkillRequest }

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

// --- Skill file management ---

export async function uploadSkillFile(
  skillId: number, file: File, opts?: { dirPath?: string; filepath?: string }
): Promise<SkillFile> {
  const formData = new FormData()
  formData.append('file', file)
  const fp = opts?.filepath ?? (opts?.dirPath ? opts.dirPath + file.name : '')
  if (fp) formData.append('filepath', fp)
  const response = await api.post(`/skills/${skillId}/files`, formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
  return response.data
}

export async function listSkillFiles(skillId: number): Promise<SkillFile[]> {
  const response = await api.get<SkillFileListResponse>(`/skills/${skillId}/files`)
  return response.data.files ?? []
}

export async function deleteSkillFile(skillId: number, filepath: string): Promise<void> {
  await api.delete(`/skills/${skillId}/files`, { params: { path: filepath } })
}

export function skillFileDownloadUrl(skillId: number, filepath: string): string {
  return `/api/skills/${skillId}/files/download?path=${encodeURIComponent(filepath)}`
}

export async function saveSkillFileContent(skillId: number, filepath: string, content: string): Promise<SkillFile> {
  const response = await api.put(`/skills/${skillId}/files/content`, { filepath, content })
  return response.data
}

export async function getSkillFileContent(skillId: number, filepath: string): Promise<SkillFileContentResponse> {
  const response = await api.get<SkillFileContentResponse>(`/skills/${skillId}/files/content`, { params: { path: filepath } })
  return response.data
}

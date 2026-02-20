import api from './api'
import type { Group, GroupMember, GroupListResponse, GroupMemberListResponse } from '../generated/sac/v1/group'

export type { Group, GroupMember }

// Backend returns GroupListResponse with groups field.
// Flatten to match existing frontend usage where member_count is on Group.
type FlatGroup = Group & { member_count?: number }
export type { FlatGroup as GroupFlat }

export const listGroups = async (): Promise<FlatGroup[]> => {
  const response = await api.get<GroupListResponse>('/groups')
  return (response.data.groups ?? []).map(g => ({ ...g.group!, member_count: g.member_count }))
}

export const getGroup = async (id: number): Promise<Group> => {
  const response = await api.get<Group>(`/groups/${id}`)
  return response.data
}

export const listMembers = async (groupId: number): Promise<GroupMember[]> => {
  const response = await api.get<GroupMemberListResponse>(`/groups/${groupId}/members`)
  return response.data.members ?? []
}

export const getGroupTemplate = async (groupId: number): Promise<string> => {
  const response = await api.get(`/groups/${groupId}/template`)
  return response.data.claude_md_template
}

export const updateGroupTemplate = async (groupId: number, template: string): Promise<void> => {
  await api.put(`/groups/${groupId}/template`, { claude_md_template: template })
}

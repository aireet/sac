import api from './api'
import type { Group, GroupMember, GroupListResponse, GroupMemberListResponse } from '../generated/sac/v1/group'
import { normalizeInt64, normalizeInt64Array } from '../utils/proto'

export type { Group, GroupMember }

// Backend returns GroupListResponse with groups field.
// Flatten to match existing frontend usage where member_count is on Group.
type FlatGroup = Group & { member_count?: number }
export type { FlatGroup as GroupFlat }

const GROUP_I64 = ['id', 'owner_id'] as const
const GROUP_MEMBER_I64 = ['id', 'group_id', 'user_id'] as const

function normalizeGroup(g: Group): Group {
  normalizeInt64(g, [...GROUP_I64])
  if (g.owner) normalizeInt64(g.owner, ['id'])
  return g
}

export const listGroups = async (): Promise<FlatGroup[]> => {
  const response = await api.get<GroupListResponse>('/groups')
  return (response.data.groups ?? []).map(g => {
    const flat = { ...g.group!, member_count: g.member_count } as FlatGroup
    normalizeGroup(flat)
    return flat
  })
}

export const getGroup = async (id: number): Promise<Group> => {
  const response = await api.get<Group>(`/groups/${id}`)
  return normalizeGroup(response.data)
}

export const listMembers = async (groupId: number): Promise<GroupMember[]> => {
  const response = await api.get<GroupMemberListResponse>(`/groups/${groupId}/members`)
  const members = response.data.members ?? []
  for (const m of members) {
    normalizeInt64(m, [...GROUP_MEMBER_I64])
    if (m.user) normalizeInt64(m.user, ['id'])
  }
  return members
}

export const getGroupTemplate = async (groupId: number): Promise<string> => {
  const response = await api.get(`/groups/${groupId}/template`)
  return response.data.claude_md_template
}

export const updateGroupTemplate = async (groupId: number, template: string): Promise<void> => {
  await api.put(`/groups/${groupId}/template`, { claude_md_template: template })
}

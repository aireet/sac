import api from './api'

export interface Group {
  id: number
  name: string
  description: string
  owner_id: number
  claude_md_template: string
  created_at: string
  updated_at: string
  owner?: {
    id: number
    username: string
    display_name: string
  }
  member_count?: number
}

export interface GroupMember {
  id: number
  group_id: number
  user_id: number
  role: string // "admin" | "member"
  created_at: string
  user?: {
    id: number
    username: string
    display_name: string
  }
}

export const listGroups = async (): Promise<Group[]> => {
  const response = await api.get('/groups')
  return response.data
}

export const getGroup = async (id: number): Promise<Group> => {
  const response = await api.get(`/groups/${id}`)
  return response.data
}

export const listMembers = async (groupId: number): Promise<GroupMember[]> => {
  const response = await api.get(`/groups/${groupId}/members`)
  return response.data
}

export const getGroupTemplate = async (groupId: number): Promise<string> => {
  const response = await api.get(`/groups/${groupId}/template`)
  return response.data.claude_md_template
}

export const updateGroupTemplate = async (groupId: number, template: string): Promise<void> => {
  await api.put(`/groups/${groupId}/template`, { claude_md_template: template })
}

import api from './api'

export interface Group {
  id: number
  name: string
  description: string
  owner_id: number
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

export const createGroup = async (name: string, description = ''): Promise<Group> => {
  const response = await api.post('/groups', { name, description })
  return response.data
}

export const getGroup = async (id: number): Promise<Group> => {
  const response = await api.get(`/groups/${id}`)
  return response.data
}

export const updateGroup = async (id: number, data: { name?: string; description?: string }): Promise<void> => {
  await api.put(`/groups/${id}`, data)
}

export const deleteGroup = async (id: number): Promise<void> => {
  await api.delete(`/groups/${id}`)
}

export const listMembers = async (groupId: number): Promise<GroupMember[]> => {
  const response = await api.get(`/groups/${groupId}/members`)
  return response.data
}

export const addMember = async (groupId: number, userId: number, role = 'member'): Promise<GroupMember> => {
  const response = await api.post(`/groups/${groupId}/members`, { user_id: userId, role })
  return response.data
}

export const removeMember = async (groupId: number, userId: number): Promise<void> => {
  await api.delete(`/groups/${groupId}/members/${userId}`)
}

export const updateMemberRole = async (groupId: number, userId: number, role: string): Promise<void> => {
  await api.put(`/groups/${groupId}/members/${userId}`, { role })
}

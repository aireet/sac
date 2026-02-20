import api from './api'
import type { UserBrief } from '../generated/sac/v1/common'
import type { UserBriefListResponse } from '../generated/sac/v1/auth'

export type UserBasic = UserBrief
export type { UserBrief }

export const searchUsers = async (query: string): Promise<UserBasic[]> => {
  const response = await api.get<UserBriefListResponse>('/users/search', { params: { q: query } })
  return response.data.users ?? []
}

export const findUserByUsername = async (username: string): Promise<UserBasic | null> => {
  const users = await searchUsers(username)
  return users.find(u => u.username === username) || null
}

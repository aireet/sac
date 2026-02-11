import api from './api'

export interface UserBasic {
  id: number
  username: string
  display_name: string
}

export const searchUsers = async (query: string): Promise<UserBasic[]> => {
  const response = await api.get('/users/search', { params: { q: query } })
  return response.data
}

export const findUserByUsername = async (username: string): Promise<UserBasic | null> => {
  const users = await searchUsers(username)
  return users.find(u => u.username === username) || null
}

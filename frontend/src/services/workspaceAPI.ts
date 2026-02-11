import api from './api'

export interface WorkspaceFile {
  name: string
  path: string
  size: number
  is_directory: boolean
  last_modified?: string
}

export interface WorkspaceQuota {
  user_id: number
  agent_id: number
  used_bytes: number
  max_bytes: number
  file_count: number
  max_file_count: number
}

export interface ListFilesResponse {
  path: string
  files: WorkspaceFile[]
}

// ---- Private workspace (per-agent) ----

export const listFiles = async (agentId: number, path = '/'): Promise<ListFilesResponse> => {
  const response = await api.get('/workspace/files', { params: { agent_id: agentId, path } })
  return response.data
}

export const uploadFile = async (
  agentId: number,
  file: File,
  path = '/',
  onProgress?: (percent: number) => void,
): Promise<void> => {
  const formData = new FormData()
  formData.append('file', file)
  formData.append('path', path)
  formData.append('agent_id', String(agentId))

  await api.post('/workspace/upload', formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
    onUploadProgress: (e) => {
      if (onProgress && e.total) {
        onProgress(Math.round((e.loaded / e.total) * 100))
      }
    },
  })
}

export const downloadFile = (agentId: number, path: string): void => {
  const token = localStorage.getItem('token')
  const baseUrl = api.defaults.baseURL
  const url = `${baseUrl}/workspace/files/download?agent_id=${agentId}&path=${encodeURIComponent(path)}`
  fetch(url, { headers: { Authorization: `Bearer ${token}` } })
    .then((r) => r.blob())
    .then((blob) => {
      const objUrl = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = objUrl
      a.download = path.split('/').pop() || 'download'
      document.body.appendChild(a)
      a.click()
      a.remove()
      URL.revokeObjectURL(objUrl)
    })
}

export const fetchFileBlob = async (agentId: number, path: string): Promise<Blob> => {
  const token = localStorage.getItem('token')
  const baseUrl = api.defaults.baseURL
  const url = `${baseUrl}/workspace/files/download?agent_id=${agentId}&path=${encodeURIComponent(path)}`
  const r = await fetch(url, { headers: { Authorization: `Bearer ${token}` } })
  if (!r.ok) throw new Error(`Download failed: ${r.status}`)
  return r.blob()
}

export const deleteFile = async (agentId: number, path: string): Promise<void> => {
  await api.delete('/workspace/files', { params: { agent_id: agentId, path } })
}

export const createDirectory = async (agentId: number, path: string): Promise<void> => {
  await api.post('/workspace/directories', { agent_id: agentId, path })
}

export const getQuota = async (agentId: number): Promise<WorkspaceQuota> => {
  const response = await api.get('/workspace/quota', { params: { agent_id: agentId } })
  return response.data
}

// ---- Public workspace ----

export const listPublicFiles = async (path = '/'): Promise<ListFilesResponse> => {
  const response = await api.get('/workspace/public/files', { params: { path } })
  return response.data
}

export const downloadPublicFile = (path: string): void => {
  const token = localStorage.getItem('token')
  const baseUrl = api.defaults.baseURL
  const url = `${baseUrl}/workspace/public/files/download?path=${encodeURIComponent(path)}`
  fetch(url, { headers: { Authorization: `Bearer ${token}` } })
    .then((r) => r.blob())
    .then((blob) => {
      const objUrl = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = objUrl
      a.download = path.split('/').pop() || 'download'
      document.body.appendChild(a)
      a.click()
      a.remove()
      URL.revokeObjectURL(objUrl)
    })
}

export const fetchPublicFileBlob = async (path: string): Promise<Blob> => {
  const token = localStorage.getItem('token')
  const baseUrl = api.defaults.baseURL
  const url = `${baseUrl}/workspace/public/files/download?path=${encodeURIComponent(path)}`
  const r = await fetch(url, { headers: { Authorization: `Bearer ${token}` } })
  if (!r.ok) throw new Error(`Download failed: ${r.status}`)
  return r.blob()
}

export const uploadPublicFile = async (
  file: File,
  path = '/',
  onProgress?: (percent: number) => void,
): Promise<void> => {
  const formData = new FormData()
  formData.append('file', file)
  formData.append('path', path)

  await api.post('/workspace/public/upload', formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
    onUploadProgress: (e) => {
      if (onProgress && e.total) {
        onProgress(Math.round((e.loaded / e.total) * 100))
      }
    },
  })
}

export const deletePublicFile = async (path: string): Promise<void> => {
  await api.delete('/workspace/public/files', { params: { path } })
}

export const createPublicDirectory = async (path: string): Promise<void> => {
  await api.post('/workspace/public/directories', { path })
}

// ---- Group workspace ----

export interface GroupWorkspaceQuota {
  group_id: number
  used_bytes: number
  max_bytes: number
  file_count: number
  max_file_count: number
}

export const listGroupFiles = async (groupId: number, path = '/'): Promise<ListFilesResponse> => {
  const response = await api.get('/workspace/group/files', { params: { group_id: groupId, path } })
  return response.data
}

export const uploadGroupFile = async (
  groupId: number,
  file: File,
  path = '/',
  onProgress?: (percent: number) => void,
): Promise<void> => {
  const formData = new FormData()
  formData.append('file', file)
  formData.append('path', path)
  formData.append('group_id', String(groupId))

  await api.post('/workspace/group/upload', formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
    onUploadProgress: (e) => {
      if (onProgress && e.total) {
        onProgress(Math.round((e.loaded / e.total) * 100))
      }
    },
  })
}

export const downloadGroupFile = (groupId: number, path: string): void => {
  const token = localStorage.getItem('token')
  const baseUrl = api.defaults.baseURL
  const url = `${baseUrl}/workspace/group/files/download?group_id=${groupId}&path=${encodeURIComponent(path)}`
  fetch(url, { headers: { Authorization: `Bearer ${token}` } })
    .then((r) => r.blob())
    .then((blob) => {
      const objUrl = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = objUrl
      a.download = path.split('/').pop() || 'download'
      document.body.appendChild(a)
      a.click()
      a.remove()
      URL.revokeObjectURL(objUrl)
    })
}

export const fetchGroupFileBlob = async (groupId: number, path: string): Promise<Blob> => {
  const token = localStorage.getItem('token')
  const baseUrl = api.defaults.baseURL
  const url = `${baseUrl}/workspace/group/files/download?group_id=${groupId}&path=${encodeURIComponent(path)}`
  const r = await fetch(url, { headers: { Authorization: `Bearer ${token}` } })
  if (!r.ok) throw new Error(`Download failed: ${r.status}`)
  return r.blob()
}

export const deleteGroupFile = async (groupId: number, path: string): Promise<void> => {
  await api.delete('/workspace/group/files', { params: { group_id: groupId, path } })
}

export const createGroupDirectory = async (groupId: number, path: string): Promise<void> => {
  await api.post('/workspace/group/directories', { group_id: groupId, path })
}

export const getGroupQuota = async (groupId: number): Promise<GroupWorkspaceQuota> => {
  const response = await api.get('/workspace/group/quota', { params: { group_id: groupId } })
  return response.data
}

// ---- Shared workspace (read-only browsing + publish) ----

export const listSharedFiles = async (path = '/'): Promise<ListFilesResponse> => {
  const response = await api.get('/workspace/shared/files', { params: { path } })
  return response.data
}

export const downloadSharedFile = (path: string): void => {
  const token = localStorage.getItem('token')
  const baseUrl = api.defaults.baseURL
  const url = `${baseUrl}/workspace/shared/files/download?path=${encodeURIComponent(path)}`
  fetch(url, { headers: { Authorization: `Bearer ${token}` } })
    .then((r) => r.blob())
    .then((blob) => {
      const objUrl = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = objUrl
      a.download = path.split('/').pop() || 'download'
      document.body.appendChild(a)
      a.click()
      a.remove()
      URL.revokeObjectURL(objUrl)
    })
}

export const fetchSharedFileBlob = async (path: string): Promise<Blob> => {
  const token = localStorage.getItem('token')
  const baseUrl = api.defaults.baseURL
  const url = `${baseUrl}/workspace/shared/files/download?path=${encodeURIComponent(path)}`
  const r = await fetch(url, { headers: { Authorization: `Bearer ${token}` } })
  if (!r.ok) throw new Error(`Download failed: ${r.status}`)
  return r.blob()
}

export const publishToShared = async (agentId: number, path: string, destPath?: string): Promise<void> => {
  await api.post('/workspace/shared/publish', { agent_id: agentId, path, dest_path: destPath })
}

export const deleteSharedFile = async (path: string): Promise<void> => {
  await api.delete('/workspace/shared/files', { params: { path } })
}

// ---- Sync workspace to pod ----

export const syncWorkspaceToPod = async (agentId: number): Promise<void> => {
  await api.post('/workspace/sync', { agent_id: agentId })
}

export interface SyncProgressEvent {
  synced: number
  total: number
  file: string
  error?: string
}

export const syncWorkspaceToPodStream = async (
  agentId: number,
  onProgress: (event: SyncProgressEvent) => void,
): Promise<void> => {
  const token = localStorage.getItem('token')
  const baseUrl = api.defaults.baseURL
  const url = `${baseUrl}/workspace/sync-stream?agent_id=${agentId}`

  const resp = await fetch(url, {
    headers: { Authorization: `Bearer ${token}` },
  })

  if (!resp.ok) throw new Error(`Sync failed: ${resp.status}`)
  if (!resp.body) throw new Error('No response body')

  const reader = resp.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  while (true) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    const lines = buffer.split('\n')
    buffer = lines.pop() || ''
    for (const line of lines) {
      if (line.startsWith('data: ')) {
        try {
          const event = JSON.parse(line.slice(6)) as SyncProgressEvent
          onProgress(event)
        } catch { /* skip malformed */ }
      }
    }
  }
}

// ---- Type exports ----

export type SpaceTab = 'private' | 'public' | 'group' | 'shared'

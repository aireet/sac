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

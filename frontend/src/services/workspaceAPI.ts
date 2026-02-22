import api, { getApiWsBaseUrl } from './api'
import type { FileItem, FileListResponse } from '../generated/sac/v1/common'
import type {
  WorkspaceQuota,
  GroupWorkspaceQuota,
  ShareResponse,
  SharedFileMeta,
} from '../generated/sac/v1/workspace'
import { normalizeInt64, normalizeInt64Array } from '../utils/proto'

export type { FileListResponse, WorkspaceQuota, GroupWorkspaceQuota, SharedFileMeta }
export type WorkspaceFile = FileItem
export type ListFilesResponse = FileListResponse

function normalizeFileList(data: FileListResponse): FileListResponse {
  if (data.files) normalizeInt64Array(data.files, ['size'])
  return data
}

const QUOTA_I64 = ['user_id', 'agent_id', 'used_bytes', 'max_bytes'] as const
const GROUP_QUOTA_I64 = ['group_id', 'used_bytes', 'max_bytes'] as const

// ---- Private workspace (per-agent) ----

export const listFiles = async (agentId: number, path = '/'): Promise<FileListResponse> => {
  const response = await api.get('/workspace/files', { params: { agent_id: agentId, path } })
  return normalizeFileList(response.data)
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
  return normalizeInt64(response.data, [...QUOTA_I64])
}

// ---- Public workspace ----

export const listPublicFiles = async (path = '/'): Promise<FileListResponse> => {
  const response = await api.get('/workspace/public/files', { params: { path } })
  return normalizeFileList(response.data)
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

export const listGroupFiles = async (groupId: number, path = '/'): Promise<FileListResponse> => {
  const response = await api.get('/workspace/group/files', { params: { group_id: groupId, path } })
  return normalizeFileList(response.data)
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
  return normalizeInt64(response.data, [...GROUP_QUOTA_I64])
}

// ---- Output workspace (read-only, populated by sidecar) ----

export const listOutputFiles = async (agentId: number, path = '/'): Promise<FileListResponse> => {
  const response = await api.get('/workspace/output/files', { params: { agent_id: agentId, path } })
  return normalizeFileList(response.data)
}

export const downloadOutputFile = (agentId: number, path: string): void => {
  const token = localStorage.getItem('token')
  const baseUrl = api.defaults.baseURL
  const url = `${baseUrl}/workspace/output/files/download?agent_id=${agentId}&path=${encodeURIComponent(path)}`
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

export const fetchOutputFileBlob = async (agentId: number, path: string): Promise<Blob> => {
  const token = localStorage.getItem('token')
  const baseUrl = api.defaults.baseURL
  const url = `${baseUrl}/workspace/output/files/download?agent_id=${agentId}&path=${encodeURIComponent(path)}`
  const r = await fetch(url, { headers: { Authorization: `Bearer ${token}` } })
  if (!r.ok) throw new Error(`Download failed: ${r.status}`)
  return r.blob()
}

export const deleteOutputFile = async (agentId: number, path: string): Promise<void> => {
  await api.delete('/workspace/output/files', { params: { agent_id: agentId, path } })
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

// ---- Output workspace WebSocket watch ----

export interface OutputWatchEvent {
  action: string // "upload" | "delete"
  path: string
  name: string
  size: number
}

/**
 * Opens a WebSocket connection to watch output workspace file changes.
 * Returns an abort function to close the connection.
 */
export const watchOutputFiles = (
  agentId: number,
  onEvent: (event: OutputWatchEvent) => void,
  onReconnect?: () => void,
): (() => void) => {
  let ws: WebSocket | null = null
  let closed = false
  let isFirstConnect = true

  const connect = () => {
    if (closed) return
    const token = localStorage.getItem('token')
    const wsBase = getApiWsBaseUrl()
    const url = `${wsBase}/api/workspace/output/watch?agent_id=${agentId}&token=${token}`

    ws = new WebSocket(url)

    ws.onopen = () => {
      if (!isFirstConnect && onReconnect) {
        onReconnect()
      }
      isFirstConnect = false
    }

    ws.onmessage = (msg) => {
      try {
        const event = JSON.parse(msg.data) as OutputWatchEvent
        onEvent(event)
      } catch { /* skip malformed */ }
    }

    ws.onclose = () => {
      if (!closed) {
        setTimeout(connect, 1000)
      }
    }

    ws.onerror = () => {
      ws?.close()
    }
  }

  connect()

  return () => {
    closed = true
    ws?.close()
  }
}

// ---- Shared links ----

export type ShareResult = ShareResponse

export const shareOutputFile = async (agentId: number, path: string): Promise<ShareResult> => {
  const response = await api.post('/workspace/output/share', { agent_id: agentId, path })
  return response.data
}

export const deleteShare = async (code: string): Promise<void> => {
  await api.delete(`/workspace/output/share/${code}`)
}

// Public endpoints (no auth required) — use raw fetch without JWT
const getPublicApiBaseUrl = () => {
  if (import.meta.env.VITE_API_URL) {
    return import.meta.env.VITE_API_URL
  }
  const protocol = window.location.protocol === 'https:' ? 'https:' : 'http:'
  const host = window.location.hostname
  if (host === 'localhost' || host === '127.0.0.1') {
    return `${protocol}//${host}:8080/api`
  }
  return `${protocol}//${window.location.host}/api`
}

export const getSharedFile = async (code: string): Promise<SharedFileMeta> => {
  const baseUrl = getPublicApiBaseUrl()
  const r = await fetch(`${baseUrl}/s/${code}`)
  if (!r.ok) throw new Error(`Not found: ${r.status}`)
  const json = await r.json()
  const meta = json.data ?? json
  return normalizeInt64(meta, ['size_bytes'])
}

export const fetchSharedFileBlob = async (code: string): Promise<Blob> => {
  const baseUrl = getPublicApiBaseUrl()
  const r = await fetch(`${baseUrl}/s/${code}/raw`)
  if (!r.ok) throw new Error(`Not found: ${r.status}`)
  return r.blob()
}

// ---- Type exports ----

export type SpaceTab = 'private' | 'public' | 'group' | 'output'

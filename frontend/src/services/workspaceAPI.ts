import api, { getApiWsBaseUrl } from './api'
import type { FileItem, FileListResponse } from '../generated/sac/v1/common'
import type {
  ShareResponse,
  SharedFileMeta,
} from '../generated/sac/v1/workspace'
import { normalizeInt64, normalizeInt64Array } from '../utils/proto'

export type { FileListResponse, SharedFileMeta }
export type WorkspaceFile = FileItem
export type ListFilesResponse = FileListResponse

function normalizeFileList(data: FileListResponse): FileListResponse {
  if (data.files) normalizeInt64Array(data.files, ['size'])
  return data
}

// ---- Output workspace ----

export const uploadOutputFile = async (
  agentId: number, file: File, path?: string,
): Promise<void> => {
  const formData = new FormData()
  formData.append('file', file)
  formData.append('agent_id', String(agentId))
  formData.append('path', path || file.name)
  await api.post('/workspace/output/files', formData, {
    headers: { 'Content-Type': 'multipart/form-data' },
  })
}

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

export type SpaceTab = 'output'

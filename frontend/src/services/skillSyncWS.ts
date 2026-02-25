import { getApiWsBaseUrl } from './api'

export interface SkillSyncEvent {
  type: string       // "skill_sync"
  action: string     // "progress" | "complete" | "error"
  skill_id: number
  skill_name: string
  command_name: string
  agent_id: number
  step: string       // "writing_skill_md" | "downloading_file" | "restarting_process" | "cleaning_stale" | "done"
  message: string
  current?: number
  total?: number
}

/**
 * Opens a WebSocket connection to watch skill sync progress events.
 * Returns an abort function to close the connection.
 */
export const watchSkillSync = (
  agentId: number,
  onEvent: (event: SkillSyncEvent) => void,
): (() => void) => {
  let ws: WebSocket | null = null
  let closed = false

  const connect = () => {
    if (closed) return
    const token = localStorage.getItem('token')
    const wsBase = getApiWsBaseUrl()
    const url = `${wsBase}/api/skill-sync/watch?agent_id=${agentId}&token=${token}`

    ws = new WebSocket(url)

    ws.onmessage = (msg) => {
      try {
        const event = JSON.parse(msg.data) as SkillSyncEvent
        onEvent(event)
      } catch { /* skip malformed */ }
    }

    ws.onclose = () => {
      if (!closed) {
        setTimeout(connect, 2000)
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

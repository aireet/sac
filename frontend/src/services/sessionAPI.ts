import axios from 'axios'

const API_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080/api'

export interface Session {
  id: number
  user_id: number
  session_id: string
  pod_name: string
  pod_ip: string
  status: 'creating' | 'running' | 'idle' | 'stopped' | 'deleted'
  last_active: string
  created_at: string
  updated_at: string
}

export interface CreateSessionRequest {
  agent_id?: number
}

export interface CreateSessionResponse {
  session_id: string
  status: string
  pod_name?: string
  created_at: string
}

/**
 * Create a new session with optional agent
 */
export async function createSession(agentId?: number): Promise<CreateSessionResponse> {
  const payload: CreateSessionRequest = {}
  if (agentId) {
    payload.agent_id = agentId
  }

  const response = await axios.post<CreateSessionResponse>(`${API_URL}/sessions`, payload)
  return response.data
}

/**
 * Get session by ID
 */
export async function getSession(sessionId: string): Promise<Session> {
  const response = await axios.get<Session>(`${API_URL}/sessions/${sessionId}`)
  return response.data
}

/**
 * List all sessions for current user
 */
export async function listSessions(): Promise<Session[]> {
  const response = await axios.get<Session[]>(`${API_URL}/sessions`)
  return response.data
}

/**
 * Delete a session
 */
export async function deleteSession(sessionId: string): Promise<void> {
  await axios.delete(`${API_URL}/sessions/${sessionId}`)
}

/**
 * Wait for a session to be ready (poll until running)
 */
export async function waitForSessionReady(
  sessionId: string,
  maxRetries: number = 60,
  retryInterval: number = 2000
): Promise<Session> {
  for (let i = 0; i < maxRetries; i++) {
    const session = await getSession(sessionId)

    if (session.status === 'running' && session.pod_ip) {
      return session
    }

    if (session.status === 'stopped') {
      throw new Error('Session failed to start')
    }

    // Wait before next retry
    await new Promise(resolve => setTimeout(resolve, retryInterval))
  }

  throw new Error('Timeout waiting for session to be ready')
}

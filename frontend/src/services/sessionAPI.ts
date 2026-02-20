import api from './api'
import type { Session, CreateSessionResponse, UserSessionListResponse } from '../generated/sac/v1/session'

export type { Session, CreateSessionResponse }

export interface CreateSessionRequest {
  agent_id?: number
}

/**
 * Create a new session with optional agent
 */
export async function createSession(agentId?: number): Promise<CreateSessionResponse> {
  const payload: CreateSessionRequest = {}
  if (agentId) {
    payload.agent_id = agentId
  }

  const response = await api.post<CreateSessionResponse>('/sessions', payload)
  return response.data
}

/**
 * Get session by ID
 */
export async function getSession(sessionId: string): Promise<Session> {
  const response = await api.get<Session>(`/sessions/${sessionId}`)
  return response.data
}

/**
 * List all sessions for current user
 */
export async function listSessions(): Promise<Session[]> {
  const response = await api.get<UserSessionListResponse>('/sessions')
  return response.data.sessions ?? []
}

/**
 * Delete a session
 */
export async function deleteSession(sessionId: string): Promise<void> {
  await api.delete(`/sessions/${sessionId}`)
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

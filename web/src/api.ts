// The one place that talks to Troy's API.

export type Connection = { kind: string; status: 'connected' | 'not_connected'; net_id?: string; first_name?: string }
export type Me = { phone_number: string; connections: Connection[] }
export type SignIn = { troy_phone_number: string; sign_in_phrase: string }
export type USCAccount = { net_id: string; password: string; duo_bypass_code: string }
export type ChatSummary = { id: string; title: string; preview: string; updated_at: string; busy: boolean }
export type Part =
  | { type: 'text'; text: string }
  | { type: 'tool'; id: string; name: string; args?: string; output?: string; failed?: boolean; status: 'running' | 'done' }
export type ChatMessage = { role: 'user'; text: string } | { role: 'assistant'; parts: Part[] }
export type ChatEvent =
  | { type: 'snapshot'; busy: boolean; messages: ChatMessage[]; pending: string[]; suggestion?: string; error?: boolean }
  | { type: 'user'; text: string }
  | { type: 'turn_start'; taken?: number }
  | { type: 'text'; text: string }
  | { type: 'tool_start'; id: string; name: string; args?: string }
  | { type: 'tool_end'; id: string; output?: string; failed?: boolean }
  | { type: 'turn_end' }
  | { type: 'suggestion'; text: string }

export class ApiError extends Error {
  status: number
  code: string
  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

// Fired when a request finds the session gone, so the app can go back to sign-in.
export const SIGNED_OUT = 'troy:signed-out'

async function call<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (res.ok) {
    const text = await res.text()
    return (text ? JSON.parse(text) : undefined) as T
  }
  const { error } = await res.json().catch(() => ({ error: undefined }))
  const err = new ApiError(res.status, error?.code ?? 'unknown', error?.message ?? res.statusText)
  if (err.code === 'not_signed_in') dispatchEvent(new Event(SIGNED_OUT))
  throw err
}

export const api = {
  signIn: () => call<SignIn>('GET', '/api/sign-in'),
  // null when signed out.
  me: () =>
    call<Me>('GET', '/api/me').catch((e) => {
      if (e instanceof ApiError && e.code === 'not_signed_in') return null
      throw e
    }),
  connectUSCAccount: (account: USCAccount) => call<Connection>('PUT', '/api/connections/usc-account', account),
  disconnectUSCAccount: () => call<void>('DELETE', '/api/connections/usc-account'),
  signOut: () => call<void>('DELETE', '/api/web-session'),
  chats: () => call<ChatSummary[]>('GET', '/api/chats'),
  createChat: () => call<{ id: string }>('POST', '/api/chats'),
  deleteChat: (id: string) => call<void>('DELETE', `/api/chats/${id}`),
  send: (id: string, text: string) => call<void>('POST', `/api/chats/${id}/messages`, { text }),
  chatEvents: (id: string) => new EventSource(`/api/chats/${id}/events`),
}

// "+13105550123" -> "(310) 555-0123"; other numbers as they are.
export const formatPhone = (n: string) => {
  const m = /^\+1(\d{3})(\d{3})(\d{4})$/.exec(n)
  return m ? `(${m[1]}) ${m[2]}-${m[3]}` : n
}

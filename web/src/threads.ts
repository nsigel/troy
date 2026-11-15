import { useCallback, useEffect, useRef, useState } from 'react'
import { api, type ChatEvent, type ChatMessage, type ChatSummary } from './api'

// What a chat looks like live, plus how its latest turn went, as far as this page has seen.
export type Thread = {
  busy: boolean
  messages: ChatMessage[]
  pending: string[]
  suggestion: string
  fresh?: boolean // the turn's reply starts a new assistant message
  status?: 'working' | 'done' | 'error'
}

const empty: Thread = { busy: false, messages: [], pending: [], suggestion: '' }

// The same reducer as the server's.
function apply(s: Thread, e: ChatEvent): Thread {
  const messages = [...s.messages]
  const reply = () => {
    const last = messages.at(-1)
    const same = last?.role === 'assistant' && !s.fresh
    const r: ChatMessage = same ? { role: 'assistant', parts: [...last.parts] } : { role: 'assistant', parts: [] }
    if (same) messages[messages.length - 1] = r
    else messages.push(r)
    return r.parts
  }
  switch (e.type) {
    case 'snapshot':
      return {
        busy: e.busy,
        messages: e.messages,
        pending: e.pending,
        suggestion: e.suggestion ?? '',
        status: e.error ? 'error' : e.busy ? 'working' : s.status === 'working' ? 'done' : s.status,
      }
    case 'user':
      return { ...s, pending: [...s.pending, e.text], suggestion: '' }
    case 'turn_start': {
      const n = Math.min(e.taken ?? 0, s.pending.length)
      if (n) messages.push({ role: 'user', text: s.pending.slice(0, n).join('\n') })
      return { ...s, busy: true, fresh: true, messages, pending: s.pending.slice(n), status: 'working' }
    }
    case 'text': {
      const parts = reply()
      const last = parts.at(-1)
      if (last?.type === 'text') parts[parts.length - 1] = { type: 'text', text: `${last.text}\n\n${e.text}` }
      else parts.push({ type: 'text', text: e.text })
      return { ...s, messages }
    }
    case 'tool_start':
      reply().push({ type: 'tool', id: e.id, name: e.name, args: e.args, status: 'running' })
      return { ...s, messages, fresh: false }
    case 'tool_end':
      return {
        ...s,
        messages: s.messages.map((m) =>
          m.role !== 'assistant'
            ? m
            : { ...m, parts: m.parts.map((p) => (p.type === 'tool' && p.id === e.id ? { ...p, output: e.output, failed: e.failed, status: 'done' } : p)) },
        ),
      }
    case 'turn_end':
      return { ...s, busy: false, status: 'done' }
    case 'suggestion':
      return { ...s, suggestion: e.text }
  }
}

// The student's chats, and live state for the ones this page follows: the open one and any
// Troy is working in. A chat stops being followed once it's idle and not open; its last state
// stays, and the server keeps its suggestion for the next time it's opened.
export function useThreads(open?: string) {
  const [chats, setChats] = useState<ChatSummary[]>([])
  const [live, setLive] = useState<Record<string, Thread>>({})
  const sources = useRef(new Map<string, EventSource>())

  const refresh = useCallback(() => api.chats().then(setChats, () => {}), [])
  useEffect(() => void refresh(), [refresh])

  const follow = useCallback((id: string) => {
    if (sources.current.has(id)) return
    const events = api.chatEvents(id)
    events.onmessage = (m) => {
      const e: ChatEvent = JSON.parse(m.data)
      setLive((all) => ({ ...all, [id]: apply(all[id] ?? empty, e) }))
      if (e.type === 'turn_end') refresh()
    }
    sources.current.set(id, events)
  }, [refresh])

  const unfollow = (id: string) => {
    sources.current.get(id)?.close()
    sources.current.delete(id)
  }

  useEffect(() => {
    if (open) follow(open)
    for (const c of chats) if (c.busy && live[c.id]?.busy !== false) follow(c.id)
    for (const id of sources.current.keys()) {
      const t = live[id]
      if (id !== open && t && !t.busy && !t.pending.length) unfollow(id)
    }
  }, [open, chats, live, follow])

  useEffect(() => () => sources.current.forEach((s) => s.close()), [])

  // A new chat exists once its first message is sent.
  const start = async (text: string) => {
    const { id } = await api.createChat()
    await api.send(id, text)
    setChats((cs) => [{ id, title: text, preview: '', updated_at: new Date().toISOString(), busy: true }, ...cs])
    follow(id)
    return id
  }

  const remove = async (id: string) => {
    await api.deleteChat(id)
    unfollow(id)
    setChats((cs) => cs.filter((c) => c.id !== id))
  }

  return { chats, live, start, remove }
}

import { api, type Part } from './api'
import { Composer } from './Composer'
import type { Thread } from './threads'
import { CodeBlock } from '@/components/ai-elements/code-block'
import { Conversation, ConversationContent, ConversationScrollButton } from '@/components/ai-elements/conversation'
import { Message, MessageContent, MessageResponse } from '@/components/ai-elements/message'
import { Shimmer } from '@/components/ai-elements/shimmer'
import { Tool, ToolContent, ToolHeader } from '@/components/ai-elements/tool'

// One conversation, then the box to write in.
export function Chat({ id, thread, onStart }: { id: string; thread?: Thread; onStart: (text: string) => Promise<unknown> }) {
  const { busy, messages, pending, suggestion } = thread ?? { busy: false, messages: [], pending: [], suggestion: '' }
  const thinking = busy && messages.at(-1)?.role !== 'assistant'

  return (
    <>
      <Conversation className="min-h-0">
        <ConversationContent className="mx-auto w-full max-w-3xl gap-6 px-6 pt-8 pb-10">
          {messages.map((m, i) =>
            m.role === 'user' ? (
              <UserMessage key={i} text={m.text} />
            ) : (
              <Message key={i} from="assistant">
                <MessageContent>
                  {m.parts.map((p, j) => (p.type === 'text' ? <MessageResponse key={j}>{p.text}</MessageResponse> : <ToolCall key={p.id} part={p} />))}
                </MessageContent>
              </Message>
            ),
          )}
          {thinking && <Shimmer className="text-[15px]">Thinking</Shimmer>}
          {pending.map((text, i) => (
            <UserMessage key={`p${i}`} text={text} pending />
          ))}
        </ConversationContent>
        <ConversationScrollButton />
      </Conversation>
      <Composer key={id} className="mx-auto w-full max-w-3xl px-6 pb-6" suggestion={suggestion} autoFocus onSend={(text) => api.send(id, text)} onStart={onStart} />
    </>
  )
}

function UserMessage({ text, pending }: { text: string; pending?: boolean }) {
  return (
    <Message from="user" className={pending ? 'opacity-60' : undefined}>
      <MessageContent className="whitespace-pre-wrap">{text}</MessageContent>
    </Message>
  )
}

// A tool call, closed by default. Python shows its code; anything else its arguments.
function ToolCall({ part }: { part: Extract<Part, { type: 'tool' }> }) {
  let args: unknown = part.args
  try {
    args = JSON.parse(part.args ?? '')
  } catch {}
  const code = typeof args === 'object' && args && 'code' in args && typeof args.code === 'string' ? args.code : undefined
  return (
    <Tool className="mb-0">
      <ToolHeader
        type="dynamic-tool"
        toolName={part.name}
        state={part.status === 'running' ? 'input-available' : part.failed ? 'output-error' : 'output-available'}
      />
      <ToolContent className="space-y-3 p-3 pt-0">
        <CodeBlock code={code ?? JSON.stringify(args, null, 2) ?? ''} language={code ? 'python' : 'json'} />
        {part.output && (
          <pre className={`max-h-64 overflow-auto rounded-md bg-paper-2 p-3 font-mono text-xs whitespace-pre-wrap ${part.failed ? 'text-cardinal' : ''}`}>
            {part.output}
          </pre>
        )}
      </ToolContent>
    </Tool>
  )
}

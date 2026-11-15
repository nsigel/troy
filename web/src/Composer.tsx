import { useState, type KeyboardEvent } from 'react'
import { PromptInput, PromptInputFooter, PromptInputSubmit, PromptInputTextarea } from '@/components/ai-elements/prompt-input'
import { Shimmer } from '@/components/ai-elements/shimmer'

// The message box, a faintly ruled index card. Troy's guess at the next message
// shimmers into it while it's empty; Tab takes it, and Cmd+Enter sends it as it is. Cmd+Plus
// starts a new chat with the message and leaves it running in the background.
export function Composer({
  onSend,
  onStart,
  suggestion = '',
  placeholder = 'Message Troy',
  autoFocus,
  className,
}: {
  onSend: (text: string) => Promise<unknown>
  onStart?: (text: string) => Promise<unknown>
  suggestion?: string
  placeholder?: string
  autoFocus?: boolean
  className?: string
}) {
  const [text, setText] = useState('')
  const [sending, setSending] = useState(false)
  const [error, setError] = useState('')
  const ghost = !text && suggestion

  const send = async (t = text.trim(), to = onSend) => {
    if (!t || sending) return
    setSending(true)
    setError('')
    try {
      await to(t)
      setText('')
    } catch {
      setError("Couldn't send. Try again.")
    } finally {
      setSending(false)
    }
  }

  const keys = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Tab' && ghost) {
      e.preventDefault()
      setText(suggestion)
    } else if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
      e.preventDefault()
      send(text.trim() || suggestion)
    } else if (onStart && (e.key === '=' || e.key === '+') && (e.metaKey || e.ctrlKey)) {
      e.preventDefault()
      send(text.trim() || suggestion, onStart)
    }
  }

  return (
    <div className={className}>
      {error && (
        <p className="px-1 pb-2 text-[15px] font-medium text-cardinal" role="alert">
          {error}
        </p>
      )}
      <PromptInput
        onSubmit={() => send()}
        className="relative rounded-[6px] shadow-card transition-shadow focus-within:shadow-sticker [&_[data-slot=input-group]]:rounded-[6px] [&_[data-slot=input-group]]:border-line [&_[data-slot=input-group]]:bg-card"
      >
        <span className="pointer-events-none absolute inset-y-0 left-11 z-1 w-px bg-cardinal/15" aria-hidden />
        <PromptInputTextarea
          value={text}
          onChange={(e) => setText(e.currentTarget.value)}
          onKeyDown={keys}
          placeholder={ghost ? '' : placeholder}
          autoFocus={autoFocus}
          className="bg-[linear-gradient(transparent_21px,var(--rule)_21px,var(--rule)_22px,transparent_22px)] bg-size-[100%_28px] bg-local bg-origin-content bg-clip-content min-h-[84px] pt-[13px] pr-4 pl-15 text-[18px] leading-[28px] text-ink placeholder:text-ink-3 md:text-[18px]"
        />
        {ghost && (
          <div className="pointer-events-none absolute top-[13px] right-4 left-15 flex items-center gap-2 text-[18px] leading-[28px]" aria-hidden>
            <Shimmer key={suggestion} once duration={1.6} as="span" className="truncate">
              {suggestion}
            </Shimmer>
            <kbd className="shrink-0 rounded border border-line bg-paper px-1.5 font-sans text-[11px] leading-[18px] text-ink-3">tab</kbd>
          </div>
        )}
        <PromptInputFooter className="justify-end px-2 pt-0 pb-2">
          <PromptInputSubmit variant="ink" className="size-8 rounded-[9px_8px_10px_7px/8px_10px_7px_9px]" status={sending ? 'submitted' : undefined} />
        </PromptInputFooter>
      </PromptInput>
    </div>
  )
}

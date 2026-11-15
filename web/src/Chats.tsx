import { useState } from 'react'
import { CircleCheckIcon, CircleXIcon, LoaderCircleIcon, MenuIcon, SquarePenIcon, XIcon } from 'lucide-react'
import { formatPhone, type ChatSummary, type Me } from './api'
import { Account } from './Account'
import { Chat } from './Chat'
import { ConnectUSC } from './ConnectUSC'
import { Composer } from './Composer'
import { Helmet, Pin, Rule, Underline } from './Doodles'
import { useThreads, type Thread } from './threads'
import { Shimmer } from '@/components/ai-elements/shimmer'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

type View = { at: 'home' } | { at: 'chat'; id: string } | { at: 'account'; justConnected?: boolean } | { at: 'connect' }

const uscConnected = (me: Me) => me.connections.some((c) => c.kind === 'usc_account' && c.status === 'connected')

// The signed-in app: chats down the side, and a sheet on the desk for whatever is open. A student
// who hasn't connected USC starts there.
export default function Chats({ me, onChanged, onSignedOut }: { me: Me; onChanged: () => Promise<unknown>; onSignedOut: () => void }) {
  const [view, setView] = useState<View>(() => (uscConnected(me) ? { at: 'home' } : { at: 'connect' }))
  const name = me.connections.find((c) => c.kind === 'usc_account')?.first_name
  const phone = formatPhone(me.phone_number)
  const [nav, setNav] = useState(false)
  const open = view.at === 'chat' ? view.id : undefined
  const { chats, live, start, remove } = useThreads(open)

  const go = (v: View) => {
    setView(v)
    setNav(false)
  }
  const del = async (id: string) => {
    await remove(id).catch(() => {})
    if (open === id) go({ at: 'home' })
  }

  return (
    <div className="flex h-dvh bg-paper-2 bg-(image:--grain) text-ink">
      <aside
        className={cn(
          'flex w-72 shrink-0 flex-col gap-1 p-3 max-md:fixed max-md:inset-0 max-md:z-40 max-md:w-auto max-md:bg-paper-2 max-md:bg-(image:--grain)',
          !nav && 'max-md:hidden',
        )}
      >
        <div className="flex h-11 items-center px-2">
          <Wordmark />
          <button
            onClick={() => go({ at: 'account' })}
            aria-label="Account"
            title={name || phone}
            className={cn(
              'ml-auto grid size-8 place-items-center rounded-full bg-card font-sans text-[15px] font-semibold text-cardinal shadow-card transition-shadow hover:shadow-sticker',
              view.at === 'account' && 'ring-2 ring-cardinal/40',
            )}
          >
            {(name || phone).charAt(0).toUpperCase()}
          </button>
          <Button variant="ghost" size="icon" className="ml-1 md:hidden" onClick={() => setNav(false)} aria-label="Close">
            <XIcon />
          </Button>
        </div>
        <button
          onClick={() => go({ at: 'home' })}
          className="relative mx-1 mt-4 flex -rotate-[0.8deg] items-center gap-2.5 rounded-[2px] bg-card bg-(image:--grain) px-3.5 py-2.5 text-[15px] font-semibold text-ink shadow-card transition-[rotate,box-shadow,translate] duration-300 hover:rotate-0 hover:shadow-sticker active:translate-y-px"
        >
          <Pin className="absolute -top-3 left-1/2 -ml-3 size-6" />
          <SquarePenIcon className="size-4" />
          New chat
        </button>
        <Rule className="mt-3 px-2 text-ink/20" />
        <nav className="-mx-3 flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-4 pt-3 pb-4">
          {chats.map((c, i) => (
            <Note key={c.id} tilt={TILTS[i % TILTS.length]} chat={c} thread={live[c.id]} open={open === c.id} onOpen={() => go({ at: 'chat', id: c.id })} onDelete={() => del(c.id)} />
          ))}
        </nav>
      </aside>

      <div className="relative m-3 flex min-w-0 flex-1 md:ml-0">
      <span className="absolute inset-0 translate-x-1 translate-y-1.5 rotate-[0.6deg] rounded-[3px] bg-paper-2 shadow-under" aria-hidden />
      <main className="relative flex min-w-0 flex-1 flex-col rounded-[3px] bg-paper bg-(image:--grain) shadow-sheet">
        <header className="flex h-12 shrink-0 items-center gap-2 px-3">
          <Button variant="ghost" size="icon" className="md:hidden" onClick={() => setNav(true)} aria-label="Chats">
            <MenuIcon />
          </Button>
          {view.at === 'chat' && <h1 className="truncate px-2 font-serif text-[17px] font-medium">{title(chats.find((c) => c.id === open), live[view.id])}</h1>}
        </header>
        {view.at === 'home' && <Home name={name} onSend={async (text) => go({ at: 'chat', id: await start(text) })} onStart={start} />}
        {view.at === 'chat' && <Chat key={view.id} id={view.id} thread={live[view.id]} onStart={start} />}
        {view.at === 'account' && (
          <div className="min-h-0 flex-1 overflow-y-auto">
            <Account
              me={me}
              justConnected={view.justConnected}
              onConnect={() => go({ at: 'connect' })}
              onChanged={onChanged}
              onSignedOut={onSignedOut}
            />
          </div>
        )}
        {view.at === 'connect' && (
          <div className="mx-auto flex min-h-0 w-full max-w-md flex-1 flex-col">
            <ConnectUSC onBack={() => go({ at: 'home' })} onConnected={() => onChanged().then(() => go({ at: 'account', justConnected: true }))} />
          </div>
        )}
      </main>
      </div>
    </div>
  )
}

function Wordmark() {
  return (
    <span className="relative font-serif text-[26px] leading-none font-medium tracking-[-0.02em] [font-variation-settings:'opsz'_144,'SOFT'_100,'WONK'_1]">
      Troy
      <Helmet className="absolute -top-1.5 -right-5 w-5 rotate-8 text-cardinal [--w:3] [&_path]:animate-none [&_path]:[stroke-dashoffset:0]" />
    </span>
  )
}

function Home({ name, onSend, onStart }: { name?: string; onSend: (text: string) => Promise<unknown>; onStart: (text: string) => Promise<unknown> }) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center px-6 pb-[12vh]">
      <span className="tape -top-3 left-1/2 -ml-14 w-28 -rotate-3" aria-hidden />
      <Helmet className="mb-5 w-14 text-cardinal [--w:2.6]" />
      <h1 className="text-center font-serif text-[40px] leading-[1.1] font-medium tracking-[-0.025em] text-balance [font-variation-settings:'opsz'_144,'SOFT'_60] sm:text-[46px]">
        What's {name ? 'up, ' : ''}
        <span className="relative inline-block">
          {name ?? 'up'}
          <Underline className="absolute -bottom-2.5 -left-[2%] h-3.5 w-[104%] stroke-[2.8] text-cardinal" delay={300} />
        </span>
        ?
      </h1>
      <Composer className="mt-10 w-full max-w-2xl" placeholder="Ask Troy anything" autoFocus onSend={onSend} onStart={onStart} />
      <div className="mt-7 flex flex-wrap justify-center gap-x-7 gap-y-1">
        {ASKS.map((ask, i) => (
          <button
            key={ask}
            onClick={() => onSend(ask)}
            style={{ rotate: `${TILTS[i] * 2}deg` }}
            className="font-hand text-[22px] text-ink-2 underline decoration-cardinal/0 decoration-[1.5px] underline-offset-4 transition-colors hover:text-ink hover:decoration-cardinal/60"
          >
            {ask}
          </button>
        ))}
      </div>
    </div>
  )
}

const HOUR = 60 * 60 * 1000

const STATUSES = {
  working: { label: 'Working', icon: LoaderCircleIcon, className: 'text-working [&_svg]:animate-spin' },
  done: { label: 'Done', icon: CircleCheckIcon, className: 'text-done' },
  error: { label: 'Error', icon: CircleXIcon, className: 'text-failed' },
}

function Status({ status }: { status: keyof typeof STATUSES }) {
  const { label, icon: Icon, className } = STATUSES[status]
  return (
    <span className={cn('inline-flex items-center gap-1 font-medium', className)}>
      <Icon className="size-3.5" />
      {label}
    </span>
  )
}
const TILTS = [-0.6, 0.4, -0.25, 0.7, -0.45]
const ASKS = ["what's due this week?", 'find me a study room', "when's my next midterm?"]

// A chat in the sidebar: a slip of paper, tilted a little. The open one straightens out and gets taped down.
function Note({
  chat,
  thread,
  tilt,
  open,
  onOpen,
  onDelete,
}: {
  chat: ChatSummary
  thread?: Thread
  tilt: number
  open: boolean
  onOpen: () => void
  onDelete: () => void
}) {
  const working = thread ? thread.busy || thread.pending.length > 0 : chat.busy
  const line = working ? activity(thread) : (thread && latest(thread)) || chat.preview
  const status = thread?.status ?? (chat.preview && recent(chat.updated_at) ? 'done' : undefined)
  return (
    <div
      style={{ rotate: open ? '0deg' : `${tilt}deg` }}
      className={cn(
        'group relative shrink-0 rounded-[3px] bg-(image:--grain) transition-[rotate,box-shadow,translate,background-color] duration-300 ease-[cubic-bezier(0.2,0.8,0.2,1)] hover:rotate-0!',
        open ? '-translate-y-px bg-card shadow-sticker' : 'bg-card/55 shadow-[0_0_0_1px_rgba(80,60,20,0.07)] hover:bg-card/90',
      )}
    >
      {open && (
        <>
          <span className="tape -top-2.5 left-1/2 -ml-8 h-4 w-16 -rotate-3" aria-hidden />
          <span className="absolute inset-y-0 left-2 w-px bg-cardinal/40" aria-hidden />
        </>
      )}
      <button onClick={onOpen} className={cn('block w-full py-2.5 pr-3 text-left', open ? 'pl-5' : 'pl-3')}>
        <span className="flex items-baseline gap-2">
          <span className={cn('min-w-0 flex-1 truncate text-[15px]', open ? 'font-semibold text-ink' : 'font-medium text-ink-2')}>{title(chat, thread)}</span>
          <span className="shrink-0 text-[13px]">
            {working ? <Status status="working" /> : status ? <Status status={status} /> : <span className="text-ink-3">{ago(chat.updated_at)}</span>}
          </span>
        </span>
        {line && (
          <span className="mt-0.5 block truncate text-[13px] text-ink-3">
            {working ? (
              <Shimmer as="span" duration={1.6}>
                {line}
              </Shimmer>
            ) : (
              line
            )}
          </span>
        )}
      </button>
      {!working && (
        <Button
          variant="ghost"
          size="icon-xs"
          className="absolute -top-2 -right-2 hidden size-5 rounded-full bg-card text-ink-3 shadow-card group-hover:inline-flex hover:bg-card hover:text-cardinal"
          onClick={onDelete}
          aria-label="Delete chat"
        >
          <XIcon />
        </Button>
      )}
    </div>
  )
}

const title = (chat?: ChatSummary, thread?: Thread) =>
  chat?.title || thread?.messages.find((m) => m.role === 'user')?.text || thread?.pending[0] || 'New chat'

// What Troy is doing right now in a thread.
function activity(thread?: Thread) {
  const last = thread?.messages.at(-1)
  const part = last?.role === 'assistant' ? last.parts.at(-1) : undefined
  if (part?.type === 'tool' && part.status === 'running') return `Running ${part.name}`
  return part ? 'Writing' : 'Thinking'
}

// The start of Troy's latest text, as plain words.
function latest(thread: Thread) {
  for (let i = thread.messages.length - 1; i >= 0; i--) {
    const m = thread.messages[i]
    if (m.role !== 'assistant') continue
    const text = m.parts.findLast((p) => p.type === 'text')
    if (text?.type === 'text') return text.text.replace(/[*_`#>|[\]]/g, '').replace(/\s+/g, ' ').trim()
  }
}

const recent = (iso: string) => Date.now() - new Date(iso).getTime() < HOUR

function ago(iso: string) {
  const m = Math.round((Date.now() - new Date(iso).getTime()) / 60000)
  if (m < 1) return 'now'
  if (m < 60) return `${m}m`
  if (m < 60 * 24) return `${Math.round(m / 60)}h`
  return `${Math.round(m / 60 / 24)}d`
}

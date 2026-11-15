import { useState, type ReactNode } from 'react'
import { api, ApiError, formatPhone, type Connection, type Me } from './api'
import { Dots, Stamp, Underline } from './Doodles'
import uscLogo from './assets/usc.svg'
import { Button } from '@/components/ui/button'
import { Dialog, DialogContent, DialogDescription, DialogTitle } from '@/components/ui/dialog'
import { cn } from '@/lib/utils'

const stamp = 'w-[108px] flex-none -my-1.5 -mr-1 -rotate-9 opacity-90 mix-blend-multiply'

type Props = { me: Me; justConnected?: boolean; onConnect: () => void; onChanged: () => void; onSignedOut: () => void }

export function Account({ me, justConnected, onConnect, onChanged, onSignedOut }: Props) {
  const [signingOut, setSigningOut] = useState(false)
  const [error, setError] = useState('')

  const signOut = async () => {
    setSigningOut(true)
    setError('')
    try {
      await api.signOut()
      onSignedOut()
    } catch {
      setError("Couldn't sign out. Try again.")
      setSigningOut(false)
    }
  }

  const usc = me.connections.find((c) => c.kind === 'usc_account')

  return (
    <div className="mx-auto flex w-full max-w-3xl animate-turn flex-col gap-10 px-6 pt-6 pb-12 sm:px-10">
        <h1 className="relative self-start font-serif text-[30px] leading-[1.12] font-medium tracking-[-0.02em] text-balance [font-variation-settings:'opsz'_72,'SOFT'_60]">
          Hi, {usc?.first_name || 'there'}
          <Underline className="absolute -bottom-2.5 -left-[2%] h-3 w-[104%] stroke-[2.6] text-cardinal" delay={150} />
        </h1>
        <div className="grid items-start gap-6 sm:grid-cols-2">
        <Sticker title="Phone number" icon={<BubbleIcon />} tone="ink">
          <div className="flex items-center justify-between gap-2 self-stretch">
            <span className="font-hand text-[32px] leading-none font-semibold text-ink lining-nums">{formatPhone(me.phone_number)}</span>
            <Stamp label="Confirmed" className={stamp} />
          </div>
        </Sticker>
        {me.connections.map((c) =>
          c.kind === 'usc_account' ? (
            <USCAccount key={c.kind} connection={c} justConnected={justConnected} onConnect={onConnect} />
          ) : (
            <OtherConnection key={c.kind} connection={c} />
          ),
        )}
        </div>
        <div className="flex flex-col items-start gap-3">
          {error && (
            <p className="text-[15px] font-medium text-cardinal" role="alert">
              {error}
            </p>
          )}
          <Button variant="pencil" size="md" className="min-w-36" onClick={signOut} disabled={signingOut} data-busy={signingOut || undefined}>
            {signingOut ? <Dots label="Signing out" /> : 'Sign out'}
          </Button>
          {usc?.status === 'connected' && <DisconnectUSC netId={usc.net_id!} onDisconnected={onChanged} />}
        </div>
    </div>
  )
}

function USCAccount({ connection, justConnected, onConnect }: { connection: Connection; justConnected?: boolean; onConnect: () => void }) {
  return (
    <Sticker title="USC account" icon={<img src={uscLogo} alt="" className="h-[38px] w-[30px] object-contain drop-shadow-[0_1px_0_rgba(0,0,0,0.25)]" />} tone="cardinal">
      {connection.status === 'connected' ? (
        <div className="flex items-center justify-between gap-2 self-stretch">
          <span className="font-hand text-[32px] leading-none font-semibold text-ink lining-nums">{connection.net_id}</span>
          <Stamp label="Connected" className={cn(stamp, justConnected && 'animate-thump')} />
        </div>
      ) : (
        <Button variant="cardinal" size="tall" className="mt-2 self-stretch" onClick={onConnect}>
          Connect USC account
        </Button>
      )}
    </Sticker>
  )
}

function DisconnectUSC({ netId, onDisconnected }: { netId: string; onDisconnected: () => void }) {
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')

  const disconnect = async () => {
    setBusy(true)
    setError('')
    try {
      await api.disconnectUSCAccount()
      setOpen(false)
      onDisconnected()
    } catch (e) {
      if (e instanceof ApiError && e.code === 'not_signed_in') return
      setError("Couldn't disconnect. Try again.")
      setBusy(false)
    }
  }

  return (
    <>
      <Button variant="link" size="inline" className="min-h-11 text-[15px]" onClick={() => (setError(''), setOpen(true))}>
        Disconnect USC account
      </Button>
      <Dialog open={open} onOpenChange={(o) => busy || setOpen(o)}>
        <DialogContent
          showCloseButton={false}
          onPointerDownOutside={(e) => e.preventDefault()}
          className="flex w-[min(340px,calc(100vw-32px))] max-w-none -rotate-1 flex-col gap-3 overflow-visible rounded-[3px] bg-card bg-(image:--grain) px-6 pt-8 pb-6 text-[16px] text-ink shadow-note ring-0 sm:max-w-none data-open:animate-[turn_0.3s_cubic-bezier(0.2,0.7,0.2,1)_backwards]"
        >
          <span className="tape -top-3 left-1/2 -ml-12 w-24 rotate-2" aria-hidden />
          <DialogTitle className="font-serif text-[24px] leading-[1.2] font-medium tracking-[-0.015em] [font-variation-settings:'opsz'_48,'SOFT'_60]">Disconnect {netId}?</DialogTitle>
          <DialogDescription className="text-[15px] text-ink-2">Troy deletes your NetID, password, and bypass code.</DialogDescription>
          {error && (
            <p className="text-[15px] font-medium text-cardinal" role="alert">
              {error}
            </p>
          )}
          <div className="mt-3 flex justify-end gap-2">
            <Button variant="pencil" size="md" onClick={() => setOpen(false)} disabled={busy}>
              Cancel
            </Button>
            <Button variant="cardinal" size="md" onClick={disconnect} disabled={busy} data-busy={busy || undefined}>
              {busy ? <Dots label="Disconnecting" /> : 'Disconnect'}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}

// A card stuck on the page: a colored band with its title, then what Troy has on file.
function Sticker({ title, icon, tone, children }: { title: string; icon: ReactNode; tone: 'ink' | 'cardinal'; children: ReactNode }) {
  return (
    <section className={cn('relative overflow-hidden rounded-md bg-card shadow-sticker', tone === 'ink' ? 'rotate-[0.5deg]' : '-rotate-[0.6deg]')} aria-label={title}>
      <header className={cn('relative flex items-center gap-3 bg-(image:--grain) px-4 py-3 text-white', tone === 'ink' ? 'bg-ink' : 'bg-cardinal')}>
        {icon}
        <h2 className="font-sans text-[15px] leading-[1.2] font-semibold">{title}</h2>
      </header>
      <div className="flex flex-col items-start gap-4 px-4 pt-6 pb-4">{children}</div>
    </section>
  )
}

function BubbleIcon() {
  return (
    <svg className="h-[38px] w-[30px] fill-white px-px py-1" viewBox="0 0 24 24" aria-hidden>
      <path d="M12 4C7 4 3 7.2 3 11.2c0 2.3 1.3 4.3 3.4 5.6L5.6 20l3.9-2.2c.8.2 1.6.3 2.5.3 5 0 9-3.2 9-7.2S17 4 12 4Z" />
    </svg>
  )
}

// A kind of connection this page doesn't know yet: show what the API says, nothing more.
function OtherConnection({ connection }: { connection: Connection }) {
  const name = connection.kind.replace(/_/g, ' ')
  return (
    <section className="relative flex items-baseline justify-between rounded-[4px] bg-card p-4 shadow-card">
      <h2 className="font-serif text-[19px] leading-[1.25] font-semibold tracking-[-0.01em] [font-variation-settings:'opsz'_24,'SOFT'_50]">{name.charAt(0).toUpperCase() + name.slice(1)}</h2>
      <span className="text-[15px] text-ink-2">{connection.status === 'connected' ? 'Connected' : 'Not connected'}</span>
    </section>
  )
}

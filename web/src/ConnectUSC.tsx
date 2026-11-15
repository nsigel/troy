import { useId, useRef, useState, type ComponentProps, type FormEvent, type ReactNode } from 'react'
import { api, ApiError } from './api'
import { Dots } from './Doodles'
import { Screen } from './Screen'
import uscLogo from './assets/usc.svg'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'

type Field = 'net_id' | 'password' | 'duo_bypass_code' | 'form'
type Failure = { field: Field; message: string }

const BYPASS_CODE_PAGE = 'https://account.usc.edu/2fa/duo-bypass-code'

function failure(e: unknown): Failure {
  if (!(e instanceof ApiError)) return { field: 'form', message: "Couldn't reach Troy. Try again." }
  switch (e.code) {
    case 'password_rejected':
      return { field: 'password', message: "USC didn't accept this NetID and password." }
    case 'duo_bypass_code_rejected':
      return { field: 'duo_bypass_code', message: "Duo didn't accept this code. Get a new one." }
    case 'invalid_request':
      return { field: 'form', message: 'Fill in all three fields.' }
    case 'usc_unavailable':
      return { field: 'form', message: "Couldn't reach USC. Try again in a bit." }
    default:
      return { field: 'form', message: e.message }
  }
}

export function ConnectUSC({ onBack, onConnected }: { onBack: () => void; onConnected: () => void }) {
  const [netId, setNetId] = useState('')
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [show, setShow] = useState(false)
  const [busy, setBusy] = useState(false)
  const [failed, setFailed] = useState<Failure>()
  const passwordInput = useRef<HTMLInputElement>(null)
  const codeInput = useRef<HTMLInputElement>(null)
  const id = useId()

  const ready = netId !== '' && password !== '' && code !== ''

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (!ready || busy) return
    setBusy(true)
    setFailed(undefined)
    try {
      await api.connectUSCAccount({ net_id: netId, password, duo_bypass_code: code })
      onConnected()
    } catch (err) {
      if (err instanceof ApiError && err.code === 'not_signed_in') return // the app goes back to sign-in
      const f = failure(err)
      setFailed(f)
      setBusy(false)
      const input = f.field === 'password' ? passwordInput : f.field === 'duo_bypass_code' ? codeInput : undefined
      requestAnimationFrame(() => input?.current?.select())
    }
  }

  // Props that tie a field to its error.
  const errorFor = (field: Field) =>
    failed?.field === field
      ? { 'aria-invalid': true, 'aria-describedby': `${id}-${field}-error` }
      : {}
  const errorText = (field: Field) =>
    failed?.field === field && (
      <p id={`${id}-${field}-error`} className={cn('font-medium text-cardinal', field === 'form' ? 'text-[15px]' : 'text-[14px]')} role="alert">
        {failed.message}
      </p>
    )
  const edited = (field: Field) => failed?.field === field && setFailed(undefined)

  return (
    <Screen onBack={busy ? undefined : onBack}>
      <div className="flex flex-1 flex-col gap-6">
        <h1 className="font-serif text-[30px] leading-[1.12] font-medium tracking-[-0.02em] text-balance [font-variation-settings:'opsz'_72,'SOFT'_60]">Connect your USC account</h1>
        <section className="relative overflow-hidden rounded-md bg-card shadow-sticker">
          <header className={cn('relative flex items-center gap-3 bg-cardinal bg-(image:--grain) px-4 py-3 text-white', busy && 'busy-line')}>
            <img src={uscLogo} alt="USC" className="w-[30px] drop-shadow-[0_1px_0_rgba(0,0,0,0.25)]" />
          </header>
          <form className="flex flex-col gap-4 px-4 pt-6 pb-4" onSubmit={submit} aria-busy={busy}>
            <fieldset className="contents" disabled={busy}>
              <div className="flex flex-col gap-1.5">
                <label className="text-[14px] font-medium text-ink-2" htmlFor={`${id}-net-id`}>
                  NetID
                </label>
                <TextBox>
                  <BoxInput
                    id={`${id}-net-id`}
                    value={netId}
                    placeholder="ttrojan"
                    autoCapitalize="none"
                    autoCorrect="off"
                    autoComplete="username"
                    spellCheck={false}
                    {...errorFor('password') /* USC rejects the NetID and password together */}
                    onChange={(e) => {
                      setNetId(e.target.value.replace(/\s/g, ''))
                      edited('password')
                    }}
                  />
                  {!netId.includes('@') && (
                    <span className="pr-3 text-ink-3" aria-hidden>
                      @usc.edu
                    </span>
                  )}
                </TextBox>
              </div>

              <div className="flex flex-col gap-1.5">
                <label className="text-[14px] font-medium text-ink-2" htmlFor={`${id}-password`}>
                  Password
                </label>
                <TextBox>
                  <BoxInput
                    id={`${id}-password`}
                    ref={passwordInput}
                    type={show ? 'text' : 'password'}
                    value={password}
                    autoComplete="current-password"
                    {...errorFor('password')}
                    onChange={(e) => {
                      setPassword(e.target.value)
                      edited('password')
                    }}
                  />
                  <button type="button" className="h-full px-3 text-[14px] font-medium text-ink-2 hover:text-ink" onClick={() => setShow(!show)} aria-pressed={show}>
                    {show ? 'Hide' : 'Show'}
                  </button>
                </TextBox>
                {errorText('password')}
              </div>

              <div className="flex flex-col gap-1.5">
                <div className="flex items-baseline justify-between">
                  <label className="text-[14px] font-medium text-ink-2" htmlFor={`${id}-code`}>
                    Duo bypass code
                  </label>
                  <Button asChild variant="link" size="inline" className="text-[14px]">
                    <a href={BYPASS_CODE_PAGE} target="_blank" rel="noreferrer">
                      Get a code
                    </a>
                  </Button>
                </div>
                <TextBox>
                  <BoxInput
                    id={`${id}-code`}
                    ref={codeInput}
                    className="tracking-[0.08em] tabular-nums"
                    value={code}
                    inputMode="numeric"
                    autoComplete="off"
                    spellCheck={false}
                    {...errorFor('duo_bypass_code')}
                    onChange={(e) => {
                      setCode(e.target.value.replace(/\D/g, ''))
                      edited('duo_bypass_code')
                    }}
                  />
                </TextBox>
                {errorText('duo_bypass_code')}
              </div>
            </fieldset>

            {errorText('form')}
            <Button variant="cardinal" size="tall" className="mt-2" data-busy={busy || undefined} disabled={!ready || busy} aria-describedby={failed?.field === 'form' ? `${id}-form-error` : undefined}>
              {busy ? <Dots label="Signing in to USC" /> : 'Connect USC account'}
            </Button>
          </form>
        </section>
      </div>
    </Screen>
  )
}

// An input in a white box, with room for a suffix or a toggle after it.
function TextBox({ children }: { children: ReactNode }) {
  return (
    <span className="flex h-[50px] items-center rounded-lg border-[1.5px] border-input bg-white transition-[border-color,box-shadow] duration-150 focus-within:border-ink focus-within:shadow-[0_0_0_3px_rgb(255_204_0/0.45)] has-aria-invalid:border-cardinal has-disabled:bg-[#faf7f0] has-disabled:text-ink-2">
      {children}
    </span>
  )
}

function BoxInput({ className, ...props }: ComponentProps<'input'>) {
  return (
    <Input
      className={cn(
        'h-full flex-1 rounded-none border-0 bg-transparent px-3 text-[17px] placeholder:text-[#b9b2a5] focus-visible:ring-0 disabled:pointer-events-auto disabled:cursor-default disabled:bg-transparent disabled:opacity-100 aria-invalid:ring-0 md:text-[17px]',
        className,
      )}
      {...props}
    />
  )
}

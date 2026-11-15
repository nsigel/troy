import { useEffect, useMemo, useState, type ReactNode } from 'react'
import { encode } from 'uqr'
import { api, formatPhone, type Me, type SignIn as Instructions } from './api'
import { Dots, Helmet, Underline } from './Doodles'
import { Screen } from './Screen'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

// Phones (and iPads, which say they are Macs) send the text themselves. A Mac can too,
// through Messages, but gets the QR code as a second way. Anything else gets the QR code.
const ua = navigator.userAgent
const isPhone = /iPhone|iPad|iPod|Android/.test(ua) || (/Macintosh/.test(ua) && navigator.maxTouchPoints > 1)
const isMac = !isPhone && /Macintosh/.test(ua)

const POLL_MS = 5000

export function SignIn({ onSignedIn }: { onSignedIn: (me: Me) => void }) {
  const [how, setHow] = useState<Instructions>()
  const [failed, setFailed] = useState(false)
  const [qr, setQr] = useState(!isPhone && !isMac)

  const load = () => api.signIn().then(setHow, () => setFailed(true))
  useEffect(() => void load(), [])

  // Opening the texted link in this browser signs it in; notice that and move on.
  useEffect(() => {
    let done = false
    const check = () => {
      if (document.visibilityState !== 'visible') return
      api.me().then((me) => {
        if (me && !done) {
          done = true
          onSignedIn(me)
        }
      }, () => {})
    }
    const timer = setInterval(check, POLL_MS)
    document.addEventListener('visibilitychange', check)
    addEventListener('focus', check)
    return () => {
      done = true
      clearInterval(timer)
      document.removeEventListener('visibilitychange', check)
      removeEventListener('focus', check)
    }
  }, [onSignedIn])

  const sms = how && `sms:${how.troy_phone_number}&body=${encodeURIComponent(how.sign_in_phrase)}`

  return (
    <Screen>
      <div className="flex flex-1 flex-col gap-6">
        <div className="relative mt-8 mb-6 self-start short:mt-3 shorter:mt-0 shorter:mb-3">
          <h1 className="relative inline-block font-serif text-[76px] leading-[0.95] font-medium tracking-[-0.035em] [font-variation-settings:'opsz'_144,'SOFT'_100,'WONK'_1] short:text-[60px] shorter:text-[52px]">
            Troy
            <Underline className="absolute -bottom-3.5 -left-[2%] h-4 w-[104%] stroke-[3.2]" delay={200} />
            <Helmet className="absolute -top-2.5 -right-[50px] w-[50px] rotate-8 text-cardinal [--w:2.6] [&_path]:[animation-delay:calc(700ms+var(--d,0ms))] [&_path]:[animation-duration:0.55s]" />
          </h1>
          <p className="mt-6 text-[15px] text-ink-2 short:mt-3">Your assistant at USC</p>
        </div>

        {/* a phone screenshot taped onto the paper; fixed height so cycling never moves the page */}
        <figure className="relative my-auto h-80 -rotate-[1.5deg] rounded-[28px] bg-white px-4 py-[22px] font-[-apple-system,BlinkMacSystemFont,'SF_Pro_Text',system-ui,sans-serif] shadow-photo">
          {/* clear tape, unlike the patterned washi holding the sheet */}
          <span className="absolute -top-[11px] left-1/2 z-1 -ml-[42px] h-6 w-[84px] rotate-3 border-x border-dashed border-black/6 bg-white/60 shadow-[0_1px_2px_rgba(0,0,0,0.08)]" aria-hidden />
          {!qr ? (
            <Convo />
          ) : (
            how && (
              <div className="flex h-full flex-col items-center justify-center gap-3">
                <QrCode text={sms!} />
                <p className="font-hand text-[24px] leading-[1.1] font-semibold text-ink">or text {formatPhone(how.troy_phone_number)}</p>
              </div>
            )
          )}
        </figure>

        <div className="mt-auto flex flex-col gap-3">
          {failed && (
            <p className="text-[15px] font-medium text-cardinal" role="alert">
              Couldn't reach Troy.
            </p>
          )}
          {failed ? (
            <Button variant="ink" size="tall" onClick={() => (setFailed(false), load())}>
              Try again
            </Button>
          ) : (
            (isPhone || isMac) && (
              <Button asChild variant="ink" size="tall">
                <a href={sms} aria-disabled={!how}>
                  Sign in with iMessage
                </a>
              </Button>
            )
          )}
          {isMac && how && !qr && (
            <Button variant="link" size="inline" className="min-h-11 self-center text-[15px]" onClick={() => setQr(true)}>
              Use my phone instead
            </Button>
          )}
        </div>
      </div>
    </Screen>
  )
}

// Things you can text Troy, played as a thread on a loop: you ask, Troy types, Troy answers.
const EXAMPLES: [ask: string, reply: string][] = [
  ['what do i have tomorrow', 'csci 104 at 10 in SGM 124, then math 226 at 2. your 4pm lab is canceled'],
  ['anything new on brightspace?', 'econ 203 posted problem set 6, due next wed at 11:59. want me to remind you monday?'],
  ['when is my writ 150 essay due', 'friday at 5pm, submitted on brightspace. 1500–2000 words'],
  ['help me study for my bisc 120 midterm', "it's thursday, covers ch 4–7. want me to quiz you on cellular respiration tonight?"],
  ['remind me to register for spring classes', 'your registration opens nov 18 at 9am. i’ll text you at 8:50'],
]

// How long each beat lasts: the question lands, Troy types, the answer sits, the thread clears.
const BEATS = [900, 1400, 4200, 400]

const reduceMotion = matchMedia('(prefers-reduced-motion: reduce)').matches

function Convo() {
  const [at, setAt] = useState({ i: 0, beat: reduceMotion ? 2 : 0 })

  useEffect(() => {
    if (reduceMotion) return
    const t = setTimeout(
      () => setAt(({ i, beat }) => (beat < BEATS.length - 1 ? { i, beat: beat + 1 } : { i: (i + 1) % EXAMPLES.length, beat: 0 })),
      BEATS[at.beat],
    )
    return () => clearTimeout(t)
  }, [at])

  const [ask, reply] = EXAMPLES[at.i]
  return (
    <>
      <ul className="sr-only">
        {EXAMPLES.map(([ask, reply]) => (
          <li key={ask}>
            You: “{ask}” Troy: “{reply}”
          </li>
        ))}
      </ul>
      {/* Messages' conversation header: avatar, then a glass name pill tucked under it */}
      <div aria-hidden className="mb-5 flex flex-col items-center">
        <Helmet className="relative z-1 size-14 overflow-hidden rounded-full bg-paper p-[9px] text-cardinal shadow-[inset_0_0_0_1.5px_var(--line)] [--w:3.4] [&_path]:animate-none [&_path]:[stroke-dashoffset:0]" />
        <span className="relative -mt-1 flex h-[30px] items-center gap-[5px] rounded-[15px] bg-white/70 pr-2 pl-[15px] text-[16px] leading-[21px] font-bold text-black shadow-pill backdrop-blur-[10px] backdrop-saturate-[1.7]">
          Troy
          <svg viewBox="0 0 8 16" className="h-4 w-2 stroke-[rgb(60_60_67/0.3)] stroke-2" fill="none" strokeLinecap="round" strokeLinejoin="round">
            <path d="M2.6 3.6 5.6 8l-3 4.4" />
          </svg>
        </span>
      </div>
      <div aria-hidden key={at.i} className={cn('flex flex-col gap-3.5 text-[17px] leading-[22px] transition-[opacity,translate] duration-300', at.beat === 3 && '-translate-y-1.5 opacity-0')}>
        <Bubble out>{ask}</Bubble>
        {at.beat === 1 && (
          <Bubble typing>
            <Dots label="" className="gap-1 [&_i]:size-2" />
          </Bubble>
        )}
        {at.beat >= 2 && <Bubble>{reply}</Bubble>}
      </div>
    </>
  )
}

// One message, with the tail Messages draws under the last bubble of each run.
// From Apple's Messages template: 20pt radius plus a separate tail.
function Bubble({ out, typing, children }: { out?: boolean; typing?: boolean; children: ReactNode }) {
  return (
    <p
      className={cn(
        'relative max-w-[72%] animate-pop rounded-[20px] bg-(--bubble) px-3.5 py-[9px]',
        out ? 'self-end text-white [--bubble:#0088ff]' : 'self-start text-black [--bubble:#e9e9ea]',
        typing && 'flex h-10 items-center text-[#8e8e93]',
      )}
    >
      {children}
      <svg className={cn('absolute -bottom-2 h-[13px] w-[33px] fill-(--bubble)', out ? 'right-0 -scale-x-100' : 'left-0')} viewBox="0 35 33 13">
        <path d="M32 40C30.87 40 23.62 40 22.3 40C20.98 40 20.58 40.45 19.25 41.37C18.54 41.85 15.8 43.91 13.53 45.13C11.57 46.19 9.88 46.91 9.46 46.94C8.21 47.04 8.05 45.88 8.54 45.25C9.03 44.63 9.59 43.81 9.8 43.37C10.29 42.38 10.43 41.87 10.43 40.39C10.43 38.57 9.11 37.01 8.07 36.11C7.81 35.89 7.34 35.48 7 35.2Z" />
      </svg>
    </p>
  )
}

function QrCode({ text }: { text: string }) {
  const { size, path } = useMemo(() => {
    const { size, data } = encode(text, { ecc: 'M', border: 0 })
    let path = ''
    data.forEach((row, y) => row.forEach((dark, x) => dark && (path += `M${x} ${y}h1v1h-1z`)))
    return { size, path }
  }, [text])
  return (
    <svg className="block size-50 fill-ink"viewBox={`-2 -2 ${size + 4} ${size + 4}`} role="img" aria-label="QR code that opens Messages with the text ready to send" shapeRendering="crispEdges">
      <path d={path} />
    </svg>
  )
}

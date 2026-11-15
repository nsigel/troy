import { lazy, Suspense, useEffect, useState } from 'react'
import { api, SIGNED_OUT, type Me } from './api'
import { SignIn } from './SignIn'

type State = { at: 'loading' } | { at: 'signed-out' } | { at: 'signed-in'; me: Me }

const Chats = lazy(() => import('./Chats'))

export default function App() {
  const [state, setState] = useState<State>({ at: 'loading' })

  const enter = (me: Me | null) => setState(me ? { at: 'signed-in', me } : { at: 'signed-out' })
  const refresh = () => api.me().then(enter)

  useEffect(() => {
    api.me().then(enter, () => setState({ at: 'signed-out' }))
    const out = () => setState((s) => (s.at === 'signed-out' ? s : { at: 'signed-out' }))
    addEventListener(SIGNED_OUT, out)
    return () => removeEventListener(SIGNED_OUT, out)
  }, [])

  const blank = <div className="h-dvh bg-paper-2 bg-(image:--grain)" />
  if (state.at === 'loading') return blank
  if (state.at === 'signed-in')
    return (
      <Suspense fallback={blank}>
        <Chats me={state.me} onChanged={refresh} onSignedOut={() => setState({ at: 'signed-out' })} />
      </Suspense>
    )

  return (
    <div className="min-h-dvh bg-paper bg-(image:--grain) sm:grid sm:place-items-center sm:px-6 sm:py-12 sm:bg-desk sm:bg-[image:radial-gradient(ellipse_at_50%_40%,rgba(255,255,255,0.35),transparent_70%),var(--grain)]">
      <main className="relative flex min-h-dvh flex-col sm:h-[min(844px,calc(100dvh-96px))] sm:min-h-0 sm:w-[390px] sm:-rotate-[0.5deg] sm:rounded-[3px] sm:bg-paper sm:bg-(image:--grain) sm:shadow-sheet sm:after:absolute sm:after:inset-[6px_-5px_-7px_5px] sm:after:-z-1 sm:after:rotate-[1.3deg] sm:after:rounded-[3px] sm:after:bg-paper-2 sm:after:shadow-under">
        <span className="tape -top-3.5 left-1/2 -ml-14 hidden w-28 -rotate-3 sm:block" aria-hidden />
        <SignIn onSignedIn={enter} />
      </main>
    </div>
  )
}

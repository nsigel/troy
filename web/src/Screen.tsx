import type { ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { BackArrow, Helmet } from './Doodles'

// The sheet every screen sits on. Signed-in screens get a top row: a back arrow when there is
// somewhere to go back to, and Troy's mark in the middle. Then the screen, which turns in like a page.
export function Screen({ mark, onBack, children }: { mark?: boolean; onBack?: () => void; children: ReactNode }) {
  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto px-6 pt-3 pb-8 short:pb-4">
      {(mark || onBack) && (
        <header className="mb-4 grid h-11 grid-cols-[40px_1fr_40px] items-center">
          {onBack && (
            <Button variant="ghost" size="icon" className="col-start-1 -ml-2 size-10 rounded-full hover:bg-ink/6" onClick={onBack} aria-label="Back">
              <BackArrow className="size-6 stroke-ink stroke-[1.9]" />
            </Button>
          )}
          {mark && <span className="relative col-start-2 inline-block justify-self-center font-serif text-[26px] leading-[0.95] font-medium tracking-[-0.02em] [font-variation-settings:'opsz'_144,'SOFT'_100,'WONK'_1]">
            Troy
            <Helmet className="absolute -top-1.5 -right-5 w-5 rotate-8 text-cardinal [--w:3] [&_path]:animate-none [&_path]:[stroke-dashoffset:0]" />
          </span>}
        </header>
      )}
      <div className="flex flex-1 animate-turn flex-col">{children}</div>
    </div>
  )
}

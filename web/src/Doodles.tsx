import type { CSSProperties } from 'react'
import { cn } from '@/lib/utils'

// Hand-drawn SVG bits. Every stroke uses pathLength=1 so `draw` can ink it on.

type P = { className?: string; delay?: number }
const d = (delay = 0) => ({ ['--d' as string]: `${delay}ms` }) as CSSProperties
const ink = { fill: 'none', stroke: 'currentColor', strokeLinecap: 'round', strokeLinejoin: 'round' } as const

export function Underline({ className, delay }: P) {
  return (
    <svg className={cn('draw overflow-visible', className)} style={d(delay)} {...ink} viewBox="0 0 200 16" preserveAspectRatio="none" aria-hidden>
      <path pathLength={1} d="M4 10.5C38 6 70 12.5 108 8.2S172 5.6 196 8.8" />
      <path pathLength={1} d="M30 13.2C72 10.4 120 12.8 168 11" style={d((delay ?? 0) + 260)} />
    </svg>
  )
}

// A Corinthian helmet, face on: T-shaped visor under a brush plume. Its stroke width is --w; the bristles get half.
export function Helmet({ className }: P) {
  return (
    <svg className={cn('draw overflow-visible [stroke-width:var(--w)]', className)} {...ink} viewBox="0 0 64 64" aria-hidden>
      <path pathLength={1} d="M29 60V43L21 40A3 3 0 0 1 21 34C25 34.5 29 35.5 32 37C35 35.5 39 34.5 43 34A3 3 0 0 1 43 40L35 43V60H41C45 60 48 57 49 53L50 36C50 25 42 16 32 16C22 16 14 25 14 36L15 53C16 57 19 60 23 60Z" />
      <path pathLength={1} style={d(500)} d="M29 16L25 5C27 3 37 3 39 5L35 16" />
      <path pathLength={1} style={d(800)} className="[stroke-width:calc(var(--w)/2)]" d="M30.3 4.6L32 16L33.7 4.6" />
    </svg>
  )
}

// Rubber stamp. Rough ink via a displacement filter + speckle mask.
export function Stamp({ label, className }: { label: string; className?: string }) {
  return (
    <svg className={cn('fill-[#a3201c] stroke-[#a3201c]', className)} viewBox="0 0 220 76" role="img" aria-label={label}>
      <defs>
        <filter id="hm-rough" x="-5%" y="-5%" width="110%" height="110%">
          <feTurbulence type="fractalNoise" baseFrequency=".9" numOctaves="2" seed="4" result="n" />
          <feDisplacementMap in="SourceGraphic" in2="n" scale="2.4" />
        </filter>
        <filter id="hm-speckle">
          <feTurbulence type="fractalNoise" baseFrequency=".75" numOctaves="1" seed="9" />
          <feColorMatrix values="0 0 0 0 1  0 0 0 0 1  0 0 0 0 1  0 0 0 -1.5 1.5" />
        </filter>
        <mask id="hm-ink">
          <rect width="220" height="76" filter="url(#hm-speckle)" />
        </mask>
      </defs>
      <g filter="url(#hm-rough)" mask="url(#hm-ink)">
        <rect x="5" y="5" width="210" height="66" rx="10" fill="none" strokeWidth="4.5" />
        <rect x="13" y="13" width="194" height="50" rx="5" fill="none" strokeWidth="1.6" />
        <text x="110" y="50" textAnchor="middle" className="stroke-none font-serif text-[27px] leading-none font-bold tracking-[0.08em] [font-variation-settings:'opsz'_72,'SOFT'_0]">
          {label.toUpperCase()}
        </text>
      </g>
    </svg>
  )
}

export function BackArrow({ className }: P) {
  return (
    <svg className={className} viewBox="0 0 24 24" aria-hidden fill="none" strokeLinecap="round" strokeLinejoin="round">
      <path d="M20 12.4C15 11.8 9.6 12 4.4 12.2" />
      <path d="M10.2 6.2C8.2 8.2 6.2 10.2 4.4 12.2C6.4 14 8.4 16 10.4 18" />
    </svg>
  )
}

export function Dots({ label, className }: { label: string; className?: string }) {
  return (
    <span className={cn('inline-flex items-baseline gap-[2px]', className)}>
      {label}
      {[0, 150, 300].map((delay) => (
        <i key={delay} aria-hidden className={cn('size-1 animate-dot rounded-full bg-current', label && delay === 0 && 'ml-1')} style={{ animationDelay: `${delay}ms` }} />
      ))}
    </span>
  )
}

// A red push pin seen from above: a domed head catching the light, its shadow falling on the paper.
export function Pin({ className }: { className?: string }) {
  return (
    <svg className={cn('overflow-visible drop-shadow-[3px_4px_2px_rgba(60,30,10,0.32)]', className)} viewBox="0 0 24 24" aria-hidden>
      <defs>
        <radialGradient id="pin-head" cx="36%" cy="32%" r="72%">
          <stop offset="0" stopColor="#ff7a6e" />
          <stop offset="0.42" stopColor="#c8171c" />
          <stop offset="1" stopColor="#5e0005" />
        </radialGradient>
      </defs>
      <circle cx="12" cy="12" r="9" fill="url(#pin-head)" />
      <circle cx="12" cy="12" r="6.2" fill="none" stroke="#000" strokeOpacity="0.14" strokeWidth="0.8" />
      <ellipse cx="8.6" cy="8.4" rx="2.6" ry="1.5" fill="#fff" opacity="0.6" transform="rotate(-35 8.6 8.4)" />
    </svg>
  )
}

// A faint pencil line, drawn freehand across the page.
export function Rule({ className }: { className?: string }) {
  return (
    <svg className={cn('h-2 w-full overflow-visible', className)} {...ink} viewBox="0 0 240 8" preserveAspectRatio="none" aria-hidden>
      <path d="M2 4.6C40 3.2 78 5.4 120 4.1S200 3 238 4.4" strokeWidth="1.2" vectorEffect="non-scaling-stroke" />
    </svg>
  )
}

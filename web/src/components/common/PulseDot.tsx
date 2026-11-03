import { cn } from '@/lib/utils'

/**
 * PulseDot is the "global pulse" beacon: a solid dot with one slow, low-opacity
 * ring. No blinking — a 1.8 s expansion that reads as activity, not alarm.
 */
export function PulseDot({
  tone = 'success',
  active = true,
  className,
}: {
  tone?: 'success' | 'warn' | 'danger' | 'accent'
  active?: boolean
  className?: string
}) {
  const tones = {
    success: 'bg-emerald-400',
    warn: 'bg-amber-400',
    danger: 'bg-red-400',
    accent: 'bg-sky-400',
  }
  return (
    <span className={cn('relative inline-flex h-2 w-2 shrink-0', className)} aria-hidden="true">
      {active ? (
        <span className={cn('beacon-ring absolute inset-0 rounded-full opacity-60', tones[tone])} />
      ) : null}
      <span className={cn('relative inline-flex h-2 w-2 rounded-full', tones[tone])} />
    </span>
  )
}

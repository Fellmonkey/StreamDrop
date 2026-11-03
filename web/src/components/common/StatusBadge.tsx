import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'

const TONES: Record<string, { dot: string; text: string; border: string }> = {
  queued: { dot: 'bg-slate-500', text: 'text-slate-300', border: 'border-white/10' },
  running: { dot: 'bg-sky-400 animate-pulse-soft', text: 'text-sky-300', border: 'border-sky-400/30' },
  paused: { dot: 'bg-amber-400', text: 'text-amber-300', border: 'border-amber-400/30' },
  retrying: { dot: 'bg-amber-400 animate-pulse-soft', text: 'text-amber-300', border: 'border-amber-400/30' },
  completed: { dot: 'bg-emerald-400', text: 'text-emerald-300', border: 'border-emerald-400/30' },
  failed: { dot: 'bg-red-400', text: 'text-red-300', border: 'border-red-400/30' },
  aborted: { dot: 'bg-slate-500', text: 'text-slate-400', border: 'border-white/10' },
  online: { dot: 'bg-emerald-400', text: 'text-emerald-300', border: 'border-emerald-400/30' },
  error: { dot: 'bg-red-400', text: 'text-red-300', border: 'border-red-400/30' },
  unavailable: { dot: 'bg-slate-500', text: 'text-slate-400', border: 'border-white/10' },
  unknown: { dot: 'bg-slate-500', text: 'text-slate-400', border: 'border-white/10' },
  installed: { dot: 'bg-emerald-400', text: 'text-emerald-300', border: 'border-emerald-400/30' },
  missing: { dot: 'bg-slate-500', text: 'text-slate-400', border: 'border-white/10' },
  outdated: { dot: 'bg-amber-400', text: 'text-amber-300', border: 'border-amber-400/30' },
  corrupted: { dot: 'bg-red-400', text: 'text-red-300', border: 'border-red-400/30' },
}

/** StatusBadge renders a localized status with a state-colored dot. */
export function StatusBadge({ status, className }: { status: string; className?: string }) {
  const { t } = useTranslation()
  const tone = TONES[status] ?? TONES.unknown
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-[11px] font-medium',
        tone.border,
        tone.text,
        className,
      )}
    >
      <span className={cn('h-1.5 w-1.5 rounded-full', tone.dot)} />
      {t(`status.${status}`, { defaultValue: status })}
    </span>
  )
}

/** ErrorText localizes an API error by its machine-readable code. */
export function ErrorText({ code, details }: { code?: string | null; details?: string }) {
  const { t } = useTranslation()
  if (!code) return null
  return (
    <p className="text-xs text-red-300">
      {t(`errors.${code}`, { defaultValue: t('errors.unknown') })}
      {details ? <span className="ml-1 text-red-300/60">({details})</span> : null}
    </p>
  )
}

/** EmptyState is the shared zero-data block. */
export function EmptyState({
  icon,
  title,
  body,
  action,
  compact,
}: {
  icon?: React.ReactNode
  title: string
  body?: string
  action?: React.ReactNode
  compact?: boolean
}) {
  return (
    <div
      className={cn(
        'flex flex-col items-center justify-center gap-2 rounded-xl border border-dashed border-white/10 text-center',
        compact ? 'px-4 py-6' : 'px-6 py-12',
      )}
    >
      {icon ? <div className="text-slate-600">{icon}</div> : null}
      <p className="text-sm font-medium text-slate-300">{title}</p>
      {body ? <p className="max-w-md text-xs leading-relaxed text-slate-500">{body}</p> : null}
      {action ? <div className="mt-2">{action}</div> : null}
    </div>
  )
}

import { useTranslation } from 'react-i18next'
import { Cloud, Cpu, Globe } from 'lucide-react'
import { formatPercent } from '@/lib/formatters'

interface StreamDiagramProps {
  mode: string
  progress: number
  bufferPct: number
  active: boolean
}

/**
 * StreamDiagram visualizes the three stages. In pure stream mode the buffer
 * node is replaced by a direct channel so the UI never implies disk spooling.
 */
export function StreamDiagram({ mode, progress, bufferPct, active }: StreamDiagramProps) {
  const { t } = useTranslation()
  const pure = mode === 'pure_stream'
  const safeProgress = Math.max(0, Math.min(100, progress))

  return (
    <div className="relative overflow-hidden rounded-xl border border-white/[0.06] bg-black/20 px-4 py-3">
      <div className="grid-lines pointer-events-none absolute inset-0 opacity-30" />
      <div className="relative flex items-center justify-between gap-2">
        <Node icon={<Globe className="h-4 w-4" />} label={t('pipeline.source')} tone="sky" />

        <div className="relative mx-1 flex-1">
          <svg viewBox="0 0 100 8" preserveAspectRatio="none" className="h-2 w-full">
            <line x1="0" y1="4" x2="100" y2="4" stroke="rgba(255,255,255,0.08)" strokeWidth="1" />
            {active ? (
              <line
                x1="0"
                y1="4"
                x2={Math.max(4, safeProgress)}
                y2="4"
                stroke="#38bdf8"
                strokeWidth="1.5"
                className="flow-line"
              />
            ) : null}
          </svg>
          {active ? (
            <div
              className="absolute top-1/2 h-1.5 w-1.5 -translate-y-1/2 rounded-full bg-sky-300 shadow-[0_0_8px_2px_rgba(56,189,248,0.6)] transition-[left] duration-500"
              style={{ left: `${safeProgress}%` }}
            />
          ) : null}
        </div>

        <Node
          icon={<Cpu className="h-4 w-4" />}
          label={pure ? t('pipeline.direct') : t('pipeline.buffer')}
          tone={pure ? 'violet' : 'slate'}
          detail={pure ? undefined : formatPercent(bufferPct, 0)}
        />

        <div className="relative mx-1 flex-1">
          <svg viewBox="0 0 100 8" preserveAspectRatio="none" className="h-2 w-full">
            <line x1="0" y1="4" x2="100" y2="4" stroke="rgba(255,255,255,0.08)" strokeWidth="1" />
            {active ? (
              <line x1="0" y1="4" x2="100" y2="4" stroke="#34d399" strokeWidth="1.5" className="flow-line" />
            ) : null}
          </svg>
        </div>

        <Node icon={<Cloud className="h-4 w-4" />} label={t('pipeline.cloud')} tone="emerald" />
      </div>
    </div>
  )
}

function Node({
  icon,
  label,
  detail,
  tone,
}: {
  icon: React.ReactNode
  label: string
  detail?: string
  tone: 'sky' | 'slate' | 'violet' | 'emerald'
}) {
  const tones = {
    sky: 'border-sky-400/25 bg-sky-400/10 text-sky-300',
    slate: 'border-white/10 bg-white/[0.04] text-slate-300',
    violet: 'border-violet-400/25 bg-violet-400/10 text-violet-300',
    emerald: 'border-emerald-400/25 bg-emerald-400/10 text-emerald-300',
  }
  return (
    <div className="flex min-w-[62px] flex-col items-center gap-1">
      <div className={`flex h-8 w-8 items-center justify-center rounded-lg border ${tones[tone]}`}>{icon}</div>
      <span className="text-center text-[9px] uppercase tracking-wide text-slate-500">{label}</span>
      {detail ? <span className="numeric text-[9px] text-slate-400">{detail}</span> : null}
    </div>
  )
}

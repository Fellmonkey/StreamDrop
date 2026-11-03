import { useTranslation } from 'react-i18next'
import type { Quota } from '@/types/api'
import { formatBytes, pct } from '@/lib/formatters'

/** RadialQuota renders used vs total storage as a compact ring. */
export function RadialQuota({ quota, size = 96 }: { quota?: Quota | null; size?: number }) {
  const { t, i18n } = useTranslation()
  if (!quota || quota.total <= 0) {
    return <p className="text-[11px] text-slate-500">{t('connections.quotaUnknown')}</p>
  }
  const percent = pct(quota.used, quota.total)
  const radius = (size - 10) / 2
  const circumference = 2 * Math.PI * radius
  const offset = circumference * (1 - percent / 100)
  const color = percent > 90 ? '#f87171' : percent > 75 ? '#fbbf24' : '#38bdf8'

  return (
    <div className="flex items-center gap-3">
      <div className="relative" style={{ width: size, height: size }}>
        <svg width={size} height={size} className="-rotate-90">
          <circle cx={size / 2} cy={size / 2} r={radius} fill="none" stroke="rgba(255,255,255,0.08)" strokeWidth="6" />
          <circle
            cx={size / 2}
            cy={size / 2}
            r={radius}
            fill="none"
            stroke={color}
            strokeWidth="6"
            strokeLinecap="round"
            strokeDasharray={circumference}
            strokeDashoffset={offset}
            className="transition-[stroke-dashoffset] duration-200 ease-[cubic-bezier(0.16,1,0.3,1)]"
          />
        </svg>
        <div className="absolute inset-0 flex flex-col items-center justify-center">
          <span className="numeric text-xs font-semibold text-slate-100">{percent.toFixed(0)}%</span>
          <span className="text-[8px] uppercase tracking-wide text-slate-500">{t('explorer.quota')}</span>
        </div>
      </div>
      <div className="space-y-1 text-[11px]">
        <div>
          <p className="text-slate-500">{t('connections.used')}</p>
          <p className="numeric text-slate-300">{formatBytes(quota.used, i18n.language)}</p>
        </div>
        <div>
          <p className="text-slate-500">{t('connections.free')}</p>
          <p className="numeric text-emerald-300">{formatBytes(quota.free, i18n.language)}</p>
        </div>
      </div>
    </div>
  )
}

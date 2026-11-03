import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Cloud, Droplet, HardDrive, Moon, Package, Settings as SettingsIcon, ShieldCheck, Sun } from 'lucide-react'
import type { Stats, Status } from '@/types/api'
import { formatBytes } from '@/lib/formatters'
import { Badge, Button, Tooltip } from '@/components/ui/primitives'
import { LanguageSwitcher } from '@/components/common/LanguageSwitcher'
import { AnimatedNumber } from '@/components/common/AnimatedNumber'
import { PulseDot } from '@/components/common/PulseDot'
import { applyTheme, readTheme } from '@/lib/theme'
import { cn } from '@/lib/utils'

interface HeaderProps {
  status?: Status
  stats?: Stats
  onOpenSettings: () => void
  onOpenConnections: () => void
  onOpenDeps: () => void
}

/** Header is the persistent product bar: logo, saved resource, engine health. */
export function Header({ status, stats, onOpenSettings, onOpenConnections, onOpenDeps }: HeaderProps) {
  const { t, i18n } = useTranslation()
  const [dark, setDark] = useState<boolean>(readTheme() === 'dark')
  const saved = stats?.saved_bytes ?? 0
  const ready = Boolean(status?.engine_ready)

  const toggleTheme = () => {
    const next = !dark
    setDark(next)
    applyTheme(next ? 'dark' : 'light')
  }

  return (
    <header className="safe-top sticky top-0 z-30 border-b border-border bg-[var(--header-bg)] backdrop-blur-xl">
      <div className="mx-auto flex h-14 w-full max-w-[1440px] items-center gap-2 px-4 sm:gap-3 sm:px-6">
        <div className="flex items-center gap-2.5">
          <div className="flex h-8 w-8 items-center justify-center rounded-xl bg-gradient-to-br from-sky-400 to-cyan-500 shadow-[0_0_24px_-6px_rgba(56,189,248,0.8)]">
            <Droplet className="h-4 w-4 text-slate-950" strokeWidth={2.5} />
          </div>
          <div className="leading-tight">
            <p className="text-sm font-semibold tracking-tight text-slate-100">StreamDrop</p>
            <p className="hidden text-[10px] uppercase tracking-[0.14em] text-slate-500 sm:block">
              {t('stream.subtitle')}
            </p>
          </div>
        </div>

        <div className="mx-2 hidden h-6 w-px bg-white/[0.07] sm:block" />

        <Tooltip
          content={
            <div className="w-[min(18rem,calc(100vw-2rem))] space-y-2">
              <p className="text-[11px] font-semibold text-slate-100">{t('header.resourceSaved')}</p>
              <p className="text-[11px] leading-relaxed text-slate-400">{t('header.resourceNote')}</p>
              <div className="space-y-1.5 pt-1">
                {(stats?.wear.classes ?? []).map((cls) => (
                  <div key={cls.class} className="space-y-1">
                    <div className="flex items-center justify-between text-[11px]">
                      <span className="text-slate-300">
                        {cls.label}
                        {cls.approximate ? <span className="ml-1 text-slate-500">≈</span> : null}
                      </span>
                      <span className="numeric text-slate-400">{formatBytes(cls.bytes, i18n.language)}</span>
                    </div>
                    <div className="h-1 w-full overflow-hidden rounded-full bg-white/10">
                      <div className="h-full rounded-full bg-sky-400/70" style={{ width: `${Math.min(100, cls.percent)}%` }} />
                    </div>
                  </div>
                ))}
              </div>
              <p className="border-t border-white/[0.07] pt-2 text-[10px] text-slate-500">
                {t('header.average')}: {formatBytes(stats?.wear.average ?? 0, i18n.language)}
              </p>
            </div>
          }
        >
          <div className="hidden cursor-default items-center gap-2 rounded-lg border border-white/[0.07] bg-white/[0.03] px-2.5 py-1.5 sm:flex">
            <HardDrive className="h-3.5 w-3.5 text-emerald-300" />
            <div className="leading-none">
              <p className="text-[9px] uppercase tracking-[0.12em] text-slate-500">{t('header.saved')}</p>
              <p className="numeric mt-0.5 text-xs font-medium text-emerald-200">
                <AnimatedNumber value={saved} format={(v) => formatBytes(v, i18n.language)} />
              </p>
            </div>
          </div>
        </Tooltip>

        <div className="flex-1" />

        <Tooltip content={t('header.tabHint')}>
          <div className="hidden items-center gap-1.5 rounded-lg border border-white/[0.07] px-2.5 py-1.5 lg:flex">
            <ShieldCheck className="h-3.5 w-3.5 text-slate-500" />
            <div className="flex items-center gap-1.5">
              <PulseDot tone={ready ? 'success' : 'warn'} active={ready} />
              <span className={cn('text-[11px]', ready ? 'text-emerald-300' : 'text-amber-300')}>
                {t('header.engine')}: {ready ? t('header.ready') : t('header.degraded')}
              </span>
            </div>
          </div>
        </Tooltip>

        <Button
          variant="ghost"
          size="sm"
          onClick={onOpenDeps}
          className="gap-1.5"
          aria-label={t('header.components')}
        >
          <Package className="h-4 w-4" />
          <span className="hidden sm:inline">
            {status ? `${status.components_ok}/${status.components_total}` : '—'}
          </span>
        </Button>

        <Button
          variant="ghost"
          size="sm"
          onClick={onOpenConnections}
          className="gap-1.5"
          aria-label={t('header.connections')}
        >
          <Cloud className="h-4 w-4" />
          <span className="hidden md:inline">{t('header.connections')}</span>
        </Button>

        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="icon"
            onClick={toggleTheme}
            aria-label={t('settings.theme')}
            title={t('settings.theme')}
          >
            {dark ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
          </Button>
          <div className="hidden sm:block">
            <LanguageSwitcher compact />
          </div>
        </div>

        <Button variant="ghost" size="icon" onClick={onOpenSettings} aria-label={t('header.settings')}>
          <SettingsIcon className="h-4 w-4" />
        </Button>

        {status ? (
          <Badge tone={ready ? 'success' : 'warn'} className="hidden xl:inline-flex">
            {t('header.uptime')} {Math.floor(status.uptime_seconds / 60)}m
          </Badge>
        ) : null}
      </div>
    </header>
  )
}

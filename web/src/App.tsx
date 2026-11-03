import { useEffect, useRef, useState } from 'react'
import { motion } from 'framer-motion'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { ArrowDownToLine, ShieldCheck } from 'lucide-react'
import { api } from '@/lib/api'
import { useStreamSocket } from '@/hooks/useStreamSocket'
import { formatBytes, formatDuration } from '@/lib/formatters'
import type { NotifyEvent } from '@/types/api'
import { Header } from '@/components/common/Header'
import { FirstRunWizard } from '@/components/common/FirstRunWizard'
import { StreamDeck } from '@/components/stream/StreamDeck'
import { PipelinePanel } from '@/components/pipeline/ActiveTaskCard'
import { QueuePanel } from '@/components/queue/QueuePanel'
import { HistoryTable } from '@/components/history/HistoryTable'
import { ConnectionsPanel } from '@/components/connections/ConnectionsPanel'
import { DependencyManagerDialog } from '@/components/deps/DependencyManagerDialog'
import { AuthProfilesDialog } from '@/components/auth/AuthProfilesDialog'
import { SettingsDialog } from '@/components/settings/SettingsDialog'
import { PulseDot } from '@/components/common/PulseDot'
import { AnimatedNumber } from '@/components/common/AnimatedNumber'
import { EASE_OUT, FADE_UP } from '@/lib/motion'

/** App composes the single-page console and owns all dialog state. */
export function App() {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()

  const [settingsOpen, setSettingsOpen] = useState(false)
  const [connectionsOpen, setConnectionsOpen] = useState(false)
  const [depsOpen, setDepsOpen] = useState(false)
  const [profilesOpen, setProfilesOpen] = useState(false)
  const [onboardingOpen, setOnboardingOpen] = useState(false)
  const onboardingShown = useRef(false)

  const status = useQuery({ queryKey: ['status'], queryFn: api.status, refetchInterval: 15000 })
  const stats = useQuery({ queryKey: ['stats'], queryFn: api.stats, refetchInterval: 30000 })
  const remotes = useQuery({ queryKey: ['remotes'], queryFn: api.remotes, refetchInterval: 30000 })
  const deps = useQuery({ queryKey: ['deps'], queryFn: api.deps })
  const templates = useQuery({ queryKey: ['templates'], queryFn: api.templates, staleTime: 300_000 })
  const settings = useQuery({ queryKey: ['settings'], queryFn: api.settings })
  const queue = useQuery({ queryKey: ['queue'], queryFn: api.queue, refetchInterval: 5000 })
  const profiles = useQuery({ queryKey: ['profiles'], queryFn: api.profiles })
  const browsers = useQuery({ queryKey: ['browsers'], queryFn: api.browsers, enabled: profilesOpen })

  const socket = useStreamSocket(() => {
    void queryClient.invalidateQueries({ queryKey: ['queue'] })
    void queryClient.invalidateQueries({ queryKey: ['history'] })
    void queryClient.invalidateQueries({ queryKey: ['status'] })
    void queryClient.invalidateQueries({ queryKey: ['deps'] })
  })

  // Onboarding appears once per fresh install, never blocks the main UI.
  useEffect(() => {
    if (onboardingShown.current) return
    if (status.data && status.data.onboarded === false) {
      onboardingShown.current = true
      setOnboardingOpen(true)
    }
  }, [status.data])

  // Notifications become toasts; the socket payload carries counters only.
  const lastNotify = useRef<string>('')
  useEffect(() => {
    const event = socket.notify as NotifyEvent | null
    if (!event) return
    const key = JSON.stringify(event)
    if (key === lastNotify.current) return
    lastNotify.current = key
    const title = t(`notify.${event.kind}`, { defaultValue: event.kind })
    const parts: string[] = []
    if (event.files_count) parts.push(t('notify.files', { count: event.files_count }))
    if (event.failed_count) parts.push(t('notify.failed', { count: event.failed_count }))
    if (event.bytes) parts.push(t('notify.volume', { value: formatBytes(event.bytes, i18n.language) }))
    if (event.duration_seconds) parts.push(t('notify.duration', { value: formatDuration(event.duration_seconds) }))
    const description = parts.join(' · ')
    if (event.level === 'error' || event.kind === 'transfer_failed') {
      toast.error(title, { description })
    } else if (event.kind === 'transfer_aborted' || event.kind === 'queue_paused') {
      toast.warning(title, { description })
    } else {
      toast.success(title, { description })
    }
    socket.clearNotify()
  }, [socket.notify])

  const activeTasks = queue.data?.active_tasks ?? []

  return (
    <div className="min-h-dvh">
      <Header
        status={status.data}
        stats={stats.data}
        onOpenSettings={() => setSettingsOpen(true)}
        onOpenConnections={() => setConnectionsOpen(true)}
        onOpenDeps={() => setDepsOpen(true)}
      />

      <main className="mx-auto w-full max-w-[1440px] px-4 py-5 sm:px-6">
        <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_360px]">
          <div className="min-w-0 space-y-4">
            <motion.div
              initial={FADE_UP.initial}
              animate={FADE_UP.animate}
              transition={{ duration: 0.2, ease: EASE_OUT }}
            >
              <StreamDeck
                remotes={remotes.data ?? []}
                profiles={profiles.data ?? []}
                deps={deps.data?.components ?? []}
                templates={templates.data ?? []}
                settings={settings.data}
                onOpenConnections={() => setConnectionsOpen(true)}
                onOpenDeps={() => setDepsOpen(true)}
                onOpenProfiles={() => setProfilesOpen(true)}
              />
            </motion.div>

            <motion.section
              className="space-y-2"
              initial={FADE_UP.initial}
              animate={FADE_UP.animate}
              transition={{ duration: 0.2, ease: EASE_OUT, delay: 0.04 }}
            >
              <div className="flex items-center justify-between">
                <h2 className="flex items-center gap-2 text-xs font-semibold uppercase tracking-wide text-slate-500">
                  <ArrowDownToLine className="h-3.5 w-3.5" />
                  {t('pipeline.title')}
                </h2>
                <span className="flex items-center gap-1.5 text-[10px] text-slate-600">
                  <PulseDot tone={socket.connected ? 'success' : 'warn'} active={socket.connected} />
                  {socket.connected ? 'WS' : t('header.notConnected')}
                </span>
              </div>
              <PipelinePanel
                tasks={activeTasks}
                metrics={socket.metrics}
                logs={socket.logs}
              />
            </motion.section>
          </div>

          <motion.aside
            className="min-w-0"
            initial={FADE_UP.initial}
            animate={FADE_UP.animate}
            transition={{ duration: 0.2, ease: EASE_OUT, delay: 0.08 }}
          >
            <div className="space-y-4 lg:sticky lg:top-[calc(72px+env(safe-area-inset-top,0px))]">
              <QueuePanel state={queue.data} metrics={socket.metrics} />

              <div className="rounded-xl border border-white/[0.06] bg-white/[0.02] p-3">
                <div className="flex items-center gap-2 text-[11px] font-medium text-slate-300">
                  <ShieldCheck className="h-3.5 w-3.5 text-emerald-300" />
                  {t('header.resourceSaved')}
                </div>
                <p className="numeric mt-1.5 text-lg font-semibold text-emerald-200">
                  <AnimatedNumber
                    value={stats.data?.saved_bytes ?? 0}
                    format={(v) => formatBytes(v, i18n.language)}
                  />
                </p>
                <p className="mt-1 text-[10px] leading-relaxed text-slate-500">{t('header.resourceNote')}</p>
              </div>
            </div>
          </motion.aside>
        </div>

        <motion.div
          className="mt-4"
          initial={FADE_UP.initial}
          animate={FADE_UP.animate}
          transition={{ duration: 0.2, ease: EASE_OUT, delay: 0.12 }}
        >
          <HistoryTable />
        </motion.div>

        <footer className="safe-bottom mt-6 flex flex-wrap items-center justify-between gap-2 border-t border-white/[0.05] pt-3 text-[10px] text-slate-600">
          <span>
            StreamDrop {status.data?.version ?? ''} · 127.0.0.1:{status.data?.port ?? ''}
          </span>
          <span>{t('header.tabHint')}</span>
        </footer>
      </main>

      <SettingsDialog
        open={settingsOpen}
        onOpenChange={setSettingsOpen}
        settings={settings.data}
        status={status.data}
      />

      <ConnectionsPanel
        open={connectionsOpen}
        onOpenChange={setConnectionsOpen}
        remotes={remotes.data ?? []}
      />

      <DependencyManagerDialog
        open={depsOpen}
        onOpenChange={setDepsOpen}
        deps={deps.data?.components ?? []}
        progress={socket.depProgress}
        loading={deps.isLoading}
      />

      <AuthProfilesDialog
        open={profilesOpen}
        onOpenChange={setProfilesOpen}
        profiles={profiles.data ?? []}
        browsers={browsers.data ?? []}
      />

      <FirstRunWizard
        open={onboardingOpen}
        onOpenChange={setOnboardingOpen}
        onOpenConnections={() => setConnectionsOpen(true)}
      />
    </div>
  )
}

export default App

import { useState, type ReactNode } from 'react'
import { AnimatePresence, motion } from 'framer-motion'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Ban, ChevronDown, ChevronUp, Clock, Pause, Play, RotateCcw } from 'lucide-react'
import type { LogLine, QueueState, Transfer, TransferMetric } from '@/types/api'
import { api, ApiError } from '@/lib/api'
import { formatBytes, formatEta, formatSpeed, pct } from '@/lib/formatters'
import { Badge, Button, Card, Progress } from '@/components/ui/primitives'
import { StatusBadge } from '@/components/common/StatusBadge'
import { StreamDiagram } from '@/components/pipeline/StreamDiagram'
import { TerminalLog } from '@/components/pipeline/TerminalLog'
import { SpeedSparkline } from '@/components/pipeline/SpeedSparkline'
import { AnimatedNumber } from '@/components/common/AnimatedNumber'
import { FADE_UP, LAYOUT_SPRING } from '@/lib/motion'

interface ActiveTaskCardProps {
  transfer: Transfer
  metric?: TransferMetric
  logs: LogLine[]
}

/** ActiveTaskCard is the live pipeline view of one running transfer. */
export function ActiveTaskCard({ transfer, metric, logs }: ActiveTaskCardProps) {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const [logOpen, setLogOpen] = useState(false)

  const downloaded = metric?.bytes_downloaded ?? transfer.bytes_transferred
  const confirmed = metric?.bytes_confirmed ?? transfer.bytes_confirmed
  const total = metric?.total_size || transfer.file_size
  const progress = total > 0 ? pct(downloaded, total) : 0
  const confirmedPct = total > 0 ? pct(confirmed, total) : 0
  const running = transfer.status === 'running' || transfer.status === 'retrying'

  const action = useMutation({
    mutationFn: (kind: 'pause' | 'resume' | 'abort') => api.transferAction(transfer.id, kind),
    // Pause/abort must read as instant (front_spec.md §4): patch the cached
    // queue first, reconcile after the server answers.
    onMutate: async (kind) => {
      await queryClient.cancelQueries({ queryKey: ['queue'] })
      const previous = queryClient.getQueryData<QueueState>(['queue'])
      queryClient.setQueryData<QueueState>(['queue'], (old) => {
        if (!old) return old
        if (kind === 'abort') {
          return {
            ...old,
            active_tasks: old.active_tasks.filter((task) => task.id !== transfer.id),
            items: old.items.filter((item) => item.id !== transfer.id),
          }
        }
        const status = kind === 'pause' ? 'paused' : 'running'
        return {
          ...old,
          active_tasks: old.active_tasks.map((task) =>
            task.id === transfer.id ? { ...task, status } : task,
          ),
          items: old.items.map((item) => (item.id === transfer.id ? { ...item, status } : item)),
        }
      })
      return { previous }
    },
    onSuccess: (_data, kind) => {
      if (kind === 'abort') toast.info(t('status.aborted'))
    },
    onError: (err: unknown, _kind, context) => {
      if (context?.previous) queryClient.setQueryData(['queue'], context.previous)
      const code = err instanceof ApiError ? err.code : 'internal_error'
      toast.error(t(`errors.${code}`, { defaultValue: code }))
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: ['queue'] })
      void queryClient.invalidateQueries({ queryKey: ['history'] })
    },
  })

  return (
    <motion.div
      layout="position"
      initial={FADE_UP.initial}
      animate={FADE_UP.animate}
      exit={FADE_UP.exit}
      transition={{ ...FADE_UP.transition, layout: LAYOUT_SPRING }}
    >
    <Card className="overflow-hidden">
      <div className="flex items-start gap-3 px-4 pb-2 pt-4">
        <div className="min-w-0 flex-1">
          <p className="truncate text-xs font-medium text-slate-100">{transfer.filename_override || transfer.resolved_filename}</p>
          <p className="mt-0.5 truncate text-[10px] text-slate-500">
            {transfer.remote_label}
            {transfer.remote_path}:{transfer.filename_override || transfer.resolved_filename}
          </p>
        </div>
        <StatusBadge status={transfer.status} />
        <div className="flex items-center gap-1">
          {transfer.status === 'paused' ? (
            <Button variant="ghost" size="icon" onClick={() => action.mutate('resume')} aria-label={t('pipeline.resume')}>
              <Play className="h-3.5 w-3.5" />
            </Button>
          ) : (
            <Button
              variant="ghost"
              size="icon"
              disabled={!running && transfer.status !== 'queued'}
              onClick={() => action.mutate('pause')}
              aria-label={t('pipeline.pause')}
            >
              <Pause className="h-3.5 w-3.5" />
            </Button>
          )}
          <Button
            variant="ghost"
            size="icon"
            onClick={() => action.mutate('abort')}
            aria-label={t('pipeline.abort')}
            className="text-slate-500 hover:text-red-300"
          >
            <Ban className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>

      <div className="px-4 pb-3">
        <StreamDiagram
          mode={transfer.mode}
          progress={progress}
          bufferPct={metric?.buffer_usage_pct ?? 0}
          active={running}
        />
      </div>

      <div className="space-y-2 px-4 pb-3">
        <div className="space-y-1">
          <div className="flex items-center justify-between text-[10px] text-slate-500">
            <span>{t('pipeline.download')}</span>
            <span className="numeric text-slate-300">
              {formatBytes(downloaded, i18n.language)} {t('pipeline.of')} {total > 0 ? formatBytes(total, i18n.language) : '—'}
            </span>
          </div>
          <Progress value={progress} />
        </div>
        <div className="space-y-1">
          <div className="flex items-center justify-between text-[10px] text-slate-500">
            <span>{t('pipeline.upload')}</span>
            <span className="numeric text-emerald-300/80">{formatBytes(confirmed, i18n.language)}</span>
          </div>
          <Progress value={confirmedPct} tone="success" />
          {downloaded > 0 && confirmedPct === 0 ? (
            total > 0 && downloaded >= total ? (
              <div className="mt-1.5 flex items-start gap-2 rounded-lg border border-warn/30 bg-warn/10 px-2.5 py-2 text-[11px] leading-relaxed text-warn">
                <Clock className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                <span>{t('pipeline.waitCloudFinal')}</span>
              </div>
            ) : (
              <p className="mt-1 text-[10px] leading-relaxed text-slate-500">{t('pipeline.waitCloud')}</p>
            )
          ) : null}
        </div>
      </div>

      <div className="grid grid-cols-2 gap-2 border-t border-white/[0.05] px-4 py-2.5 sm:grid-cols-4">
        <Stat
          label={t('pipeline.speedIn')}
          value={
            <AnimatedNumber
              value={metric?.download_bps ?? 0}
              format={(v) => formatSpeed(v, i18n.language)}
            />
          }
          tone="sky"
        />
        <Stat
          label={t('pipeline.speedOut')}
          value={
            <AnimatedNumber
              value={metric?.upload_bps ?? 0}
              format={(v) => formatSpeed(v, i18n.language)}
            />
          }
          tone="emerald"
        />
        <Stat
          label={t('pipeline.bufferUsage')}
          value={transfer.mode === 'pure_stream' ? '—' : `${(metric?.buffer_usage_pct ?? 0).toFixed(0)}%`}
        />
        <Stat label={t('pipeline.eta')} value={formatEta(metric?.eta_seconds ?? 0)} />
      </div>

      <div className="px-4 pb-2">
        <SpeedSparkline down={metric?.download_bps ?? 0} up={metric?.upload_bps ?? 0} active={running} />
      </div>

      {transfer.status === 'retrying' ? (
        <div className="flex items-center gap-2 border-t border-amber-400/15 bg-amber-400/[0.05] px-4 py-2 text-[11px] text-amber-200">
          <RotateCcw className="h-3.5 w-3.5 animate-spin" />
          {t('pipeline.attempt', { count: (metric?.retry_attempt ?? transfer.retry_attempts) + 1 })}
        </div>
      ) : null}

      {transfer.error_kind ? (
        <div className="border-t border-red-400/15 bg-red-500/[0.05] px-4 py-2 text-[11px] text-red-200">
          {t(`errors.${transfer.error_kind}`, { defaultValue: transfer.error_kind })}
        </div>
      ) : null}

      <div className="px-4 pb-3">
        <TerminalLog lines={logs} open={logOpen} onOpenChange={setLogOpen} />
      </div>
    </Card>
    </motion.div>
  )
}

function Stat({ label, value, tone }: { label: string; value: ReactNode; tone?: 'sky' | 'emerald' }) {
  return (
    <div>
      <p className="text-[9px] uppercase tracking-wide text-slate-600">{label}</p>
      <p
        className={`numeric mt-0.5 text-xs font-medium ${
          tone === 'sky' ? 'text-sky-300' : tone === 'emerald' ? 'text-emerald-300' : 'text-slate-300'
        }`}
      >
        {value}
      </p>
    </div>
  )
}

/** PipelinePanel renders all active cards or the idle state. */
export function PipelinePanel({
  tasks,
  metrics,
  logs,
}: {
  tasks: Transfer[]
  metrics: Record<string, TransferMetric>
  logs: Record<string, LogLine[]>
}) {
  const { t } = useTranslation()
  if (tasks.length === 0) {
    return (
      <div className="flex items-center justify-between rounded-xl border border-dashed border-white/10 px-4 py-3">
        <div className="flex items-center gap-2 text-xs text-slate-500">
          <RotateCcw className="h-3.5 w-3.5" />
          {t('pipeline.noActive')}
        </div>
        <div className="flex items-center gap-1 text-[10px] text-slate-600">
          <ChevronUp className="h-3 w-3" />
          <ChevronDown className="h-3 w-3" />
        </div>
      </div>
    )
  }
  return (
    <div className="space-y-3">
      <AnimatePresence initial={false}>
        {tasks.map((transfer) => (
          <ActiveTaskCard
            key={transfer.id}
            transfer={transfer}
            metric={metrics[transfer.id]}
            logs={logs[transfer.id] ?? []}
          />
        ))}
      </AnimatePresence>
    </div>
  )
}

/** Re-exported for the queue panel's compact rows. */
export const taskBadge = (status: string) => <Badge tone="muted">{status}</Badge>

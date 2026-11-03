import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { AlertTriangle, CheckCircle2, Download, Loader2, Package, ShieldAlert, Trash2 } from 'lucide-react'
import type { DepItem } from '@/types/api'
import { api, ApiError } from '@/lib/api'
import { formatBytes } from '@/lib/formatters'
import {
  Badge,
  Button,
  Dialog,
  DialogContent,
  Progress,
  Skeleton,
} from '@/components/ui/primitives'
import { EmptyState, ErrorText, StatusBadge } from '@/components/common/StatusBadge'

export interface DepProgress {
  received: number
  total: number
  speed_bps: number
  state: string
  code?: string
}

interface DependencyManagerDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  deps: DepItem[]
  progress: Record<string, DepProgress>
  loading: boolean
}

/** DependencyManagerDialog installs and removes the external components. */
export function DependencyManagerDialog({
  open,
  onOpenChange,
  deps,
  progress,
  loading,
}: DependencyManagerDialogProps) {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()

  const install = useMutation({
    mutationFn: (id: string) => api.installDep(id),
    onSuccess: () => {
      setTimeout(() => {
        void queryClient.invalidateQueries({ queryKey: ['deps'] })
        void queryClient.invalidateQueries({ queryKey: ['status'] })
      }, 1500)
    },
  })

  const remove = useMutation({
    mutationFn: (id: string) => api.removeDep(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['deps'] })
      void queryClient.invalidateQueries({ queryKey: ['status'] })
    },
  })

  const rcloneMissing = deps.some((item) => item.id === 'rclone' && item.status !== 'installed')
  const mutationError = install.error ?? remove.error
  const errorCode = mutationError instanceof ApiError ? mutationError.code : mutationError ? 'internal_error' : null

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent wide title={t('deps.title')} description={t('deps.subtitle')}>
        <div className="space-y-3">
          {rcloneMissing ? (
            <div className="flex items-start gap-2 rounded-xl border border-amber-400/25 bg-amber-400/[0.06] px-3 py-2 text-[11px] leading-relaxed text-amber-200">
              <ShieldAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
              {t('deps.requiredBanner')}
            </div>
          ) : null}

          {loading ? (
            <div className="space-y-2">
              {[0, 1, 2].map((i) => (
                <Skeleton key={i} className="h-20 w-full" />
              ))}
            </div>
          ) : deps.length === 0 ? (
            <EmptyState icon={<Package className="h-6 w-6" />} title={t('deps.title')} compact />
          ) : (
            <div className="space-y-2">
              {deps.map((item) => {
                const job = progress[item.id]
                const installing = job?.state === 'downloading' || install.isPending
                const percent = job && job.total > 0 ? Math.min(100, (job.received / job.total) * 100) : 0
                return (
                  <div key={item.id} className="rounded-xl border border-white/[0.07] bg-white/[0.02] p-3">
                    <div className="flex items-start gap-3">
                      <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-white/[0.04] text-slate-300">
                        <Package className="h-4 w-4" />
                      </div>
                      <div className="min-w-0 flex-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <p className="text-xs font-medium text-slate-100">{item.name}</p>
                          <Badge tone="muted">{item.version}</Badge>
                          <StatusBadge status={item.status} />
                          {item.required ? <Badge tone="accent">{t('deps.required')}</Badge> : null}
                          {item.status === 'installed' ? (
                            item.verified ? (
                              <span className="flex items-center gap-1 text-[10px] text-emerald-300">
                                <CheckCircle2 className="h-3 w-3" />
                                {t('deps.verified')}
                              </span>
                            ) : (
                              <span className="flex items-center gap-1 text-[10px] text-amber-300">
                                <AlertTriangle className="h-3 w-3" />
                                {t('deps.unverified')}
                              </span>
                            )
                          ) : null}
                        </div>
                        <p className="mt-1 text-[11px] text-slate-500">
                          {t(`deps.purpose_${item.id}`, { defaultValue: item.purpose })}
                        </p>
                        <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-0.5 text-[10px] text-slate-600">
                          <span>{item.license}</span>
                          <span className="numeric">{formatBytes(item.size_bytes, i18n.language)}</span>
                          <span className="truncate font-mono">{item.source_url}</span>
                          {item.installed_version ? <span>v{item.installed_version}</span> : null}
                        </div>
                        {item.note ? <p className="mt-1 text-[10px] text-amber-300/80">{item.note}</p> : null}
                      </div>
                      <div className="flex shrink-0 gap-1.5">
                        {item.status === 'installed' ? (
                          <>
                            <Button
                              variant="ghost"
                              size="sm"
                              disabled={installing}
                              onClick={() => install.mutate(item.id)}
                            >
                              <Download className="h-3.5 w-3.5" />
                              {t('deps.update')}
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              className="text-slate-500 hover:text-red-300"
                              disabled={installing}
                              onClick={() => remove.mutate(item.id)}
                              aria-label={t('deps.remove')}
                            >
                              <Trash2 className="h-3.5 w-3.5" />
                            </Button>
                          </>
                        ) : (
                          <Button
                            variant="primary"
                            size="sm"
                            disabled={installing}
                            onClick={() => install.mutate(item.id)}
                          >
                            {installing ? (
                              <>
                                <Loader2 className="h-3.5 w-3.5 animate-spin" />
                                {t('common.loading')}
                              </>
                            ) : (
                              <>
                                <Download className="h-3.5 w-3.5" />
                                {t('deps.download')}
                              </>
                            )}
                          </Button>
                        )}
                      </div>
                    </div>
                    {installing ? (
                      <div className="mt-2 space-y-1">
                        <Progress value={percent} />
                        <div className="flex justify-between text-[10px] text-slate-500">
                          <span>
                            {job ? formatBytes(job.received, i18n.language) : t('common.loading')}
                            {job && job.total > 0 ? ` / ${formatBytes(job.total, i18n.language)}` : ''}
                          </span>
                          <span className="numeric">{job ? `${Math.round(percent)}%` : ''}</span>
                        </div>
                      </div>
                    ) : null}
                    {job?.state === 'failed' ? <ErrorText code={job.code ?? 'internal_error'} /> : null}
                  </div>
                )
              })}
            </div>
          )}

          {errorCode ? <ErrorText code={errorCode} /> : null}
          <p className="text-[10px] leading-relaxed text-slate-600">{t('deps.restartHint')}</p>
        </div>
      </DialogContent>
    </Dialog>
  )
}

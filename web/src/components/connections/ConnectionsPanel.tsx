import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Cloud, Plus, RefreshCw, Trash2, Wifi } from 'lucide-react'
import type { Remote } from '@/types/api'
import { api, ApiError } from '@/lib/api'
import { formatBytes, pct } from '@/lib/formatters'
import { Badge, Button, Progress, Sheet, SheetContent } from '@/components/ui/primitives'
import { EmptyState, ErrorText, StatusBadge } from '@/components/common/StatusBadge'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'
import { AddCloudWizard } from '@/components/connections/AddCloudWizard'

/** ConnectionsPanel is the cloud drawer with per-connection health. */
export function ConnectionsPanel({
  open,
  onOpenChange,
  remotes,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  remotes: Remote[]
}) {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const [wizardOpen, setWizardOpen] = useState(false)
  const [deleteTarget, setDeleteTarget] = useState<Remote | null>(null)
  const [testing, setTesting] = useState<string | null>(null)

  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ['remotes'] })
    void queryClient.invalidateQueries({ queryKey: ['status'] })
  }

  const test = useMutation({
    mutationFn: (id: string) => api.testRemote(id),
    onMutate: (id) => setTesting(id),
    onSuccess: (data) => {
      if (data.ok) {
        toast.success(t('connections.verifyOk'), {
          description: data.rtt_ms ? t('connections.ping', { ms: data.rtt_ms }) : undefined,
        })
      } else {
        const code = data.error?.error.code ?? 'cloud_unreachable'
        toast.error(t('connections.verifyFailed'), { description: t(`errors.${code}`, { defaultValue: code }) })
      }
      refresh()
    },
    onError: () => toast.error(t('connections.verifyFailed')),
    onSettled: () => setTesting(null),
  })

  const remove = useMutation({
    mutationFn: (id: string) => api.deleteRemote(id),
    onSuccess: () => {
      toast.success(t('connections.removed'))
      refresh()
    },
    onError: (err: unknown) => {
      const code = err instanceof ApiError ? err.code : 'internal_error'
      toast.error(t(`errors.${code}`, { defaultValue: code }))
    },
  })

  return (
    <>
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent title={t('connections.title')} description={t('stream.noCloudBody')}>
          <div className="space-y-3">
            <Button variant="primary" size="sm" className="w-full" onClick={() => setWizardOpen(true)}>
              <Plus className="h-3.5 w-3.5" />
              {t('connections.add')}
            </Button>

            {remotes.length === 0 ? (
              <EmptyState icon={<Cloud className="h-6 w-6" />} title={t('connections.empty')} compact />
            ) : (
              remotes.map((remote) => {
                const quota = remote.quota
                const percent = quota && quota.total > 0 ? pct(quota.used, quota.total) : 0
                return (
                  <div key={remote.id} className="rounded-xl border border-white/[0.07] bg-white/[0.02] p-3">
                    <div className="flex items-start gap-2">
                      <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-white/[0.04] text-slate-300">
                        <Cloud className="h-4 w-4" />
                      </div>
                      <div className="min-w-0 flex-1">
                        <p className="truncate text-xs font-medium text-slate-100">{remote.name}</p>
                        <p className="truncate text-[10px] text-slate-500">
                          {t(`connections.provider_${remote.provider}`, { defaultValue: remote.provider })}
                          {remote.rtt_ms > 0 ? ` • ${t('connections.ping', { ms: remote.rtt_ms })}` : ''}
                        </p>
                      </div>
                      <StatusBadge status={remote.status} />
                    </div>

                    {remote.last_error ? (
                      <div className="mt-2">
                        <ErrorText code={remote.last_error} />
                      </div>
                    ) : null}

                    {quota && quota.total > 0 ? (
                      <div className="mt-2 space-y-1">
                        <Progress value={percent} tone={percent > 90 ? 'warn' : 'accent'} className="h-1" />
                        <div className="flex justify-between text-[10px] text-slate-500">
                          <span>{formatBytes(quota.used, i18n.language)}</span>
                          <span>{formatBytes(quota.total, i18n.language)}</span>
                        </div>
                      </div>
                    ) : (
                      <p className="mt-2 text-[10px] text-slate-600">{t('connections.quotaUnknown')}</p>
                    )}

                    <div className="mt-2 flex items-center justify-between">
                      {remote.rclone_name ? <Badge tone="muted">{remote.rclone_name}</Badge> : <span />}
                      <div className="flex gap-1">
                        <Button
                          variant="ghost"
                          size="sm"
                          disabled={testing === remote.id}
                          onClick={() => test.mutate(remote.id)}
                        >
                          {testing === remote.id ? (
                            <RefreshCw className="h-3.5 w-3.5 animate-spin" />
                          ) : (
                            <Wifi className="h-3.5 w-3.5" />
                          )}
                          {t('connections.test')}
                        </Button>
                        <Button
                          variant="ghost"
                          size="icon"
                          className="text-slate-500 hover:text-red-300"
                          onClick={() => setDeleteTarget(remote)}
                          aria-label={t('connections.remove')}
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                        </Button>
                      </div>
                    </div>
                  </div>
                )
              })
            )}
          </div>
        </SheetContent>
      </Sheet>

      <AddCloudWizard open={wizardOpen} onOpenChange={setWizardOpen} onSaved={refresh} />

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        title={t('connections.remove')}
        body={t('connections.confirmRemove')}
        confirmLabel={t('common.delete')}
        danger
        onOpenChange={(value) => !value && setDeleteTarget(null)}
        onConfirm={() => {
          if (deleteTarget) remove.mutate(deleteTarget.id)
          setDeleteTarget(null)
        }}
      />
    </>
  )
}

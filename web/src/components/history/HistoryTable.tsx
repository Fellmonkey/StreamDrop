import { useState } from 'react'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Copy, FileText, History, MoreHorizontal, RotateCcw, Trash2 } from 'lucide-react'
import type { Transfer } from '@/types/api'
import { api, ApiError } from '@/lib/api'
import { formatBytes, formatDate, formatDuration, formatSpeed, shortHash } from '@/lib/formatters'
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Dialog,
  DialogContent,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  Skeleton,
  Tabs,
  TabsList,
  TabsTrigger,
  Table,
  TBody,
  THead,
} from '@/components/ui/primitives'
import { EmptyState, ErrorText, StatusBadge } from '@/components/common/StatusBadge'
import { ConfirmDialog, useDeleteTransfer } from '@/components/common/ConfirmDialog'

const FILTERS = ['all', 'completed', 'failed', 'aborted'] as const
type Filter = (typeof FILTERS)[number]
const PAGE_SIZE = 20

/** HistoryTable is the paginated task journal with repeat and log access. */
export function HistoryTable() {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const [filter, setFilter] = useState<Filter>('all')
  const [page, setPage] = useState(1)
  const [detailsId, setDetailsId] = useState<string | null>(null)
  const [deleteTarget, setDeleteTarget] = useState<Transfer | null>(null)

  const history = useQuery({
    queryKey: ['history', filter, page],
    queryFn: () => api.listTransfers({ status: filter === 'all' ? '' : filter, page, limit: PAGE_SIZE }),
    placeholderData: keepPreviousData,
    refetchInterval: 15000,
  })

  const details = useQuery({
    queryKey: ['transfer', detailsId],
    queryFn: () => api.getTransfer(detailsId as string),
    enabled: Boolean(detailsId),
  })

  const repeat = useMutation({
    mutationFn: (row: Transfer) =>
      api.createTransfers({
        source_url: row.source_url,
        remote_id: row.remote_id ?? '',
        remote_path: row.remote_path,
        engine: row.engine === 'auto' ? '' : row.engine,
        auth_profile_id: row.auth_profile_id ?? '',
        mode: row.mode,
        buffer_size_bytes: row.buffer_size_bytes,
        file_size: row.file_size,
      }),
    onSuccess: () => {
      toast.success(t('stream.created'))
      void queryClient.invalidateQueries({ queryKey: ['queue'] })
      void queryClient.invalidateQueries({ queryKey: ['history'] })
    },
    onError: (err: unknown) => {
      const code = err instanceof ApiError ? err.code : 'internal_error'
      toast.error(t(`errors.${code}`, { defaultValue: code }))
    },
  })

  const remove = useDeleteTransfer()
  const rows = history.data?.transfers ?? []
  const total = history.data?.total ?? 0
  const pages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  const copyUrl = async (url: string) => {
    try {
      await navigator.clipboard.writeText(url)
      toast.success(t('common.copied'))
    } catch {
      toast.error(t('errors.unknown'))
    }
  }

  return (
    <Card>
      <CardHeader className="flex-wrap pb-2">
        <div className="flex items-center gap-2">
          <History className="h-4 w-4 text-slate-400" />
          <CardTitle>{t('history.title')}</CardTitle>
          <Badge tone="muted">{t('history.summary', { count: total })}</Badge>
        </div>
        <Tabs value={filter} onValueChange={(value) => { setFilter(value as Filter); setPage(1) }}>
          <TabsList className="w-full flex-wrap sm:w-auto">
            {FILTERS.map((value) => (
              <TabsTrigger key={value} value={value} className="px-2 sm:px-3">
                {t(`history.filter${value.charAt(0).toUpperCase()}${value.slice(1)}`)}
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
      </CardHeader>

      <CardContent className="pt-1">
        {history.isLoading ? (
          <div className="space-y-2">
            {[0, 1, 2, 3, 4].map((i) => (
              <Skeleton key={i} className="h-9 w-full" />
            ))}
          </div>
        ) : history.error ? (
          <ErrorText code={history.error instanceof ApiError ? history.error.code : 'internal_error'} />
        ) : rows.length === 0 ? (
          <EmptyState icon={<History className="h-6 w-6" />} title={t('history.empty')} compact />
        ) : (
          <Table>
            <THead>
              <tr>
                <th>{t('history.status')}</th>
                <th>{t('history.filename')}</th>
                <th className="hidden lg:table-cell">{t('history.target')}</th>
                <th className="hidden sm:table-cell">{t('history.size')}</th>
                <th className="hidden xl:table-cell">{t('history.speed')}</th>
                <th className="hidden md:table-cell">{t('history.date')}</th>
                <th className="w-10" />
              </tr>
            </THead>
            <TBody>
              {rows.map((row) => (
                <tr key={row.id} className="transition-colors hover:bg-white/[0.02]">
                  <td>
                    <StatusBadge status={row.status} />
                  </td>
                  <td>
                    <p className="max-w-[220px] truncate text-xs text-slate-200">
                      {row.filename_override || row.resolved_filename}
                    </p>
                    <p className="max-w-[220px] truncate text-[10px] text-slate-600">{row.source_url}</p>
                    <p className="mt-0.5 text-[10px] text-slate-500 md:hidden">
                      {formatDate(row.created_at, i18n.language)}
                      {row.file_size > 0 ? ` · ${formatBytes(row.file_size, i18n.language)}` : ''}
                    </p>
                  </td>
                  <td className="hidden lg:table-cell">
                    <p className="max-w-[180px] truncate text-[11px] text-slate-300">{row.remote_label}</p>
                    <p className="max-w-[180px] truncate font-mono text-[10px] text-slate-600">{row.remote_path}</p>
                  </td>
                  <td className="numeric hidden text-[11px] text-slate-400 sm:table-cell">
                    {row.file_size > 0 ? formatBytes(row.file_size, i18n.language) : '—'}
                  </td>
                  <td className="numeric hidden text-[11px] text-slate-400 xl:table-cell">
                    {row.average_speed_bps > 0 ? formatSpeed(row.average_speed_bps, i18n.language) : '—'}
                  </td>
                  <td className="hidden text-[11px] text-slate-500 md:table-cell">
                    {formatDate(row.created_at, i18n.language)}
                  </td>
                  <td>
                    <DropdownMenu>
                      <DropdownMenuTrigger asChild>
                        <Button variant="ghost" size="icon" aria-label={t('history.actions')}>
                          <MoreHorizontal className="h-4 w-4" />
                        </Button>
                      </DropdownMenuTrigger>
                      <DropdownMenuContent>
                        <DropdownMenuItem
                          disabled={!row.remote_id}
                          onSelect={() => {
                            if (!row.remote_id) {
                              toast.warning(t('history.repeatBlocked'))
                              return
                            }
                            repeat.mutate(row)
                          }}
                        >
                          <RotateCcw className="mr-2 inline h-3.5 w-3.5" />
                          {t('history.repeat')}
                        </DropdownMenuItem>
                        <DropdownMenuItem onSelect={() => void copyUrl(row.source_url)}>
                          <Copy className="mr-2 inline h-3.5 w-3.5" />
                          {t('history.copyUrl')}
                        </DropdownMenuItem>
                        <DropdownMenuItem onSelect={() => setDetailsId(row.id)}>
                          <FileText className="mr-2 inline h-3.5 w-3.5" />
                          {t('history.details')}
                        </DropdownMenuItem>
                        <DropdownMenuItem danger onSelect={() => setDeleteTarget(row)}>
                          <Trash2 className="mr-2 inline h-3.5 w-3.5" />
                          {t('history.delete')}
                        </DropdownMenuItem>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </td>
                </tr>
              ))}
            </TBody>
          </Table>
        )}

        {pages > 1 ? (
          <div className="mt-3 flex items-center justify-between text-[11px] text-slate-500">
            <span>
              {t('common.page')} {page} {t('common.of')} {pages}
            </span>
            <div className="flex gap-1.5">
              <Button variant="ghost" size="sm" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
                {t('common.back')}
              </Button>
              <Button variant="ghost" size="sm" disabled={page >= pages} onClick={() => setPage((p) => p + 1)}>
                {t('common.next')}
              </Button>
            </div>
          </div>
        ) : null}
      </CardContent>

      <Dialog open={Boolean(detailsId)} onOpenChange={(value) => !value && setDetailsId(null)}>
        <DialogContent title={t('history.details')} wide>
          {details.isLoading ? (
            <Skeleton className="h-32 w-full" />
          ) : details.data ? (
            <div className="space-y-4">
              <div className="grid gap-3 sm:grid-cols-2">
                <Field label={t('history.filename')} value={details.data.transfer.filename_override || details.data.transfer.resolved_filename} />
                <Field label={t('history.status')} value={t(`status.${details.data.transfer.status}`)} />
                <Field label={t('history.size')} value={formatBytes(details.data.transfer.file_size, i18n.language)} />
                <Field
                  label={t('history.speed')}
                  value={
                    details.data.transfer.average_speed_bps > 0
                      ? formatSpeed(details.data.transfer.average_speed_bps, i18n.language)
                      : '—'
                  }
                />
                <Field
                  label={t('history.duration')}
                  value={formatDuration(
                    details.data.transfer.started_at && details.data.transfer.completed_at
                      ? (new Date(details.data.transfer.completed_at).getTime() -
                          new Date(details.data.transfer.started_at).getTime()) /
                          1000
                      : 0,
                  )}
                />
                <Field label={t('history.date')} value={formatDate(details.data.transfer.created_at, i18n.language)} />
                <Field label={t('history.target')} value={`${details.data.transfer.remote_label}:${details.data.transfer.remote_path}`} />
                <Field label="Source" value={details.data.transfer.source_url} mono />
              </div>

              <div className="space-y-1">
                <p className="text-[10px] uppercase tracking-wide text-slate-500">{t('history.sha')}</p>
                <p className="break-all font-mono text-[11px] text-slate-300">
                  {details.data.transfer.sha256 || t('history.shaDisabled')}
                </p>
                {details.data.transfer.sha256 ? (
                  <p className="numeric text-[10px] text-slate-600">{shortHash(details.data.transfer.sha256)}</p>
                ) : null}
              </div>

              {details.data.transfer.error_kind ? (
                <div className="rounded-lg border border-red-400/20 bg-red-500/[0.05] p-2">
                  <ErrorText code={details.data.transfer.error_kind} details={details.data.transfer.error_message} />
                </div>
              ) : null}

              <div className="space-y-1">
                <p className="text-[10px] uppercase tracking-wide text-slate-500">{t('pipeline.log')}</p>
                <div className="scroll-contain max-h-64 overflow-y-auto rounded-lg border border-white/[0.06] bg-black/50 p-2.5 font-mono text-[10px] leading-relaxed">
                  {(details.data.logs ?? []).length === 0 ? (
                    <p className="text-slate-600">{t('history.logEmpty')}</p>
                  ) : (
                    details.data.logs.map((line, index) => (
                      <div key={index} className="flex gap-2">
                        <span className="shrink-0 text-slate-600">
                          {new Date(line.ts).toLocaleTimeString(undefined, { hour12: false })}
                        </span>
                        <span className="shrink-0 uppercase text-slate-500">{line.source}</span>
                        <span className="break-all text-slate-300">{line.message}</span>
                      </div>
                    ))
                  )}
                </div>
              </div>
            </div>
          ) : (
            <ErrorText code="not_found" />
          )}
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        title={t('history.delete')}
        body={t('history.confirmDelete')}
        confirmLabel={t('common.delete')}
        danger
        onOpenChange={(value) => !value && setDeleteTarget(null)}
        onConfirm={() => {
          if (!deleteTarget) return
          remove.mutate(deleteTarget.id, {
            onSuccess: () => toast.success(t('history.deleted')),
            onError: (err) => toast.error(err instanceof ApiError ? t(`errors.${err.code}`) : t('errors.unknown')),
          })
          setDeleteTarget(null)
        }}
      />
    </Card>
  )
}

function Field({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div>
      <p className="text-[10px] uppercase tracking-wide text-slate-500">{label}</p>
      <p className={`mt-0.5 break-all text-[11px] text-slate-300 ${mono ? 'font-mono' : ''}`}>{value}</p>
    </div>
  )
}

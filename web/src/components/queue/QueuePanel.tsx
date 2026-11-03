import { useMutation, useQueryClient } from '@tanstack/react-query'
import { AnimatePresence, motion } from 'framer-motion'
import { useTranslation } from 'react-i18next'
import { Ban, ChevronDown, ChevronUp, ListOrdered, MoreHorizontal, Pause, Play } from 'lucide-react'
import type { QueueItem, QueueState, TransferMetric } from '@/types/api'
import { api, ApiError } from '@/lib/api'
import { formatBytes, pct } from '@/lib/formatters'
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
  Progress,
  Select,
} from '@/components/ui/primitives'
import { EmptyState, ErrorText, StatusBadge } from '@/components/common/StatusBadge'
import { EASE_OUT, LAYOUT_SPRING } from '@/lib/motion'

/** QueuePanel controls the scheduler and the pending task order. */
export function QueuePanel({
  state,
  metrics,
}: {
  state?: QueueState
  metrics: Record<string, TransferMetric>
}) {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()

  const refresh = () => {
    void queryClient.invalidateQueries({ queryKey: ['queue'] })
    void queryClient.invalidateQueries({ queryKey: ['history'] })
    void queryClient.invalidateQueries({ queryKey: ['status'] })
  }

  const toggleQueue = useMutation({
    mutationFn: () => (state?.paused ? api.resumeQueue() : api.pauseQueue()),
    onMutate: async () => {
      await queryClient.cancelQueries({ queryKey: ['queue'] })
      const previous = queryClient.getQueryData<QueueState>(['queue'])
      queryClient.setQueryData<QueueState>(['queue'], (old) =>
        old ? { ...old, paused: !old.paused } : old,
      )
      return { previous }
    },
    onError: (_err, _vars, context) => {
      if (context?.previous) queryClient.setQueryData(['queue'], context.previous)
    },
    onSettled: refresh,
  })

  const reorder = useMutation({
    mutationFn: (ids: string[]) => api.reorderQueue(ids),
    onMutate: async (ids) => {
      await queryClient.cancelQueries({ queryKey: ['queue'] })
      const previous = queryClient.getQueryData<QueueState>(['queue'])
      queryClient.setQueryData<QueueState>(['queue'], (old) => {
        if (!old) return old
        const byID = new Map(old.items.map((item) => [item.id, item]))
        const items = ids
          .map((id) => byID.get(id))
          .filter((item): item is QueueItem => Boolean(item))
        return { ...old, items }
      })
      return { previous }
    },
    onError: (_err, _vars, context) => {
      if (context?.previous) queryClient.setQueryData(['queue'], context.previous)
    },
    onSettled: refresh,
  })

  const itemAction = useMutation({
    mutationFn: ({ id, action }: { id: string; action: 'pause' | 'resume' | 'abort' }) =>
      api.transferAction(id, action),
    onMutate: async ({ id, action }) => {
      await queryClient.cancelQueries({ queryKey: ['queue'] })
      const previous = queryClient.getQueryData<QueueState>(['queue'])
      queryClient.setQueryData<QueueState>(['queue'], (old) => {
        if (!old) return old
        if (action === 'abort') {
          return {
            ...old,
            active_tasks: old.active_tasks.filter((task) => task.id !== id),
            items: old.items.filter((item) => item.id !== id),
          }
        }
        const status = action === 'pause' ? 'paused' : 'running'
        return {
          ...old,
          active_tasks: old.active_tasks.map((task) => (task.id === id ? { ...task, status } : task)),
          items: old.items.map((item) => (item.id === id ? { ...item, status } : item)),
        }
      })
      return { previous }
    },
    onError: (_err, _vars, context) => {
      if (context?.previous) queryClient.setQueryData(['queue'], context.previous)
    },
    onSettled: refresh,
  })

  const setLimit = useMutation({
    mutationFn: (limit: number) => api.updateSettings({ parallel_limit: limit }),
    onSuccess: refresh,
  })

  const items = state?.items ?? []
  const move = (index: number, direction: -1 | 1) => {
    const ids = items.map((item) => item.id)
    const target = index + direction
    if (target < 0 || target >= ids.length) return
    ;[ids[index], ids[target]] = [ids[target], ids[index]]
    reorder.mutate(ids)
  }

  const error = toggleQueue.error ?? reorder.error ?? itemAction.error ?? setLimit.error
  const errorCode = error instanceof ApiError ? error.code : error ? 'internal_error' : null

  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex items-center gap-2">
          <ListOrdered className="h-4 w-4 text-slate-400" />
          <CardTitle>{t('queue.title')}</CardTitle>
          {state?.paused ? <Badge tone="warn">{t('queue.paused')}</Badge> : null}
        </div>
        <div className="flex flex-wrap items-center justify-end gap-1.5">
          <label className="flex items-center gap-1.5 text-[10px] text-slate-500">
            {t('queue.limitLabel')}
            <Select
              className="h-7 w-14 px-1.5 text-xs"
              value={state?.parallel_limit ?? 1}
              onChange={(e) => setLimit.mutate(Number(e.target.value))}
            >
              {[1, 2, 3, 4, 5].map((n) => (
                <option key={n} value={n}>
                  {n}
                </option>
              ))}
            </Select>
          </label>
          <Button variant="ghost" size="sm" onClick={() => toggleQueue.mutate()}>
            {state?.paused ? <Play className="h-3.5 w-3.5" /> : <Pause className="h-3.5 w-3.5" />}
            {state?.paused ? t('queue.resume') : t('queue.pause')}
          </Button>
        </div>
      </CardHeader>

      <CardContent className="space-y-2 pt-1">
        <div className="flex items-center gap-3 text-[10px] text-slate-500">
          <span>{t('queue.active', { count: state?.active ?? 0 })}</span>
          <span>{t('queue.pending', { count: state?.pending ?? 0 })}</span>
        </div>

        {items.length === 0 ? (
          <EmptyState compact title={t('queue.empty')} />
        ) : (
          <ul className="divide-y divide-white/[0.05] rounded-lg border border-white/[0.06]">
            <AnimatePresence initial={false}>
            {items.map((item, index) => {
              const metric = metrics[item.id]
              const transferred = metric?.bytes_downloaded ?? item.bytes_transferred
              const percent = item.size > 0 ? pct(transferred, item.size) : 0
              return (
                <motion.li
                  key={item.id}
                  layout="position"
                  initial={{ opacity: 0, y: 4 }}
                  animate={{ opacity: 1, y: 0 }}
                  exit={{ opacity: 0, scale: 0.98 }}
                  transition={{ duration: 0.18, ease: EASE_OUT, layout: LAYOUT_SPRING }}
                  className="flex items-center gap-2 px-2.5 py-2"
                >
                  <div className="flex flex-col">
                    <button
                      type="button"
                      disabled={index === 0}
                      onClick={() => move(index, -1)}
                      className="text-slate-600 hover:text-slate-300 disabled:opacity-20"
                      aria-label={t('queue.moveUp')}
                    >
                      <ChevronUp className="h-3 w-3" />
                    </button>
                    <button
                      type="button"
                      disabled={index === items.length - 1}
                      onClick={() => move(index, 1)}
                      className="text-slate-600 hover:text-slate-300 disabled:opacity-20"
                      aria-label={t('queue.moveDown')}
                    >
                      <ChevronDown className="h-3 w-3" />
                    </button>
                  </div>
                  <span className="numeric w-5 text-[10px] text-slate-600">{index + 1}</span>
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-[11px] text-slate-200">{item.filename}</p>
                    <div className="mt-1 flex items-center gap-2">
                      <Progress value={percent} className="h-1" />
                      <span className="numeric shrink-0 text-[9px] text-slate-500">
                        {item.size > 0 ? formatBytes(item.size, i18n.language) : '—'}
                      </span>
                    </div>
                  </div>
                  <StatusBadge status={item.status} />
                  <DropdownMenu>
                    <DropdownMenuTrigger asChild>
                      <Button variant="ghost" size="icon" aria-label={t('history.actions')}>
                        <MoreHorizontal className="h-4 w-4" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent>
                      {item.status === 'paused' ? (
                        <DropdownMenuItem onSelect={() => itemAction.mutate({ id: item.id, action: 'resume' })}>
                          <Play className="mr-2 inline h-3.5 w-3.5" />
                          {t('queue.resume')}
                        </DropdownMenuItem>
                      ) : (
                        <DropdownMenuItem onSelect={() => itemAction.mutate({ id: item.id, action: 'pause' })}>
                          <Pause className="mr-2 inline h-3.5 w-3.5" />
                          {t('pipeline.pause')}
                        </DropdownMenuItem>
                      )}
                      <DropdownMenuItem
                        danger
                        onSelect={() => itemAction.mutate({ id: item.id, action: 'abort' })}
                      >
                        <Ban className="mr-2 inline h-3.5 w-3.5" />
                        {t('queue.abort')}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </motion.li>
              )
            })}
            </AnimatePresence>
          </ul>
        )}
        {errorCode ? <ErrorText code={errorCode} /> : null}
      </CardContent>
    </Card>
  )
}

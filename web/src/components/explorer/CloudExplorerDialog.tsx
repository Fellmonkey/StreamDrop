import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { ChevronRight, Folder, FolderPlus, HardDrive, Loader2, Search } from 'lucide-react'
import type { Remote } from '@/types/api'
import { api, ApiError } from '@/lib/api'
import {
  Button,
  Dialog,
  DialogContent,
  Input,
  Skeleton,
} from '@/components/ui/primitives'
import { EmptyState, ErrorText } from '@/components/common/StatusBadge'
import { RadialQuota } from '@/components/explorer/RadialQuota'

interface CloudExplorerDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  remote: Remote
  initialPath: string
  onSelect: (path: string) => void
}

/** CloudExplorerDialog browses one cloud level at a time. */
export function CloudExplorerDialog({
  open,
  onOpenChange,
  remote,
  initialPath,
  onSelect,
}: CloudExplorerDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [path, setPath] = useState(initialPath || '/')
  const [search, setSearch] = useState('')
  const [creating, setCreating] = useState(false)
  const [folderName, setFolderName] = useState('')

  useEffect(() => {
    if (open) {
      setPath(initialPath || '/')
      setSearch('')
      setCreating(false)
      setFolderName('')
    }
  }, [open, initialPath])

  const list = useQuery({
    queryKey: ['explorer', remote.id, path, search],
    queryFn: () => api.explorer(remote.id, path, search),
    enabled: open,
    retry: false,
  })

  const mkdir = useMutation({
    mutationFn: () => api.mkdir(remote.id, path, folderName.trim()),
    onSuccess: () => {
      setFolderName('')
      setCreating(false)
      void queryClient.invalidateQueries({ queryKey: ['explorer', remote.id] })
    },
  })

  const segments = useMemo(() => path.split('/').filter(Boolean), [path])
  const error = list.error instanceof ApiError ? list.error : null

  const goTo = (next: string) => {
    setSearch('')
    setPath(next.startsWith('/') ? next : `/${next}`)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent wide title={t('explorer.title')} description={remote.name}>
        <div className="space-y-4">
          <div className="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-white/[0.06] bg-white/[0.02] p-3">
            <RadialQuota quota={list.data?.quota ?? remote.quota} size={84} />
            <div className="flex items-center gap-2">
              <div className="relative">
                <Search className="pointer-events-none absolute left-2.5 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-500" />
                <Input
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                  placeholder={t('explorer.search')}
                  className="h-9 w-full pl-8 text-xs sm:w-52"
                />
              </div>
              <Button variant="secondary" size="sm" onClick={() => setCreating((v) => !v)}>
                <FolderPlus className="h-3.5 w-3.5" />
                {t('explorer.newFolder')}
              </Button>
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-1 text-[11px] text-slate-400">
            <button type="button" onClick={() => goTo('/')} className="rounded px-1.5 py-0.5 hover:bg-white/[0.06]">
              {t('explorer.root')}
            </button>
            {segments.map((segment, index) => {
              const next = `/${segments.slice(0, index + 1).join('/')}`
              return (
                <span key={next} className="flex items-center gap-1">
                  <ChevronRight className="h-3 w-3 text-slate-600" />
                  <button
                    type="button"
                    onClick={() => goTo(next)}
                    className="rounded px-1.5 py-0.5 hover:bg-white/[0.06]"
                  >
                    {segment}
                  </button>
                </span>
              )
            })}
          </div>

          {creating ? (
            <div className="flex items-end gap-2 rounded-lg border border-white/[0.06] p-2">
              <div className="flex-1">
                <Input
                  autoFocus
                  value={folderName}
                  onChange={(e) => setFolderName(e.target.value)}
                  placeholder={t('explorer.name')}
                  className="h-8 text-xs"
                />
              </div>
              <Button
                size="sm"
                variant="primary"
                disabled={!folderName.trim() || mkdir.isPending}
                onClick={() => mkdir.mutate()}
              >
                {t('explorer.create')}
              </Button>
              <Button size="sm" variant="ghost" onClick={() => setCreating(false)}>
                {t('common.cancel')}
              </Button>
            </div>
          ) : null}
          {mkdir.error ? <ErrorText code={mkdir.error instanceof ApiError ? mkdir.error.code : 'internal_error'} /> : null}

          <div className="min-h-[220px] rounded-xl border border-white/[0.06]">
            {list.isLoading ? (
              <div className="space-y-2 p-3">
                {[0, 1, 2, 3].map((i) => (
                  <Skeleton key={i} className="h-8 w-full" />
                ))}
              </div>
            ) : error ? (
              <div className="p-4">
                <ErrorText code={error.code} details={error.details} />
              </div>
            ) : (list.data?.directories ?? []).length === 0 ? (
              <div className="p-4">
                <EmptyState
                  compact
                  icon={<Folder className="h-6 w-6" />}
                  title={search ? t('explorer.emptySearch') : t('explorer.empty')}
                  body={t('explorer.searchHint')}
                />
              </div>
            ) : (
              <ul className="divide-y divide-white/[0.05]">
                {(list.data?.directories ?? []).map((dir) => (
                  <li key={dir.path || dir.name}>
                    <button
                      type="button"
                      onClick={() => goTo(dir.path || `${path.replace(/\/$/, '')}/${dir.name}`)}
                      className="flex w-full items-center gap-2.5 px-3 py-2.5 text-left transition-colors hover:bg-white/[0.03]"
                    >
                      <Folder className="h-4 w-4 text-sky-300/80" />
                      <span className="flex-1 truncate text-xs text-slate-200">{dir.name}</span>
                      <ChevronRight className="h-3.5 w-3.5 text-slate-600" />
                    </button>
                  </li>
                ))}
              </ul>
            )}
            {list.isFetching && !list.isLoading ? (
              <div className="flex items-center gap-2 border-t border-white/[0.05] px-3 py-1.5 text-[10px] text-slate-500">
                <Loader2 className="h-3 w-3 animate-spin" />
                {t('explorer.loading')}
              </div>
            ) : null}
          </div>

          <div className="flex items-center justify-between gap-3">
            <div className="flex items-center gap-1.5 text-[11px] text-slate-500">
              <HardDrive className="h-3.5 w-3.5" />
              <span className="truncate font-mono">{path}</span>
            </div>
            <div className="flex gap-2">
              <Button variant="ghost" size="sm" onClick={() => onOpenChange(false)}>
                {t('common.cancel')}
              </Button>
              <Button
                variant="primary"
                size="sm"
                onClick={() => {
                  onSelect(path)
                  onOpenChange(false)
                }}
              >
                {t('explorer.select')}
              </Button>
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}

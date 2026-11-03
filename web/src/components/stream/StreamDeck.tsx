import { useMemo, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import {
  ClipboardPaste,
  Cloud,
  Download,
  FolderTree,
  Layers,
  Link2,
  PackageX,
  Play,
  Plus,
  Sparkles,
  X,
} from 'lucide-react'
import type { AuthProfile, DepItem, Remote, Settings, Template, Transfer } from '@/types/api'
import { api, ApiError } from '@/lib/api'
import { useInspector } from '@/hooks/useInspector'
import { formatBytes } from '@/lib/formatters'
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
  Input,
  Label,
  Select,
  Textarea,
} from '@/components/ui/primitives'
import { AdvancedTuning, BUFFER_SIZES, type TuningValues } from '@/components/stream/AdvancedTuning'
import { LinkInspector } from '@/components/stream/LinkInspector'
import { CloudExplorerDialog } from '@/components/explorer/CloudExplorerDialog'
import { cn } from '@/lib/utils'

const MIB = 1024 * 1024
const MBPS = 1_000_000

interface StreamDeckProps {
  remotes: Remote[]
  profiles: AuthProfile[]
  deps: DepItem[]
  templates: Template[]
  settings?: Settings
  onOpenConnections: () => void
  onOpenDeps: () => void
  onOpenProfiles: () => void
}

/** StreamDeck is the main injection panel: URL, destination and start. */
export function StreamDeck({
  remotes,
  profiles,
  deps,
  templates,
  settings,
  onOpenConnections,
  onOpenDeps,
  onOpenProfiles,
}: StreamDeckProps) {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()

  const [url, setUrl] = useState('')
  const [batch, setBatch] = useState(false)
  const [batchText, setBatchText] = useState('')
  const [filename, setFilename] = useState('')
  const [editingName, setEditingName] = useState(false)
  const [remoteId, setRemoteId] = useState('')
  const [remotePath, setRemotePath] = useState('')
  const [advanced, setAdvanced] = useState(false)
  const [explorerOpen, setExplorerOpen] = useState(false)

  const [tuning, setTuning] = useState<TuningValues>(() => ({
    mode: (settings?.default_mode as TuningValues['mode']) || 'ring_buffer',
    bufferMiB: Math.round(Number(settings?.default_buffer_bytes ?? 128 * MIB) / MIB) || 128,
    engine: '',
    authProfileId: '',
    bandwidthDownMbps: Math.round(Number(settings?.bandwidth_down_bps ?? 0) / MBPS),
    bandwidthUpMbps: Math.round(Number(settings?.bandwidth_up_bps ?? 0) / MBPS),
  }))

  const activeRemote = remotes.find((r) => r.id === remoteId) ?? null
  const selectedProfile = tuning.authProfileId
  const single = !batch

  const inspector = useInspector(url, selectedProfile, single)

  // A new probe result resets the editable name to the suggested one.
  const suggested = inspector.metadata?.filename ?? ''
  const effectiveName = editingName ? filename : filename || suggested

  const rclone = deps.find((d) => d.id === 'rclone')
  const rcloneReady = rclone?.status === 'installed'

  const urls = useMemo(() => {
    if (!batch) return url.trim() ? [url.trim()] : []
    return batchText
      .split('\n')
      .map((line) => line.trim())
      .filter((line) => /^https?:\/\//i.test(line))
  }, [batch, url, batchText])

  const htmlBlocked =
    single &&
    Boolean(inspector.metadata?.is_html) &&
    tuning.engine !== 'yt-dlp' &&
    !tuning.authProfileId

  const effectiveMode = tuning.mode
  const effectiveBuffer = tuning.bufferMiB

  const create = useMutation({
    mutationFn: () =>
      api.createTransfers({
        urls,
        filename_override: single ? effectiveName : '',
        remote_id: remoteId,
        remote_path: remotePath,
        engine: tuning.engine,
        auth_profile_id: tuning.authProfileId,
        mode: effectiveMode,
        buffer_size_bytes: effectiveBuffer * MIB,
        bandwidth_down_bps: tuning.bandwidthDownMbps * MBPS,
        bandwidth_up_bps: tuning.bandwidthUpMbps * MBPS,
        file_size: single ? inspector.metadata?.size_bytes ?? 0 : 0,
      }),
    onSuccess: (data: { transfers: Transfer[] }) => {
      toast.success(t('stream.created'), {
        description: data.transfers.map((tr) => tr.resolved_filename).slice(0, 3).join(', '),
      })
      setUrl('')
      setBatchText('')
      setFilename('')
      setEditingName(false)
      void queryClient.invalidateQueries({ queryKey: ['queue'] })
      void queryClient.invalidateQueries({ queryKey: ['history'] })
      void queryClient.invalidateQueries({ queryKey: ['status'] })
    },
    onError: (err: unknown) => {
      const code = err instanceof ApiError ? err.code : 'internal_error'
      toast.error(t('stream.startFailed'), { description: t(`errors.${code}`, { defaultValue: code }) })
    },
  })

  const patch = (value: Partial<TuningValues>) => setTuning((prev) => ({ ...prev, ...value }))

  const paste = async () => {
    try {
      const text = await navigator.clipboard.readText()
      if (batch) setBatchText((prev) => (prev ? `${prev}\n${text.trim()}` : text.trim()))
      else setUrl(text.trim())
    } catch {
      // Clipboard permission denied: the field stays manually editable.
    }
  }

  const canStart =
    urls.length > 0 && remoteId !== '' && rcloneReady && !htmlBlocked && !inspector.loading && !create.isPending

  if (remotes.length === 0) {
    return (
      <Card className="p-6">
        <div className="flex flex-col items-center gap-3 py-6 text-center">
          <div className="flex h-11 w-11 items-center justify-center rounded-xl bg-sky-400/10 text-sky-300">
            <Cloud className="h-5 w-5" />
          </div>
          <div>
            <p className="text-sm font-semibold text-slate-100">{t('stream.noCloud')}</p>
            <p className="mt-1 max-w-sm text-xs text-slate-500">{t('stream.noCloudBody')}</p>
          </div>
          <Button variant="primary" size="sm" onClick={onOpenConnections}>
            <Plus className="h-3.5 w-3.5" />
            {t('stream.addCloud')}
          </Button>
        </div>
      </Card>
    )
  }

  return (
    <Card className="overflow-hidden">
      <CardHeader className="pb-3">
        <div className="flex items-center gap-2">
          <Sparkles className="h-4 w-4 text-sky-300" />
          <CardTitle>{t('stream.title')}</CardTitle>
        </div>
        <div className="flex items-center gap-1">
          <Button
            variant={batch ? 'secondary' : 'ghost'}
            size="sm"
            onClick={() => setBatch((v) => !v)}
            className="gap-1.5"
          >
            <Layers className="h-3.5 w-3.5" />
            <span className="hidden sm:inline">{t('stream.batch')}</span>
          </Button>
          {templates.length > 0 ? (
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="icon" aria-label={t('stream.templates')}>
                  <Download className="h-4 w-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent>
                {templates.map((template) => (
                  <DropdownMenuItem
                    key={template.id}
                    onSelect={() => {
                      setUrl(template.example)
                      setFilename('')
                    }}
                  >
                    <div className="space-y-0.5">
                      <p>{t(`templates.${template.id}`, { defaultValue: template.id })}</p>
                      <p className="max-w-[260px] truncate text-[10px] text-slate-500">{template.example}</p>
                    </div>
                  </DropdownMenuItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>
          ) : null}
        </div>
      </CardHeader>

      <CardContent className="space-y-3 pt-0">
        {batch ? (
          <div className="space-y-1.5">
            <div className="flex items-center justify-between">
              <Label>{t('stream.urlLabel')}</Label>
              <Badge tone="muted">{t('stream.urlsCount', { count: urls.length })}</Badge>
            </div>
            <Textarea
              rows={5}
              placeholder={t('stream.batchPlaceholder')}
              value={batchText}
              onChange={(e) => setBatchText(e.target.value)}
              className="font-mono text-xs"
            />
            <p className="text-[10px] text-slate-600">{t('stream.batchHint')}</p>
          </div>
        ) : (
          <div className="space-y-2">
            <div className="relative">
              <Link2 className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-500" />
              <Input
                value={url}
                onChange={(e) => {
                  setUrl(e.target.value)
                  setFilename('')
                  setEditingName(false)
                }}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && canStart && !create.isPending) create.mutate()
                }}
                placeholder={t('stream.urlPlaceholder')}
                className="h-11 pl-9 pr-20 text-xs"
                spellCheck={false}
              />
              <div className="absolute right-1.5 top-1/2 flex -translate-y-1/2 items-center gap-0.5">
                {url ? (
                  <Button variant="ghost" size="icon" onClick={() => setUrl('')} aria-label={t('common.close')}>
                    <X className="h-3.5 w-3.5" />
                  </Button>
                ) : null}
                <Button variant="ghost" size="icon" onClick={paste} aria-label={t('stream.paste')}>
                  <ClipboardPaste className="h-3.5 w-3.5" />
                </Button>
              </div>
            </div>
            <LinkInspector
              metadata={inspector.metadata}
              loading={inspector.loading}
              error={inspector.error}
              filename={effectiveName}
              editing={editingName && Boolean(suggested)}
              onEdit={(value) => {
                setEditingName(value)
                if (value && !filename) setFilename(suggested)
              }}
              onFilenameChange={setFilename}
              htmlBlocked={htmlBlocked}
            />
          </div>
        )}

        <div className="grid gap-3 sm:grid-cols-2">
          <div className="space-y-1.5">
            <Label>{t('stream.targetCloud')}</Label>
            <Select
              value={remoteId}
              onChange={(e) => {
                setRemoteId(e.target.value)
                setRemotePath('')
              }}
            >
              <option value="">{t('stream.selectRemote')}</option>
              {remotes.map((remote) => (
                <option key={remote.id} value={remote.id}>
                  {remote.name}
                  {remote.status === 'online' ? '' : ' •'}
                </option>
              ))}
            </Select>
          </div>
          <div className="space-y-1.5">
            <Label>{t('stream.targetFolder')}</Label>
            <div className="flex gap-1.5">
              <Input
                value={remotePath}
                onChange={(e) => setRemotePath(e.target.value)}
                placeholder={t('stream.folderPlaceholder')}
                className="h-9 font-mono text-xs"
              />
              <Button
                variant="secondary"
                size="icon"
                disabled={!activeRemote}
                onClick={() => setExplorerOpen(true)}
                aria-label={t('common.browse')}
              >
                <FolderTree className="h-4 w-4" />
              </Button>
            </div>
          </div>
        </div>

        {!rcloneReady ? (
          <div className="flex items-center justify-between gap-3 rounded-xl border border-amber-400/20 bg-amber-400/[0.05] px-3 py-2">
            <div className="flex items-center gap-2 text-[11px] text-amber-200">
              <PackageX className="h-3.5 w-3.5" />
              {t('stream.rcloneMissing')}
            </div>
            <Button variant="ghost" size="sm" onClick={onOpenDeps} className="text-amber-200">
              {t('stream.rcloneAction')}
            </Button>
          </div>
        ) : null}

        <AdvancedTuning
          open={advanced}
          onOpenChange={setAdvanced}
          values={tuning}
          onChange={patch}
          profiles={profiles}
          onManageProfiles={onOpenProfiles}
        />

        <div className="flex flex-col-reverse gap-3 pt-1 sm:flex-row sm:items-center sm:justify-between">
          <div className="flex flex-wrap items-center gap-1.5 text-[10px] text-slate-600">
            <Badge tone="muted">
              {effectiveMode === 'ring_buffer' ? t('stream.modeRing') : t('stream.modePure')}
            </Badge>
            <Badge tone="muted">{effectiveBuffer} MiB</Badge>
            {activeRemote?.quota ? (
              <Badge tone="muted">
                {t('connections.free')}: {formatBytes(activeRemote.quota.free, i18n.language)}
              </Badge>
            ) : null}
          </div>
          <Button
            variant="primary"
            size="lg"
            disabled={!canStart}
            onClick={() => create.mutate()}
            className={cn('w-full sm:w-auto sm:min-w-[190px]', canStart && 'shadow-[0_0_32px_-8px_rgba(56,189,248,0.8)]')}
          >
            <Play className="h-4 w-4" fill="currentColor" />
            {create.isPending ? t('common.loading') : t('stream.start')}
          </Button>
        </div>
      </CardContent>

      {activeRemote ? (
        <CloudExplorerDialog
          open={explorerOpen}
          onOpenChange={setExplorerOpen}
          remote={activeRemote}
          initialPath={remotePath || '/'}
          onSelect={(path) => setRemotePath(path)}
        />
      ) : null}
    </Card>
  )
}

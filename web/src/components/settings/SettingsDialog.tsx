import { useEffect, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Bell, DownloadCloud, Info, Languages, ListOrdered, Send, SlidersHorizontal, Zap } from 'lucide-react'
import type { Settings, Status } from '@/types/api'
import { api, ApiError } from '@/lib/api'
import { formatBytes } from '@/lib/formatters'
import {
  Badge,
  Button,
  Dialog,
  DialogContent,
  Input,
  Label,
  Select,
  Switch,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from '@/components/ui/primitives'
import { LanguageSwitcher } from '@/components/common/LanguageSwitcher'
import { ErrorText } from '@/components/common/StatusBadge'

const MIB = 1024 * 1024
const MBPS = 1_000_000
const BUFFERS = [32, 64, 128, 256, 512, 1024]

interface Draft {
  parallel_limit: number
  default_mode: string
  default_buffer_mib: number
  retry_max_attempts: number
  checksum_enabled: boolean
  bandwidth_down_mbps: number
  bandwidth_up_mbps: number
  notifications_toast: boolean
  telegram_enabled: boolean
  telegram_token: string
  telegram_chat_id: string
  crash_reporting_enabled: boolean
}

function toDraft(settings?: Settings): Draft {
  return {
    parallel_limit: Number(settings?.parallel_limit ?? 1) || 1,
    default_mode: settings?.default_mode === 'pure_stream' ? 'pure_stream' : 'ring_buffer',
    default_buffer_mib: Math.round(Number(settings?.default_buffer_bytes ?? 128 * MIB) / MIB) || 128,
    retry_max_attempts: Number(settings?.retry_max_attempts ?? 3),
    checksum_enabled: settings?.checksum_enabled === '1',
    bandwidth_down_mbps: Math.round(Number(settings?.bandwidth_down_bps ?? 0) / MBPS),
    bandwidth_up_mbps: Math.round(Number(settings?.bandwidth_up_bps ?? 0) / MBPS),
    notifications_toast: settings?.notifications_toast !== '0',
    telegram_enabled: settings?.telegram_enabled === '1',
    telegram_token: '',
    telegram_chat_id: '',
    crash_reporting_enabled: settings?.crash_reporting_enabled === '1',
  }
}

/** SettingsDialog edits the persisted configuration by tab. */
export function SettingsDialog({
  open,
  onOpenChange,
  settings,
  status,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  settings?: Settings
  status?: Status
}) {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<Draft>(() => toDraft(settings))
  const [updateInfo, setUpdateInfo] = useState<{
    available: boolean
    current_version: string
    version?: string
    size_bytes?: number
    sha256?: string
    notes?: string
    signature_checked?: boolean
    error_code?: string
  } | null>(null)
  const [checking, setChecking] = useState(false)

  useEffect(() => {
    if (open) setDraft(toDraft(settings))
  }, [open, settings])

  const save = useMutation({
    mutationFn: () => {
      const payload: Record<string, string | number | boolean> = {
        parallel_limit: draft.parallel_limit,
        default_mode: draft.default_mode,
        default_buffer_bytes: draft.default_buffer_mib * MIB,
        retry_max_attempts: draft.retry_max_attempts,
        checksum_enabled: draft.checksum_enabled,
        bandwidth_down_bps: draft.bandwidth_down_mbps * MBPS,
        bandwidth_up_bps: draft.bandwidth_up_mbps * MBPS,
        notifications_toast: draft.notifications_toast,
        telegram_enabled: draft.telegram_enabled,
        crash_reporting_enabled: draft.crash_reporting_enabled,
      }
      if (draft.telegram_token.trim()) payload.telegram_token = draft.telegram_token.trim()
      if (draft.telegram_chat_id.trim()) payload.telegram_chat_id = draft.telegram_chat_id.trim()
      return api.updateSettings(payload)
    },
    onSuccess: () => {
      toast.success(t('settings.saved'))
      setDraft((d) => ({ ...d, telegram_token: '', telegram_chat_id: '' }))
      void queryClient.invalidateQueries({ queryKey: ['settings'] })
      void queryClient.invalidateQueries({ queryKey: ['status'] })
      void queryClient.invalidateQueries({ queryKey: ['queue'] })
    },
    onError: (err: unknown) => {
      const code = err instanceof ApiError ? err.code : 'internal_error'
      toast.error(t(`errors.${code}`, { defaultValue: code }))
    },
  })

  const testNotification = useMutation({
    mutationFn: () => api.testNotification(),
    onSuccess: () => toast.success(t('settings.testSent')),
    onError: (err: unknown) => {
      const code = err instanceof ApiError ? err.code : 'internal_error'
      toast.error(t(`errors.${code}`, { defaultValue: code }))
    },
  })

  const apply = useMutation({
    mutationFn: () => api.applyUpdate(),
    onSuccess: () => toast.success(t('settings.applied')),
  })

  const check = async () => {
    setChecking(true)
    try {
      setUpdateInfo(await api.update())
    } catch {
      setUpdateInfo({ available: false, current_version: status?.version ?? '' })
    } finally {
      setChecking(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent wide title={t('settings.title')}>
        <Tabs defaultValue="queue">
          <TabsList className="flex-wrap">
            <TabsTrigger value="queue">
              <ListOrdered className="mr-1 inline h-3 w-3" />
              {t('settings.queue')}
            </TabsTrigger>
            <TabsTrigger value="stream">
              <Zap className="mr-1 inline h-3 w-3" />
              {t('settings.stream')}
            </TabsTrigger>
            <TabsTrigger value="notifications">
              <Bell className="mr-1 inline h-3 w-3" />
              {t('settings.notifications')}
            </TabsTrigger>
            <TabsTrigger value="update">
              <DownloadCloud className="mr-1 inline h-3 w-3" />
              {t('settings.update')}
            </TabsTrigger>
            <TabsTrigger value="system">
              <Info className="mr-1 inline h-3 w-3" />
              {t('settings.system')}
            </TabsTrigger>
            <TabsTrigger value="interface">
              <Languages className="mr-1 inline h-3 w-3" />
              {t('settings.interface')}
            </TabsTrigger>
          </TabsList>

          <TabsContent value="queue" className="space-y-4">
            <Row label={t('settings.parallel')} hint="1–5">
              <Select
                className="w-24"
                value={draft.parallel_limit}
                onChange={(e) => setDraft({ ...draft, parallel_limit: Number(e.target.value) })}
              >
                {[1, 2, 3, 4, 5].map((n) => (
                  <option key={n} value={n}>
                    {n}
                  </option>
                ))}
              </Select>
            </Row>
            <Row label={t('settings.retryAttempts')} hint="0–10">
              <Input
                type="number"
                min={0}
                max={10}
                className="w-24"
                value={draft.retry_max_attempts}
                onChange={(e) => setDraft({ ...draft, retry_max_attempts: Number(e.target.value) })}
              />
            </Row>
          </TabsContent>

          <TabsContent value="stream" className="space-y-4">
            <Row label={t('settings.defaultMode')}>
              <Select
                className="w-44"
                value={draft.default_mode}
                onChange={(e) => setDraft({ ...draft, default_mode: e.target.value })}
              >
                <option value="ring_buffer">{t('stream.modeRing')}</option>
                <option value="pure_stream">{t('stream.modePure')}</option>
              </Select>
            </Row>
            <Row label={t('settings.defaultBuffer')}>
              <Select
                className="w-28"
                value={draft.default_buffer_mib}
                onChange={(e) => setDraft({ ...draft, default_buffer_mib: Number(e.target.value) })}
              >
                {BUFFERS.map((size) => (
                  <option key={size} value={size}>
                    {size} MiB
                  </option>
                ))}
              </Select>
            </Row>
            <Row label={t('settings.bwDown')}>
              <Input
                type="number"
                min={0}
                className="w-28"
                value={draft.bandwidth_down_mbps}
                onChange={(e) => setDraft({ ...draft, bandwidth_down_mbps: Math.max(0, Number(e.target.value) || 0) })}
              />
            </Row>
            <Row label={t('settings.bwUp')}>
              <Input
                type="number"
                min={0}
                className="w-28"
                value={draft.bandwidth_up_mbps}
                onChange={(e) => setDraft({ ...draft, bandwidth_up_mbps: Math.max(0, Number(e.target.value) || 0) })}
              />
            </Row>
            <Row label={t('settings.checksum')} hint={t('settings.checksumHint')}>
              <Switch checked={draft.checksum_enabled} onChange={(v) => setDraft({ ...draft, checksum_enabled: v })} />
            </Row>
          </TabsContent>

          <TabsContent value="notifications" className="space-y-4">
            <Row label={t('settings.toast')}>
              <Switch
                checked={draft.notifications_toast}
                onChange={(v) => setDraft({ ...draft, notifications_toast: v })}
              />
            </Row>
            <Row label={t('settings.telegram')}>
              <Switch
                checked={draft.telegram_enabled}
                onChange={(v) => setDraft({ ...draft, telegram_enabled: v })}
              />
            </Row>
            <div className="space-y-2">
              <Label>{t('settings.telegramToken')}</Label>
              <Input
                type="password"
                autoComplete="off"
                value={draft.telegram_token}
                onChange={(e) => setDraft({ ...draft, telegram_token: e.target.value })}
                placeholder={settings?.has_telegram_token === '1' ? '••••••••' : ''}
                className="font-mono text-xs"
              />
              <Label>{t('settings.telegramChat')}</Label>
              <Input
                value={draft.telegram_chat_id}
                onChange={(e) => setDraft({ ...draft, telegram_chat_id: e.target.value })}
                className="font-mono text-xs"
              />
            </div>
            <Button
              size="sm"
              variant="secondary"
              disabled={testNotification.isPending}
              onClick={() => testNotification.mutate()}
            >
              <Send className="h-3.5 w-3.5" />
              {t('settings.testNotification')}
            </Button>
          </TabsContent>

          <TabsContent value="update" className="space-y-3">
            <Row label={t('settings.updateCurrent')}>
              <div className="flex items-center gap-2">
                <Badge tone="accent">{status?.version ?? '—'}</Badge>
                <Button size="sm" variant="secondary" disabled={checking} onClick={() => void check()}>
                  {checking ? t('settings.checking') : t('settings.check')}
                </Button>
              </div>
            </Row>
            {updateInfo ? (
              updateInfo.available ? (
                <div className="space-y-2 rounded-xl border border-sky-400/20 bg-sky-400/[0.05] p-3">
                  <p className="text-xs text-sky-200">{t('settings.available', { version: updateInfo.version })}</p>
                  <div className="space-y-0.5 text-[10px] text-slate-400">
                    <p>{formatBytes(updateInfo.size_bytes ?? 0, i18n.language)}</p>
                    <p className="break-all font-mono">{updateInfo.sha256}</p>
                    <p>{updateInfo.signature_checked ? t('settings.signature') : t('settings.noSignature')}</p>
                  </div>
                  {updateInfo.notes ? (
                    <div>
                      <p className="text-[10px] uppercase tracking-wide text-slate-500">{t('settings.updateNotes')}</p>
                      <p className="mt-0.5 whitespace-pre-wrap text-[11px] text-slate-300">{updateInfo.notes}</p>
                    </div>
                  ) : null}
                  <Button size="sm" variant="primary" disabled={apply.isPending} onClick={() => apply.mutate()}>
                    {t('settings.apply')}
                  </Button>
                </div>
              ) : (
                <p className="text-[11px] text-slate-500">
                  {updateInfo.error_code
                    ? t(`errors.${updateInfo.error_code}`, { defaultValue: updateInfo.error_code })
                    : t('settings.notAvailable')}
                </p>
              )
            ) : null}
          </TabsContent>

          <TabsContent value="system" className="space-y-4">
            <Row label={t('settings.version')}>
              <Badge tone="muted">{status?.version ?? '—'}</Badge>
            </Row>
            <Row label="Port">
              <Badge tone="muted">{status?.port ?? '—'}</Badge>
            </Row>
            <Row label={t('settings.crash')} hint={t('settings.crashHint')}>
              <Switch
                checked={draft.crash_reporting_enabled}
                onChange={(v) => setDraft({ ...draft, crash_reporting_enabled: v })}
              />
            </Row>
            <Row label={t('header.uptime')}>
              <span className="numeric text-xs text-slate-300">
                {Math.floor((status?.uptime_seconds ?? 0) / 60)}m
              </span>
            </Row>
          </TabsContent>

          <TabsContent value="interface" className="space-y-4">
            <Row label={t('settings.locale')}>
              <LanguageSwitcher />
            </Row>
            <p className="text-[11px] leading-relaxed text-slate-500">{t('settings.restartHint')}</p>
          </TabsContent>
        </Tabs>

        <div className="mt-4 flex items-center justify-between gap-2 border-t border-white/[0.07] pt-3">
          <ErrorText code={save.error instanceof ApiError ? save.error.code : null} />
          <div className="flex gap-2">
            <Button variant="ghost" size="sm" onClick={() => onOpenChange(false)}>
              {t('common.close')}
            </Button>
            <Button variant="primary" size="sm" disabled={save.isPending} onClick={() => save.mutate()}>
              {t('common.save')}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}

function Row({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-4 border-b border-white/[0.05] pb-3 last:border-0">
      <div>
        <p className="text-xs text-slate-300">{label}</p>
        {hint ? <p className="mt-0.5 max-w-xs text-[10px] leading-relaxed text-slate-500">{hint}</p> : null}
      </div>
      {children}
    </div>
  )
}

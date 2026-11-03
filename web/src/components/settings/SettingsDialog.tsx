import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Bell, DownloadCloud, Info, Languages, ListOrdered, Send, ShieldCheck, SlidersHorizontal, Zap } from 'lucide-react'
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

/** Third-party components shipped or used by StreamDrop (settings → Licenses). */
const THIRD_PARTY: { name: string; license: string; url: string }[] = [
  { name: 'rclone', license: 'MIT', url: 'https://rclone.org/' },
  { name: 'yt-dlp', license: 'Unlicense', url: 'https://github.com/yt-dlp/yt-dlp' },
  { name: 'curl', license: 'curl / MIT', url: 'https://curl.se/' },
  { name: 'Go', license: 'BSD-3-Clause', url: 'https://go.dev/' },
  { name: 'React', license: 'MIT', url: 'https://react.dev/' },
  { name: 'Vite', license: 'MIT', url: 'https://vitejs.dev/' },
  { name: 'Tailwind CSS', license: 'MIT', url: 'https://tailwindcss.com/' },
  { name: 'Radix UI', license: 'MIT', url: 'https://www.radix-ui.com/' },
  { name: 'framer-motion', license: 'MIT', url: 'https://motion.dev/' },
  { name: 'recharts', license: 'MIT', url: 'https://recharts.org/' },
  { name: 'sonner', license: 'MIT', url: 'https://sonner.emilkowal.ski/' },
  { name: 'lucide-react', license: 'ISC', url: 'https://lucide.dev/' },
  { name: 'i18next', license: 'MIT', url: 'https://www.i18next.com/' },
  { name: 'modernc.org/sqlite', license: 'BSD-3-Clause', url: 'https://pkg.go.dev/modernc.org/sqlite' },
  { name: 'go-chi/chi', license: 'MIT', url: 'https://github.com/go-chi/chi' },
  { name: 'goose', license: 'MIT', url: 'https://github.com/pressly/goose' },
  { name: 'energye/systray', license: 'Apache-2.0', url: 'https://github.com/energye/systray' },
  { name: 'go-toast', license: 'MIT', url: 'https://github.com/go-toast/toast' },
  { name: 'golang.org/x/crypto & x/sys', license: 'BSD-3-Clause', url: 'https://pkg.go.dev/golang.org/x/crypto' },
]

/** SettingsDialog edits persisted configuration. Changes apply immediately. */
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
  const [telegramToken, setTelegramToken] = useState('')
  const [telegramChat, setTelegramChat] = useState('')

  const save = useMutation({
    mutationFn: (patch: Record<string, string | number | boolean>) => api.updateSettings(patch),
    onSuccess: () => {
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

  const saveTelegram = () => {
    const patch: Record<string, string> = {}
    if (telegramToken.trim()) patch.telegram_token = telegramToken.trim()
    if (telegramChat.trim()) patch.telegram_chat_id = telegramChat.trim()
    if (Object.keys(patch).length) {
      save.mutate(patch)
      setTelegramToken('')
      setTelegramChat('')
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
            <TabsTrigger value="licenses">
              <ShieldCheck className="mr-1 inline h-3 w-3" />
              {t('settings.licenses')}
            </TabsTrigger>
          </TabsList>

          <TabsContent value="queue" className="space-y-4">
            <Row label={t('settings.parallel')} hint="1–5">
              <Select
                className="w-24"
                value={settings?.parallel_limit ?? '1'}
                onChange={(e) => save.mutate({ parallel_limit: Number(e.target.value) })}
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
                value={settings?.retry_max_attempts ?? '3'}
                onChange={(e) => save.mutate({ retry_max_attempts: Number(e.target.value) })}
              />
            </Row>
          </TabsContent>

          <TabsContent value="stream" className="space-y-4">
            <Row label={t('settings.defaultMode')}>
              <Select
                className="w-44"
                value={settings?.default_mode ?? 'ring_buffer'}
                onChange={(e) => save.mutate({ default_mode: e.target.value })}
              >
                <option value="ring_buffer">{t('stream.modeRing')}</option>
                <option value="pure_stream">{t('stream.modePure')}</option>
              </Select>
            </Row>
            <Row label={t('settings.defaultBuffer')}>
              <Select
                className="w-28"
                value={settings?.default_buffer_bytes ?? String(128 * MIB)}
                onChange={(e) => save.mutate({ default_buffer_bytes: Number(e.target.value) })}
              >
                {BUFFERS.map((size) => (
                  <option key={size} value={size * MIB}>
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
                value={settings?.bandwidth_down_bps ?? '0'}
                onChange={(e) => save.mutate({ bandwidth_down_bps: Math.max(0, Number(e.target.value) || 0) * MBPS })}
              />
            </Row>
            <Row label={t('settings.bwUp')}>
              <Input
                type="number"
                min={0}
                className="w-28"
                value={settings?.bandwidth_up_bps ?? '0'}
                onChange={(e) => save.mutate({ bandwidth_up_bps: Math.max(0, Number(e.target.value) || 0) * MBPS })}
              />
            </Row>
            <Row label={t('settings.checksum')} hint={t('settings.checksumHint')}>
              <Switch
                checked={settings?.checksum_enabled === '1'}
                onChange={(v) => save.mutate({ checksum_enabled: v ? '1' : '0' })}
              />
            </Row>
          </TabsContent>

          <TabsContent value="notifications" className="space-y-4">
            <Row label={t('settings.toast')}>
              <Switch
                checked={settings?.notifications_toast !== '0'}
                onChange={(v) => save.mutate({ notifications_toast: v ? '1' : '0' })}
              />
            </Row>
            <Row label={t('settings.telegram')}>
              <Switch
                checked={settings?.telegram_enabled === '1'}
                onChange={(v) => save.mutate({ telegram_enabled: v ? '1' : '0' })}
              />
            </Row>
            <div className="space-y-2">
              <Label>{t('settings.telegramToken')}</Label>
              <Input
                type="password"
                autoComplete="off"
                value={telegramToken}
                onChange={(e) => setTelegramToken(e.target.value)}
                placeholder={settings?.has_telegram_token === '1' ? '••••••••' : ''}
                className="font-mono text-xs"
              />
              <Label>{t('settings.telegramChat')}</Label>
              <Input
                value={telegramChat}
                onChange={(e) => setTelegramChat(e.target.value)}
                className="font-mono text-xs"
              />
              <div className="flex items-center gap-2 pt-1">
                <Button size="sm" variant="secondary" disabled={save.isPending} onClick={saveTelegram}>
                  <Send className="h-3.5 w-3.5" />
                  {t('common.save')}
                </Button>
                <Button size="sm" variant="ghost" disabled={testNotification.isPending} onClick={() => testNotification.mutate()}>
                  {t('settings.testNotification')}
                </Button>
              </div>
            </div>
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
                checked={settings?.crash_reporting_enabled === '1'}
                onChange={(v) => save.mutate({ crash_reporting_enabled: v ? '1' : '0' })}
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

          <TabsContent value="licenses" className="space-y-4">
            <p className="text-[11px] leading-relaxed text-slate-500">{t('settings.licensesNote')}</p>
            <div className="divide-y divide-white/[0.05] rounded-xl border border-white/[0.07]">
              {THIRD_PARTY.map((item) => (
                <div key={item.name} className="flex items-center justify-between gap-3 px-3 py-2">
                  <div className="min-w-0">
                    <p className="truncate text-xs text-slate-200">{item.name}</p>
                    <p className="text-[10px] text-slate-500">{item.license}</p>
                  </div>
                  <a
                    href={item.url}
                    target="_blank"
                    rel="noreferrer"
                    className="shrink-0 text-[11px] text-sky-300 hover:text-sky-200"
                  >
                    {t('common.open')}
                  </a>
                </div>
              ))}
            </div>
          </TabsContent>
        </Tabs>

        {save.error ? (
          <div className="mt-4 border-t border-white/[0.07] pt-3">
            <ErrorText code={save.error instanceof ApiError ? save.error.code : null} />
          </div>
        ) : null}
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

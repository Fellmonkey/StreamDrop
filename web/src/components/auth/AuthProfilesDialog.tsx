import { useRef, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Cookie, FileUp, Globe, Plus, ShieldAlert, Trash2, Upload } from 'lucide-react'
import type { AuthProfile, DetectedBrowser } from '@/types/api'
import { api, ApiError } from '@/lib/api'
import {
  Badge,
  Button,
  Dialog,
  DialogContent,
  Input,
  Label,
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
  Textarea,
} from '@/components/ui/primitives'
import { EmptyState, ErrorText } from '@/components/common/StatusBadge'
import { ConfirmDialog } from '@/components/common/ConfirmDialog'

type Mode = 'browser' | 'cookies_file'

/** Realistic User-Agent stubs per detected browser, used to prefill profiles
 * even when the cookies of that browser cannot be extracted (encrypted). */
const BROWSER_UA: Record<string, string> = {
  chrome:
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36',
  edge:
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36 Edg/140.0.0.0',
  firefox:
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:140.0) Gecko/20100101 Firefox/140.0',
  brave:
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36',
  opera:
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36 OPR/126.0.0.0',
  safari:
    'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15',
}

/** AuthProfilesDialog creates source-auth profiles and tests them. */
export function AuthProfilesDialog({
  open,
  onOpenChange,
  profiles,
  browsers,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  profiles: AuthProfile[]
  browsers: DetectedBrowser[]
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const fileInputRef = useRef<HTMLInputElement>(null)
  const [adding, setAdding] = useState(false)
  const [mode, setMode] = useState<Mode>('cookies_file')
  const [name, setName] = useState('')
  const [kind, setKind] = useState('')
  const [profileDir, setProfileDir] = useState('')
  const [cookiesTxt, setCookiesTxt] = useState('')
  const [cookieFile, setCookieFile] = useState('')
  const [userAgent, setUserAgent] = useState('')
  const [referer, setReferer] = useState('')
  const [testUrl, setTestUrl] = useState('')
  const [deleteTarget, setDeleteTarget] = useState<AuthProfile | null>(null)
  const [testResult, setTestResult] = useState<{ id: string; ok: boolean; text: string } | null>(null)

  /** pickCookiesFile opens the OS file picker and loads a cookies.txt file. */
  const pickCookiesFile = async (file: File | undefined) => {
    if (!file) return
    setCookieFile(file.name)
    const text = await file.text()
    setCookiesTxt(text)
    if (!name) setName(file.name.replace(/\.txt$/i, '') || 'cookies.txt')
  }

  const create = useMutation({
    mutationFn: () =>
      api.createProfile({
        name: name.trim(),
        mode,
        kind: mode === 'browser' ? kind : '',
        profile_dir: mode === 'browser' ? profileDir : '',
        cookies_txt: mode === 'cookies_file' ? cookiesTxt : '',
        user_agent: userAgent,
        referer,
      }),
    onSuccess: () => {
      toast.success(t('auth.created'))
      setName('')
      setCookiesTxt('')
      setCookieFile('')
      setAdding(false)
      void queryClient.invalidateQueries({ queryKey: ['profiles'] })
    },
    onError: (err: unknown) => {
      const code = err instanceof ApiError ? err.code : 'internal_error'
      toast.error(t(`errors.${code}`, { defaultValue: code }))
    },
  })

  const remove = useMutation({
    mutationFn: (id: string) => api.deleteProfile(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['profiles'] })
      void queryClient.invalidateQueries({ queryKey: ['settings'] })
    },
  })

  const test = useMutation({
    mutationFn: (id: string) => api.testProfile(id, testUrl.trim()),
    onSuccess: (data, id) => {
      setTestResult({ id, ok: true, text: data.metadata?.filename ?? 'OK' })
    },
    onError: (err: unknown, id) => {
      const code = err instanceof ApiError ? err.code : 'internal_error'
      setTestResult({ id, ok: false, text: t(`errors.${code}`, { defaultValue: code }) })
    },
  })

  const canCreate =
    mode === 'browser' ? Boolean(kind) : cookiesTxt.trim().length > 10

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent wide title={t('auth.title')} description={t('auth.subtitle')}>
        <div className="space-y-4">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <div className="flex items-center gap-2">
              <Input
                value={testUrl}
                onChange={(e) => setTestUrl(e.target.value)}
                placeholder={t('auth.testUrl')}
                className="h-9 w-full text-xs sm:w-72"
              />
            </div>
            <Button size="sm" variant={adding ? 'secondary' : 'primary'} onClick={() => setAdding((v) => !v)}>
              <Plus className="h-3.5 w-3.5" />
              {t('auth.add')}
            </Button>
          </div>

          {adding ? (
            <div className="space-y-3 rounded-xl border border-white/[0.07] bg-white/[0.02] p-3">
              <Tabs value={mode} onValueChange={(value) => setMode(value as Mode)}>
                <TabsList>
                  <TabsTrigger value="browser">{t('auth.browser')}</TabsTrigger>
                  <TabsTrigger value="cookies_file">{t('auth.file')}</TabsTrigger>
                </TabsList>

                <TabsContent value="browser" className="space-y-2">
                  <div className="space-y-1">
                    <Label>{t('auth.detected')}</Label>
                    <div className="grid gap-1.5 sm:grid-cols-2">
                      {browsers.length === 0 ? (
                        <p className="text-[11px] text-slate-500">{t('auth.unsupported')}</p>
                      ) : (
                        browsers.map((browser) => (
                          <button
                            key={`${browser.browser}-${browser.dir}`}
                            type="button"
                            onClick={() => {
                              setKind(browser.browser)
                              setProfileDir(browser.dir)
                              if (!name) setName(browser.label)
                              // Prefill a realistic User-Agent from the
                              // selected browser — works even when the
                              // cookies cannot be extracted (encrypted).
                              setUserAgent(BROWSER_UA[browser.browser] ?? '')
                            }}
                            className={`rounded-lg border px-2.5 py-2 text-left text-[11px] transition-colors ${
                              profileDir === browser.dir
                                ? 'border-sky-400/40 bg-sky-400/10 text-sky-200'
                                : 'border-white/[0.07] text-slate-300 hover:bg-white/[0.04]'
                            }`}
                          >
                            <span className="flex items-center gap-1.5">
                              <Globe className="h-3 w-3" />
                              {browser.label}
                              {browser.supported ? null : <ShieldAlert className="h-3 w-3 text-amber-300" />}
                            </span>
                            {browser.note ? (
                              <span className="mt-1 block text-[10px] leading-snug text-amber-300/80">{browser.note}</span>
                            ) : null}
                          </button>
                        ))
                      )}
                    </div>
                  </div>
                </TabsContent>

                <TabsContent value="cookies_file" className="space-y-2">
                  <div className="flex items-center gap-2">
                    <Label className="shrink-0">{t('auth.cookieFile')}</Label>
                    <input
                      ref={fileInputRef}
                      type="file"
                      accept=".txt,.cookies,text/plain"
                      className="hidden"
                      onChange={(e) => void pickCookiesFile(e.target.files?.[0])}
                    />
                    <Button size="sm" variant="secondary" onClick={() => fileInputRef.current?.click()}>
                      <FileUp className="h-3.5 w-3.5" />
                      {t('auth.chooseFile')}
                    </Button>
                    {cookieFile ? <span className="truncate text-[10px] text-slate-400">{cookieFile}</span> : null}
                  </div>
                  <Textarea
                    rows={5}
                    value={cookiesTxt}
                    onChange={(e) => setCookiesTxt(e.target.value)}
                    placeholder="# Netscape HTTP Cookie File&#10;.example.com	TRUE	/	TRUE	0	sid	abc123"
                    className="font-mono text-[11px]"
                  />
                  <p className="text-[10px] text-slate-500">{t('auth.importHint')}</p>
                </TabsContent>
              </Tabs>

              <div className="grid gap-2 sm:grid-cols-2">
                <div className="space-y-1">
                  <Label>{t('auth.name')}</Label>
                  <Input value={name} onChange={(e) => setName(e.target.value)} className="h-8 text-xs" />
                </div>
                <div className="space-y-1">
                  <Label>{t('auth.userAgent')}</Label>
                  <Input value={userAgent} onChange={(e) => setUserAgent(e.target.value)} className="h-8 font-mono text-[10px]" />
                  {!userAgent ? <p className="text-[10px] text-slate-500">{t('auth.uaHint')}</p> : null}
                </div>
                <div className="space-y-1">
                  <Label>{t('auth.referer')}</Label>
                  <Input value={referer} onChange={(e) => setReferer(e.target.value)} placeholder="https://" className="h-8 text-xs" />
                  {!referer ? <p className="text-[10px] text-slate-500">{t('auth.refererHint')}</p> : null}
                </div>
              </div>

              <div className="flex justify-end gap-2">
                <Button size="sm" variant="ghost" onClick={() => setAdding(false)}>
                  {t('common.cancel')}
                </Button>
                <Button size="sm" variant="primary" disabled={!canCreate || create.isPending} onClick={() => create.mutate()}>
                  {t('common.save')}
                </Button>
              </div>
              {create.error ? <ErrorText code={create.error instanceof ApiError ? create.error.code : 'internal_error'} /> : null}
            </div>
          ) : null}

          {profiles.length === 0 ? (
            <EmptyState icon={<Cookie className="h-6 w-6" />} title={t('auth.empty')} compact />
          ) : (
            <ul className="divide-y divide-white/[0.05] rounded-xl border border-white/[0.07]">
              {profiles.map((profile) => (
                <li key={profile.id} className="flex items-center gap-3 px-3 py-2.5">
                  <div className="flex h-8 w-8 items-center justify-center rounded-lg bg-white/[0.04] text-slate-400">
                    {profile.mode === 'browser' ? <Globe className="h-4 w-4" /> : profile.mode === 'cookies_file' ? <Upload className="h-4 w-4" /> : <Cookie className="h-4 w-4" />}
                  </div>
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <p className="truncate text-xs text-slate-200">{profile.name}</p>
                      <Badge tone="muted">{profile.mode}</Badge>
                      {profile.has_manual_cookie ? <Badge tone="success">{profile.cookie_mask}</Badge> : null}
                    </div>
                    <p className="truncate text-[10px] text-slate-600">
                      {profile.note || profile.profile_dir || profile.cookie_file || profile.user_agent}
                    </p>
                    {testResult?.id === profile.id ? (
                      <p className={`text-[10px] ${testResult.ok ? 'text-emerald-300' : 'text-red-300'}`}>
                        {testResult.ok ? t('auth.testOk') : t('auth.testFailed')}: {testResult.text}
                      </p>
                    ) : null}
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    disabled={!testUrl.trim() || test.isPending}
                    onClick={() => test.mutate(profile.id)}
                  >
                    {t('common.test')}
                  </Button>
                  <Button
                    variant="ghost"
                    size="icon"
                    className="text-slate-500 hover:text-red-300"
                    onClick={() => setDeleteTarget(profile)}
                    aria-label={t('common.delete')}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </div>

        <ConfirmDialog
          open={Boolean(deleteTarget)}
          title={t('common.delete')}
          body={deleteTarget?.name}
          danger
          confirmLabel={t('common.delete')}
          onOpenChange={(value) => !value && setDeleteTarget(null)}
          onConfirm={() => {
            if (deleteTarget) remove.mutate(deleteTarget.id)
            setDeleteTarget(null)
          }}
        />
      </DialogContent>
    </Dialog>
  )
}

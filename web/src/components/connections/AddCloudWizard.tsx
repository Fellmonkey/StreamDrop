import { useEffect, useMemo, useState } from 'react'
import { AnimatePresence, motion } from 'framer-motion'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { ArrowLeft, ArrowRight, Check, Cloud, ExternalLink, Loader2, ShieldCheck } from 'lucide-react'
import { EASE_OUT } from '@/lib/motion'
import type { OAuthProvider } from '@/types/api'
import { api, ApiError } from '@/lib/api'
import { useOAuth } from '@/hooks/useOAuth'
import {
  Badge,
  Button,
  Dialog,
  DialogContent,
  Input,
  Label,
  Progress,
  Select,
} from '@/components/ui/primitives'
import { ErrorText } from '@/components/common/StatusBadge'
import { cn } from '@/lib/utils'

type AccessKind = 'password' | 'keys' | 'host' | 'oauth' | 'token'

interface ProviderSpec {
  id: string
  access: AccessKind
  needsURL?: boolean
  needsHost?: boolean
}

const PROVIDERS: ProviderSpec[] = [
  { id: 'mailru', access: 'password', needsURL: true },
  { id: 'yandex', access: 'token' },
  { id: 'drive', access: 'oauth' },
  { id: 'onedrive', access: 'oauth' },
  { id: 's3', access: 'keys' },
  { id: 'webdav', access: 'password', needsURL: true },
  { id: 'ftp', access: 'host', needsHost: true },
  { id: 'sftp', access: 'host', needsHost: true },
]

const STEP_KEYS = ['stepProvider', 'stepAccess', 'stepVerify', 'stepName']

/** AddCloudWizard drives provider choice, credentials, verification and save. */
export function AddCloudWizard({
  open,
  onOpenChange,
  onSaved,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onSaved: () => void
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [step, setStep] = useState(0)
  const [dir, setDir] = useState(1)

  const goTo = (next: number) => {
    setDir(next > step ? 1 : -1)
    setStep(next)
  }

  const [providerId, setProviderId] = useState('')
  const [name, setName] = useState('')
  const [user, setUser] = useState('')
  const [pass, setPass] = useState('')
  const [url, setUrl] = useState('')
  const [host, setHost] = useState('')
  const [port, setPort] = useState('')
  const [accessKey, setAccessKey] = useState('')
  const [secretKey, setSecretKey] = useState('')
  const [endpoint, setEndpoint] = useState('')
  const [region, setRegion] = useState('')
  const [token, setToken] = useState('')
  const [clientId, setClientId] = useState('')
  const [clientSecret, setClientSecret] = useState('')
  const [verifyState, setVerifyState] = useState<'idle' | 'ok' | 'failed'>('idle')

  const oauth = useOAuth(() => setVerifyState('ok'))

  const providers = useQuery({
    queryKey: ['oauthProviders'],
    queryFn: api.oauthProviders,
    enabled: open,
    staleTime: 60_000,
  })

  const spec = PROVIDERS.find((p) => p.id === providerId)
  const oauthProvider: OAuthProvider | undefined = useMemo(
    () => (providers.data ?? []).find((p) => p.id === providerId),
    [providers.data, providerId],
  )
  const access: AccessKind = spec?.access === 'oauth' && !oauthProvider ? 'token' : spec?.access ?? 'password'

  useEffect(() => {
    if (!open) return
    setStep(0)
    setProviderId('')
    setName('')
    setUser('')
    setPass('')
    setUrl('')
    setHost('')
    setPort('')
    setAccessKey('')
    setSecretKey('')
    setEndpoint('')
    setRegion('')
    setToken('')
    setClientId('')
    setClientSecret('')
    setVerifyState('idle')
    oauth.reset()
  }, [open])

  const options = useMemo(() => {
    const out: Record<string, string> = {}
    if (providerId === 'mailru') {
      out.url = url || 'https://webdav.cloud.mail.ru'
      out.user = user
      out.pass = pass
    } else if (providerId === 'webdav') {
      out.url = url
      out.user = user
      out.pass = pass
    } else if (providerId === 'yandex') {
      out.token = token
    } else if (providerId === 's3') {
      out.access_key_id = accessKey
      out.secret_access_key = secretKey
      if (endpoint) out.endpoint = endpoint
      if (region) out.region = region
    } else if (providerId === 'ftp' || providerId === 'sftp') {
      out.host = host
      out.user = user
      out.pass = pass
      if (port) out.port = port
    }
    return out
  }, [providerId, url, user, pass, token, accessKey, secretKey, endpoint, region, host, port])

  const verify = useMutation({
    mutationFn: () => api.testRemoteCredentials(providerId, options),
    onSuccess: (data) => {
      if (data.ok) {
        setVerifyState('ok')
      } else {
        setVerifyState('failed')
      }
    },
    onError: () => setVerifyState('failed'),
  })

  const create = useMutation({
    mutationFn: () =>
      api.createRemote({
        name: name.trim(),
        provider: providerId,
        endpoint: options.url || options.endpoint || '',
        options,
      }),
    onSuccess: (data) => {
      if (data.error) {
        toast.error(t(`errors.${data.error.error.code}`, { defaultValue: data.error.error.code }))
      } else {
        toast.success(t('connections.saved'))
      }
      void queryClient.invalidateQueries({ queryKey: ['remotes'] })
      void queryClient.invalidateQueries({ queryKey: ['status'] })
      onSaved()
      onOpenChange(false)
    },
    onError: (err: unknown) => {
      const code = err instanceof ApiError ? err.code : 'internal_error'
      toast.error(t(`errors.${code}`, { defaultValue: code }))
    },
  })

  const credentialsReady =
    access === 'oauth'
      ? verifyState === 'ok'
      : access === 'keys'
        ? Boolean(accessKey && secretKey)
        : access === 'token'
          ? Boolean(token)
          : access === 'host'
            ? Boolean(host && user)
            : Boolean(user && pass) && (!spec?.needsURL || Boolean(url))

  const next = () => {
    if (step === 1 && access !== 'oauth') {
      goTo(2)
      verify.mutate()
      return
    }
    if (step === 1 && access === 'oauth') {
      goTo(2)
      return
    }
    goTo(Math.min(3, step + 1))
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent wide title={t('connections.wizardTitle')} description={t('connections.step', { current: step + 1, total: 4 })}>
        <div className="space-y-4">
          <div className="flex items-center gap-2">
            {STEP_KEYS.map((key, index) => (
              <div key={key} className="flex flex-1 items-center gap-2">
                <div
                  className={cn(
                    'flex h-6 w-6 shrink-0 items-center justify-center rounded-full border text-[10px]',
                    index < step
                      ? 'border-emerald-400/40 bg-emerald-400/10 text-emerald-300'
                      : index === step
                        ? 'border-sky-400/50 bg-sky-400/10 text-sky-300'
                        : 'border-white/10 text-slate-600',
                  )}
                >
                  {index < step ? <Check className="h-3 w-3" /> : index + 1}
                </div>
                <span
                  className={cn(
                    'hidden text-[10px] sm:inline',
                    index === step ? 'text-slate-200' : 'text-slate-600',
                  )}
                >
                  {t(`connections.${key}`)}
                </span>
                {index < 3 ? <div className="h-px flex-1 bg-white/[0.07]" /> : null}
              </div>
            ))}
          </div>

          <AnimatePresence mode="wait" initial={false} custom={dir}>
            <motion.div
              key={step}
              custom={dir}
              initial={{ opacity: 0, x: 12 * dir }}
              animate={{ opacity: 1, x: 0 }}
              exit={{ opacity: 0, x: -12 * dir }}
              transition={{ duration: 0.15, ease: EASE_OUT }}
            >
          {step === 0 ? (
            <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
              {PROVIDERS.map((provider) => (
                <button
                  key={provider.id}
                  type="button"
                  onClick={() => {
                    setProviderId(provider.id)
                    setVerifyState('idle')
                  }}
                  className={cn(
                    'rounded-xl border p-3 text-left transition-colors',
                    providerId === provider.id
                      ? 'border-sky-400/40 bg-sky-400/10'
                      : 'border-white/[0.07] bg-white/[0.02] hover:bg-white/[0.05]',
                  )}
                >
                  <Cloud className="h-4 w-4 text-slate-300" />
                  <p className="mt-2 text-[11px] font-medium text-slate-200">
                    {t(`connections.provider_${provider.id}`)}
                  </p>
                </button>
              ))}
            </div>
          ) : null}

          {step === 1 && spec ? (
            <div className="space-y-3">
              <div className="flex items-center justify-between rounded-lg border border-white/[0.07] px-3 py-2">
                <span className="text-[11px] text-slate-300">{t(`connections.provider_${providerId}`)}</span>
                <Badge tone="accent">
                  {access === 'oauth'
                    ? t('connections.accessOAuth')
                    : access === 'keys'
                      ? t('connections.accessKeys')
                      : t('connections.accessPassword')}
                </Badge>
              </div>

              {access === 'oauth' ? (
                <div className="space-y-3">
                  <p className="text-[11px] leading-relaxed text-slate-400">{t('connections.clientHint')}</p>
                  {oauthProvider?.needs_client_id ? (
                    <div className="grid gap-2 sm:grid-cols-2">
                      <div className="space-y-1">
                        <Label>{t('connections.clientId')}</Label>
                        <Input value={clientId} onChange={(e) => setClientId(e.target.value)} className="h-8 text-xs" />
                      </div>
                      <div className="space-y-1">
                        <Label>{t('connections.clientSecret')}</Label>
                        <Input
                          type="password"
                          value={clientSecret}
                          onChange={(e) => setClientSecret(e.target.value)}
                          className="h-8 text-xs"
                        />
                      </div>
                    </div>
                  ) : null}
                  <Button
                    variant="primary"
                    size="sm"
                    disabled={oauth.status === 'pending'}
                    onClick={() => void oauth.start(providerId, clientId, clientSecret)}
                  >
                    {oauth.status === 'pending' ? (
                      <>
                        <Loader2 className="h-3.5 w-3.5 animate-spin" />
                        {t('connections.oauthWaiting')}
                      </>
                    ) : (
                      <>
                        <ExternalLink className="h-3.5 w-3.5" />
                        {t('connections.oauthStart')}
                      </>
                    )}
                  </Button>
                  {verifyState === 'ok' ? (
                    <p className="flex items-center gap-1.5 text-[11px] text-emerald-300">
                      <ShieldCheck className="h-3.5 w-3.5" />
                      {t('connections.oauthDone')}
                    </p>
                  ) : null}
                  {oauth.status === 'failed' ? (
                    <ErrorText code={oauth.error?.code ?? 'auth_rejected'} />
                  ) : null}
                </div>
              ) : (
                <div className="grid gap-2 sm:grid-cols-2">
                  {access === 'token' ? (
                    <div className="space-y-1 sm:col-span-2">
                      <Label>OAuth token</Label>
                      <Input value={token} onChange={(e) => setToken(e.target.value)} className="h-8 font-mono text-xs" />
                    </div>
                  ) : null}

                  {access === 'keys' ? (
                    <>
                      <div className="space-y-1">
                        <Label>{t('connections.accessKey')}</Label>
                        <Input value={accessKey} onChange={(e) => setAccessKey(e.target.value)} className="h-8 font-mono text-xs" />
                      </div>
                      <div className="space-y-1">
                        <Label>{t('connections.secretKey')}</Label>
                        <Input
                          type="password"
                          value={secretKey}
                          onChange={(e) => setSecretKey(e.target.value)}
                          className="h-8 font-mono text-xs"
                        />
                      </div>
                      <div className="space-y-1">
                        <Label>{t('connections.endpoint')}</Label>
                        <Input value={endpoint} onChange={(e) => setEndpoint(e.target.value)} className="h-8 text-xs" />
                      </div>
                      <div className="space-y-1">
                        <Label>{t('connections.region')}</Label>
                        <Input value={region} onChange={(e) => setRegion(e.target.value)} className="h-8 text-xs" />
                      </div>
                    </>
                  ) : null}

                  {access === 'host' ? (
                    <>
                      <div className="space-y-1">
                        <Label>{t('connections.host')}</Label>
                        <Input value={host} onChange={(e) => setHost(e.target.value)} className="h-8 text-xs" />
                      </div>
                      <div className="space-y-1">
                        <Label>{t('connections.port')}</Label>
                        <Input value={port} onChange={(e) => setPort(e.target.value)} className="h-8 text-xs" />
                      </div>
                    </>
                  ) : null}

                  {access === 'password' ? (
                    <>
                      {spec.needsURL ? (
                        <div className="space-y-1 sm:col-span-2">
                          <Label>{t('connections.webdavUrl')}</Label>
                          <Input
                            value={url}
                            onChange={(e) => setUrl(e.target.value)}
                            placeholder="https://webdav.cloud.mail.ru"
                            className="h-8 text-xs"
                          />
                        </div>
                      ) : null}
                      <div className="space-y-1">
                        <Label>{t('connections.login')}</Label>
                        <Input value={user} onChange={(e) => setUser(e.target.value)} className="h-8 text-xs" />
                      </div>
                      <div className="space-y-1">
                        <Label>{t('connections.password')}</Label>
                        <Input
                          type="password"
                          value={pass}
                          onChange={(e) => setPass(e.target.value)}
                          className="h-8 text-xs"
                        />
                      </div>
                    </>
                  ) : null}
                </div>
              )}
            </div>
          ) : null}

          {step === 2 ? (
            <div className="space-y-3">
              {access === 'oauth' ? (
                <p className="text-[11px] text-slate-400">
                  {verifyState === 'ok' ? t('connections.oauthDone') : t('connections.oauthWaiting')}
                </p>
              ) : verify.isPending ? (
                <div className="space-y-2">
                  <p className="text-[11px] text-slate-400">{t('connections.verifying')}</p>
                  <Progress value={60} />
                </div>
              ) : verifyState === 'ok' ? (
                <p className="flex items-center gap-1.5 text-[11px] text-emerald-300">
                  <ShieldCheck className="h-3.5 w-3.5" />
                  {t('connections.verifyOk')}
                </p>
              ) : verifyState === 'failed' ? (
                <div className="space-y-2">
                  <p className="text-[11px] text-red-300">{t('connections.verifyFailed')}</p>
                  <ErrorText
                    code={
                      verify.data && !verify.data.ok
                        ? verify.data.error?.error.code
                        : verify.error instanceof ApiError
                          ? verify.error.code
                          : 'auth_rejected'
                    }
                  />
                </div>
              ) : (
                <p className="text-[11px] text-slate-500">{t('connections.verify')}</p>
              )}
            </div>
          ) : null}

          {step === 3 ? (
            <div className="space-y-2">
              <Label>{t('connections.name')}</Label>
              <Input
                autoFocus
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder={t('connections.namePlaceholder')}
                className="h-9 text-sm"
              />
              <p className="text-[10px] text-slate-500">
                {t(`connections.provider_${providerId}`)} • {access === 'oauth' ? 'OAuth' : access}
              </p>
            </div>
          ) : null}
            </motion.div>
          </AnimatePresence>

          <div className="flex items-center justify-between gap-2 border-t border-white/[0.07] pt-3">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => (step === 0 ? onOpenChange(false) : goTo(step - 1))}
            >
              <ArrowLeft className="h-3.5 w-3.5" />
              {step === 0 ? t('common.cancel') : t('common.back')}
            </Button>

            {step < 3 ? (
              <Button
                variant="primary"
                size="sm"
                disabled={
                  (step === 0 && !providerId) ||
                  (step === 1 && (!credentialsReady || (access === 'oauth' && verifyState !== 'ok'))) ||
                  (step === 2 && verifyState === 'failed')
                }
                onClick={next}
              >
                {t('common.next')}
                <ArrowRight className="h-3.5 w-3.5" />
              </Button>
            ) : (
              <Button
                variant="primary"
                size="sm"
                disabled={!name.trim() || create.isPending}
                onClick={() => create.mutate()}
              >
                {create.isPending ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Check className="h-3.5 w-3.5" />}
                {t('connections.save')}
              </Button>
            )}
          </div>

          {create.error ? (
            <ErrorText code={create.error instanceof ApiError ? create.error.code : 'internal_error'} />
          ) : null}
        </div>
      </DialogContent>
    </Dialog>
  )
}

import { useEffect, useRef, useState } from 'react'
import { AnimatePresence, motion } from 'framer-motion'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Cloud, Droplet, Link2, Package } from 'lucide-react'
import type { DepItem } from '@/types/api'
import { api } from '@/lib/api'
import { Badge, Button, Dialog, DialogContent, Progress } from '@/components/ui/primitives'
import { cn } from '@/lib/utils'
import { EASE_OUT } from '@/lib/motion'

export interface DepProgress {
  received: number
  total: number
  speed_bps: number
  state: string
  code?: string
}

/**
 * FirstRunWizard is the onboarding flow. When a required component is missing
 * (fresh install, empty appdata), dependency installation becomes the first
 * step so the user is never left wondering why transfers won't start.
 */
export function FirstRunWizard({
  open,
  onOpenChange,
  onOpenConnections,
  deps,
  depProgress,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  onOpenConnections: () => void
  deps: DepItem[]
  depProgress: Record<string, DepProgress>
}) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [step, setStep] = useState(0)
  const [installing, setInstalling] = useState('')

  const prevStep = useRef(0)
  const [dir, setDir] = useState(1)

  const changeStep = (next: number) => {
    setDir(next > step ? 1 : -1)
    prevStep.current = next
    setStep(next)
  }

  const complete = useMutation({
    mutationFn: () => api.updateSettings({ onboarded: true }),
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: ['status'] })
      void queryClient.invalidateQueries({ queryKey: ['settings'] })
    },
  })

  const finish = () => {
    complete.mutate()
    onOpenChange(false)
  }

  const missingRequired = deps.filter((d) => d.required && d.status !== 'installed')
  const hasDepsStep = missingRequired.length > 0

  const steps = [
    ...(hasDepsStep ? [{ icon: Package, kind: 'deps' as const }] : []),
    { icon: Cloud, kind: 'connect' as const, title: t('onboarding.step1Title'), body: t('onboarding.step1Body') },
    { icon: Link2, kind: 'link' as const, title: t('onboarding.step2Title'), body: t('onboarding.step2Body') },
    { icon: Droplet, kind: 'start' as const, title: t('onboarding.step3Title'), body: t('onboarding.step3Body') },
  ]
  const current = steps[step]
  const Icon = current.icon

  const rcloneReady = deps.some((d) => d.id === 'rclone' && d.status === 'installed')
  const canNext = current.kind !== 'deps' || rcloneReady

  const install = async (id: string) => {
    setInstalling(id)
    try {
      await api.installDep(id)
    } finally {
      setInstalling('')
      void queryClient.invalidateQueries({ queryKey: ['deps'] })
      void queryClient.invalidateQueries({ queryKey: ['status'] })
    }
  }

  // Fresh installs install the missing required components automatically; the
  // daemon starts in the background via the backend after rclone lands.
  const [autoStarted, setAutoStarted] = useState(false)
  useEffect(() => {
    if (!open || autoStarted || !hasDepsStep) return
    const missingIds = missingRequired.map((d) => d.id)
    if (missingIds.length === 0) return
    setAutoStarted(true)
    void (async () => {
      for (const id of missingIds) {
        setInstalling(id)
        try {
          await api.installDep(id)
        } catch {
          /* leave the row for manual retry */
        }
      }
      setInstalling('')
      queryClient.invalidateQueries({ queryKey: ['deps'] })
      queryClient.invalidateQueries({ queryKey: ['status'] })
    })()
  }, [open])

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent title={t('onboarding.title')} description={t('onboarding.subtitle')}>
        <div className="space-y-4">
          <div className="flex items-center gap-1.5">
            {steps.map((_, index) => (
              <div
                key={index}
                className={cn(
                  'h-1 flex-1 rounded-full transition-colors',
                  index <= step ? 'bg-sky-400' : 'bg-white/10',
                )}
              />
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
              <div className="rounded-xl border border-white/[0.07] bg-white/[0.02] p-5">
                {current.kind === 'deps' ? (
                  <>
                    <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-sky-400/10 text-sky-300">
                      <Package className="h-5 w-5" />
                    </div>
                    <h3 className="mt-3 text-sm font-semibold text-slate-100">{t('onboarding.step0Title')}</h3>
                    <p className="mt-1.5 text-xs leading-relaxed text-slate-400">{t('onboarding.step0Body')}</p>
                    <div className="mt-3 space-y-2">
                      {deps.map((dep) => {
                        const job = depProgress[dep.id]
                        const busy = installing === dep.id || job?.state === 'downloading'
                        const done = dep.status === 'installed'
                        return (
                          <div
                            key={dep.id}
                            className="flex items-center justify-between gap-2 rounded-lg border border-white/[0.07] px-3 py-2"
                          >
                            <div className="min-w-0">
                              <p className="text-xs font-medium text-slate-200">{dep.name}</p>
                              <p className="text-[10px] text-slate-500">{dep.purpose}</p>
                            </div>
                            <div className="flex shrink-0 items-center gap-2">
                              {done ? (
                                <Badge tone="success">{t('status.installed')}</Badge>
                              ) : (
                                <Button size="sm" variant="primary" disabled={busy} onClick={() => void install(dep.id)}>
                                  {busy ? t('common.loading') : dep.status === 'outdated' ? t('deps.update') : t('deps.download')}
                                </Button>
                              )}
                            </div>
                          </div>
                        )
                      })}
                    </div>
                    {!rcloneReady ? (
                      <p className="mt-2 text-[10px] text-amber-300/80">{t('deps.requiredBanner')}</p>
                    ) : null}
                  </>
                ) : (
                  <>
                    <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-sky-400/10 text-sky-300">
                      <Icon className="h-5 w-5" />
                    </div>
                    <h3 className="mt-3 text-sm font-semibold text-slate-100">{current.title}</h3>
                    <p className="mt-1.5 text-xs leading-relaxed text-slate-400">{current.body}</p>
                  </>
                )}
              </div>
            </motion.div>
          </AnimatePresence>

          <div className="flex flex-wrap items-center justify-between gap-2">
            <Button variant="ghost" size="sm" onClick={finish}>
              {t('onboarding.finish')}
            </Button>
            <div className="flex gap-2">
              {step > 0 ? (
                <Button variant="secondary" size="sm" onClick={() => changeStep(step - 1)}>
                  {t('common.back')}
                </Button>
              ) : null}
              {current.kind === 'connect' ? (
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => {
                    // Open the connections panel without leaving the step, so
                    // the user can finish onboarding after adding a cloud.
                    onOpenConnections()
                  }}
                >
                  {t('onboarding.openConnections')}
                </Button>
              ) : null}
              {step < steps.length - 1 ? (
                <Button size="sm" variant="primary" disabled={!canNext} onClick={() => changeStep(step + 1)}>
                  {t('common.next')}
                </Button>
              ) : (
                <Button size="sm" variant="primary" onClick={finish}>
                  {t('onboarding.finish')}
                </Button>
              )}
            </div>
          </div>
          {complete.isPending ? <Progress value={100} className="opacity-40" /> : null}
        </div>
      </DialogContent>
    </Dialog>
  )
}

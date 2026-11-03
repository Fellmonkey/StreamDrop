import { AnimatePresence, motion } from 'framer-motion'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/primitives'
import { EASE_OUT } from '@/lib/motion'

/** ConfirmDialog is the shared destructive-action confirmation. */
export function ConfirmDialog({
  open,
  title,
  body,
  confirmLabel,
  danger,
  onOpenChange,
  onConfirm,
}: {
  open: boolean
  title: string
  body?: string
  confirmLabel?: string
  danger?: boolean
  onOpenChange: (open: boolean) => void
  onConfirm: () => void
}) {
  const { t } = useTranslation()
  return (
    <AnimatePresence>
      {open ? (
        <div className="fixed inset-0 z-[60]">
          <motion.div
            className="absolute inset-0 bg-black/70 backdrop-blur-sm"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={{ duration: 0.18, ease: EASE_OUT }}
          />
          <motion.div
            role="dialog"
            aria-modal="true"
            className="absolute left-1/2 top-1/2 w-full max-w-sm rounded-2xl border border-white/10 bg-[#0d0f16] p-5 shadow-2xl"
            initial={{ opacity: 0, x: '-50%', y: '-50%', scale: 0.96 }}
            animate={{ opacity: 1, x: '-50%', y: '-50%', scale: 1 }}
            exit={{ opacity: 0, x: '-50%', y: '-50%', scale: 0.98 }}
            transition={{ duration: 0.2, ease: EASE_OUT }}
          >
            <h3 className="text-sm font-semibold text-slate-100">{title}</h3>
            {body ? <p className="mt-2 text-xs leading-relaxed text-slate-400">{body}</p> : null}
            <div className="mt-4 flex justify-end gap-2">
              <Button size="sm" variant="ghost" onClick={() => onOpenChange(false)}>
                {t('common.cancel')}
              </Button>
              <Button
                size="sm"
                variant={danger ? 'danger' : 'primary'}
                onClick={() => {
                  onConfirm()
                  onOpenChange(false)
                }}
              >
                {confirmLabel ?? t('common.confirm')}
              </Button>
            </div>
          </motion.div>
        </div>
      ) : null}
    </AnimatePresence>
  )
}

/** useDeleteTransfer centralizes history deletion invalidation. */
export function useDeleteTransfer() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => api.deleteTransfer(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['history'] })
      void queryClient.invalidateQueries({ queryKey: ['queue'] })
      void queryClient.invalidateQueries({ queryKey: ['stats'] })
    },
  })
}

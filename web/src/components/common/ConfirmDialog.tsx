import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/primitives'

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
    <div
      className={
        open
          ? 'fixed inset-0 z-[60] flex items-center justify-center bg-black/70 p-4 backdrop-blur-sm'
          : 'hidden'
      }
      role="dialog"
      aria-modal="true"
    >
      <div className="w-full max-w-sm rounded-2xl border border-white/10 bg-[#0d0f16] p-5 shadow-2xl">
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
      </div>
    </div>
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

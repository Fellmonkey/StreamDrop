import { motion } from 'framer-motion'
import { useTranslation } from 'react-i18next'
import { AlertTriangle, FileText, Loader2, Pencil, RotateCcw, Ruler, ShieldAlert, X } from 'lucide-react'
import type { InspectMetadata } from '@/types/api'
import { ApiError } from '@/lib/api'
import { formatBytes } from '@/lib/formatters'
import { Badge, Button, Card, Input } from '@/components/ui/primitives'
import { ErrorText } from '@/components/common/StatusBadge'
import { EASE_OUT } from '@/lib/motion'

interface LinkInspectorProps {
  metadata: InspectMetadata | null
  loading: boolean
  error: ApiError | null
  filename: string
  editing: boolean
  onEdit: (editing: boolean) => void
  onFilenameChange: (name: string) => void
  htmlBlocked: boolean
}

/** LinkInspector shows the pre-flight probe and lets the file be renamed. */
export function LinkInspector({
  metadata,
  loading,
  error,
  filename,
  editing,
  onEdit,
  onFilenameChange,
  htmlBlocked,
}: LinkInspectorProps) {
  const { t, i18n } = useTranslation()

  if (loading) {
    return (
      <Card className="flex items-center gap-2 px-4 py-3 text-xs text-slate-400">
        <Loader2 className="h-3.5 w-3.5 animate-spin text-sky-300" />
        {t('stream.inspecting')}
      </Card>
    )
  }

  if (error) {
    return (
      <Card className="space-y-1 border-red-400/20 bg-red-500/[0.04] px-4 py-3">
        <div className="flex items-center gap-2 text-xs font-medium text-red-300">
          <AlertTriangle className="h-3.5 w-3.5" />
          {t('errors.inspect_failed')}
        </div>
        <ErrorText code={error.code} details={error.details} />
      </Card>
    )
  }

  if (!metadata) return null

  return (
    <motion.div
      key={metadata.filename}
      initial={{ opacity: 0, y: -6 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.2, ease: EASE_OUT }}
    >
    <Card className="overflow-hidden">
      <div className="flex items-start gap-3 px-4 py-3">
        <div className="mt-0.5 flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-white/[0.04] text-slate-400">
          <FileText className="h-4 w-4" />
        </div>
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            {editing ? (
              <div className="flex w-full items-center gap-1.5">
                <Input
                  autoFocus
                  value={filename}
                  onChange={(e) => onFilenameChange(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') onEdit(false)
                    if (e.key === 'Escape') onEdit(false)
                  }}
                  className="h-7 text-xs"
                />
                <Button size="icon" variant="ghost" onClick={() => onEdit(false)} aria-label={t('common.done')}>
                  <X className="h-3.5 w-3.5" />
                </Button>
              </div>
            ) : (
              <>
                <p className="truncate text-xs font-medium text-slate-200">{filename || metadata.filename}</p>
                <button
                  type="button"
                  onClick={() => onEdit(true)}
                  className="shrink-0 text-slate-500 transition-colors hover:text-slate-300"
                  aria-label={t('stream.editName')}
                >
                  <Pencil className="h-3 w-3" />
                </button>
              </>
            )}
          </div>
          <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px] text-slate-500">
            <span className="numeric">{formatBytes(metadata.size_bytes, i18n.language)}</span>
            <span className="truncate">{metadata.content_type || t('common.notSet')}</span>
            <Badge tone={metadata.supports_range ? 'success' : 'warn'}>
              {metadata.supports_range ? 'Range' : t('stream.noResume')}
            </Badge>
            <Badge tone="muted">{metadata.engine_hint}</Badge>
          </div>
        </div>
      </div>

      {metadata.supports_range === false ? (
        <div className="flex items-center gap-2 border-t border-amber-400/15 bg-amber-400/[0.05] px-4 py-2 text-[11px] text-amber-200">
          <RotateCcw className="h-3.5 w-3.5" />
          {t('stream.noResume')}
        </div>
      ) : null}

      {htmlBlocked ? (
        <div className="flex items-start gap-2 border-t border-red-400/20 bg-red-500/[0.06] px-4 py-2.5 text-[11px] leading-relaxed text-red-200">
          <ShieldAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
          <div>
            <p className="font-medium">{t('stream.htmlTitle')}</p>
            <p className="mt-0.5 text-red-200/70">{t('stream.htmlBody')}</p>
          </div>
        </div>
      ) : null}

      {metadata.is_html && !htmlBlocked ? (
        <div className="flex items-center gap-2 border-t border-white/[0.06] px-4 py-2 text-[11px] text-slate-400">
          <Ruler className="h-3.5 w-3.5" />
          {t('stream.checksumNote')}
        </div>
      ) : null}
    </Card>
    </motion.div>
  )
}

import { useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { ChevronDown, Terminal } from 'lucide-react'
import type { LogLine } from '@/types/api'
import { Button, Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/primitives'
import { cn } from '@/lib/utils'

/** TerminalLog is the per-task diagnostics stream with auto-follow. */
export function TerminalLog({
  lines,
  open,
  onOpenChange,
  height = 'h-32',
}: {
  lines: LogLine[]
  open: boolean
  onOpenChange: (open: boolean) => void
  height?: string
}) {
  const { t } = useTranslation()
  const end = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (open) end.current?.scrollIntoView({ block: 'end' })
  }, [lines.length, open])

  return (
    <Collapsible open={open} onOpenChange={onOpenChange}>
      <CollapsibleTrigger asChild>
        <Button variant="ghost" size="sm" className="w-full justify-between px-0 text-slate-500 hover:text-slate-300">
          <span className="flex items-center gap-1.5">
            <Terminal className="h-3.5 w-3.5" />
            {t('pipeline.log')}
            <span className="numeric text-[10px] text-slate-600">{lines.length}</span>
          </span>
          <ChevronDown className={cn('h-3.5 w-3.5 transition-transform', open && 'rotate-180')} />
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div
          className={cn(
            'scroll-contain mt-1.5 overflow-y-auto rounded-lg border border-white/[0.06] bg-black/50 p-2.5 font-mono text-[10px] leading-relaxed',
            height,
          )}
        >
          {lines.length === 0 ? (
            <p className="text-slate-600">—</p>
          ) : (
            lines.map((line, index) => (
              <div key={`${line.ts}-${index}`} className="flex gap-2">
                <span className="shrink-0 text-slate-600">{formatTime(line.ts)}</span>
                <span
                  className={cn(
                    'shrink-0 uppercase',
                    line.level === 'error'
                      ? 'text-red-400'
                      : line.level === 'warn'
                        ? 'text-amber-400'
                        : 'text-slate-500',
                  )}
                >
                  {line.source}
                </span>
                <span className="break-all text-slate-300">{line.message}</span>
              </div>
            ))
          )}
          <div ref={end} />
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}

function formatTime(ts: string): string {
  const date = new Date(ts)
  if (Number.isNaN(date.getTime())) return '--:--:--'
  return date.toLocaleTimeString(undefined, { hour12: false })
}

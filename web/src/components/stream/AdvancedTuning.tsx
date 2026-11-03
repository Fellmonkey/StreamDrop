import { useTranslation } from 'react-i18next'
import { ChevronDown, Gauge, Shield, SlidersHorizontal } from 'lucide-react'
import type { AuthProfile } from '@/types/api'
import {
  Badge,
  Button,
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
  Input,
  Label,
  Select,
  Slider,
} from '@/components/ui/primitives'

export const BUFFER_SIZES = [32, 64, 128, 256, 512, 1024] as const

export interface TuningValues {
  mode: 'ring_buffer' | 'pure_stream'
  bufferMiB: number
  engine: '' | 'curl' | 'yt-dlp'
  authProfileId: string
  bandwidthDownMbps: number
  bandwidthUpMbps: number
}

interface AdvancedTuningProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  values: TuningValues
  onChange: (patch: Partial<TuningValues>) => void
  profiles: AuthProfile[]
  onManageProfiles: () => void
}

/** AdvancedTuning exposes the engine, buffer, auth profile and rate limits. */
export function AdvancedTuning({
  open,
  onOpenChange,
  values,
  onChange,
  profiles,
  onManageProfiles,
}: AdvancedTuningProps) {
  const { t } = useTranslation()

  return (
    <Collapsible open={open} onOpenChange={onOpenChange}>
      <CollapsibleTrigger asChild>
        <Button variant="ghost" size="sm" className="w-full justify-between px-1 text-slate-400">
          <span className="flex items-center gap-2">
            <SlidersHorizontal className="h-3.5 w-3.5" />
            {t('stream.advanced')}
          </span>
          <ChevronDown className={`h-3.5 w-3.5 transition-transform ${open ? 'rotate-180' : ''}`} />
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="space-y-4 pt-3">
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-1.5">
            <Label>{t('stream.mode')}</Label>
            <div className="flex rounded-lg border border-white/10 bg-black/30 p-1">
              {(['ring_buffer', 'pure_stream'] as const).map((mode) => (
                <button
                  key={mode}
                  type="button"
                  onClick={() => onChange({ mode })}
                  className={`flex-1 rounded-md px-2 py-1.5 text-[11px] font-medium transition-colors ${
                    values.mode === mode ? 'bg-white/[0.08] text-slate-100' : 'text-slate-500 hover:text-slate-300'
                  }`}
                >
                  {mode === 'ring_buffer' ? t('stream.modeRing') : t('stream.modePure')}
                </button>
              ))}
            </div>
          </div>

          <div className="space-y-1.5">
            <Label>{t('stream.engine')}</Label>
            <Select
              value={values.engine}
              onChange={(e) => onChange({ engine: e.target.value as TuningValues['engine'] })}
            >
              <option value="">{t('stream.engineAuto')}</option>
              <option value="curl">curl</option>
              <option value="yt-dlp">yt-dlp</option>
            </Select>
          </div>
        </div>

        <div className={`space-y-2 ${values.mode === 'pure_stream' ? 'opacity-50' : ''}`}>
          <div className="flex items-center justify-between">
            <Label>
              {t('stream.buffer')} — {values.bufferMiB} MiB
            </Label>
            {values.mode === 'pure_stream' ? <Badge tone="muted">{t('stream.modePure')}</Badge> : null}
          </div>
          <Slider
            min={0}
            max={BUFFER_SIZES.length - 1}
            value={Math.max(0, BUFFER_SIZES.indexOf(values.bufferMiB as (typeof BUFFER_SIZES)[number]))}
            onValueChange={(index) => onChange({ bufferMiB: BUFFER_SIZES[index] })}
          />
          <div className="flex justify-between text-[10px] text-slate-600">
            {BUFFER_SIZES.map((size) => (
              <span key={size}>{size}</span>
            ))}
          </div>
        </div>

        <div className="space-y-1.5">
          <div className="flex items-center justify-between">
            <Label>{t('stream.auth')}</Label>
            <button
              type="button"
              onClick={onManageProfiles}
              className="flex items-center gap-1 text-[11px] text-sky-300 hover:text-sky-200"
            >
              <Shield className="h-3 w-3" />
              {t('stream.authManage')}
            </button>
          </div>
          <Select value={values.authProfileId} onChange={(e) => onChange({ authProfileId: e.target.value })}>
            <option value="">{t('stream.authNone')}</option>
            {profiles.map((profile) => (
              <option key={profile.id} value={profile.id}>
                {profile.name}
                {profile.supported ? '' : ' ⚠'}
              </option>
            ))}
          </Select>
        </div>

        <div className="space-y-2">
          <Label className="flex items-center gap-1.5">
            <Gauge className="h-3 w-3" />
            {t('stream.limits')}
          </Label>
          <div className="grid grid-cols-2 gap-3">
            <div>
              <Input
                type="number"
                min={0}
                value={values.bandwidthDownMbps}
                onChange={(e) => onChange({ bandwidthDownMbps: Math.max(0, Number(e.target.value) || 0) })}
                className="h-8 text-xs"
              />
              <p className="mt-1 text-[10px] text-slate-600">{t('stream.limitDown')}</p>
            </div>
            <div>
              <Input
                type="number"
                min={0}
                value={values.bandwidthUpMbps}
                onChange={(e) => onChange({ bandwidthUpMbps: Math.max(0, Number(e.target.value) || 0) })}
                className="h-8 text-xs"
              />
              <p className="mt-1 text-[10px] text-slate-600">{t('stream.limitUp')}</p>
            </div>
          </div>
          <p className="text-[10px] text-slate-600">{t('stream.noLimit')}</p>
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}

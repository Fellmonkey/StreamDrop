import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Check, Languages } from 'lucide-react'
import { api } from '@/lib/api'
import { setLocale, type Locale } from '@/i18n'
import {
  Button,
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/primitives'
import { cn } from '@/lib/utils'

/** LanguageSwitcher persists the locale locally and in settings. */
export function LanguageSwitcher({ compact }: { compact?: boolean }) {
  const { t, i18n } = useTranslation()
  const queryClient = useQueryClient()
  const [pending, setPending] = useState<Locale | null>(null)

  const mutation = useMutation({
    mutationFn: (locale: Locale) => api.updateSettings({ locale }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['settings'] })
    },
  })

  const choose = (locale: Locale) => {
    setPending(locale)
    void setLocale(locale).then(() => {
      mutation.mutate(locale, { onSettled: () => setPending(null) })
    })
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size={compact ? 'icon' : 'sm'} aria-label={t('header.language')}>
          <Languages className="h-4 w-4" />
          {!compact && <span className="uppercase">{i18n.language}</span>}
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent>
        {(['ru', 'en'] as Locale[]).map((locale) => (
          <DropdownMenuItem key={locale} onSelect={() => choose(locale)}>
            <span className="flex w-full items-center justify-between gap-3">
              <span>{locale === 'ru' ? 'Русский' : 'English'}</span>
              <Check
                className={cn(
                  'h-3.5 w-3.5 text-sky-300',
                  (pending ?? i18n.language) === locale ? 'opacity-100' : 'opacity-0',
                )}
              />
            </span>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

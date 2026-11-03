import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import ru from './locales/ru.json'
import en from './locales/en.json'

export type Locale = 'ru' | 'en'

/** resolveLocale follows saved choice -> browser language -> ru. */
export function resolveLocale(): Locale {
  const saved = localStorage.getItem('streamdrop.locale')
  if (saved === 'ru' || saved === 'en') return saved
  const nav = navigator.language.toLowerCase()
  if (nav.startsWith('ru')) return 'ru'
  if (nav.startsWith('en')) return 'en'
  return 'ru'
}

void i18n.use(initReactI18next).init({
  resources: {
    ru: { translation: ru },
    en: { translation: en },
  },
  lng: resolveLocale(),
  fallbackLng: 'ru',
  interpolation: { escapeValue: false },
  returnNull: false,
})

document.documentElement.lang = i18n.language

export async function setLocale(locale: Locale) {
  await i18n.changeLanguage(locale)
  localStorage.setItem('streamdrop.locale', locale)
  document.documentElement.lang = locale
}

export default i18n

/** Minimal theme + accent helpers: choices live in localStorage. */

export type Theme = 'dark' | 'light'
export type Accent = 'sky' | 'amber' | 'emerald' | 'violet'

const THEME_KEY = 'streamdrop.theme'
const ACCENT_KEY = 'streamdrop.accent'

export function readTheme(): Theme {
  try {
    return localStorage.getItem(THEME_KEY) === 'light' ? 'light' : 'dark'
  } catch {
    return 'dark'
  }
}

export function applyTheme(theme: Theme): void {
  document.documentElement.classList.toggle('dark', theme === 'dark')
  try {
    localStorage.setItem(THEME_KEY, theme)
  } catch {
    /* private mode */
  }
}

export function readAccent(): Accent {
  try {
    const v = localStorage.getItem(ACCENT_KEY)
    return v === 'amber' || v === 'emerald' || v === 'violet' ? v : 'sky'
  } catch {
    return 'sky'
  }
}

export function applyAccent(accent: Accent): void {
  if (accent === 'sky') {
    document.documentElement.removeAttribute('data-accent')
  } else {
    document.documentElement.setAttribute('data-accent', accent)
  }
  try {
    localStorage.setItem(ACCENT_KEY, accent)
  } catch {
    /* private mode */
  }
}

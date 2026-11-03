/** Minimal theme helper: dark is the default; choice lives in localStorage. */

export type Theme = 'dark' | 'light'

const KEY = 'streamdrop.theme'

export function readTheme(): Theme {
  try {
    return localStorage.getItem(KEY) === 'light' ? 'light' : 'dark'
  } catch {
    return 'dark'
  }
}

export function applyTheme(theme: Theme): void {
  document.documentElement.classList.toggle('dark', theme === 'dark')
  try {
    localStorage.setItem(KEY, theme)
  } catch {
    /* private mode */
  }
}

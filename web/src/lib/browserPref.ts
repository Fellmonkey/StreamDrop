/** Preferred browser for OAuth / external opening; stored locally. */

export type BrowserChoice = 'default' | 'edge' | 'chrome' | 'firefox'

const KEY = 'streamdrop.browser'

export function readBrowser(): BrowserChoice {
  try {
    const v = localStorage.getItem(KEY)
    return v === 'edge' || v === 'chrome' || v === 'firefox' ? v : 'default'
  } catch {
    return 'default'
  }
}

export function applyBrowser(choice: BrowserChoice): void {
  try {
    localStorage.setItem(KEY, choice)
  } catch {
    /* private mode */
  }
}

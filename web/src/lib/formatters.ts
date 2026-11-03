const UNITS_EN = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
const UNITS_RU = ['Б', 'КБ', 'МБ', 'ГБ', 'ТБ', 'ПБ']

function units(locale: string): string[] {
  return locale.startsWith('ru') ? UNITS_RU : UNITS_EN
}

/** formatBytes formats a byte count with binary divisors (1024^3), matching
 * the server wear model, and localizes the decimal separator. */
export function formatBytes(bytes: number, locale = 'ru', digits = 2): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 B'
  const list = units(locale)
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < list.length - 1) {
    value /= 1024
    unit++
  }
  const fraction = value >= 100 ? 0 : value >= 10 ? 1 : digits
  const formatted = new Intl.NumberFormat(locale, {
    minimumFractionDigits: 0,
    maximumFractionDigits: fraction,
  }).format(value)
  return `${formatted} ${list[unit]}`
}

/** formatBytesPerSecond shows a transfer speed. */
export function formatSpeed(bps: number, locale = 'ru'): string {
  if (!Number.isFinite(bps) || bps <= 0) return '0 B/s'
  return `${formatBytes(bps, locale, 1)}/s`
}

/** formatDuration renders seconds as a compact timer. */
export function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return '—'
  const s = Math.round(seconds)
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  const rest = s % 60
  if (m < 60) return `${m}m ${rest.toString().padStart(2, '0')}s`
  const h = Math.floor(m / 60)
  return `${h}h ${(m % 60).toString().padStart(2, '0')}m`
}

/** formatEta is an alias with a dash for unknown values. */
export function formatEta(seconds: number): string {
  if (!seconds || seconds <= 0) return '—'
  return formatDuration(seconds)
}

/** formatDate localizes an RFC3339 timestamp. */
export function formatDate(value?: string | null, locale = 'ru'): string {
  if (!value) return '—'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return '—'
  return new Intl.DateTimeFormat(locale, {
    dateStyle: 'short',
    timeStyle: 'short',
  }).format(d)
}

/** formatPercent renders a percentage with fixed precision. */
export function formatPercent(value: number, digits = 4): string {
  if (!Number.isFinite(value)) return '0%'
  return `${value.toFixed(digits)}%`
}

/** shortHash abbreviates a SHA-256 digest for dense tables. */
export function shortHash(hash?: string): string {
  if (!hash) return '—'
  return `${hash.slice(0, 8)}…${hash.slice(-6)}`
}

/** pct computes a clamped percentage. */
export function pct(part: number, total: number): number {
  if (!total || total <= 0) return 0
  return Math.max(0, Math.min(100, (part / total) * 100))
}

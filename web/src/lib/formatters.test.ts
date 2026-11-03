import { describe, expect, it } from 'vitest'
import { formatBytes, formatDate, formatDuration, formatSpeed, pct, shortHash } from '@/lib/formatters'

describe('formatters', () => {
  it('formats bytes in base-1024 with locale units', () => {
    expect(formatBytes(0, 'ru')).toBe('0 B')
    expect(formatBytes(1024, 'ru')).toBe('1 КБ')
    expect(formatBytes(1024 ** 3, 'en')).toBe('1 GB')
    expect(formatBytes(1480 * 1024 ** 3, 'ru')).toContain('ТБ')
  })

  it('computes percentages with clamping', () => {
    expect(pct(50, 200)).toBe(25)
    expect(pct(-5, 100)).toBe(0)
    expect(pct(150, 100)).toBe(100)
    expect(pct(0, 0)).toBe(0)
  })

  it('formats speed and duration', () => {
    expect(formatSpeed(42_100_000, 'en')).toContain('MB/s')
    expect(formatDuration(26)).toBe('26s')
    expect(formatDuration(130)).toContain('m')
  })

  it('formats dates and short hashes', () => {
    expect(shortHash('')).toBe('—')
    expect(shortHash('a'.repeat(64))).toHaveLength(15)
    expect(() => formatDate('2026-11-03T10:00:00Z', 'ru')).not.toThrow()
  })
})

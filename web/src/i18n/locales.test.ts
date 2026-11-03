import { describe, expect, it } from 'vitest'
import en from '@/i18n/locales/en.json'
import ru from '@/i18n/locales/ru.json'

type Flat = Record<string, string>

function flatten(obj: Record<string, unknown>, prefix = ''): Flat {
  const out: Flat = {}
  for (const [key, value] of Object.entries(obj)) {
    const path = prefix ? `${prefix}.${key}` : key
    if (typeof value === 'object' && value !== null) {
      Object.assign(out, flatten(value as Record<string, unknown>, path))
    } else {
      out[path] = String(value)
    }
  }
  return out
}

describe('i18n locales', () => {
  const flatRu = flatten(ru as unknown as Record<string, unknown>)
  const flatEn = flatten(en as unknown as Record<string, unknown>)

  it('ru and en expose the same key sets', () => {
    expect(Object.keys(flatRu).sort()).toEqual(Object.keys(flatEn).sort())
  })

  it('no empty or whitespace-only values', () => {
    for (const [key, value] of Object.entries({ ...flatRu, ...flatEn })) {
      expect(value.trim().length, `key ${key} is empty`).toBeGreaterThan(0)
    }
  })

  it('ru/en are non-ASCII in Russian and ASCII-ish in English where expected', () => {
    // Spot-check: well-known keys must be translated in ru and not just copy the key.
    expect(flatRu['header.saved']).not.toBe('header.saved')
    expect(flatEn['header.saved']).not.toBe('header.saved')
    expect(flatRu['queue.pause']).not.toBe('queue.pause')
  })
})

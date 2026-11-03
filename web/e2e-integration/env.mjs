import { readFileSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const here = path.dirname(fileURLToPath(import.meta.url))
const root = path.resolve(here, '..', '..')

/**
 * loadEnv reads the repository-root .env (MAILRU_USER / MAILRU_PASS /
 * MAILRU_URL) plus overrides from the process environment.
 */
export function loadEnv() {
  let user = ''
  let pass = ''
  let url = 'https://webdav.cloud.mail.ru'
  try {
    const text = readFileSync(path.join(root, '.env'), 'utf8')
    for (const line of text.split(/\r?\n/)) {
      const m = line.match(/^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*)$/)
      if (!m) continue
      const key = m[1]
      const value = m[2].trim().replace(/^["']|["']$/g, '')
      if (key === 'MAILRU_USER') user = value
      else if (key === 'MAILRU_PASS') pass = value
      else if (key === 'MAILRU_URL') url = value || url
    }
  } catch {
    /* no .env — integration tests will be skipped */
  }
  return {
    ok: Boolean(user && pass),
    user,
    pass,
    url,
    root,
    base: process.env.STREAMDROP_URL || 'http://127.0.0.1:8085',
    fixturePort: Number(process.env.FIXTURE_PORT || 8792),
  }
}

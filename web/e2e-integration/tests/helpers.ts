import { expect, type Page } from '@playwright/test'
import { Api } from '../api'

export const ISO = 'ubuntu-24.04-live-server.iso'

export function isoUrl(env: { fixturePort: number }) {
  return `http://127.0.0.1:${env.fixturePort}/iso/${ISO}`
}
export function htmlUrl(env: { fixturePort: number }) {
  return `http://127.0.0.1:${env.fixturePort}/page/download-page.html`
}
export function zipUrl(env: { fixturePort: number }) {
  return `http://127.0.0.1:${env.fixturePort}/assets/notes.zip`
}
export function jsonUrl(env: { fixturePort: number }) {
  return `http://127.0.0.1:${env.fixturePort}/assets/data.json`
}

export async function makeApi(env: { base: string }) {
  const api = new Api(env.base)
  await api.init()
  return api
}

export async function openApp(page: Page, env: { base: string }) {
  const api = await makeApi(env)
  await api.putSettings({ locale: 'ru', onboarded: true })
  await page.addInitScript(() => localStorage.setItem('streamdrop.locale', 'ru'))
  await page.goto('/')
  await expect(page.locator('main')).toBeVisible()
  await expect(page.getByText('StreamDrop').first()).toBeVisible()
  return api
}

export function activePipeline(page: Page) {
  return page.locator('section').filter({ hasText: 'Активные трансляции' })
}

export function queueCard(page: Page) {
  return page.locator('div').filter({ has: page.getByText('Очередь', { exact: true }) }).last()
}

export async function waitStatus(page: Page, statusRu: string, timeout = 15_000) {
  await expect(page.getByText(statusRu, { exact: true }).first()).toBeVisible({ timeout })
}

/**
 * cleanupTransfers aborts active jobs and removes all transfers so each cloud
 * suite starts from a clean slate.
 */
export async function cleanupTransfers(api: Api) {
  const list = await api.transfers()
  for (const t of list) {
    if (t.status === 'running' || t.status === 'retrying' || t.status === 'queued') {
      try {
        await api.transferAction(t.id, 'abort')
      } catch {
        /* already gone */
      }
    }
    try {
      await api.deleteTransfer(t.id)
    } catch {
      /* already gone */
    }
  }
}

/**
 * cloudReady probes the real cloud once (explorer root list). Mail.ru and
 * friends occasionally return upstream HTTP 500 for minutes at a time; the
 * cloud-dependent suites skip with a clear message instead of flaking.
 */
export async function cloudReady(env: { base: string; user: string; pass: string; url: string }) {
  try {
    const { ensureRemote } = await import('../api')
    const remote = await ensureRemote(env)
    const api = await makeApi(env)
    await api.explorer(remote.id, '/')
    return true
  } catch {
    return false
  }
}

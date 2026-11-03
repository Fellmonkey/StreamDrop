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

/**
 * findRclone resolves the local rclone binary used by the app.
 */
function findRclone(): string | null {
  const candidates = [
    process.env.RCLONE_BIN,
    process.env.LOCALAPPDATA
      ? `${process.env.LOCALAPPDATA}\\StreamDrop\\deps\\rclone\\v1.68.2\\rclone.exe`
      : '',
    'C:\\Users\\nikita\\AppData\\Local\\StreamDrop\\deps\\rclone\\v1.68.2\\rclone.exe',
  ].filter(Boolean)
  for (const c of candidates) {
    try {
      if (require('node:fs').existsSync(c)) return c
    } catch {
      /* ignore */
    }
  }
  return null
}

/**
 * removeCloudFolders deletes test-created folders from the cloud via the
 * rclone CLI (the app intentionally exposes no folder-delete API). Folders
 * must be empty, as they are after the explorer suite's own checks.
 */
export function removeCloudFolders(
  env: { user: string; pass: string; url: string },
  folders: string[],
): void {
  const rclone = findRclone()
  if (!rclone || folders.length === 0) return
  const fs = require('node:fs')
  const os = require('node:os')
  const path = require('node:path')
  const cp = require('node:child_process')

  const cfg = path.join(os.tmpdir(), `sd-clean-${Date.now()}.conf`)
  const create = cp.spawnSync(rclone, ['config', 'create', '--config', cfg, 'probe', 'webdav', 'url=' + env.url, 'vendor=other', 'user=' + env.user, 'pass=' + env.pass], { encoding: 'utf8' })
  if (create.status !== 0) return
  for (const folder of folders) {
    cp.spawnSync(rclone, ['rmdir', '--config', cfg, `probe:/${folder}`], { encoding: 'utf8' })
  }
  try {
    fs.rmSync(cfg, { force: true })
  } catch {
    /* ignore */
  }
}

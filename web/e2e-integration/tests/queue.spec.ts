import { test, expect } from '@playwright/test'
import { loadEnv } from '../env.mjs'
import { ensureRemote } from '../api'
import { cloudReady, jsonUrl, makeApi, openApp, zipUrl } from './helpers'

const env = loadEnv()

test.describe('Queue management', () => {
  test.beforeEach(async () => {
    if (!(await cloudReady(env))) test.skip('Mail.ru upstream unavailable (HTTP 500) — retry later')
  })

  test('pause/resume queue, parallel limit, reorder persistence', async ({ page }) => {
    const api = await makeApi(env)
    const remote = await ensureRemote(env)
    await api.putSettings({ locale: 'ru', parallel_limit: 1 })

    // seed two pending jobs (pure stream, small)
    await api.createTransfers({
      urls: [zipUrl(env), jsonUrl(env)],
      remote_id: remote.id,
      remote_path: '/',
      mode: 'pure_stream',
      buffer_size_bytes: 0,
      engine: '',
      auth_profile_id: '',
      file_size: 0,
    })

    await openApp(page, env)

    const before = (await api.queue()).items.map((i: any) => i.id)
    expect(before.length).toBeGreaterThanOrEqual(2)

    // reorder: move first item down -> order flips
    const firstRow = page.locator('li').filter({ hasText: 'notes.zip' }).first()
    await firstRow.getByRole('button', { name: 'Ниже' }).click()
    await expect.poll(async () => (await api.queue()).items.map((i: any) => i.id)).not.toEqual(before)

    // parallel limit select persists
    await page.getByText('Параллельно').locator('..').locator('select').selectOption('2')
    await expect.poll(async () => (await api.settings()).parallel_limit).toBe('2')

    // queue pause / resume
    await page.getByRole('button', { name: 'Пауза очереди' }).click()
    await expect(page.getByText('Очередь на паузе')).toBeVisible()
    await page.getByRole('button', { name: 'Возобновить' }).last().click()
    await expect(page.getByText('Очередь на паузе')).toHaveCount(0)

    await api.putSettings({ parallel_limit: 1 })
  })
})

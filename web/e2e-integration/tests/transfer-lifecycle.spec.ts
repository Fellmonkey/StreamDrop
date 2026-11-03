import { test, expect } from '@playwright/test'
import { loadEnv } from '../env.mjs'
import { ensureRemote } from '../api'
import { activePipeline, cloudReady, isoUrl, makeApi, openApp, waitStatus } from './helpers'

const env = loadEnv()

test.describe('Transfer lifecycle (real Mail.ru)', () => {
  test.beforeEach(async () => {
    if (!(await cloudReady(env))) test.skip('Mail.ru upstream unavailable (HTTP 500) — retry later')
  })

  test('ISO transfer completes; history shows SHA-256', async ({ page }) => {
    const api = await makeApi(env)
    const remote = await ensureRemote(env)
    await api.putSettings({ locale: 'ru', checksum_enabled: true })

    const src = isoUrl(env)
    await api.createTransfers({
      urls: [src],
      remote_id: remote.id,
      remote_path: '/',
      mode: 'ring_buffer',
      buffer_size_bytes: 64 * 1024 * 1024,
      engine: '',
      auth_profile_id: '',
      bandwidth_down_bps: 0,
      bandwidth_up_bps: 0,
      file_size: 0,
    })

    await openApp(page, env)

    // active card visible
    await expect(activePipeline(page).getByText('ubuntu-24.04-live-server.iso').first()).toBeVisible({ timeout: 20_000 })
    await expect(activePipeline(page).getByText('Вход').first()).toBeVisible()
    await expect(activePipeline(page).getByText('Выход').first()).toBeVisible()

    // wait completion
    await expect
      .poll(
        async () => {
          const list = await api.transfers()
          const t = list.find((x) => x.source_url === src && x.status === 'completed')
          return t ? t.status : null
        },
        { timeout: 240_000, intervals: [3_000] },
      )
      .toBe('completed')

    // history row + details dialog with SHA-256
    const row = page.locator('tr').filter({ hasText: 'ubuntu-24.04-live-server.iso' })
    await expect(row).toBeVisible()
    await row.getByRole('button', { name: 'Действия' }).click()
    await page.getByText('Детали задачи').click()
    await expect(page.getByText('SHA-256')).toBeVisible()
    const hex = await page
      .locator('p')
      .filter({ hasText: 'SHA-256' })
      .locator('xpath=following-sibling::p[1]')
      .textContent()
    expect(hex?.trim()).toMatch(/^[0-9a-f]{64}$/)
    await page.keyboard.press('Escape')
  })

  test('pause / resume / abort on a throttled transfer', async ({ page }) => {
    const api = await makeApi(env)
    const remote = await ensureRemote(env)
    await api.putSettings({ locale: 'ru' })

    const src = isoUrl(env)
    await api.createTransfers({
      urls: [src],
      remote_id: remote.id,
      remote_path: '/',
      mode: 'ring_buffer',
      buffer_size_bytes: 32 * 1024 * 1024,
      engine: '',
      auth_profile_id: '',
      bandwidth_down_bps: 3_000_000,
      bandwidth_up_bps: 3_000_000,
      file_size: 0,
    })

    await openApp(page, env)
    const section = activePipeline(page)

    // Mail.ru is flaky: accept early terminal state as an upstream issue.
    await expect
      .poll(
        async () => {
          const statuses = await section.locator('span').allTextContents()
          return statuses.some((s) => ['Передача', 'Пауза', 'Прервано', 'Ошибка'].includes(s.trim()))
            ? true
            : null
        },
        { timeout: 40_000, intervals: [1_000] },
      )
      .not.toBeNull()
    if (await page.getByText('Ошибка', { exact: true }).first().isVisible().catch(() => false)) {
      test.skip('Mail.ru upstream rejected the transfer (HTTP 500) — retry later')
    }

    await section.getByRole('button', { name: 'Пауза' }).first().click()
    await waitStatus(page, 'Пауза')
    await section.getByRole('button', { name: 'Возобновить' }).first().click()
    await waitStatus(page, 'Передача')
    await section.getByRole('button', { name: 'Отмена' }).first().click()
    await waitStatus(page, 'Прервано', 25_000)
  })
})

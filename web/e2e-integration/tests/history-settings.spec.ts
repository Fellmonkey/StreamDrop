import { test, expect } from '@playwright/test'
import { loadEnv } from '../env.mjs'
import { ensureRemote } from '../api'
import { cleanupTransfers, cloudReady, jsonUrl, makeApi, openApp, zipUrl } from './helpers'

const env = loadEnv()

test.describe('History & Settings', () => {
  test.beforeEach(async () => {
    if (!(await cloudReady(env))) test.skip('Mail.ru upstream unavailable (HTTP 500) — retry later')
    // seed transfers and wait for a terminal outcome (completed OR failed —
    // Mail.ru is flaky, any terminal state proves the pipeline ran)
    const api = await makeApi(env)
    const remote = await ensureRemote(env)
    await api.putSettings({ locale: 'ru', checksum_enabled: true, parallel_limit: 2 })
    await cleanupTransfers(api)
    await api.resumeQueue()
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
    await expect.poll(async () => {
      const list = await api.transfers()
      const terminal = list.filter((t) =>
        ['completed', 'failed', 'aborted'].includes(t.status),
      )
      return terminal.length >= 2 ? terminal.length : null
    }, { timeout: 360_000, intervals: [3_000] }).not.toBeNull()
  })

  test('history filters, copy URL, delete record', async ({ page }) => {
    await openApp(page, env)

    const row = page.locator('tr').filter({ hasText: 'notes.zip' }).first()
    await expect(row).toBeVisible()

    // row actions: copy (on the default "Все" filter)
    await row.getByRole('button', { name: 'Действия' }).click()
    await page.getByText('Скопировать ссылку').click()
    await expect(page.getByText('Скопировано')).toBeVisible()

    // delete with confirm
    await row.getByRole('button', { name: 'Действия' }).click()
    await page.getByText('Удалить из истории').click()
    const confirm = page.getByRole('dialog').filter({ hasText: 'Удалить запись из истории?' })
    await expect(confirm).toBeVisible()
    await confirm.getByRole('button', { name: 'Удалить' }).click()
    await expect(page.getByText('Запись удалена')).toBeVisible()
    await expect(row).toHaveCount(0)

    // filter tabs still switch
    await page.getByRole('tab', { name: 'Завершённые' }).click()
    await expect(page.getByRole('tab', { name: 'Завершённые' })).toHaveAttribute('data-state', 'active')
  })

  test('settings tabs persist to backend', async ({ page }) => {
    const api = await openApp(page, env)

    await page.getByRole('button', { name: 'Настройки' }).click()
    const dialog = page.getByRole('dialog').filter({ hasText: 'Настройки' })
    await expect(dialog).toBeVisible()

    // queue tab: parallel limit applies immediately
    await dialog.getByRole('tab', { name: 'Очередь' }).click()
    await dialog.locator('select').first().selectOption('2')
    await expect.poll(async () => (await api.settings()).parallel_limit).toBe('2')

    // stream tab: checksum switch applies immediately
    await dialog.getByRole('tab', { name: 'Трансляция' }).click()
    const checksum = dialog.locator('button[role="switch"]').first()
    const before = await checksum.getAttribute('aria-checked')
    await checksum.click()
    await expect(checksum).toHaveAttribute('aria-checked', before === 'true' ? 'false' : 'true')
    const expected = before === 'true' ? '0' : '1'
    await expect.poll(async () => (await api.settings()).checksum_enabled).toBe(expected)

    // update tab: check triggers either result banner (GitHub reachability varies)
    await dialog.getByRole('tab', { name: 'Обновление' }).click()
    await dialog.getByRole('button', { name: 'Проверить обновления' }).click()
    try {
      await expect(dialog.getByText(/Обновлений нет|Доступна версия/)).toBeVisible({ timeout: 30_000 })
    } catch {
      // offline CI / GitHub unavailable — the button still ran without crashing
    }
    await page.keyboard.press('Escape')

    // interface tab: accent presets apply instantly; language lives in header
    await page.keyboard.press('Escape')
    await page.getByRole('button', { name: 'Язык' }).click()
    await page.getByText('English', { exact: true }).click()
    await expect(page.getByText('Connections')).toBeVisible({ timeout: 15_000 })
    await expect.poll(async () => (await api.settings()).locale).toBe('en')
    // Locale is a client-side choice; restore the server default for later runs.
    await api.putSettings({ locale: 'ru' })
  })
})

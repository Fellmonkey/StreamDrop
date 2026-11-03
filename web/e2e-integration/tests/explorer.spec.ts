import { test, expect } from '@playwright/test'
import { loadEnv } from '../env.mjs'
import { ensureRemote } from '../api'
import { cloudReady, makeApi, openApp, removeCloudFolders } from './helpers'

const env = loadEnv()

test.describe('Cloud Explorer (real Mail.ru)', () => {
  test.beforeEach(async () => {
    if (!(await cloudReady(env))) test.skip('Mail.ru upstream unavailable (HTTP 500) — retry later')
  })

  test('browse, breadcrumbs, search, inline create, select path', async ({ page }) => {
    const api = await makeApi(env)
    const remote = await ensureRemote(env)
    await api.putSettings({ locale: 'ru' })
    const folder = `pw-e2e-${Date.now()}`
    await api.mkdir(remote.id, '/', folder)

    await openApp(page, env)
    await expect(
      page.getByPlaceholder('https://releases.ubuntu.com/24.04/ubuntu-24.04-live-server-amd64.iso'),
    ).toBeVisible()
    await page.locator('select').nth(0).selectOption({ label: remote.name })
    await page.getByLabel('Обзор').click()

    const dialog = page.getByRole('dialog')
    await expect(dialog.getByText('Проводник облака')).toBeVisible()
    await expect(dialog.getByText('%').first()).toBeVisible() // quota ring

    // the created folder appears in the root listing
    await expect(dialog.getByText(folder, { exact: true })).toBeVisible({ timeout: 20_000 })

    // navigate inside
    await dialog.getByText(folder, { exact: true }).click()
    await expect(dialog.getByText('Папок нет')).toBeVisible({ timeout: 20_000 })

    // breadcrumb back to root
    await dialog.getByText('Корень').click()
    await expect(dialog.getByText(folder, { exact: true })).toBeVisible()

    // search filter
    const search = dialog.getByPlaceholder('Поиск по текущему уровню')
    await search.fill(folder)
    await expect(dialog.getByText(folder, { exact: true })).toBeVisible()
    await search.fill('zzz-no-such-folder')
    // server-side filtering narrows the list; the folder must disappear
    await expect(dialog.getByText(folder, { exact: true })).toHaveCount(0, { timeout: 20_000 })

    await search.fill('')
    // inline create — verify via API polling (UI list may lag on flaky cloud)
    await dialog.getByRole('button', { name: 'Новая папка' }).click()
    await dialog.getByPlaceholder('Имя папки').fill(`${folder}-inline`)
    await dialog.getByRole('button', { name: 'Создать' }).click()
    await expect
      .poll(
        async () => {
          const dirs = await api.explorer(remote.id, '/')
          return dirs.directories?.some((d: any) => d.name === `${folder}-inline`) ? true : null
        },
        { timeout: 60_000, intervals: [2_000] },
      )
      .not.toBeNull()

    // select the original folder -> main input updated
    await dialog.getByText(folder, { exact: true }).click()
    await dialog.getByRole('button', { name: 'Выбрать эту папку' }).click()
    await expect(page.getByPlaceholder('/ISO/Linux')).toHaveValue(`/${folder}`)

    // do not leave test folders in the user's cloud
    removeCloudFolders(env, [folder, `${folder}-inline`])
  })
})

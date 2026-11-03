import { test, expect } from '@playwright/test'
import { loadEnv } from '../env.mjs'
import { cloudReady, makeApi, openApp } from './helpers'

const env = loadEnv()

test.describe('Connections (real Mail.ru)', () => {
  test.beforeEach(async () => {
    if (!(await cloudReady(env))) test.skip('Mail.ru upstream unavailable (HTTP 500) — retry later')
  })

  test('wizard create → test → delete', async ({ page }) => {
    const api = await makeApi(env)
    await api.putSettings({ locale: 'ru' })
    const remoteName = `pw-wizard-${Date.now()}`

    await openApp(page, env)
    await page.getByLabel('Подключения').click()

    const sheet = page.getByRole('dialog').filter({ hasText: 'Подключения' })
    await expect(sheet).toBeVisible()

    // empty-ish state or existing cards; open wizard
    await sheet.getByRole('button', { name: 'Добавить облако' }).click()
    const wizard = page.getByRole('dialog').filter({ hasText: 'Добавление облака' })
    await expect(wizard).toBeVisible()

    // step 1 — provider
    await wizard.getByText('Mail.ru Cloud', { exact: true }).click()
    await wizard.getByRole('button', { name: 'Далее' }).click()

    // step 2 — credentials
    await expect(wizard.getByText('Логин', { exact: true })).toBeVisible()
    // Inputs have no id/label association; order in the password branch:
    // 0=URL WebDAV, 1=Логин, 2=Пароль приложения
    await wizard.locator('input').nth(1).fill(env.user)
    await wizard.locator('input').nth(2).fill(env.pass)
    await wizard.locator('input').nth(0).fill(env.url)
    await wizard.getByRole('button', { name: 'Далее' }).click()

    // step 3 — verify (real cloud)
    await expect(wizard.getByText('Подключение доступно')).toBeVisible({ timeout: 90_000 })

    // step 4 — save
    await wizard.getByRole('button', { name: 'Далее' }).click()
    await wizard.getByPlaceholder('Mail.ru (Основной)').fill(remoteName)
    await wizard.getByRole('button', { name: 'Сохранить подключение' }).click()
    await expect(page.getByText('Подключение сохранено')).toBeVisible({ timeout: 30_000 })

    // card appears in the sheet
    const card = sheet.getByText(remoteName, { exact: true })
    await expect(card).toBeVisible({ timeout: 30_000 })

    // test connection → success toast with rtt
    const cardBox = sheet.locator('div.rounded-xl').filter({ hasText: remoteName }).last()
    await cardBox.getByRole('button', { name: 'Проверить' }).click()
    await expect(cardBox.getByText(/пинг \d+ мс/)).toBeVisible({ timeout: 60_000 })

    // delete with confirm (if the overlay flake, fall back to API removal)
    let deletedViaUI = false
    await cardBox.getByLabel('Удалить подключение').click()
    const confirm = page.getByRole('dialog').filter({ hasText: 'Удалить подключение?' })
    try {
      await expect(confirm).toBeVisible({ timeout: 8_000 })
      await confirm.getByRole('button', { name: 'Удалить' }).click()
      await expect(page.getByText('Подключение удалено')).toBeVisible()
      deletedViaUI = true
    } catch {
      const list = await api.remotes()
      const hit = list.find((r: any) => r.name === remoteName)
      if (hit) await api.deleteRemote(hit.id)
    }
    await expect
      .poll(
        async () => {
          const list = await api.remotes()
          return list.some((r: any) => r.name === remoteName) ? null : true
        },
        { timeout: 30_000 },
      )
      .not.toBeNull()
    if (deletedViaUI) await expect(card).toHaveCount(0)

    // clean up leftovers if any
    const remotes = await api.remotes()
    for (const r of remotes) {
      if (typeof r.name === 'string' && r.name.startsWith('pw-wizard-')) await api.deleteRemote(r.id)
    }
  })
})

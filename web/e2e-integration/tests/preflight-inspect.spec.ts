import { test, expect } from '@playwright/test'
import { loadEnv } from '../env.mjs'
import { ensureRemote } from '../api'
import { htmlUrl, isoUrl, openApp } from './helpers'

const env = loadEnv()

test.describe('Pre-flight inspection', () => {
  test('ISO: metadata card, editable name, Range badge', async ({ page }) => {
    await ensureRemote(env)
    await openApp(page, env)
    const input = page.getByPlaceholder('https://releases.ubuntu.com/24.04/ubuntu-24.04-live-server-amd64.iso')
    await input.fill(isoUrl(env))
    await expect(page.getByText('ubuntu-24.04-live-server.iso')).toBeVisible({ timeout: 25_000 })
    await expect(page.getByText('Range')).toBeVisible()

    await page.getByLabel('Изменить имя').click()
    const nameInput = page.getByRole('textbox').nth(1)
    await nameInput.fill('my-custom-name.iso')
    await page.getByLabel('Готово').click()
    await expect(page.getByText('my-custom-name.iso')).toBeVisible()
  })

  test('HTML source is blocked until yt-dlp is selected', async ({ page }) => {
    const remote = await ensureRemote(env)
    await openApp(page, env)
    const input = page.getByPlaceholder('https://releases.ubuntu.com/24.04/ubuntu-24.04-live-server-amd64.iso')
    await expect(input).toBeVisible()
    await page.locator('select').first().selectOption({ label: remote.name })

    await input.fill(htmlUrl(env))

    await expect(page.getByText('Обнаружена веб-страница скачивания')).toBeVisible({ timeout: 25_000 })
    const start = page.getByRole('button', { name: 'Начать трансляцию' })
    await expect(start).toBeDisabled()

    await page.getByRole('button', { name: 'Дополнительные параметры' }).click()
    await page.locator('select').nth(1).selectOption('yt-dlp')

    await expect(page.getByText('Обнаружена веб-страница скачивания')).toHaveCount(0, { timeout: 25_000 })
    await expect(start).toBeEnabled()
  })

  test('templates dropdown fills the URL field', async ({ page }) => {
    await openApp(page, env)
    await page.getByRole('button', { name: 'Шаблоны источников' }).click()
    await page.getByText('Ubuntu ISO').click()
    await expect(
      page.getByPlaceholder('https://releases.ubuntu.com/24.04/ubuntu-24.04-live-server-amd64.iso'),
    ).toHaveValue(/ubuntu/)
  })
})

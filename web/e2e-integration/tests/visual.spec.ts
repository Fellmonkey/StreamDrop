import { test, expect } from '@playwright/test'
import { loadEnv } from '../env.mjs'
import { openApp } from './helpers'

const env = loadEnv()
const SHOT = { maxDiffPixelRatio: 0.02, animations: 'disabled' }

test.describe('Visual snapshots', () => {
  test.use({ viewport: { width: 1440, height: 900 } })

  async function openLicenses(page: import('@playwright/test').Page, dark: boolean) {
    await openApp(page, env)
    const html = page.locator('html')
    const isDark = (await html.getAttribute('class'))?.includes('dark') ?? false
    if (isDark !== dark) {
      await page.getByRole('button', { name: 'Тёмная тема' }).click()
    }
    await page.getByRole('button', { name: 'Настройки' }).click()
    const dialog = page.getByRole('dialog').filter({ hasText: 'Настройки' })
    await dialog.getByRole('tab', { name: 'Лицензии' }).click()
    await expect(dialog.getByText('rclone').first()).toBeVisible()
    return dialog
  }

  test('licenses — dark theme', async ({ page }) => {
    const dialog = await openLicenses(page, true)
    await expect(dialog).toHaveScreenshot('licenses-dark.png', SHOT)
  })

  test('licenses — light theme', async ({ page }) => {
    const dialog = await openLicenses(page, false)
    await expect(dialog).toHaveScreenshot('licenses-light.png', SHOT)
  })

  test('stream deck — dark theme (inspected ISO)', async ({ page }) => {
    await openApp(page, env)
    const input = page.getByPlaceholder('https://releases.ubuntu.com/24.04/ubuntu-24.04-live-server-amd64.iso')
    await input.fill(`http://127.0.0.1:${env.fixturePort}/iso/ubuntu-24.04-live-server.iso`)
    await expect(page.getByText('ubuntu-24.04-live-server.iso').first()).toBeVisible({ timeout: 25_000 })
    await expect(page.getByText('Range')).toBeVisible()
    const deck = page.locator('main div.rounded-2xl').first()
    await expect(deck).toHaveScreenshot('deck-dark.png', SHOT)
  })
})

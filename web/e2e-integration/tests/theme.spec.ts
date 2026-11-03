import { test, expect } from '@playwright/test'
import { loadEnv } from '../env.mjs'
import { openApp } from './helpers'

const env = loadEnv()

test.describe('Theme', () => {
  test('settings toggle switches between dark and light', async ({ page }) => {
    await openApp(page, env)
    await expect(page.locator('html')).toHaveClass(/dark/)

    await page.getByRole('button', { name: 'Настройки' }).click()
    const dialog = page.getByRole('dialog').filter({ hasText: 'Настройки' })
    await dialog.getByRole('tab', { name: 'Интерфейс' }).click()

    const themeSwitch = dialog.getByRole('switch').first()
    await themeSwitch.click()
    await expect(page.locator('html')).not.toHaveClass(/dark/)

    await themeSwitch.click()
    await expect(page.locator('html')).toHaveClass(/dark/)

    await page.keyboard.press('Escape')
  })
})

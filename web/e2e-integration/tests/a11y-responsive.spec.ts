import { test, expect } from '@playwright/test'
import { loadEnv } from '../env.mjs'
import { openApp } from './helpers'

const env = loadEnv()

test.describe('Accessibility & responsive', () => {
  test('reduced motion: UI stays functional and dialogs still open', async ({ page }) => {
    await page.emulateMedia({ reducedMotion: 'reduce' })
    await openApp(page, env)

    await page.getByLabel('Настройки').click()
    await expect(page.getByRole('dialog').filter({ hasText: 'Настройки' })).toBeVisible()
    await page.keyboard.press('Escape')

    await page.getByLabel('Подключения').click()
    await expect(page.getByRole('dialog').filter({ hasText: 'Подключения' })).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(page.getByRole('dialog').filter({ hasText: 'Подключения' })).toHaveCount(0)
  })

  test('mobile viewport renders main deck without horizontal overflow', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 })
    await openApp(page, env)
    await expect(page.getByText('Потоковая передача')).toBeVisible()
    await expect(page.getByText('Начать трансляцию')).toBeVisible()
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth)
    expect(overflow).toBeLessThanOrEqual(0)
  })
})

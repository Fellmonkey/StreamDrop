import { test, expect } from '@playwright/test'
import { loadEnv } from '../env.mjs'
import { openApp } from './helpers'

const env = loadEnv()

test.describe('Theme', () => {
  test('header toggle switches between dark and light', async ({ page }) => {
    await openApp(page, env)
    await expect(page.locator('html')).toHaveClass(/dark/)

    const toggle = page.getByRole('button', { name: 'Тёмная тема' })
    await toggle.click()
    await expect(page.locator('html')).not.toHaveClass(/dark/)

    await toggle.click()
    await expect(page.locator('html')).toHaveClass(/dark/)
  })
})

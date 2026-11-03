import { test, expect } from '@playwright/test'
import { loadEnv } from '../env.mjs'
import { makeApi, openApp } from './helpers'

const env = loadEnv()

test.describe('Header & onboarding', () => {
  test('saved-resource tooltip, engine status, deps manager', async ({ page }) => {
    const api = await openApp(page, env)

    // engine status
    await expect(page.getByText('Движок: Готов')).toBeVisible({ timeout: 20_000 })

    // tooltip with wear table
    await page.getByText('Сбережено').hover()
    await expect(page.getByText('Сбережённый ресурс накопителя')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByText('Среднее')).toBeVisible()

    // components dialog
    await page.getByLabel('Компоненты').click()
    const dialog = page.getByRole('dialog').filter({ hasText: 'Компоненты' })
    await expect(dialog).toBeVisible()
    await expect(dialog.getByText('rclone', { exact: true }).first()).toBeVisible()
    await expect(dialog.getByText('curl', { exact: true }).first()).toBeVisible()
    await expect(dialog.getByText('yt-dlp', { exact: true }).first()).toBeVisible()
    await page.keyboard.press('Escape')

    expect(api).toBeTruthy()
  })

  test('first-run wizard: steps, back, finish persists onboarded', async ({ page }) => {
    const api = await makeApi(env)
    await api.putSettings({ locale: 'ru', onboarded: false })
    await page.addInitScript(() => localStorage.setItem('streamdrop.locale', 'ru'))
    await page.goto('/')

    await expect(page.getByText('Добро пожаловать в StreamDrop')).toBeVisible({ timeout: 20_000 })
    await expect(page.getByText('Подключите облако')).toBeVisible()

    await page.getByRole('button', { name: 'Далее' }).click()
    await expect(page.getByText('Вставьте ссылку')).toBeVisible()

    await page.getByRole('button', { name: 'Назад' }).click()
    await expect(page.getByText('Подключите облако')).toBeVisible()

    await page.getByRole('button', { name: 'Далее' }).click()
    await page.getByRole('button', { name: 'Далее' }).click()
    await expect(page.getByText('Запустите трансляцию')).toBeVisible()

    await page.getByRole('button', { name: 'Начать работу' }).nth(1).click()
    await expect(page.getByText('Добро пожаловать в StreamDrop')).toHaveCount(0)
    await expect.poll(async () => (await api.settings()).onboarded).toBe('1')
  })
})

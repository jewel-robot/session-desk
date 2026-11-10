import { test, expect } from '@playwright/test'

test.beforeEach(async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.session-title').first()).toBeVisible()
})

test('about dialog shows version, shortcuts and update preference', async ({ page }) => {
  const version = page.getByRole('button', { name: '关于 Session Desk' })
  await expect(version).toContainText(/v\d+\.\d+\.\d+|dev/)
  await page.keyboard.press('F1')
  const dialog = page.getByRole('dialog', { name: 'Session Desk' })
  await expect(dialog).toBeVisible()
  await expect(dialog.locator('.about-version code')).toHaveText(/v\d+\.\d+\.\d+|dev/)
  await expect(dialog.getByRole('heading', { name: '键盘快捷键' })).toBeVisible()
  await expect(dialog.locator('.path-list')).toContainText('设置与缓存')

  // Demo mode never checks automatically, so nothing reaches the network.
  await expect(dialog.getByRole('status')).toHaveText(/尚未检查更新/)
  const toggle = dialog.getByRole('switch', { name: '启动时自动检查更新' })
  await expect(toggle).toBeChecked()
  await dialog.locator('.switch-row').click()
  await expect(toggle).not.toBeChecked()
  await page.reload()
  await page.keyboard.press('F1')
  await expect(page.getByRole('switch', { name: '启动时自动检查更新' })).not.toBeChecked()
  await page.locator('.switch-row').click()
  await expect(page.getByRole('switch', { name: '启动时自动检查更新' })).toBeChecked()
  await page.getByRole('button', { name: '关闭关于' }).click()
  await expect(dialog).toBeHidden()
})

test('manual update check reports a new release without installing it', async ({ page }) => {
  await page.route('**/api/update?force=1', route => route.fulfill({ json: {
    status: 'available', current: '0.1.0', latest: '9.9.9',
    url: 'https://github.com/owner/name/releases/tag/v9.9.9',
    download_url: 'https://github.com/owner/name/releases/download/v9.9.9/session-desk-9.9.9-setup.exe',
    asset_name: 'session-desk-9.9.9-setup.exe', notes: '- 新功能', checked_at: new Date().toISOString(),
  } }))
  await page.getByRole('button', { name: '关于 Session Desk' }).click()
  const dialog = page.getByRole('dialog', { name: 'Session Desk' })
  await dialog.getByRole('button', { name: '检查更新' }).click()
  await expect(dialog.getByRole('status')).toHaveText(/发现新版本 v9\.9\.9/)
  await expect(dialog.getByRole('button', { name: /下载 session-desk-9\.9\.9-setup\.exe/ })).toBeVisible()
  await expect(dialog.locator('.update-notes')).toContainText('新功能')
  await page.getByRole('button', { name: '关闭关于' }).click()
  await expect(page.locator('.update-pill')).toContainText('新版本 v9.9.9')
})

test('manual update check stays calm when offline', async ({ page }) => {
  await page.route('**/api/update?force=1', route => route.fulfill({ json: { status: 'error', current: '0.1.0', error: '无法连接 GitHub，请检查网络后重试', checked_at: new Date().toISOString() } }))
  await page.keyboard.press('F1')
  const dialog = page.getByRole('dialog', { name: 'Session Desk' })
  await dialog.getByRole('button', { name: '检查更新' }).click()
  await expect(dialog.getByRole('status')).toHaveText(/无法连接 GitHub/)
  await expect(page.locator('.update-pill')).toHaveCount(0)
})

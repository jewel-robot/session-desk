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
  await expect(dialog.locator('.about-version')).toHaveText(/v\d+\.\d+\.\d+|dev/)
  await expect(dialog.locator('.dialog-footer')).toHaveCount(0)
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

test('manual update check offers a download when the app cannot install', async ({ page }) => {
  await page.route('**/api/update?force=1', route => route.fulfill({ json: {
    status: 'available', current: '0.1.0', latest: '9.9.9',
    url: 'https://github.com/owner/name/releases/tag/v9.9.9',
    download_url: 'https://github.com/owner/name/releases/download/v9.9.9/session-desk-9.9.9-setup.exe',
    asset_name: 'session-desk-9.9.9-setup.exe', checked_at: new Date().toISOString(),
  } }))
  await page.getByRole('button', { name: '关于 Session Desk' }).click()
  const dialog = page.getByRole('dialog', { name: 'Session Desk' })
  await dialog.getByRole('button', { name: '检查更新' }).click()
  await expect(dialog.getByRole('status')).toHaveText(/发现新版本 v9\.9\.9/)
  await expect(dialog.getByRole('button', { name: '下载', exact: true })).toBeVisible()
  await expect(dialog.getByRole('button', { name: '更新', exact: true })).toHaveCount(0)
  await expect(dialog.locator('.about-card').first()).not.toContainText('##')
  await expect(dialog.getByRole('button', { name: '检查更新' })).toHaveCount(0)
  await expect(dialog.getByRole('button', { name: '发布说明' })).toBeVisible()
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

async function installable(page: import('@playwright/test').Page, digest = 'sha256:' + 'a'.repeat(64)) {
  await page.route('**/api/app', async route => route.fulfill({ json: { ...(await (await route.fetch()).json()), can_install: true } }))
  await page.route('**/api/update?force=1', route => route.fulfill({ json: {
    status: 'available', current: '0.1.0', latest: '9.9.9', url: 'https://github.com/owner/name/releases/tag/v9.9.9',
    download_url: 'https://github.com/owner/name/releases/download/v9.9.9/session-desk-9.9.9-setup.exe',
    asset_name: 'session-desk-9.9.9-setup.exe', digest, size: 100, checked_at: new Date().toISOString(),
  } }))
  await page.reload()
  await page.keyboard.press('F1')
  const dialog = page.getByRole('dialog', { name: 'Session Desk' })
  await dialog.getByRole('button', { name: '检查更新' }).click()
  return dialog
}

test('one-click update shows progress until the installer starts', async ({ page }) => {
  const states = [
    { phase: 'downloading', received: 0, total: 100 },
    { phase: 'downloading', received: 50, total: 100 },
    { phase: 'launching', received: 100, total: 100 },
  ]
  let posted = 0
  await page.route('**/api/update/install', async route => {
    if (route.request().method() === 'POST') posted++
    await new Promise(resolve => setTimeout(resolve, 150))
    await route.fulfill({ json: states.shift() ?? { phase: 'launched', received: 100, total: 100 } })
  })
  const dialog = await installable(page)
  await dialog.getByRole('button', { name: '更新', exact: true }).click()
  await expect(dialog.getByRole('button', { name: '50%' })).toBeDisabled()
  await expect(dialog.getByRole('button', { name: '正在安装' })).toBeVisible()
  expect(posted).toBe(1)
})

test('one-click update falls back to the release page when verification fails', async ({ page, context }) => {
  await page.route('**/api/update/install', route => route.fulfill({ json: { phase: 'error', received: 0, total: 0, error: '安装程序校验失败' } }))
  await context.route('https://github.com/**', route => route.fulfill({ body: 'release page' }))
  const dialog = await installable(page)
  const popup = page.waitForEvent('popup')
  await dialog.getByRole('button', { name: '更新', exact: true }).click()
  expect((await popup).url()).toContain('/releases/tag/v9.9.9')
  await expect(page.locator('.toast')).toContainText('安装程序校验失败')
  await expect(dialog.getByRole('button', { name: '更新', exact: true })).toBeEnabled()
})

test('a release without a digest is never installed in the app', async ({ page }) => {
  const dialog = await installable(page, '')
  await expect(dialog.getByRole('button', { name: '下载', exact: true })).toBeVisible()
  await expect(dialog.getByRole('button', { name: '更新', exact: true })).toHaveCount(0)
})

test('shortcut list includes Enter and Delete', async ({ page }) => {
  await page.keyboard.press('F1')
  const list = page.locator('.shortcut-list')
  await expect(list).toContainText('继续会话')
  await expect(list).toContainText('移入回收站')
  await expect(list.locator('kbd', { hasText: 'Del' })).toBeVisible()
})

test('the app name links to the project homepage', async ({ page, context }) => {
  await context.route('https://github.com/**', route => route.fulfill({ body: 'project page' }))
  await page.keyboard.press('F1')
  const dialog = page.getByRole('dialog', { name: 'Session Desk' })
  const link = dialog.getByRole('link', { name: 'Session Desk' })
  const repository = await link.getAttribute('href')
  expect(repository).toMatch(/^https:\/\/github\.com\//)
  const popup = page.waitForEvent('popup')
  await link.click()
  expect((await popup).url()).toBe(repository)
})

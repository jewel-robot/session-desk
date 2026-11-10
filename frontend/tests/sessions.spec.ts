import { test, expect } from '@playwright/test'
import { resolve } from 'node:path'

test.beforeEach(async ({ page }) => {
  await page.goto('/')
  await expect(page.locator('.session-title').first()).toBeVisible()
})

test('groups real parsed fixtures and filters providers and environments', async ({ page }) => {
  await expect(page.getByRole('heading', { level: 1 })).toContainText('会话')
  await expect(page.locator('.project-heading')).toHaveCount(4)
  await expect(page.locator('.session-item')).toHaveCount(16)
  await page.getByRole('button', { name: /^Codex/ }).click()
  await expect(page.locator('.session-item')).toHaveCount(5)
  await page.getByLabel('筛选环境').click()
  await page.getByRole('option', { name: 'WSL · Ubuntu-20.04' }).click()
  await expect(page.locator('.session-item')).toHaveCount(3)
  await expect(page.locator('.project-heading')).toHaveCount(2)
  await expect(page.locator('.project-heading').first()).toContainText('WSL · Ubuntu-20.04')
})

test('reads transcript on demand and folds reasoning and tool results', async ({ page }) => {
  const requests: string[] = []
  page.on('request', request => { if (request.url().includes('/api/transcript')) requests.push(request.url()) })
  expect(requests).toHaveLength(0)
  await page.getByRole('button', { name: '展开会话：为会话列表添加项目分组与环境筛选', exact: true }).click()
  const detail = page.locator('.session-detail').first()
  await expect(detail.getByText('实现方式', { exact: true })).toBeVisible()
  await expect(detail.locator('details.thinking')).not.toHaveAttribute('open')
  await detail.getByText('思考过程', { exact: true }).click()
  await expect(detail.locator('details.thinking pre')).toContainText('确认数据源')
  await detail.getByText('工具结果', { exact: true }).click()
  await expect(detail.locator('details.tool_result pre')).toContainText('文件读取完成')
  expect(requests).toHaveLength(1)
  await detail.getByRole('button', { name: '收起' }).click()
  await page.getByRole('button', { name: '展开会话：为会话列表添加项目分组与环境筛选', exact: true }).click()
  expect(requests).toHaveLength(1)
})

test('searches conversation text that is absent from titles', async ({ page }) => {
  await page.getByLabel('搜索会话').fill('文件读取完成')
  await expect(page.locator('.session-item')).toHaveCount(5)
  await expect(page.locator('.search-excerpt')).toHaveCount(5)
  await page.getByLabel('搜索会话').fill('不存在的搜索词xyz')
  await expect(page.getByRole('heading', { name: '没有找到匹配的会话' })).toBeVisible()
  await page.getByRole('button', { name: '清除搜索' }).click()
  await expect(page.locator('.session-item')).toHaveCount(16)
})

test('shows generated resume command and exports Markdown', async ({ page }) => {
  const row = page.locator('.session-item').filter({ hasText: '为会话列表添加项目分组与环境筛选' }).first()
  await row.locator('.session-title-button').click()
  const detail = page.getByRole('region', { name: '会话详情' })
  await detail.getByRole('button', { name: '查看命令' }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog.locator('pre')).toContainText('Set-Location -LiteralPath')
  await expect(dialog.locator('pre')).toContainText('Get-Command claude -CommandType Application')
  await expect(dialog.locator('pre')).toContainText('--resume')
  await dialog.getByRole('button', { name: '关闭', exact: true }).click()
  const download = page.waitForEvent('download')
  await detail.getByRole('button', { name: '导出', exact: true }).click()
  const file = await download
  expect(file.suggestedFilename()).toContain('.md')
})

test('keyboard search, source dialog, refresh and theme remain usable', async ({ page }) => {
  await page.keyboard.press('Control+k')
  await expect(page.getByLabel('搜索会话')).toBeFocused()
  await page.locator('.header-actions').getByRole('button', { name: '数据源' }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByRole('heading', { name: '数据源', exact: true })).toBeVisible()
  for (const name of ['Claude Code', 'Codex', 'OpenCode', 'Antigravity CLI']) await expect(dialog.getByLabel(`Windows ${name} 数据目录`)).toBeDisabled()
  await page.keyboard.press('Escape')
  await expect(dialog).toBeHidden()
  await page.getByRole('button', { name: '刷新会话', exact: true }).click()
  await expect(page.getByRole('status')).toContainText('会话已刷新')
  await page.getByRole('button', { name: '切换深色主题' }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await page.reload()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
})

test('layouts fit desktop and narrow windows', async ({ page }) => {
  for (const width of [1440, 1280, 1024, 768, 390]) {
    await page.setViewportSize({ width, height: 1000 })
    await expect(page.getByLabel('筛选环境')).toBeVisible()
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  }
  await page.setViewportSize({ width: 1440, height: 1000 })
  await page.screenshot({ path: resolve('../bin/session-desk-preview.png'), fullPage: true })
  await page.getByRole('button', { name: '展开会话：为会话列表添加项目分组与环境筛选', exact: true }).click()
  await expect(page.locator('.markdown').first()).toBeVisible()
  await page.screenshot({ path: resolve('../bin/session-desk-transcript.png'), fullPage: true })
})

test('API rejects arbitrary file paths and invalid terminal targets', async ({ request }) => {
  const transcript = await request.get('/api/transcript?key=/etc/passwd')
  expect(transcript.status()).toBe(400)
  const resume = await request.post('/api/resume', { data: { key: 'unregistered|claude|id' } })
  expect(resume.status()).toBe(400)
  const settings = await request.post('/api/settings', { data: { sources: [{ id: 'unknown', claude_dir: '/tmp', codex_dir: '/tmp' }] } })
  expect(settings.status()).toBe(400)
})


test('uses Magpie agent assets and reads both new providers', async ({ page }) => {
  for (const [name, count, icon] of [['Claude Code', 5, 'claudecode-color'], ['Codex', 5, 'codex-color'], ['OpenCode', 4, 'opencode'], ['Antigravity CLI', 2, 'antigravity-color']] as const) {
    const tab = page.locator('.provider-tabs').getByRole('button', { name: new RegExp(`^${name}`) })
    await expect(tab.locator('.provider-icon')).toHaveAttribute('data-icon', icon)
    const asset = await page.request.get(`/agents/${icon}.svg`)
    expect(asset.ok()).toBe(true)
    await tab.click()
    await expect(page.locator('.session-item')).toHaveCount(count)
  }
  await page.getByLabel('搜索会话').fill('分析导航模块')
  await page.getByRole('button', { name: '展开会话：分析导航模块的会话日志', exact: true }).click()
  await expect(page.locator('.markdown')).toContainText('重复步骤已更新')
  await expect(page.locator('.message-head').last()).toContainText('Antigravity CLI')
  await page.getByLabel('搜索会话').fill('整理 Antigravity')
  await page.getByRole('button', { name: '展开会话：整理 Antigravity CLI 项目摘要', exact: true }).click()
  await expect(page.locator('.talk-empty')).toContainText('会话摘要')
})

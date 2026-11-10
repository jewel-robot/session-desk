import { test, expect } from '@playwright/test'
import { spawn, type ChildProcess } from 'node:child_process'
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, resolve, join } from 'node:path'

let server: ChildProcess
let directory: string
const base = 'http://127.0.0.1:7437'

test.beforeEach(async ({ page, request }) => {
  directory = mkdtempSync(join(tmpdir(), 'session-desk-trash-e2e-'))
  server = spawn(resolve('../bin', process.platform === 'win32' ? 'session-desk.exe' : 'session-desk'), ['--serve', '--demo', '--listen', '127.0.0.1:7437', '--config', join(directory, 'settings.json')], { stdio: 'ignore' })
  await expect.poll(async () => {
    try { return (await request.get(`${base}/api/state`)).status() } catch { return 0 }
  }).toBe(200)
  await page.goto(base)
  await expect(page.locator('.session-item')).toHaveCount(16)
})

test.afterEach(async () => {
  if (server?.exitCode === null) {
    const exited = new Promise(resolve => server.once('exit', resolve))
    server.kill()
    await exited
  }
  rmSync(directory, { recursive: true, force: true })
})

test('single delete can be cancelled and restored from recycle bin', async ({ page }) => {
  const rescans: string[] = []
  page.on('request', request => { if (request.url().endsWith('/api/refresh')) rescans.push(request.url()) })
  const title = '为会话列表添加项目分组与环境筛选'
  const row = page.locator('.session-item').filter({ hasText: title })
  await row.locator('.session-title-button').click()
  const detail = page.getByRole('region', { name: '会话详情' })
  await detail.getByRole('button', { name: '删除此会话' }).click()
  let dialog = page.getByRole('dialog')
  await expect(dialog.getByRole('heading')).toHaveText('移入回收站')
  await expect(dialog.getByRole('button', { name: '取消', exact: true })).toBeFocused()
  await dialog.getByRole('button', { name: '取消', exact: true }).click()
  await expect(page.locator('.session-item')).toHaveCount(16)
  await detail.getByRole('button', { name: '删除此会话' }).click()
  await page.getByRole('dialog').getByRole('button', { name: '移入回收站', exact: true }).click()
  await expect(page.locator('.session-item')).toHaveCount(15)
  await page.locator('.nav-item').filter({ hasText: '回收站' }).click()
  await expect(page.locator('.trash-row')).toHaveCount(1)
  await expect(page.locator('.trash-row')).toContainText(title)
  await page.getByRole('button', { name: '恢复', exact: true }).click()
  dialog = page.getByRole('dialog')
  await dialog.getByRole('button', { name: '恢复会话', exact: true }).click()
  await expect(page.getByRole('heading', { name: '回收站为空' })).toBeVisible()
  await page.locator('.nav-item').filter({ hasText: '所有会话' }).click()
  await expect(page.locator('.session-item')).toHaveCount(16)
  expect(rescans).toEqual([])
})

test('batch confirmation includes children outside the current search and resets selection on filter changes', async ({ page }) => {
  await page.locator('.provider-tabs').getByRole('button', { name: /^OpenCode/ }).click()
  await page.getByLabel('筛选环境').click()
  await page.getByRole('option', { name: 'Windows', exact: true }).click()
  await page.getByLabel('搜索会话').fill('接入 OpenCode')
  await expect(page.locator('.session-item')).toHaveCount(1)
  await page.getByRole('button', { name: '批量选择', exact: true }).click()
  await page.getByLabel('选择当前结果', { exact: true }).check()
  await expect(page.locator('.selection-bar')).toContainText('已选择 1 项')
  await page.getByLabel('筛选环境').click()
  await page.getByRole('option', { name: 'WSL · Ubuntu-20.04' }).click()
  await expect(page.locator('.selection-bar')).toContainText('已选择 0 项')
  await page.getByLabel('选择当前结果', { exact: true }).check()
  await page.locator('.selection-bar').getByRole('button', { name: '移入回收站' }).click()
  const dialog = page.getByRole('dialog')
  await expect(dialog.locator('.related-note')).toContainText('1 个关联子会话')
  await expect(dialog.locator('.confirmation-list li')).toHaveCount(2)
  await expect(dialog).toContainText('验证 OpenCode 关联子会话')
  await dialog.getByRole('button', { name: '移入回收站', exact: true }).click()
  await expect(page.locator('.session-item')).toHaveCount(0)
  await page.locator('.nav-item').filter({ hasText: '回收站' }).click()
  await page.getByLabel('搜索会话').fill('')
  await expect(page.locator('.trash-row')).toHaveCount(1)
  await expect(page.locator('.trash-row')).toContainText('共 2 个会话')
  await page.getByRole('button', { name: '恢复', exact: true }).click()
  await page.getByRole('dialog').getByRole('button', { name: '恢复会话', exact: true }).click()
  await expect(page.locator('.trash-row')).toHaveCount(0)
})

test('permanent delete and empty bin both require explicit confirmation', async ({ page }) => {
  await page.getByRole('button', { name: '批量选择', exact: true }).click()
  await page.getByLabel('选择当前结果', { exact: true }).check()
  await page.locator('.selection-bar').getByRole('button', { name: '移入回收站' }).click()
  await expect(page.getByRole('dialog').locator('.confirmation-list li')).toHaveCount(16)
  await page.getByRole('dialog').getByRole('button', { name: '移入回收站', exact: true }).click()
  await expect(page.getByRole('heading', { name: '还没有找到会话' })).toBeVisible()
  await page.locator('.nav-item').filter({ hasText: '回收站' }).click()
  await expect(page.locator('.trash-row')).toHaveCount(8)
  await page.screenshot({ path: resolve('../bin/session-desk-trash.png'), fullPage: true })
  await page.locator('.trash-actions .danger-text').first().click()
  await expect(page.getByRole('dialog')).toContainText('此操作无法撤销')
  await page.getByRole('dialog').getByRole('button', { name: '取消', exact: true }).click()
  await expect(page.locator('.trash-row')).toHaveCount(8)
  await page.locator('.trash-actions .danger-text').first().click()
  await page.getByRole('dialog').getByRole('button', { name: '彻底删除', exact: true }).click()
  await expect(page.locator('.trash-row')).toHaveCount(7)
  await page.getByRole('button', { name: '清空回收站', exact: true }).click()
  await expect(page.getByRole('dialog').locator('.confirmation-list li')).toHaveCount(7)
  await page.getByRole('dialog').getByRole('button', { name: '取消', exact: true }).click()
  await expect(page.locator('.trash-row')).toHaveCount(7)
  await page.getByRole('button', { name: '清空回收站', exact: true }).click()
  await page.getByRole('dialog').getByRole('button', { name: '彻底删除', exact: true }).click()
  await expect(page.getByRole('heading', { name: '回收站为空' })).toBeVisible()
})

test('restore reports partial failure and retains conflicting records', async ({ page, request }) => {
  const state = await (await request.get(`${base}/api/state`)).json()
  const chosen = state.sessions.filter((session: any) => session.source_id === 'windows:local' && ['claude', 'codex'].includes(session.provider)).slice(0, 2)
  const plan = await (await request.post(`${base}/api/delete/preview`, { data: { keys: chosen.map((session: any) => session.key) } })).json()
  const result = await (await request.post(`${base}/api/delete`, { data: { keys: plan.keys, revisions: plan.revisions } })).json()
  expect(result.failed).toHaveLength(0)
  mkdirSync(dirname(chosen[0].path), { recursive: true })
  writeFileSync(chosen[0].path, 'conflicting live data')
  await page.reload()
  await page.locator('.nav-item').filter({ hasText: '回收站' }).click()
  await expect(page.locator('.trash-row')).toHaveCount(2)
  await page.getByRole('button', { name: '批量选择', exact: true }).click()
  await page.getByLabel('选择当前结果', { exact: true }).check()
  await page.getByRole('button', { name: '恢复所选', exact: true }).click()
  await page.getByRole('dialog').getByRole('button', { name: '恢复会话', exact: true }).click()
  await expect(page.locator('.trash-row')).toHaveCount(1)
  await expect(page.getByRole('alert')).toContainText('原位置已有文件')
  await page.getByRole('button', { name: '切换深色主题' }).click()
  await page.screenshot({ path: resolve('../bin/session-desk-trash-dark.png'), fullPage: true })
  for (const width of [1024, 768, 390]) {
    await page.setViewportSize({ width, height: 1000 })
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true)
  }
})

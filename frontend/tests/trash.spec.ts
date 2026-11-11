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
  await expect(page.locator('.selection-count')).toHaveText('1')
  await page.getByLabel('筛选环境').click()
  await page.getByRole('option', { name: 'WSL · Ubuntu-20.04' }).click()
  await expect(page.locator('.selection-count')).toHaveText('0')
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

test('right-click moves a session to the recycle bin without opening it', async ({ page }) => {
  const title = '为会话列表添加项目分组与环境筛选'
  const row = page.locator('.session-item').filter({ hasText: title })
  await row.click({ button: 'right' })
  const menu = page.getByRole('menu')
  await expect(menu).toBeVisible()
  const resume: boolean = await page.evaluate(async () => (await (await fetch('/api/state')).json()).can_resume)
  await expect(menu.getByRole('menuitem')).toHaveText(resume ? ['继续会话', '复制命令', '导出', '移入回收站'] : ['复制命令', '导出', '移入回收站'])
  await expect(page.locator('.session-item.active')).toHaveCount(0)
  await expect(row.locator('.session-title-button')).toHaveAttribute('aria-expanded', 'false')
  await page.keyboard.press('Escape')
  await expect(menu).toBeHidden()
  await row.click({ button: 'right' })
  await page.mouse.click(5, 5)
  await expect(menu).toBeHidden()

  await row.click({ button: 'right' })
  await page.keyboard.press('ArrowUp')
  await expect(menu.getByRole('menuitem', { name: '移入回收站' })).toBeFocused()
  await page.keyboard.press('Enter')
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByRole('heading')).toHaveText('移入回收站')
  await dialog.getByRole('button', { name: '移入回收站', exact: true }).click()
  await expect(page.locator('.session-item')).toHaveCount(15)
  await expect(page.locator('.session-item.active')).toHaveCount(0)

  await page.locator('.nav-item').filter({ hasText: '回收站' }).click()
  await page.locator('.trash-row').filter({ hasText: title }).click({ button: 'right' })
  await expect(menu.getByRole('menuitem')).toHaveText(['恢复', '彻底删除'])
  await menu.getByRole('menuitem', { name: '恢复' }).click()
  await page.getByRole('dialog').getByRole('button', { name: '恢复会话', exact: true }).click()
  await expect(page.getByRole('heading', { name: '回收站为空' })).toBeVisible()
})

test('context menu stays inside the viewport and closes on scroll', async ({ page }) => {
  await page.locator('.session-item').first().evaluate(element => {
    element.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true, cancelable: true, clientX: window.innerWidth - 2, clientY: window.innerHeight - 2 }))
  })
  const menu = page.getByRole('menu')
  await expect(menu).toBeVisible()
  const box = (await menu.boundingBox())!
  const viewport = page.viewportSize()!
  expect(box.x + box.width).toBeLessThanOrEqual(viewport.width)
  expect(box.y + box.height).toBeLessThanOrEqual(viewport.height)
  await page.locator('.page-content').evaluate(element => { element.scrollTop = 200; element.dispatchEvent(new Event('scroll')) })
  await expect(menu).toBeHidden()
})
test('Delete key, Shift range, Ctrl A and right-click act on the selection', async ({ page }) => {
  const rows = page.locator('.session-item')
  await rows.first().locator('.session-title-button').click()
  await page.keyboard.press('Delete')
  const dialog = page.getByRole('dialog')
  await expect(dialog.getByRole('heading')).toHaveText('移入回收站')
  await expect(dialog.locator('.confirmation-list li')).toHaveCount(1)
  await dialog.getByRole('button', { name: '取消', exact: true }).click()

  await page.getByRole('button', { name: '批量选择', exact: true }).click()
  await rows.nth(0).locator('input[type=checkbox]').click()
  await rows.nth(3).locator('input[type=checkbox]').click({ modifiers: ['Shift'] })
  await expect(page.locator('.selection-count')).toHaveText('4')
  await expect(rows.nth(4).locator('input[type=checkbox]')).not.toBeChecked()

  await rows.nth(1).click({ button: 'right' })
  const menu = page.getByRole('menu')
  await expect(menu.getByRole('menuitem')).toHaveText(['移入回收站'])
  await menu.getByRole('menuitem', { name: '移入回收站' }).click()
  // The four selected sessions plus one related child session.
  await expect(dialog.locator('.confirmation-list li')).toHaveCount(5)
  await dialog.getByRole('button', { name: '取消', exact: true }).click()

  await rows.nth(0).locator('input[type=checkbox]').focus()
  await page.keyboard.press('Control+a')
  await expect(page.locator('.selection-count')).toHaveText('16')
  await page.keyboard.press('Delete')
  await expect(dialog.locator('.confirmation-list li')).toHaveCount(16)
  await dialog.getByRole('button', { name: '取消', exact: true }).click()
  await expect(rows).toHaveCount(16)
})

test('right-click on selected trash entries restores them together', async ({ page }) => {
  await page.getByRole('button', { name: '批量选择', exact: true }).click()
  const rows = page.locator('.session-item')
  await rows.nth(0).locator('input[type=checkbox]').click()
  await rows.nth(1).locator('input[type=checkbox]').click()
  await page.locator('.selection-bar').getByRole('button', { name: '移入回收站' }).click()
  await page.getByRole('dialog').getByRole('button', { name: '移入回收站', exact: true }).click()
  await expect(rows).toHaveCount(14)
  await page.locator('.nav-item').filter({ hasText: '回收站' }).click()
  const entries = page.locator('.trash-row')
  await expect(entries).toHaveCount(2)
  await entries.nth(0).locator('input[type=checkbox]').click()
  await entries.nth(1).locator('input[type=checkbox]').click()
  await entries.nth(1).click({ button: 'right' })
  await page.getByRole('menu').getByRole('menuitem', { name: '恢复' }).click()
  await page.getByRole('dialog').getByRole('button', { name: '恢复会话', exact: true }).click()
  await expect(page.getByRole('heading', { name: '回收站为空' })).toBeVisible()
})

import { test, expect } from '@playwright/test'

// Exercise the desktop bridge through the same origin used by the native window.
test('desktop controls follow window events and support keyboard actions', async ({ page, baseURL }) => {
  await page.route('http://wails.localhost/**', async route => {
    const url = new URL(route.request().url())
    if (url.pathname === '/wails/runtime.js') {
      await route.fulfill({ contentType: 'text/javascript', body: `
        const callbacks = new Map();
        let maximised = false;
        window.desktopCalls = [];
        export const Events = { On(name, callback) { callbacks.set(name, callback); return () => callbacks.delete(name); } };
        export const Window = {
          async IsMaximised() { return maximised; },
          async Minimise() { window.desktopCalls.push('minimise'); },
          async ToggleMaximise() {
            maximised = !maximised;
            window.desktopCalls.push(maximised ? 'maximise' : 'restore');
            callbacks.get(maximised ? 'common:WindowMaximise' : 'common:WindowUnMaximise')?.();
          },
          async Close() { window.desktopCalls.push('close'); },
        };
      ` })
      return
    }
    const response = await page.request.get(`${baseURL}${url.pathname}${url.search}`)
    await route.fulfill({ response })
  })
  await page.goto('http://wails.localhost/')
  const controls = page.getByRole('group', { name: '窗口控制' })
  await expect(controls).toBeVisible()
  const maximise = controls.getByRole('button', { name: '最大化窗口' })
  await expect(maximise).toBeEnabled()
  await expect(page.locator('.session-title').first()).toBeVisible()
  await page.locator('.page-content').evaluate(element => { element.scrollTop = element.scrollHeight })
  expect(await page.locator('.page-content').evaluate(element => element.scrollTop)).toBeGreaterThan(0)
  await expect(controls.getByRole('button', { name: '关闭窗口' })).toBeInViewport()
  await maximise.focus()
  await page.keyboard.press('Enter')
  const restore = controls.getByRole('button', { name: '还原窗口' })
  await expect(restore).toBeVisible()
  await restore.click()
  await expect(maximise).toBeVisible()
  await page.locator('.window-header .connection').dblclick()
  await expect(restore).toBeVisible()
  await page.locator('.window-header .connection').dblclick()
  await expect(maximise).toBeVisible()
  await controls.getByRole('button', { name: '最小化窗口' }).click()
  await page.getByRole('button', { name: '切换深色主题' }).click()
  await expect(page.locator('html')).toHaveAttribute('data-theme', 'dark')
  await expect(controls.getByRole('button', { name: '关闭窗口' })).toBeEnabled()
  await controls.getByRole('button', { name: '关闭窗口' }).click()
  expect(await page.evaluate(() => (window as Window & { desktopCalls: string[] }).desktopCalls)).toEqual(['maximise', 'restore', 'maximise', 'restore', 'minimise', 'close'])
  await expect(page.getByRole('alert')).toHaveCount(0)
})

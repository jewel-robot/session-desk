import { defineConfig } from '@playwright/test'

export default defineConfig({
  testDir: './tests',
  timeout: 30000,
  fullyParallel: false,
  workers: 1,
  reporter: 'list',
  use: {
    baseURL: 'http://127.0.0.1:7436',
    viewport: { width: 1440, height: 1000 },
    locale: 'zh-CN',
    launchOptions: { executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH || undefined },
    trace: 'retain-on-failure',
  },
  webServer: {
    command: 'node tests/start-server.mjs',
    url: 'http://127.0.0.1:7436',
    timeout: 30000,
    reuseExistingServer: false,
  },
})

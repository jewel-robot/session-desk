// Renders the SVG app icon into PNGs and a multi-size Windows .ico.
// Usage (from session-desk/frontend, after npm ci): node ../scripts/icons.mjs
// Sizes up to 32 px use the simplified glyph in build/icons/icon-small.svg so they stay crisp.
import { createRequire } from 'node:module'
import { mkdirSync, readFileSync, writeFileSync, copyFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = join(dirname(fileURLToPath(import.meta.url)), '..')
const require = createRequire(join(root, 'frontend', 'package.json'))
const { chromium } = require('@playwright/test')

const large = readFileSync(join(root, 'build/icons/icon.svg'), 'utf8')
const small = readFileSync(join(root, 'build/icons/icon-small.svg'), 'utf8')
const sizes = [16, 24, 32, 48, 64, 128, 256]

const browser = await chromium.launch()
const page = await browser.newPage({ deviceScaleFactor: 1 })
const images = []
for (const size of sizes) {
  const svg = (size <= 32 ? small : large).replace(/width="\d+" height="\d+"/, `width="${size}" height="${size}"`)
  await page.setViewportSize({ width: size, height: size })
  await page.setContent(`<html><body style="margin:0;background:transparent">${svg}</body></html>`)
  const png = await page.locator('svg').screenshot({ omitBackground: true })
  images.push({ size, png })
  mkdirSync(join(root, 'build/icons/png'), { recursive: true })
  writeFileSync(join(root, `build/icons/png/icon-${size}.png`), png)
}
await browser.close()

// ICO container with PNG-compressed entries (supported by Windows Vista and later).
const header = Buffer.alloc(6)
header.writeUInt16LE(0, 0)
header.writeUInt16LE(1, 2)
header.writeUInt16LE(images.length, 4)
const entries = []
let offset = 6 + 16 * images.length
for (const { size, png } of images) {
  const entry = Buffer.alloc(16)
  entry.writeUInt8(size >= 256 ? 0 : size, 0)
  entry.writeUInt8(size >= 256 ? 0 : size, 1)
  entry.writeUInt8(0, 2)
  entry.writeUInt8(0, 3)
  entry.writeUInt16LE(1, 4)
  entry.writeUInt16LE(32, 6)
  entry.writeUInt32LE(png.length, 8)
  entry.writeUInt32LE(offset, 12)
  offset += png.length
  entries.push(entry)
}
mkdirSync(join(root, 'build/windows'), { recursive: true })
writeFileSync(join(root, 'build/windows/icon.ico'), Buffer.concat([header, ...entries, ...images.map((image) => image.png)]))
writeFileSync(join(root, 'build/appicon.png'), images.find((image) => image.size === 256).png)
copyFileSync(join(root, 'build/icons/icon.svg'), join(root, 'frontend/public/icon.svg'))
copyFileSync(join(root, 'build/icons/icon-small.svg'), join(root, 'frontend/public/icon-small.svg'))
console.log(`icons: ${sizes.join(', ')} px -> build/windows/icon.ico, build/appicon.png, frontend/public/icon.svg`)

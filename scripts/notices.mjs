import { execFileSync } from 'node:child_process'
import { chmod, mkdir, readFile, writeFile } from 'node:fs/promises'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

async function copyNotice(source, target) {
  const data = await readFile(source)
  try {
    await chmod(target, 0o644)
  } catch (error) {
    if (error.code !== 'ENOENT') throw error
  }
  await writeFile(target, data)
}

// Copies the first license file that exists in a package directory.
async function copyLicense(directory, target) {
  for (const filename of ['LICENSE', 'LICENSE.md', 'LICENSE.txt', 'COPYING']) {
    try {
      await copyNotice(join(directory, filename), target)
      return
    } catch (error) {
      if (error.code !== 'ENOENT') throw error
    }
  }
  throw new Error(`License not found: ${directory}`)
}

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const output = join(root, 'bin', 'licenses')
await mkdir(output, { recursive: true })
const go = process.env.SESSION_DESK_GO_BINARY || 'go'
const modules = execFileSync(go, ['list', '-deps', '-f', '{{if .Module}}{{.Module.Path}}|{{.Module.Dir}}{{end}}', '.'], {
  cwd: root,
  env: { ...process.env, GOOS: 'windows', CGO_ENABLED: '0' },
  encoding: 'utf8',
})
const entries = new Map(modules.trim().split('\n').filter(Boolean).map(line => line.split('|')))
for (const [name, directory] of entries) {
  if (name === 'sessiondesk') continue
  await copyLicense(directory, join(output, `${name.replace(/[^a-zA-Z0-9._-]/g, '_')}.txt`))
}
for (const [name, directory] of [
  ['Vue', 'vue'],
  ['Marked', 'marked'],
  ['DOMPurify', 'dompurify'],
  ['Lucide', '@lucide/vue'],
  ['Geist', '@fontsource-variable/geist'],
  ['GeistMono', '@fontsource-variable/geist-mono'],
]) {
  await copyLicense(join(root, 'frontend', 'node_modules', directory), join(output, `${name}.txt`))
}
const goroot = execFileSync(go, ['env', 'GOROOT'], { encoding: 'utf8' }).trim()
await copyNotice(join(goroot, 'LICENSE'), join(output, 'Go.txt'))
const wails = entries.get('github.com/wailsapp/wails/v3')
for (const [name, path] of [
  ['Wails-FileDialog', 'internal/go-common-file-dialog/LICENSE'],
  ['WebView2Loader', 'internal/webview2/webviewloader/LICENSE'],
]) {
  await copyNotice(join(wails, path), join(output, `${name}.txt`))
}
await copyNotice(join(root, 'licenses', 'Magpie.txt'), join(output, 'Magpie.txt'))
await copyNotice(join(root, 'LICENSE'), join(root, 'bin', 'LICENSE.txt'))
await writeFile(join(root, 'bin', 'THIRD_PARTY_NOTICES.md'), await readFile(join(root, 'THIRD_PARTY_NOTICES.md')))
console.log('Third-party notices collected in bin/licenses')

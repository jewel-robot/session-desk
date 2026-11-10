import { spawn } from 'node:child_process'
import { mkdtempSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { resolve, join } from 'node:path'

const binary = resolve('../bin', process.platform === 'win32' ? 'session-desk.exe' : 'session-desk')
const directory = mkdtempSync(join(tmpdir(), 'session-desk-e2e-'))
const child = spawn(binary, ['--serve', '--demo', '--listen', '127.0.0.1:7436', '--config', join(directory, 'settings.json')], { stdio: 'inherit' })
child.on('error', error => { console.error(error.message); process.exit(1) })
child.on('exit', code => process.exit(code ?? 0))
for (const signal of ['SIGTERM', 'SIGINT']) process.on(signal, () => child.kill(signal))

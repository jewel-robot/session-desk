export const agents = [
  { id: 'claude', name: 'Claude Code', icon: 'claudecode-color', mono: false, directory: 'claude_dir' },
  { id: 'codex', name: 'Codex', icon: 'codex-color', mono: false, directory: 'codex_dir' },
  { id: 'opencode', name: 'OpenCode', icon: 'opencode', mono: true, directory: 'opencode_dir' },
  { id: 'agy', name: 'Antigravity CLI', icon: 'antigravity-color', mono: false, directory: 'agy_dir' },
] as const

export type Provider = typeof agents[number]['id']

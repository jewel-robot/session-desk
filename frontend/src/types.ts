import type { Provider } from './agents'

export interface Source {
  id: string
  name: string
  kind: 'windows' | 'wsl' | 'local'
  distro?: string
  home: string
  claude_dir: string
  codex_dir: string
  opencode_dir: string
  agy_dir: string
  running: boolean
  error?: string
}

export interface Session {
  key: string
  id: string
  source_id: string
  provider: Provider
  title: string
  cwd: string
  project: string
  model: string
  branch?: string
  parent_id?: string
  started_at: string
  updated_at: string
  messages: number
  size: number
  revision: string
  tokens: { input: number; output: number; cached: number }
  path: string
  warning?: string
  store?: string
  paths?: string[]
  archived?: boolean
}

export interface Snapshot {
  sources: Source[]
  sessions: Session[]
  updated_at: string
  warnings: string[]
  demo: boolean
  platform: string
  can_resume: boolean
}

export interface Part {
  role: string
  kind: string
  name?: string
  text: string
  model?: string
}

export interface Transcript {
  parts: Part[]
  next_cursor: number
  has_more: boolean
}

export interface Talk extends Transcript {
  busy: boolean
  error: string
}

export interface SearchResult {
  hits: { key: string; excerpt: string }[]
  limited: boolean
  errors: string[]
}

export interface SourceOverride {
  id: string
  claude_dir: string
  codex_dir: string
  opencode_dir: string
  agy_dir: string
}

export interface DeletePlan {
  keys: string[]
  targets: Session[]
  revisions: Record<string, string>
  related: number
}

export interface MutationResult {
  done: string[]
  failed: { key: string; error: string }[]
}

export interface TrashEntry {
  key: string
  source_id: string
  provider: Provider
  title: string
  deleted_at: string
  sessions: Session[]
  status: 'trashed' | 'pending'
  offline: boolean
}

export interface AppInfo {
  version: string
  commit: string
  date: string
  repository: string
  desktop: boolean
  update_check: boolean
  config_dir: string
  log_path: string
  can_open_url: boolean
}

export interface UpdateResult {
  status: 'available' | 'latest' | 'none' | 'error' | 'disabled'
  current: string
  latest?: string
  url?: string
  download_url?: string
  asset_name?: string
  published_at?: string
  notes?: string
  error?: string
  checked_at?: string
}

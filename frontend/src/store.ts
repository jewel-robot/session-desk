import { computed, reactive, ref, shallowRef, watch } from 'vue'
import { api, params } from './api'
import { agents } from './agents'
import type { DeletePlan, MutationResult, SearchResult, Session, Snapshot, Source, SourceOverride, Talk, Transcript, TrashEntry } from './types'

export interface ProjectGroup { key: string; project: string; cwd: string; source: Source | undefined; sessions: Session[] }

// Shared application state. The app has a single window, so one module-level store is enough.
export const snapshot = shallowRef<Snapshot>({ sources: [], sessions: [], warnings: [], updated_at: '', demo: false, platform: '', can_resume: false })
export const loading = ref(true)
export const view = ref<'sessions' | 'trash'>('sessions')
export const trash = shallowRef<TrashEntry[]>([])
export const trashError = ref('')
export const selecting = ref(false)
export const selected = ref(new Set<string>())
export const mutation = ref<'delete' | 'restore' | 'purge'>('delete')
export const deletePlan = shallowRef<DeletePlan>()
export const trashTargets = shallowRef<TrashEntry[]>([])
export const mutationBusy = ref(false)
export const mutationErrors = ref<string[]>([])
export const previewing = ref(false)
export const error = ref('')
export const provider = ref('')
export const sourceID = ref('')
export const query = ref('')
export const sort = ref('updated')
export const closedGroups = ref(new Set<string>())
export const pageSizes = reactive(new Map<string, number>())
export const talks = reactive(new Map<string, Talk>())
export const activeKey = ref('')
export const menuKey = ref('')
export const launching = ref('')
export const searchInput = ref<HTMLInputElement>()
export const commandText = ref('')
export const commandTitle = ref('')
export const drafts = ref<SourceOverride[]>([])
export const saving = ref(false)
export const starting = ref('')
export const toast = ref<{ text: string; error: boolean } | null>(null)
export const theme = ref(localStorage.getItem('session-desk-theme') || 'light')
export const found = shallowRef<SearchResult | null>(null)
export const searching = ref(false)
export const sidebarOpen = ref(false)
export const dialogs = reactive({ sources: false, command: false, mutation: false, about: false })

let searchTimer: ReturnType<typeof setTimeout>
let toastTimer: ReturnType<typeof setTimeout>
let searchController: AbortController | undefined

export const sourceMap = computed(() => new Map(snapshot.value.sources.map(source => [source.id, source])))
export const sourceSessions = computed(() => snapshot.value.sessions.filter(s => !sourceID.value || s.source_id === sourceID.value))
export const sourceTrash = computed(() => trash.value.filter(entry => !sourceID.value || entry.source_id === sourceID.value))
export const counts = computed(() => {
  const list = view.value === 'trash' ? sourceTrash.value : sourceSessions.value
  return Object.fromEntries([['all', list.length], ...agents.map(agent => [agent.id, list.filter(item => item.provider === agent.id).length])]) as Record<string, number>
})
export const sourceCounts = computed(() => {
  const result = new Map<string, number>()
  for (const session of snapshot.value.sessions) result.set(session.source_id, (result.get(session.source_id) || 0) + 1)
  return result
})
export const shownTrash = computed(() => {
  const needle = query.value.trim().toLowerCase()
  return sourceTrash.value.filter(entry => (!provider.value || entry.provider === provider.value) &&
    (!needle || entry.sessions.some(session => [session.title, session.cwd, session.id, session.model].some(text => text.toLowerCase().includes(needle)))))
})
export const searchHits = computed(() => new Map(found.value?.hits.map(hit => [hit.key, hit.excerpt]) || []))
export const shown = computed(() => {
  const needle = query.value.trim().toLowerCase()
  const list = sourceSessions.value.filter(s => {
    if (provider.value && s.provider !== provider.value) return false
    if (!needle) return true
    return searchHits.value.has(s.key) || [s.title, s.cwd, s.model, s.id].some(value => value.toLowerCase().includes(needle))
  })
  return [...list].sort((a, b) => sort.value === 'messages'
    ? b.messages - a.messages : b.updated_at.localeCompare(a.updated_at))
})
export const groups = computed(() => {
  const result = new Map<string, ProjectGroup>()
  for (const session of shown.value) {
    const key = `${session.source_id}|${session.cwd}`
    if (!result.has(key)) result.set(key, { key, project: session.project, cwd: session.cwd, source: sourceMap.value.get(session.source_id), sessions: [] })
    result.get(key)!.sessions.push(session)
  }
  return [...result.values()]
})
export const visibleKeys = computed(() => view.value === 'trash' ? shownTrash.value.map(entry => entry.key) : shown.value.map(session => session.key))
export const allSelected = computed(() => visibleKeys.value.length > 0 && visibleKeys.value.every(key => selected.value.has(key)))
export const mutationTitle = computed(() => mutation.value === 'delete' ? '移入回收站' : mutation.value === 'restore' ? '恢复会话' : '彻底删除')
export const activeSources = computed(() => snapshot.value.sources.filter(source => source.running && !source.error).length)
export const activeSession = computed(() => snapshot.value.sessions.find(session => session.key === activeKey.value))
export const providerOptions = computed(() => [
  { value: '', label: `全部 Agent · ${counts.value.all}` },
  ...agents.map(agent => ({ value: agent.id, label: `${agent.name} · ${counts.value[agent.id]}` })),
])
export const environmentOptions = computed(() => [
  { value: '', label: '全部环境' },
  ...snapshot.value.sources.map(source => ({ value: source.id, label: sourceLabel(source) })),
])
export const sortOptions = [
  { value: 'updated', label: '最近更新' },
  { value: 'messages', label: '消息最多' },
]

export function sourceLabel(source: Source | undefined): string {
  if (!source) return ''
  return source.kind === 'wsl' ? `WSL · ${source.name}` : source.name
}

export function groupOpen(key: string): boolean {
  return !closedGroups.value.has(key) || !!query.value.trim()
}

watch(theme, value => {
  document.documentElement.dataset.theme = value
  localStorage.setItem('session-desk-theme', value)
}, { immediate: true })

watch([query, provider, sourceID, view, () => snapshot.value.updated_at], () => {
  clearTimeout(searchTimer)
  searchController?.abort()
  found.value = null
  searching.value = false
  pageSizes.clear()
  if (!query.value.trim() || view.value === 'trash') return
  searching.value = true
  searchTimer = setTimeout(async () => {
    const controller = new AbortController()
    searchController = controller
    try {
      const result = await api<SearchResult>(`search?${params({ q: query.value, provider: provider.value, source: sourceID.value })}`, undefined, controller.signal)
      if (!controller.signal.aborted) found.value = result
    } catch (cause) {
      if (!controller.signal.aborted) notify(cause instanceof Error ? cause.message : '搜索失败', true)
    } finally {
      if (!controller.signal.aborted) searching.value = false
    }
  }, 300)
})

watch([query, provider, sourceID, view], () => { selected.value.clear(); menuKey.value = '' })

export function disposeStore() {
  clearTimeout(searchTimer)
  clearTimeout(toastTimer)
  searchController?.abort()
}

export function toggleSelected(keys: string[]) {
  const remove = keys.every(key => selected.value.has(key))
  for (const key of keys) {
    if (remove) selected.value.delete(key)
    else selected.value.add(key)
  }
}

export function toggleSelection() {
  selecting.value = !selecting.value
  selected.value.clear()
}

export function goSessions() {
  view.value = 'sessions'
  provider.value = ''; sourceID.value = ''; query.value = ''
  sidebarOpen.value = false
}

export function goTrash() {
  view.value = 'trash'
  query.value = ''
  sidebarOpen.value = false
  void loadTrash()
}

export async function loadTrash() {
  try {
    trash.value = (await api<{ entries: TrashEntry[] }>('trash')).entries
    trashError.value = ''
  } catch (cause) {
    trashError.value = cause instanceof Error ? cause.message : '读取回收站失败'
  }
}

export async function prepareDelete(keys: string[]) {
  if (!keys.length || previewing.value || mutationBusy.value) return
  menuKey.value = ''
  previewing.value = true
  try {
    deletePlan.value = await api<DeletePlan>('delete/preview', { keys })
    mutation.value = 'delete'
    dialogs.mutation = true
  } catch (cause) {
    notify(cause instanceof Error ? cause.message : '读取删除预览失败', true)
  } finally {
    previewing.value = false
  }
}

export function confirmTrash(action: 'restore' | 'purge', entries: TrashEntry[]) {
  if (!entries.length) return
  mutation.value = action
  trashTargets.value = entries
  dialogs.mutation = true
}

export async function runMutation() {
  if (mutationBusy.value) return
  mutationBusy.value = true
  mutationErrors.value = []
  let result: MutationResult = { done: [], failed: [] }
  try {
    if (mutation.value === 'delete') {
      result = await api<MutationResult>('delete', { keys: deletePlan.value!.keys, revisions: deletePlan.value!.revisions })
    } else if (mutation.value === 'purge') {
      result = await api<MutationResult>('trash/purge', { keys: trashTargets.value.map(entry => entry.key) })
    } else {
      result = await api<MutationResult>('trash/restore', { keys: trashTargets.value.map(entry => entry.key) })
    }
    mutationErrors.value = [...new Set(result.failed.map(failure => failure.error))]
    selected.value.clear()
    dialogs.mutation = false
    notify(`${mutationTitle.value}：成功 ${result.done.length} 项${result.failed.length ? `，失败 ${result.failed.length} 项` : ''}`, !!result.failed.length)
    await load(false, false)
  } catch (cause) {
    mutationErrors.value = [cause instanceof Error ? cause.message : '操作失败']
    dialogs.mutation = false
    await load(false, false)
  } finally {
    mutationBusy.value = false
  }
}

export function notify(text: string, failed = false) {
  toast.value = { text, error: failed }
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => { toast.value = null }, 4500)
}

export async function load(refresh = false, announce = true) {
  if (refresh && loading.value) return
  loading.value = true
  error.value = ''
  try {
    const next = await api<Snapshot>(refresh ? 'refresh' : 'state', refresh ? {} : undefined)
    const previous = new Map(snapshot.value.sessions.map(s => [s.key, s.revision]))
    snapshot.value = next
    for (const session of next.sessions) {
      if (previous.get(session.key) !== session.revision) talks.delete(session.key)
    }
    if (activeKey.value && !next.sessions.some(item => item.key === activeKey.value)) activeKey.value = ''
    if (activeKey.value && !talks.has(activeKey.value)) void readTalk(activeKey.value)
    await loadTrash()
    const existing = new Set([...next.sessions.map(item => item.key), ...trash.value.map(item => item.key)])
    for (const key of selected.value) if (!existing.has(key)) selected.value.delete(key)
    if (refresh && announce) notify(view.value === 'trash' ? '回收站已刷新' : '会话已刷新')
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '读取会话失败'
  } finally {
    loading.value = false
  }
}

export function toggleGroup(key: string) {
  if (closedGroups.value.has(key)) closedGroups.value.delete(key)
  else closedGroups.value.add(key)
}

// Opens a session in the detail pane; selecting the open session again closes it.
export async function toggleSession(session: Session) {
  if (activeKey.value === session.key) {
    activeKey.value = ''
    return
  }
  await openSession(session)
}

export async function openSession(session: Session) {
  activeKey.value = session.key
  if (!talks.has(session.key)) await readTalk(session.key)
}

export function closeSession() {
  activeKey.value = ''
}

export async function readTalk(key: string, more = false) {
  const old = talks.get(key)
  if (old?.busy) return
  const talk: Talk = { parts: more ? old?.parts || [] : [], next_cursor: more ? old?.next_cursor || 0 : 0, has_more: false, busy: true, error: '' }
  talks.set(key, talk)
  const pending = talks.get(key)
  try {
    const page = await api<Transcript>(`transcript?${params({ key, cursor: talk.next_cursor })}`)
    if (talks.get(key) !== pending) return
    talks.set(key, { ...page, parts: [...talk.parts, ...page.parts], busy: false, error: '' })
  } catch (cause) {
    if (talks.get(key) !== pending) return
    talks.set(key, { ...talk, busy: false, error: cause instanceof Error ? cause.message : '读取正文失败' })
  }
}

export async function copyCommand(session: Session) {
  menuKey.value = ''
  try {
    const plan = await api<{ command: string }>(`command?${params({ key: session.key })}`)
    if (snapshot.value.platform === 'windows') await api('copy', { key: session.key })
    else await navigator.clipboard.writeText(plan.command)
    notify('续接命令已复制')
  } catch (cause) {
    notify(cause instanceof Error ? cause.message : '复制失败', true)
  }
}

export async function continueSession(session: Session) {
  menuKey.value = ''
  if (!snapshot.value.can_resume) return copyCommand(session)
  launching.value = session.key
  try {
    await api('resume', { key: session.key })
    notify('已在 Windows Terminal 中打开会话')
  } catch (cause) {
    notify(cause instanceof Error ? cause.message : '打开终端失败', true)
  } finally {
    launching.value = ''
  }
}

export async function showCommand(session: Session) {
  menuKey.value = ''
  try {
    const plan = await api<{ command: string }>(`command?${params({ key: session.key })}`)
    commandText.value = plan.command
    commandTitle.value = session.title
    dialogs.command = true
  } catch (cause) {
    notify(cause instanceof Error ? cause.message : '读取命令失败', true)
  }
}

export async function exportSession(session: Session) {
  menuKey.value = ''
  try {
    const response = await fetch(`/api/export?${params({ key: session.key })}`)
    if (!response.ok) throw new Error((await response.json()).error)
    const url = URL.createObjectURL(await response.blob())
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = `${session.title.slice(0, 60).replace(/[<>:"/\\|?*]/g, '-')}.md`
    anchor.click()
    setTimeout(() => URL.revokeObjectURL(url), 1000)
    notify('已导出 Markdown')
  } catch (cause) {
    notify(cause instanceof Error ? cause.message : '导出失败', true)
  }
}

function sourceDrafts(): SourceOverride[] {
  return snapshot.value.sources.map(source => ({ id: source.id, claude_dir: source.claude_dir, codex_dir: source.codex_dir, opencode_dir: source.opencode_dir, agy_dir: source.agy_dir }))
}

export function openSources() {
  drafts.value = sourceDrafts()
  sidebarOpen.value = false
  dialogs.sources = true
}

export async function saveSources() {
  saving.value = true
  try {
    await api('settings', { sources: drafts.value })
    dialogs.sources = false
    await load(true)
  } catch (cause) {
    notify(cause instanceof Error ? cause.message : '保存失败', true)
  } finally {
    saving.value = false
  }
}

export async function startSource(source: Source) {
  starting.value = source.id
  try {
    await api('sources/start', { id: source.id })
    await load(true)
    drafts.value = sourceDrafts()
  } catch (cause) {
    notify(cause instanceof Error ? cause.message : '启动 WSL 失败', true)
  } finally {
    starting.value = ''
  }
}

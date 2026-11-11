// Application-level state: version info, update checks and external links.
import { computed, ref, shallowRef } from 'vue'
import { api } from './api'
import { notify } from './store'
import type { AppInfo, InstallState, UpdateResult } from './types'

export const appInfo = shallowRef<AppInfo | null>(null)
export const update = shallowRef<UpdateResult | null>(null)
export const checkingUpdate = ref(false)
export const updateAvailable = computed(() => update.value?.status === 'available')
export const installing = shallowRef<InstallState | null>(null)
// One-click update needs the desktop app and a release installer with a published SHA-256 digest.
export const canInstall = computed(() => !!appInfo.value?.can_install && updateAvailable.value
  && !!update.value?.digest && /-setup\.exe$/i.test(update.value?.asset_name || ''))
export const versionLabel = computed(() => {
  const version = appInfo.value?.version
  if (!version) return ''
  return version === 'dev' ? 'dev' : `v${version}`
})

export async function loadAppInfo() {
  try {
    appInfo.value = await api<AppInfo>('app')
  } catch {
    appInfo.value = null
  }
}

// Automatic check on startup; the server skips it when disabled in settings or in demo mode.
export async function autoCheckUpdates() {
  try {
    update.value = await api<UpdateResult>('update')
  } catch {
    // Offline or blocked: stay quiet, the About dialog offers a manual check.
  }
}

export async function checkUpdates() {
  if (checkingUpdate.value) return
  checkingUpdate.value = true
  try {
    update.value = await api<UpdateResult>('update?force=1')
  } catch (cause) {
    update.value = { status: 'error', current: appInfo.value?.version || '', error: cause instanceof Error ? cause.message : '检查更新失败' }
  } finally {
    checkingUpdate.value = false
  }
}

export async function setAutoUpdateCheck(enabled: boolean) {
  await api('app/preferences', { update_check: enabled })
  if (appInfo.value) appInfo.value = { ...appInfo.value, update_check: enabled }
}

// Downloads and verifies the installer in the app, then the installer upgrades and restarts it.
// Any failure falls back to the release page.
export async function installUpdate() {
  if (installing.value) return
  let phase = ''
  try {
    let state = await api<InstallState>('update/install', {})
    installing.value = state
    while (state.phase === 'downloading' || state.phase === 'launching') {
      phase = state.phase
      await new Promise(resolve => setTimeout(resolve, 250))
      state = await api<InstallState>('update/install')
      installing.value = state
    }
    if (state.phase === 'error') throw new Error(state.error || '更新失败')
  } catch (cause) {
    // The app quits right after the installer starts, so a dropped connection then is expected.
    if (phase === 'launching') return
    installing.value = null
    notify(cause instanceof Error ? cause.message : '更新失败', true)
    if (update.value?.url) await openLink(update.value.url).catch(() => undefined)
  }
}

// Links open in the system browser from the desktop window; the browser preview uses a new tab.
export async function openLink(url: string) {
  if (appInfo.value?.can_open_url) {
    await api('open-url', { url })
    return
  }
  window.open(url, '_blank', 'noopener,noreferrer')
}

// Application-level state: version info, update checks and external links.
import { computed, ref, shallowRef } from 'vue'
import { api } from './api'
import type { AppInfo, UpdateResult } from './types'

export const appInfo = shallowRef<AppInfo | null>(null)
export const update = shallowRef<UpdateResult | null>(null)
export const checkingUpdate = ref(false)
export const updateAvailable = computed(() => update.value?.status === 'available')
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

// Links open in the system browser from the desktop window; the browser preview uses a new tab.
export async function openLink(url: string) {
  if (appInfo.value?.can_open_url) {
    await api('open-url', { url })
    return
  }
  window.open(url, '_blank', 'noopener,noreferrer')
}

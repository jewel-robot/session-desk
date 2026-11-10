<script setup lang="ts">
import { onMounted, onUnmounted } from 'vue'
import TitleBar from './components/TitleBar.vue'
import AppSidebar from './components/AppSidebar.vue'
import ListPane from './components/ListPane.vue'
import SessionDetail from './components/SessionDetail.vue'
import AppToast from './components/AppToast.vue'
import SourcesDialog from './dialogs/SourcesDialog.vue'
import CommandDialog from './dialogs/CommandDialog.vue'
import MutationDialog from './dialogs/MutationDialog.vue'
import AboutDialog from './dialogs/AboutDialog.vue'
import { autoCheckUpdates, loadAppInfo } from './app'
import { activeKey, activeSession, dialogs, disposeStore, load, menuKey, openSession, searchInput, shown, sidebarOpen, view } from './store'

const desktopWindow = location.hostname === 'wails.localhost'

function typing(target: EventTarget | null): boolean {
  const element = target as HTMLElement | null
  return !!element?.closest('input, textarea, select, [contenteditable], [role="listbox"], dialog, .more-menu')
}

// Ctrl K focuses search; ↑/↓ move between sessions in the list when focus is not in a field; F1 opens About.
function keyboard(event: KeyboardEvent) {
  if (event.key === 'F1') {
    event.preventDefault()
    dialogs.about = true
    return
  }
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 'k') {
    event.preventDefault()
    searchInput.value?.focus()
    return
  }
  if (event.key === 'Escape') {
    menuKey.value = ''
    sidebarOpen.value = false
    return
  }
  if ((event.key === 'ArrowDown' || event.key === 'ArrowUp') && view.value === 'sessions' && !event.altKey && !event.ctrlKey && !event.metaKey
    && !typing(event.target) && !dialogs.sources && !dialogs.command && !dialogs.mutation && !dialogs.about) {
    const list = shown.value
    if (!list.length) return
    event.preventDefault()
    const index = list.findIndex(session => session.key === activeKey.value)
    const next = event.key === 'ArrowDown' ? Math.min(list.length - 1, index + 1) : Math.max(0, index < 0 ? 0 : index - 1)
    void openSession(list[next]!)
    requestAnimationFrame(() => document.querySelector(`.session-item[data-key="${CSS.escape(list[next]!.key)}"]`)?.scrollIntoView({ block: 'nearest' }))
  }
}

let updateTimer: ReturnType<typeof setTimeout> | undefined

function closeMenu(event: MouseEvent) {
  if (!(event.target as HTMLElement).closest('.more-wrap')) menuKey.value = ''
}

onMounted(() => {
  void load()
  void loadAppInfo()
  // Defer the network check so it never competes with the first session scan.
  updateTimer = setTimeout(() => void autoCheckUpdates(), 4000)
  window.addEventListener('keydown', keyboard)
  window.addEventListener('click', closeMenu)
})

onUnmounted(() => {
  clearTimeout(updateTimer)
  disposeStore()
  window.removeEventListener('keydown', keyboard)
  window.removeEventListener('click', closeMenu)
})
</script>

<template>
  <div class="app-shell" :class="{ 'desktop-window': desktopWindow, 'trash-view': view === 'trash', 'has-detail': view === 'sessions' && !!activeSession, 'sidebar-open': sidebarOpen }">
    <a href="#session-content" class="skip-link">跳转到会话列表</a>
    <TitleBar :desktop="desktopWindow" />
    <AppSidebar />
    <div class="sidebar-scrim" aria-hidden="true" @click="sidebarOpen = false"></div>
    <ListPane />
    <SessionDetail v-if="view === 'sessions'" />
    <SourcesDialog />
    <CommandDialog />
    <MutationDialog />
    <AboutDialog />
    <AppToast />
  </div>
</template>

<style>
.app-shell {
  display: grid;
  height: 100vh;
  grid-template-columns: var(--sidebar-w) var(--list-w) minmax(0, 1fr);
  grid-template-rows: var(--titlebar-h) minmax(0, 1fr);
  grid-template-areas: 'title title title' 'sidebar list detail';
  background: var(--color-canvas);
}
.app-shell.trash-view { grid-template-areas: 'title title title' 'sidebar list list'; }
.sidebar-scrim { display: none; }

@media (min-width: 1600px) {
  .app-shell { --list-w: 440px; }
}
@media (max-width: 1279px) {
  .app-shell { --list-w: 360px; grid-template-columns: var(--rail-w) var(--list-w) minmax(0, 1fr); }
}
@media (max-width: 1023px) {
  .app-shell, .app-shell.trash-view { grid-template-columns: var(--rail-w) minmax(0, 1fr); grid-template-areas: 'title title' 'sidebar list'; }
  .app-shell .detail-pane { grid-area: list; z-index: 5; }
  .app-shell:not(.has-detail) .detail-pane { display: none; }
}
@media (max-width: 767px) {
  .app-shell, .app-shell.trash-view { grid-template-columns: minmax(0, 1fr); grid-template-areas: 'title' 'list'; }
  .app-shell .sidebar { display: none; }
  .app-shell.sidebar-open .sidebar { display: flex; position: fixed; z-index: 40; top: 0; bottom: 0; left: 0; width: min(280px, 86vw); box-shadow: var(--shadow-lg); animation: slide-in var(--duration) var(--ease); }
  .app-shell.sidebar-open .sidebar-scrim { display: block; position: fixed; inset: 0; z-index: 39; background: var(--color-backdrop); animation: fade-in var(--duration) var(--ease); }
}
@keyframes slide-in { from { transform: translateX(-16px); opacity: 0; } }
</style>

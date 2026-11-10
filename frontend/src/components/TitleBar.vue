<script setup lang="ts">
import { ref } from 'vue'
import WindowControls from './WindowControls.vue'
import { activeSources, snapshot } from '../store'

defineProps<{ desktop: boolean }>()
const windowControls = ref<InstanceType<typeof WindowControls>>()

function toggleMaximise(event: MouseEvent, desktop: boolean) {
  if (desktop && !(event.target as Element).closest('.window-controls')) void windowControls.value?.toggleMaximise()
}
</script>

<template>
  <header class="topbar" :class="{ 'window-header': desktop }" @dblclick="toggleMaximise($event, desktop)">
    <div class="topbar-brand">
      <img src="/icon-small.svg" alt="" width="18" height="18" draggable="false" />
      <span>Session Desk</span>
    </div>
    <span class="connection" :title="`${activeSources}/${snapshot.sources.length} 个环境已连接`" :aria-label="`${activeSources}/${snapshot.sources.length} 个环境已连接`"><span class="status-dot" :class="{ offline: !activeSources }"></span>{{ activeSources }}/{{ snapshot.sources.length }}</span>
    <WindowControls v-if="desktop" ref="windowControls" />
  </header>
</template>

<style>
.topbar {
  grid-area: title;
  display: flex; align-items: center; gap: var(--space-4);
  height: var(--titlebar-h); padding: 0 var(--space-4);
  background: var(--color-canvas); user-select: none;
}
.topbar.window-header { padding-right: 0; --wails-draggable: drag; }
.topbar-brand { display: flex; align-items: center; gap: var(--space-2); font-size: var(--text-sm); font-weight: var(--weight-semibold); letter-spacing: -.1px; color: var(--color-text); }
.topbar-brand img { border-radius: 5px; }
.connection {
  display: inline-flex; align-items: center; gap: var(--space-2); margin-left: auto;
  height: 24px; padding: 0 var(--space-2-5); border-radius: var(--radius-full);
  font-size: var(--text-2xs); color: var(--color-text-secondary); white-space: nowrap;
  background: var(--color-panel); box-shadow: var(--shadow-sm);
}
.window-controls { display: flex; align-self: stretch; flex-shrink: 0; --wails-draggable: no-drag; }
.window-controls button { display: flex; align-items: center; justify-content: center; width: 46px; padding: 0; color: var(--color-text-secondary); border-radius: 0; transition: background-color var(--duration) var(--ease), color var(--duration) var(--ease); }
.window-controls button:not(:disabled):hover { background: var(--color-hover); color: var(--color-text); }
.window-controls button:focus-visible { box-shadow: none; background: var(--color-hover); border-radius: 0; }
.window-controls .window-close:not(:disabled):hover { background: var(--color-close-hover); color: #fff; }
.window-controls-error { align-self: center; font-size: var(--text-xs); color: var(--color-danger); padding: 0 var(--space-3); }
</style>

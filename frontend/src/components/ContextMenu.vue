<script setup lang="ts">
import { nextTick, onUnmounted, ref, watch } from 'vue'
import { closeContextMenu, contextMenu, type MenuItem } from '../contextMenu'

const element = ref<HTMLElement>()
const position = ref({ left: 0, top: 0 })

function items(): HTMLButtonElement[] {
  return [...(element.value?.querySelectorAll<HTMLButtonElement>('button:not(:disabled)') ?? [])]
}

function choose(item: MenuItem) {
  if (item.disabled) return
  closeContextMenu(true)
  void item.action()
}

function onKeydown(event: KeyboardEvent) {
  const buttons = items()
  const index = buttons.indexOf(document.activeElement as HTMLButtonElement)
  let next = -1
  if (event.key === 'ArrowDown') next = (index + 1) % buttons.length
  else if (event.key === 'ArrowUp') next = index < 0 ? buttons.length - 1 : (index - 1 + buttons.length) % buttons.length
  else if (event.key === 'Home') next = 0
  else if (event.key === 'End') next = buttons.length - 1
  else if (event.key === 'Escape' || event.key === 'Tab') {
    event.preventDefault()
    event.stopPropagation()
    closeContextMenu(true)
    return
  } else return
  event.preventDefault()
  event.stopPropagation()
  buttons[next]?.focus()
}

function onPointerDown(event: PointerEvent) {
  if (!element.value?.contains(event.target as Node)) closeContextMenu()
}
function onDismiss() { closeContextMenu() }

function listen(on: boolean) {
  const method = on ? 'addEventListener' : 'removeEventListener'
  window[method]('pointerdown', onPointerDown as EventListener, true)
  window[method]('scroll', onDismiss, true)
  window[method]('resize', onDismiss)
  window[method]('blur', onDismiss)
}

watch(contextMenu, async (menu, previous) => {
  if (!menu) { listen(false); return }
  if (!previous) listen(true)
  position.value = { left: menu.x, top: menu.y }
  await nextTick()
  const rect = element.value?.getBoundingClientRect()
  if (!rect) return
  const margin = 8
  position.value = {
    left: Math.max(margin, Math.min(menu.x, window.innerWidth - rect.width - margin)),
    top: Math.max(margin, menu.y + rect.height > window.innerHeight - margin ? menu.y - rect.height : menu.y),
  }
  if (menu.keyboard) items()[0]?.focus()
  else element.value?.focus()
})

onUnmounted(() => listen(false))
</script>

<template>
  <Teleport to="body">
    <div v-if="contextMenu" ref="element" class="context-menu" role="menu" tabindex="-1" :aria-label="contextMenu.label" :style="{ left: `${position.left}px`, top: `${position.top}px` }" @keydown="onKeydown" @contextmenu.prevent>
      <template v-for="item in contextMenu.items" :key="item.label">
        <div v-if="item.separated" class="context-menu-separator" role="separator"></div>
        <button type="button" role="menuitem" :class="{ danger: item.danger }" :disabled="item.disabled" @click="choose(item)">
          <component :is="item.icon" v-if="item.icon" :size="14" />{{ item.label }}
        </button>
      </template>
    </div>
  </Teleport>
</template>

<style>
.context-menu {
  position: fixed; outline: none; z-index: 100; min-width: 168px; padding: var(--space-1); border-radius: var(--radius-md);
  background: var(--color-panel); box-shadow: var(--shadow-lg);
}
.context-menu button {
  display: flex; align-items: center; gap: var(--space-2); width: 100%; height: 30px; padding: 0 var(--space-2);
  border-radius: var(--radius-sm); color: var(--color-text); font-size: var(--text-xs); text-align: left;
}
.context-menu button svg { color: var(--color-text-tertiary); }
.context-menu button:hover:not(:disabled), .context-menu button:focus-visible { background: var(--color-hover); box-shadow: none; }
.context-menu button:focus { outline: none; }
.context-menu button:focus:not(:hover) { background: var(--color-hover); }
.context-menu button:disabled { opacity: .45; cursor: default; }
.context-menu button.danger, .context-menu button.danger svg { color: var(--color-danger, #dc2626); }
.context-menu-separator { height: 1px; margin: var(--space-1) var(--space-1); background: var(--color-border-subtle); }
</style>

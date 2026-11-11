import { shallowRef, type Component } from 'vue'

export interface MenuItem {
  label: string
  icon?: Component
  danger?: boolean
  disabled?: boolean
  separated?: boolean
  action: () => unknown
}

export interface MenuState { x: number; y: number; label: string; items: MenuItem[]; invoker: HTMLElement | null; keyboard: boolean }

export const contextMenu = shallowRef<MenuState | null>(null)

// Opens at the pointer, or below the focused element for the keyboard context-menu key / Shift+F10.
export function openContextMenu(event: MouseEvent, label: string, items: MenuItem[]) {
  event.preventDefault()
  event.stopPropagation()
  const target = event.currentTarget as HTMLElement
  let { clientX: x, clientY: y } = event
  const keyboard = !x && !y
  if (keyboard) {
    const rect = target.getBoundingClientRect()
    x = rect.left + 16
    y = rect.top + Math.min(rect.height, 32)
  }
  const focused = document.activeElement instanceof HTMLElement && target.contains(document.activeElement) ? document.activeElement : null
  contextMenu.value = { x, y, label, items, invoker: focused, keyboard }
}

export function closeContextMenu(restoreFocus = false) {
  const invoker = contextMenu.value?.invoker
  contextMenu.value = null
  if (restoreFocus) invoker?.focus()
}

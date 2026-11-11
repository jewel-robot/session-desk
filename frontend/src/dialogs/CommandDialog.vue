<script setup lang="ts">
import { X } from '@lucide/vue'
import { commandText, commandTitle, dialogs } from '../store'
import { useModal } from './useModal'

const { element, onClose } = useModal(() => dialogs.command, value => { dialogs.command = value })
function backdrop(event: MouseEvent) { if (event.target === element.value) dialogs.command = false }
</script>

<template>
  <dialog ref="element" class="command-dialog" @close="onClose" @click="backdrop">
    <div class="dialog-header"><div><h2>续接命令</h2><p>{{ commandTitle }}</p></div><button class="icon-button" aria-label="关闭续接命令" @click="dialogs.command = false"><X :size="18" /></button></div>
    <pre class="command-content" tabindex="0">{{ commandText }}</pre>
    <div class="dialog-footer"><button class="button secondary" @click="dialogs.command = false">关闭</button></div>
  </dialog>
</template>

<style>
.command-dialog { width: min(680px, calc(100vw - 32px)); }
.command-content {
  margin: 0 var(--space-5) var(--space-4); padding: var(--space-3) var(--space-4); max-height: 50vh; overflow: auto;
  border-radius: var(--radius-lg); background: var(--color-inset); box-shadow: inset 0 0 0 1px var(--color-border-subtle);
  font-size: var(--text-xs); line-height: 22px; white-space: pre-wrap; overflow-wrap: anywhere;
}
</style>

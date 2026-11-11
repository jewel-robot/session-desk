<script setup lang="ts">
import { AlertCircle, Check, X } from '@lucide/vue'
import { toast } from '../store'
</script>

<template>
  <div v-if="toast" class="toast" :class="{ 'toast-error': toast.error }" role="status"><AlertCircle v-if="toast.error" :size="15" /><Check v-else :size="15" />{{ toast.text }}<button class="icon-button small" aria-label="关闭提示" @click="toast = null"><X :size="13" /></button></div>
</template>

<style>
.toast {
  position: fixed; z-index: 60; bottom: var(--space-6); left: 50%; transform: translateX(-50%);
  display: flex; align-items: center; gap: var(--space-2); max-width: calc(100vw - 40px); padding: var(--space-2) var(--space-2) var(--space-2) var(--space-4);
  border-radius: var(--radius-lg); background: var(--color-toast); color: var(--color-toast-fg); box-shadow: var(--shadow-lg);
  font-size: var(--text-sm); animation: toast-in var(--duration) var(--ease);
}
.toast .icon-button { color: inherit; opacity: .7; }
.toast .icon-button:hover { background: rgba(127, 127, 127, .2); color: inherit; opacity: 1; }
.toast-error { background: var(--color-toast-error); color: var(--color-toast-error-fg); }
@keyframes toast-in { from { opacity: 0; transform: translate(-50%, 8px); } }
</style>

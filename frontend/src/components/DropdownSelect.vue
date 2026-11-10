<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import { Check, ChevronDown } from 'lucide-vue-next'

const props = defineProps<{
  modelValue: string
  options: { value: string; label: string }[]
  ariaLabel?: string
}>()
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

const open = ref(false)
const activeIndex = ref(-1)
const root = ref<HTMLElement>()
const trigger = ref<HTMLButtonElement>()
const list = ref<HTMLElement>()

const selectedLabel = computed(() => props.options.find(option => option.value === props.modelValue)?.label ?? '')

function toggle() {
  if (open.value) {
    close()
    return
  }
  open.value = true
  activeIndex.value = Math.max(0, props.options.findIndex(option => option.value === props.modelValue))
  void nextTick(() => list.value?.focus())
}

function choose(value: string) {
  emit('update:modelValue', value)
  close()
  trigger.value?.focus()
}

function close() {
  open.value = false
}

function onKeydown(event: KeyboardEvent) {
  if (!open.value) {
    if (event.key === 'ArrowDown' || event.key === 'Enter' || event.key === ' ') {
      event.preventDefault()
      toggle()
    }
    return
  }
  if (event.key === 'Escape') {
    event.preventDefault()
    close()
    trigger.value?.focus()
  } else if (event.key === 'ArrowDown') {
    event.preventDefault()
    activeIndex.value = Math.min(props.options.length - 1, activeIndex.value + 1)
  } else if (event.key === 'ArrowUp') {
    event.preventDefault()
    activeIndex.value = Math.max(0, activeIndex.value - 1)
  } else if (event.key === 'Home') {
    event.preventDefault()
    activeIndex.value = 0
  } else if (event.key === 'End') {
    event.preventDefault()
    activeIndex.value = props.options.length - 1
  } else if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    const option = props.options[activeIndex.value]
    if (option) choose(option.value)
  }
}

function onOutside(event: MouseEvent) {
  if (!root.value?.contains(event.target as Node)) close()
}

onMounted(() => document.addEventListener('click', onOutside))
onUnmounted(() => document.removeEventListener('click', onOutside))
</script>

<template>
  <div ref="root" class="select-menu" :class="{ open }">
    <button ref="trigger" type="button" class="select-trigger" :aria-label="ariaLabel" aria-haspopup="listbox" :aria-expanded="open" @click.stop="toggle" @keydown="onKeydown">
      <slot name="prefix" />
      <span class="select-value">{{ selectedLabel }}</span>
      <ChevronDown :size="12" class="select-chevron" />
    </button>
    <ul v-if="open" ref="list" class="select-list" role="listbox" tabindex="-1" :aria-label="ariaLabel" @keydown="onKeydown">
      <li v-for="(option, index) in options" :key="option.value || 'all'" role="option" :aria-selected="option.value === modelValue" :class="{ active: index === activeIndex, selected: option.value === modelValue }" @click.stop="choose(option.value)" @mouseenter="activeIndex = index">
        <span>{{ option.label }}</span>
        <Check v-if="option.value === modelValue" :size="14" />
      </li>
    </ul>
  </div>
</template>

<style>
.select-menu { position: relative; display: inline-flex; }
.select-trigger {
  display: inline-flex; align-items: center; gap: var(--space-1-5); height: var(--control-h-sm); padding: 0 var(--space-2);
  border-radius: var(--radius-sm); color: var(--color-text-secondary); font-size: var(--text-xs); font-weight: var(--weight-medium);
  box-shadow: inset 0 0 0 1px var(--color-border); background: var(--color-panel);
  transition: background-color var(--duration-fast) var(--ease), box-shadow var(--duration-fast) var(--ease);
}
.select-trigger:hover { background: var(--color-hover); color: var(--color-text); }
.select-menu.open .select-trigger { box-shadow: inset 0 0 0 1px var(--color-border-strong); color: var(--color-text); }
.select-trigger .provider-icon { width: 14px; height: 14px; }
.select-value { white-space: nowrap; }
.select-chevron { color: var(--color-text-tertiary); transition: transform var(--duration) var(--ease); }
.select-menu.open .select-chevron { transform: rotate(180deg); }
.select-list {
  position: absolute; z-index: 30; top: calc(100% + 4px); left: 0; min-width: max(100%, 168px); max-height: 260px; overflow-y: auto;
  margin: 0; padding: var(--space-1); list-style: none; outline: none;
  background: var(--color-panel); border-radius: var(--radius-lg); box-shadow: var(--shadow-md); animation: pop-in var(--duration-fast) var(--ease);
}
.select-list li { display: flex; align-items: center; justify-content: space-between; gap: var(--space-3); height: 30px; padding: 0 var(--space-2); border-radius: var(--radius-sm); color: var(--color-text-secondary); font-size: var(--text-sm); white-space: nowrap; cursor: pointer; }
.select-list li.active { background: var(--color-hover); color: var(--color-text); }
.select-list li.selected { color: var(--color-text); font-weight: var(--weight-medium); }
.select-list li svg { color: var(--color-accent); }
</style>

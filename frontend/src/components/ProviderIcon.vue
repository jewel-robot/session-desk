<script setup lang="ts">
import { computed } from 'vue'
import { agents } from '../agents'
const props = defineProps<{ provider: string }>()
const agent = computed(() => agents.find(item => item.id === props.provider))
</script>

<template>
  <span v-if="agent" class="provider-icon" :class="provider" :data-icon="agent.icon" aria-hidden="true">
    <span v-if="agent.mono" class="agent-mask" :style="{ '--agent-icon': `url('/agents/${agent.icon}.svg')` }"></span>
    <img v-else :src="`/agents/${agent.icon}.svg`" alt="" draggable="false" />
  </span>
</template>

<style>
.provider-icon { display: inline-flex; align-items: center; justify-content: center; flex-shrink: 0; width: 24px; height: 24px; }
.provider-icon > * { display: block; width: 100%; height: 100%; object-fit: contain; }
.provider-icon[data-icon='codex-color'] > * { width: 86%; height: 86%; }
.agent-mask { background: currentColor; mask: var(--agent-icon) center / contain no-repeat; -webkit-mask: var(--agent-icon) center / contain no-repeat; }
</style>

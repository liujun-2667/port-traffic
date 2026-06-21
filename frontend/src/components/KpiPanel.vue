<script setup lang="ts">
import { computed } from 'vue'
import type { Frame } from '../api/types'

const props = defineProps<{ frame: Frame | null }>()

const cards = computed(() => {
  const k = props.frame?.kpi
  return [
    { label: '在港船舶', value: k?.inPort ?? 0, unit: '艘', color: 'text-glow-cyan' },
    { label: '等待队列', value: k?.queueLength ?? 0, unit: '艘', color: 'text-glow-amber' },
    { label: '拥堵航段', value: k?.congestedSegments ?? 0, unit: '段', color: 'text-glow-amber' },
    { label: '累计危险会遇', value: k?.cumDangerous ?? 0, unit: '次', color: 'text-glow-red' },
    { label: '碰撞预警', value: k?.cumWarnings ?? 0, unit: '次', color: 'text-glow-red' },
    { label: '进港 / 出港', value: `${k?.throughputIn ?? 0} / ${k?.throughputOut ?? 0}`, unit: '船次', color: 'text-slate-100' },
    { label: '平均等待', value: (k?.avgWait ?? 0).toFixed(0), unit: 'min', color: 'text-slate-100' },
    { label: '最大等待', value: k?.maxWait ?? 0, unit: 'min', color: 'text-slate-100' }
  ]
})

const tide = computed(() => ({
  level: props.frame?.tideLevel ?? 0,
  depth: props.frame?.navigableDepth ?? 0
}))
</script>

<template>
  <div class="grid grid-cols-2 gap-2 sm:grid-cols-4">
    <div v-for="c in cards" :key="c.label" class="panel px-3 py-2">
      <div class="panel-title">{{ c.label }}</div>
      <div class="mt-1 flex items-baseline gap-1">
        <span class="font-mono text-xl font-semibold" :class="c.color">{{ c.value }}</span>
        <span class="text-[10px] text-slate-500">{{ c.unit }}</span>
      </div>
    </div>
    <div class="panel px-3 py-2 col-span-2 sm:col-span-1">
      <div class="panel-title">潮位 / 可航水深</div>
      <div class="mt-1 flex items-baseline gap-1">
        <span class="font-mono text-xl font-semibold text-glow-cyan">{{ tide.level.toFixed(2) }}</span>
        <span class="text-[10px] text-slate-500">m</span>
        <span class="font-mono text-base text-slate-300">/ {{ tide.depth.toFixed(1) }}m</span>
      </div>
    </div>
  </div>
</template>

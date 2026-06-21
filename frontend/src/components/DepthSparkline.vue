<script setup lang="ts">
import { computed } from 'vue'
import type { ChannelStatus } from '../api/types'

const props = defineProps<{
  channel: ChannelStatus
  width?: number
  height?: number
}>()

const emit = defineEmits<{
  click: []
}>()

const W = computed(() => props.width || 120)
const H = computed(() => props.height || 32)

const MONTHS = 12

const thresholdDepth = computed(() => props.channel.restrictedDraft * 1.3)

const dataPoints = computed(() => {
  const points: { month: number; date: Date; depth: number }[] = []
  const now = new Date()
  for (let i = 0; i <= MONTHS; i++) {
    const d = new Date(now)
    d.setMonth(d.getMonth() + i)
    const monthsElapsed = i
    const depth = props.channel.currentEffectiveDepth - monthsElapsed * props.channel.decayRate
    points.push({ month: i, date: d, depth: Math.max(0, depth) })
  }
  return points
})

const minDepth = computed(() => {
  const depths = dataPoints.value.map((p) => p.depth)
  return Math.min(thresholdDepth.value - 0.5, ...depths)
})

const maxDepth = computed(() => {
  return Math.max(props.channel.currentEffectiveDepth + 0.5, props.channel.baseDepth)
})

function yToPx(depth: number): number {
  const range = maxDepth.value - minDepth.value
  if (range <= 0) return H.value / 2
  return H.value - ((depth - minDepth.value) / range) * (H.value - 4) - 2
}

function xToPx(month: number): number {
  return (month / MONTHS) * W.value
}

const linePath = computed(() => {
  return dataPoints.value
    .map((p, i) => `${i === 0 ? 'M' : 'L'} ${xToPx(p.month).toFixed(1)} ${yToPx(p.depth).toFixed(1)}`)
    .join(' ')
})

const areaPath = computed(() => {
  const thresholdY = yToPx(thresholdDepth.value)
  const linePart = dataPoints.value
    .map((p, i) => `${i === 0 ? 'M' : 'L'} ${xToPx(p.month).toFixed(1)} ${Math.max(yToPx(p.depth), thresholdY).toFixed(1)}`)
    .join(' ')
  const lastX = xToPx(MONTHS)
  return `${linePart} L ${lastX.toFixed(1)} ${thresholdY.toFixed(1)} L ${xToPx(0).toFixed(1)} ${thresholdY.toFixed(1)} Z`
})

const belowThreshold = computed(() => {
  return dataPoints.value.some((p) => p.depth < thresholdDepth.value)
})
</script>

<template>
  <svg
    :width="W"
    :height="H"
    class="cursor-pointer transition hover:brightness-125"
    :viewBox="`0 0 ${W} ${H}`"
    @click="emit('click')"
  >
    <defs>
      <linearGradient :id="'dangerGrad-' + channel.segmentId" x1="0%" y1="0%" x2="0%" y2="100%">
        <stop offset="0%" stop-color="#ef4444" stop-opacity="0.6" />
        <stop offset="100%" stop-color="#ef4444" stop-opacity="0.1" />
      </linearGradient>
    </defs>

    <line
      :x1="0"
      :x2="W"
      :y1="yToPx(thresholdDepth)"
      :y2="yToPx(thresholdDepth)"
      stroke="#f59e0b"
      stroke-width="1"
      stroke-dasharray="3,2"
      opacity="0.5"
    />

    <path
      v-if="belowThreshold"
      :d="areaPath"
      :fill="'url(#dangerGrad-' + channel.segmentId + ')'"
    />

    <path
      :d="linePath"
      fill="none"
      stroke="#38bdf8"
      stroke-width="1.5"
      stroke-linecap="round"
      stroke-linejoin="round"
    />

    <circle
      :cx="xToPx(0)"
      :cy="yToPx(channel.currentEffectiveDepth)"
      r="2"
      fill="#38bdf8"
    />
  </svg>
</template>

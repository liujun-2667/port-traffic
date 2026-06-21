<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, watch } from 'vue'
import type { BatchStatus, DredgingBatch } from '../api/types'

interface GanttBatch {
  id: number
  name: string
  segmentId: string
  status: BatchStatus
  startDate: Date
  endDate: Date
  targetDepth: number
}

const props = defineProps<{
  batches: DredgingBatch[]
  channels: { segmentId: string }[]
}>()

const scrollContainer = ref<HTMLDivElement | null>(null)
const viewportWidth = ref(1000)

const zoomLevel = ref(1)
const ZOOM_LEVELS = [
  { unit: 'day', pxPerUnit: 20, label: '天' },
  { unit: 'day', pxPerUnit: 40, label: '天' },
  { unit: 'week', pxPerUnit: 80, label: '周' },
  { unit: 'month', pxPerUnit: 120, label: '月' },
  { unit: 'month', pxPerUnit: 200, label: '月' }
]

const STATUS_COLOR: Record<BatchStatus, string> = {
  planned: '#38bdf8',
  ongoing: '#d946ef',
  completed: '#64748b'
}

const today = computed(() => {
  const d = new Date()
  d.setHours(0, 0, 0, 0)
  return d
})

const rangeEnd = computed(() => {
  const d = new Date(today.value)
  d.setMonth(d.getMonth() + 6)
  return d
})

const totalDays = computed(() => {
  return Math.ceil((rangeEnd.value.getTime() - today.value.getTime()) / (1000 * 60 * 60 * 24))
})

const currentZoom = computed(() => ZOOM_LEVELS[zoomLevel.value])

const pxPerDay = computed(() => {
  const z = currentZoom.value
  if (z.unit === 'day') return z.pxPerUnit
  if (z.unit === 'week') return z.pxPerUnit / 7
  return z.pxPerUnit / 30
})

const timelineWidth = computed(() => totalDays.value * pxPerDay.value)

const ganttBatches = computed<GanttBatch[]>(() => {
  const result: GanttBatch[] = []
  for (const b of props.batches) {
    const start = new Date(b.plannedStartDate)
    start.setHours(0, 0, 0, 0)
    const end = new Date(start)
    end.setDate(end.getDate() + b.estimatedDurationDays)
    for (const seg of b.segments) {
      result.push({
        id: b.id,
        name: b.name,
        segmentId: seg.segmentId,
        status: b.status,
        startDate: start,
        endDate: end,
        targetDepth: b.targetDepth
      })
    }
  }
  return result
})

const batchesByChannel = computed(() => {
  const map = new Map<string, GanttBatch[]>()
  for (const b of ganttBatches.value) {
    if (!map.has(b.segmentId)) map.set(b.segmentId, [])
    map.get(b.segmentId)!.push(b)
  }
  return map
})

const hoveredBatch = ref<GanttBatch | null>(null)
const hoverPos = ref({ x: 0, y: 0 })

function dateToX(d: Date): number {
  const diff = d.getTime() - today.value.getTime()
  return Math.max(0, (diff / (1000 * 60 * 60 * 24)) * pxPerDay.value)
}

function getBatchStyle(b: GanttBatch) {
  const left = dateToX(b.startDate)
  const width = Math.max(pxPerDay.value * 2, dateToX(b.endDate) - left)
  return {
    left: `${left}px`,
    width: `${width}px`,
    backgroundColor: STATUS_COLOR[b.status]
  }
}

function showTooltip(b: GanttBatch, e: MouseEvent) {
  hoveredBatch.value = b
  const container = scrollContainer.value
  if (container) {
    const rect = container.getBoundingClientRect()
    hoverPos.value = {
      x: e.clientX - rect.left + container.scrollLeft,
      y: e.clientY - rect.top
    }
  }
}

function hideTooltip() {
  hoveredBatch.value = null
}

function onWheel(e: WheelEvent) {
  if (!e.ctrlKey && !e.metaKey) return
  e.preventDefault()
  const delta = e.deltaY > 0 ? -1 : 1
  zoomLevel.value = Math.max(0, Math.min(ZOOM_LEVELS.length - 1, zoomLevel.value + delta))
}

function zoomIn() {
  zoomLevel.value = Math.min(ZOOM_LEVELS.length - 1, zoomLevel.value + 1)
}

function zoomOut() {
  zoomLevel.value = Math.max(0, zoomLevel.value - 1)
}

const headerLabels = computed(() => {
  const labels: { x: number; text: string; isMajor: boolean }[] = []
  const z = currentZoom.value
  const d = new Date(today.value)

  if (z.unit === 'day') {
    while (d <= rangeEnd.value) {
      const x = dateToX(d)
      const isMajor = d.getDate() === 1 || d.getDay() === 0
      labels.push({
        x,
        text: `${d.getMonth() + 1}/${d.getDate()}`,
        isMajor
      })
      d.setDate(d.getDate() + 1)
    }
  } else if (z.unit === 'week') {
    while (d <= rangeEnd.value) {
      const x = dateToX(d)
      const isMajor = d.getDate() === 1
      labels.push({
        x,
        text: `${d.getMonth() + 1}/${d.getDate()}`,
        isMajor
      })
      d.setDate(d.getDate() + 7)
    }
  } else {
    while (d <= rangeEnd.value) {
      const x = dateToX(d)
      labels.push({
        x,
        text: `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}`,
        isMajor: true
      })
      d.setMonth(d.getMonth() + 1)
    }
  }
  return labels
})

const monthLines = computed(() => {
  const lines: number[] = []
  const d = new Date(today.value)
  d.setDate(1)
  while (d <= rangeEnd.value) {
    lines.push(dateToX(d))
    d.setMonth(d.getMonth() + 1)
  }
  return lines
})

function fmtDate(d: Date) {
  return d.toLocaleDateString('zh-CN')
}

let wheelHandler: ((e: WheelEvent) => void) | null = null

onMounted(() => {
  wheelHandler = (e: WheelEvent) => {
    if (e.target instanceof HTMLElement && scrollContainer.value?.contains(e.target)) {
      onWheel(e)
    }
  }
  window.addEventListener('wheel', wheelHandler, { passive: false })
})

onBeforeUnmount(() => {
  if (wheelHandler) {
    window.removeEventListener('wheel', wheelHandler)
  }
})

watch(() => props.batches, () => {}, { deep: true })
</script>

<template>
  <div class="flex h-full min-h-0 flex-col rounded-lg border border-slate-700/60 bg-navy-950/60">
    <div class="flex items-center justify-between border-b border-slate-700/50 px-3 py-2">
      <div class="flex items-center gap-3">
        <span class="text-sm font-medium text-slate-200">疏浚批次时间线</span>
        <div class="flex items-center gap-2 text-[10px]">
          <span class="flex items-center gap-1">
            <span class="inline-block h-2.5 w-2.5 rounded-sm" :style="{ backgroundColor: STATUS_COLOR.planned }"></span>
            <span class="text-slate-400">计划中</span>
          </span>
          <span class="flex items-center gap-1">
            <span class="inline-block h-2.5 w-2.5 rounded-sm" :style="{ backgroundColor: STATUS_COLOR.ongoing }"></span>
            <span class="text-slate-400">进行中</span>
          </span>
          <span class="flex items-center gap-1">
            <span class="inline-block h-2.5 w-2.5 rounded-sm" :style="{ backgroundColor: STATUS_COLOR.completed }"></span>
            <span class="text-slate-400">已完成</span>
          </span>
        </div>
      </div>
      <div class="flex items-center gap-1">
        <span class="text-[10px] text-slate-500">缩放: {{ currentZoom.label }}</span>
        <button
          class="rounded border border-slate-600/60 px-1.5 py-0.5 text-xs text-slate-300 hover:bg-slate-700/40"
          @click="zoomOut"
          :disabled="zoomLevel === 0"
        >−</button>
        <button
          class="rounded border border-slate-600/60 px-1.5 py-0.5 text-xs text-slate-300 hover:bg-slate-700/40"
          @click="zoomIn"
          :disabled="zoomLevel === ZOOM_LEVELS.length - 1"
        >+</button>
      </div>
    </div>

    <div class="relative flex min-h-0 flex-1 overflow-hidden">
      <div class="flex-shrink-0 border-r border-slate-700/50 bg-navy-900/80">
        <div class="h-8 border-b border-slate-700/50"></div>
        <div
          v-for="c in channels"
          :key="c.segmentId"
          class="flex h-9 items-center border-b border-slate-800/40 px-3 text-[11px] font-mono text-slate-300"
        >{{ c.segmentId }}</div>
      </div>

      <div
        ref="scrollContainer"
        class="relative min-h-0 flex-1 overflow-x-auto overflow-y-hidden"
        @mouseleave="hideTooltip"
      >
        <div class="relative" :style="{ width: timelineWidth + 'px', minWidth: '100%' }">
          <div class="sticky top-0 z-10 h-8 border-b border-slate-700/50 bg-navy-900/95">
            <div class="relative h-full">
              <div
                v-for="(m, i) in monthLines"
                :key="'ml-' + i"
                class="absolute top-0 h-full border-l border-slate-700/50"
                :style="{ left: m + 'px' }"
              ></div>
              <div
                v-for="(l, i) in headerLabels"
                :key="'hl-' + i"
                class="absolute top-1 whitespace-nowrap text-[10px]"
                :class="l.isMajor ? 'text-slate-300 font-medium' : 'text-slate-500'"
                :style="{ left: l.x + 2 + 'px' }"
              >{{ l.text }}</div>
            </div>
          </div>

          <div class="relative">
            <div
              v-for="(m, i) in monthLines"
              :key="'mlb-' + i"
              class="pointer-events-none absolute top-0 border-l border-slate-800/40"
              :style="{ left: m + 'px', height: (channels.length * 36) + 'px' }"
            ></div>

            <div
              v-for="c in channels"
              :key="'row-' + c.segmentId"
              class="relative h-9 border-b border-slate-800/40"
            >
              <div
                v-for="b in (batchesByChannel.get(c.segmentId) || [])"
                :key="'b-' + b.id + '-' + b.segmentId"
                class="absolute top-1.5 h-6 cursor-pointer rounded-md shadow-md transition hover:brightness-110 hover:shadow-lg"
                :style="getBatchStyle(b)"
                @mouseenter="showTooltip(b, $event)"
                @mousemove="showTooltip(b, $event)"
                @mouseleave="hideTooltip"
              >
                <div class="truncate px-2 pt-0.5 text-[10px] font-medium text-white/90">
                  {{ b.name }}
                </div>
              </div>
            </div>
          </div>
        </div>

        <div
          v-if="hoveredBatch"
          class="pointer-events-none absolute z-50 min-w-[180px] rounded-md border border-slate-600/60 bg-navy-900/95 px-3 py-2 text-[11px] shadow-xl backdrop-blur-sm"
          :style="{
            left: Math.min(hoverPos.x + 12, viewportWidth - 200) + 'px',
            top: hoverPos.y + 12 + 'px'
          }"
        >
          <div class="mb-1 font-medium text-slate-200">{{ hoveredBatch.name }}</div>
          <div class="space-y-0.5 text-slate-400">
            <div>航道: <span class="font-mono text-slate-200">{{ hoveredBatch.segmentId }}</span></div>
            <div>起止: <span class="text-slate-200">{{ fmtDate(hoveredBatch.startDate) }} ~ {{ fmtDate(hoveredBatch.endDate) }}</span></div>
            <div>目标水深: <span class="font-mono text-slate-200">{{ hoveredBatch.targetDepth.toFixed(2) }}m</span></div>
            <div>
              状态:
              <span
                class="ml-1 inline-block rounded px-1 py-0.5 text-[9px]"
                :style="{ backgroundColor: STATUS_COLOR[hoveredBatch.status] + '30', color: STATUS_COLOR[hoveredBatch.status] }"
              >
                {{ hoveredBatch.status === 'planned' ? '计划中' : hoveredBatch.status === 'ongoing' ? '进行中' : '已完成' }}
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div class="border-t border-slate-700/50 px-3 py-1.5 text-[10px] text-slate-500">
      提示: 按住 Ctrl/Cmd + 鼠标滚轮可缩放时间轴 · 当前粒度: {{ currentZoom.label }} · 范围: {{ fmtDate(today) }} ~ {{ fmtDate(rangeEnd) }}
    </div>
  </div>
</template>

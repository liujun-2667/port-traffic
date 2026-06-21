<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import * as echarts from 'echarts'
import { onMounted, onBeforeUnmount } from 'vue'
import type { ChannelStatus, DredgingBatch } from '../api/types'

const props = defineProps<{
  visible: boolean
  channel: ChannelStatus | null
  batches: DredgingBatch[]
}>()

const emit = defineEmits<{
  close: []
}>()

const chartEl = ref<HTMLDivElement | null>(null)
let chart: echarts.ECharts | null = null

const MONTHS = 12

const thresholdDepth = computed(() => {
  if (!props.channel) return 0
  return props.channel.restrictedDraft * 1.3
})

const historicalBatches = computed(() => {
  if (!props.channel) return []
  return props.batches
    .filter((b) => b.segments.some((s) => s.segmentId === props.channel!.segmentId) && b.status === 'completed')
    .sort((a, b) => new Date(a.plannedStartDate).getTime() - new Date(b.plannedStartDate).getTime())
})

function buildOption() {
  if (!props.channel) return {}
  const ch = props.channel
  const now = new Date()
  const startDate = new Date(ch.lastDredgedAt)
  if (startDate > now) startDate.setTime(now.getTime())

  const futurePoints: { date: string; depth: number }[] = []
  for (let i = 0; i <= MONTHS; i++) {
    const d = new Date(now)
    d.setMonth(d.getMonth() + i)
    const monthsElapsed = i
    const depth = ch.currentEffectiveDepth - monthsElapsed * ch.decayRate
    futurePoints.push({
      date: d.toISOString().slice(0, 7),
      depth: Math.max(0, depth)
    })
  }

  const historicalPoints: { date: string; depth: number }[] = []
  const lastDredge = new Date(ch.lastDredgedAt)
  const monthsSinceLast = Math.max(0, (now.getTime() - lastDredge.getTime()) / (1000 * 60 * 60 * 24 * 30))
  for (let i = 0; i <= Math.ceil(monthsSinceLast); i++) {
    const d = new Date(lastDredge)
    d.setMonth(d.getMonth() + i)
    if (d > now) break
    const depth = ch.baseDepth - i * ch.decayRate
    historicalPoints.push({
      date: d.toISOString().slice(0, 7),
      depth: Math.max(0, depth)
    })
  }
  historicalPoints.push({
    date: now.toISOString().slice(0, 7),
    depth: ch.currentEffectiveDepth
  })

  const allDates = [...historicalPoints.map((p) => p.date), ...futurePoints.map((p) => p.date)]
  const markLines: any[] = []
  for (const b of historicalBatches.value) {
    const d = new Date(b.plannedStartDate)
    const dateStr = d.toISOString().slice(0, 7)
    markLines.push({
      xAxis: dateStr,
      lineStyle: { color: '#10b981', type: 'dashed', width: 2 },
      label: {
        formatter: `疏浚: ${b.targetDepth.toFixed(1)}m`,
        position: 'start',
        color: '#10b981',
        fontSize: 10
      }
    })
  }

  const allDepths = [...historicalPoints.map(p => p.depth), ...futurePoints.map(p => p.depth), thresholdDepth.value, ch.baseDepth]
  const minD = Math.min(...allDepths) - 0.5
  const maxD = Math.max(...allDepths) + 0.5

  return {
    backgroundColor: 'transparent',
    tooltip: {
      trigger: 'axis',
      backgroundColor: 'rgba(15, 23, 42, 0.95)',
      borderColor: '#334155',
      textStyle: { color: '#e2e8f0' }
    },
    legend: {
      data: ['历史水深', '预测水深', '疏浚阈值'],
      textStyle: { color: '#94a3b8', fontSize: 11 },
      top: 5
    },
    grid: { left: 50, right: 20, top: 40, bottom: 40 },
    xAxis: {
      type: 'category',
      data: allDates,
      axisLabel: { color: '#64748b', fontSize: 10, rotate: 30 },
      axisLine: { lineStyle: { color: '#334155' } }
    },
    yAxis: {
      type: 'value',
      name: '水深(m)',
      nameTextStyle: { color: '#64748b', fontSize: 11 },
      min: Math.max(0, minD),
      max: maxD,
      axisLabel: { color: '#64748b', fontSize: 10 },
      splitLine: { lineStyle: { color: '#1e293b' } }
    },
    series: [
      {
        name: '历史水深',
        type: 'line',
        smooth: true,
        data: historicalPoints.map((p) => [p.date, p.depth]),
        lineStyle: { color: '#38bdf8', width: 2 },
        itemStyle: { color: '#38bdf8' },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(56, 189, 248, 0.25)' },
            { offset: 1, color: 'rgba(56, 189, 248, 0.02)' }
          ])
        }
      },
      {
        name: '预测水深',
        type: 'line',
        smooth: true,
        data: futurePoints.map((p) => [p.date, p.depth]),
        lineStyle: { color: '#d946ef', width: 2, type: 'dashed' },
        itemStyle: { color: '#d946ef' },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(217, 70, 239, 0.2)' },
            { offset: 1, color: 'rgba(217, 70, 239, 0.02)' }
          ])
        },
        markLine: {
          silent: true,
          symbol: 'none',
          data: markLines.length > 0 ? markLines : undefined
        }
      },
      {
        name: '疏浚阈值',
        type: 'line',
        data: allDates.map((d) => [d, thresholdDepth.value]),
        lineStyle: { color: '#f59e0b', width: 1.5, type: 'dashed' },
        itemStyle: { opacity: 0 },
        symbol: 'none',
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(239, 68, 68, 0)' },
            { offset: 1, color: 'rgba(239, 68, 68, 0.25)' }
          ])
        }
      }
    ]
  }
}

function render() {
  if (chart && props.visible) {
    chart.setOption(buildOption(), true)
    chart.resize()
  }
}

watch(
  () => [props.visible, props.channel, props.batches],
  () => {
    setTimeout(render, 50)
  },
  { deep: true }
)

onMounted(() => {
  if (chartEl.value) {
    chart = echarts.init(chartEl.value, 'dark')
    window.addEventListener('resize', () => chart?.resize())
  }
})

onBeforeUnmount(() => {
  chart?.dispose()
  chart = null
})
</script>

<template>
  <Teleport to="body">
    <div
      v-if="visible"
      class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
      @click.self="emit('close')"
    >
      <div class="flex w-[720px] max-w-[90vw] flex-col rounded-lg border border-slate-700/60 bg-navy-950 shadow-2xl">
        <div class="flex items-center justify-between border-b border-slate-700/50 px-4 py-3">
          <div>
            <h3 class="text-sm font-semibold text-slate-200">
              航道淤积趋势预测
            </h3>
            <p v-if="channel" class="mt-0.5 text-xs text-slate-500">
              <span class="font-mono text-slate-400">{{ channel.segmentId }}</span>
              · 衰减率 <span class="font-mono">{{ channel.decayRate.toFixed(3) }}m/月</span>
              · 疏浚阈值 <span class="font-mono text-amber-300">{{ thresholdDepth.toFixed(2) }}m</span>
            </p>
          </div>
          <button
            class="rounded px-2 py-1 text-slate-400 hover:bg-slate-700/40 hover:text-slate-200"
            @click="emit('close')"
          >✕</button>
        </div>

        <div ref="chartEl" class="h-[380px] w-full"></div>

        <div class="border-t border-slate-700/50 px-4 py-2 text-[11px] text-slate-500">
          <div class="flex flex-wrap gap-x-4 gap-y-1">
            <span class="flex items-center gap-1.5">
              <span class="inline-block h-1 w-4 rounded bg-sky-400"></span>
              历史水深 (实测/推算)
            </span>
            <span class="flex items-center gap-1.5">
              <span class="inline-block h-1 w-4 rounded border border-dashed border-fuchsia-400"></span>
              预测水深 (线性外推)
            </span>
            <span class="flex items-center gap-1.5">
              <span class="inline-block h-1 w-4 rounded border border-dashed border-amber-400"></span>
              疏浚阈值 (限制吃水 × 1.3)
            </span>
            <span class="flex items-center gap-1.5">
              <span class="inline-block h-3 w-0.5 border-l border-dashed border-emerald-400"></span>
              历史疏浚记录
            </span>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>

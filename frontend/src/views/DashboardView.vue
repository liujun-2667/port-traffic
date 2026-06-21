<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useSimStore } from '../stores/sim'
import { api } from '../api/client'
import type { TidePoint } from '../api/types'
import PortCanvas from '../components/PortCanvas.vue'
import KpiPanel from '../components/KpiPanel.vue'
import ControlBar from '../components/ControlBar.vue'
import BaseChart from '../components/BaseChart.vue'

const store = useSimStore()
const router = useRouter()
const tideSeries = ref<TidePoint[]>([])

onMounted(async () => {
  if (!store.config) await store.loadConfig().catch(() => {})
  try {
    const t = await api.getTide(24)
    tideSeries.value = t.series
  } catch {
    /* tide optional */
  }
})

const throughputOption = computed(() => {
  const data = store.throughputHist
  return {
    backgroundColor: 'transparent',
    grid: { left: 36, right: 12, top: 24, bottom: 24 },
    tooltip: { trigger: 'axis' },
    legend: { data: ['进港', '出港'], textStyle: { color: '#94a3b8' }, top: 0, right: 8, itemWidth: 10, itemHeight: 6 },
    xAxis: { type: 'category', data: data.map((d) => d.minute), axisLabel: { color: '#64748b', fontSize: 10 }, name: 'min', nameTextStyle: { color: '#64748b' } },
    yAxis: { type: 'value', axisLabel: { color: '#64748b', fontSize: 10 }, splitLine: { lineStyle: { color: 'rgba(148,163,184,0.1)' } } },
    series: [
      { name: '进港', type: 'line', smooth: true, symbol: 'none', data: data.map((d) => d.in), lineStyle: { color: '#22c55e' }, areaStyle: { color: 'rgba(34,197,94,0.12)' } },
      { name: '出港', type: 'line', smooth: true, symbol: 'none', data: data.map((d) => d.out), lineStyle: { color: '#00e5c7' }, areaStyle: { color: 'rgba(0,229,199,0.12)' } }
    ]
  }
})

const waitBuckets = computed(() => {
  const waits = (store.frame?.ships ?? [])
    .map((s) => s.waitMinutes)
    .filter((w) => w > 0)
  const edges = [0, 10, 20, 30, 45, 60, 90, 120]
  const labels = ['<10', '10-20', '20-30', '30-45', '45-60', '60-90', '90-120', '>120']
  const counts = new Array(labels.length).fill(0)
  for (const w of waits) {
    let idx = labels.length - 1
    for (let i = 0; i < edges.length; i++) {
      if (w < edges[i]) { idx = i; break }
    }
    counts[idx]++
  }
  return { labels, counts }
})

const waitOption = computed(() => {
  const b = waitBuckets.value
  return {
    backgroundColor: 'transparent',
    grid: { left: 30, right: 12, top: 16, bottom: 28 },
    tooltip: { trigger: 'axis' },
    xAxis: { type: 'category', data: b.labels, axisLabel: { color: '#64748b', fontSize: 9 } },
    yAxis: { type: 'value', axisLabel: { color: '#64748b', fontSize: 10 }, splitLine: { lineStyle: { color: 'rgba(148,163,184,0.1)' } } },
    series: [{ type: 'bar', data: b.counts, itemStyle: { color: '#ffb547' } }]
  }
})

const radarMetrics = computed(() => {
  const k = store.frame?.kpi
  const segs = store.frame?.segmentCongestion ?? []
  const avgCong = segs.length ? segs.reduce((s, c) => s + c.congestion, 0) / segs.length : 0
  const congestion = Math.min(1, avgCong)
  const wait = Math.min(1, (k?.avgWait ?? 0) / 60)
  const risk = Math.min(1, (k?.cumDangerous ?? 0) / 30)
  const expected = Math.max(1, (store.frame?.minute ?? 0) / 60 * (store.config?.sim.arrivalRate ?? 3))
  const efficiency = Math.min(1, (k?.throughputIn ?? 0) / expected)
  return [
    { name: '拥堵度', value: congestion * 100 },
    { name: '等待时间', value: wait * 100 },
    { name: '会遇风险', value: risk * 100 },
    { name: '通行效率', value: efficiency * 100 }
  ]
})

const radarOption = computed(() => {
  const m = radarMetrics.value
  return {
    backgroundColor: 'transparent',
    radar: {
      indicator: m.map((x) => ({ name: x.name, max: 100 })),
      axisName: { color: '#94a3b8', fontSize: 10 },
      splitArea: { areaStyle: { color: ['rgba(0,229,199,0.03)', 'rgba(0,229,199,0.06)'] } },
      splitLine: { lineStyle: { color: 'rgba(148,163,184,0.2)' } },
      axisLine: { lineStyle: { color: 'rgba(148,163,184,0.2)' } }
    },
    series: [
      {
        type: 'radar',
        data: [{ value: m.map((x) => x.value), name: '当前', areaStyle: { color: 'rgba(0,229,199,0.25)' }, lineStyle: { color: '#00e5c7' } }]
      }
    ]
  }
})

const tideOption = computed(() => {
  const series = tideSeries.value
  const curMin = store.frame?.minute ?? 0
  return {
    backgroundColor: 'transparent',
    grid: { left: 32, right: 12, top: 16, bottom: 24 },
    tooltip: { trigger: 'axis' },
    xAxis: { type: 'category', data: series.map((p) => p.t), axisLabel: { color: '#64748b', fontSize: 9 } },
    yAxis: { type: 'value', axisLabel: { color: '#64748b', fontSize: 10 }, splitLine: { lineStyle: { color: 'rgba(148,163,184,0.1)' } } },
    series: [
      {
        type: 'line', smooth: true, symbol: 'none', data: series.map((p) => p.level),
        lineStyle: { color: '#3b82f6' }, areaStyle: { color: 'rgba(59,130,246,0.15)' },
        markLine: curMin ? { symbol: 'none', data: [{ xAxis: curMin }] as any, lineStyle: { color: '#ffb547' }, label: { formatter: '当前', color: '#ffb547' } } : undefined
      }
    ]
  }
})

function viewReport() {
  if (store.runId) router.push(`/report/${store.runId}`)
}
</script>

<template>
  <div class="flex h-full flex-col gap-3 p-3">
    <ControlBar />
    <KpiPanel :frame="store.frame" />
    <div class="grid min-h-0 flex-1 grid-cols-1 gap-3 lg:grid-cols-3">
      <div class="panel relative overflow-hidden lg:col-span-2">
        <div class="absolute left-3 top-3 z-10 panel-title">港口实时仿真俯视图</div>
        <PortCanvas :config="store.config" :frame="store.frame" />
      </div>
      <div class="flex min-h-0 flex-col gap-3">
        <div class="panel p-3">
          <div class="panel-title mb-1">吞吐量随时间</div>
          <BaseChart :option="throughputOption" height="200px" />
        </div>
        <div class="panel p-3">
          <div class="panel-title mb-1">等待时间分布 (min)</div>
          <BaseChart :option="waitOption" height="180px" />
        </div>
      </div>
    </div>
    <div class="grid grid-cols-1 gap-3 lg:grid-cols-3">
      <div class="panel p-3">
        <div class="panel-title mb-1">安全评估雷达</div>
        <BaseChart :option="radarOption" height="220px" />
      </div>
      <div class="panel p-3">
        <div class="panel-title mb-1">潮位曲线 (24h)</div>
        <BaseChart :option="tideOption" height="220px" />
      </div>
      <div class="panel flex flex-col p-3">
        <div class="panel-title mb-2">仿真状态</div>
        <div class="space-y-1.5 text-sm text-slate-300">
          <div class="flex justify-between"><span class="text-slate-500">运行 ID</span><span class="font-mono">{{ store.runId ?? '—' }}</span></div>
          <div class="flex justify-between"><span class="text-slate-500">仿真分钟</span><span class="font-mono">{{ store.frame?.minute ?? 0 }}</span></div>
          <div class="flex justify-between"><span class="text-slate-500">状态</span>
            <span v-if="store.done" class="text-glow-cyan">已完成</span>
            <span v-else-if="store.playing" class="text-glow-cyan animate-pulse">运行中</span>
            <span v-else-if="store.connecting" class="text-glow-amber">连接中</span>
            <span v-else class="text-slate-400">待启动</span>
          </div>
        </div>
        <button v-if="store.done" class="btn mt-auto self-start" @click="viewReport">查看评估报告 →</button>
      </div>
    </div>
  </div>
</template>

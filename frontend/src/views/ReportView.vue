<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api } from '../api/client'
import type { Report, RunMeta, SchedulingStrategy } from '../api/types'
import BaseChart from '../components/BaseChart.vue'

const props = defineProps<{ runId: string | string[] }>()
const id = computed(() => Number(Array.isArray(props.runId) ? props.runId[0] : props.runId))

const report = ref<Report | null>(null)
const error = ref('')
const loading = ref(true)

const allRuns = ref<RunMeta[]>([])
const compareRunId = ref<number | null>(null)
const compareReport = ref<Report | null>(null)
const compareLoading = ref(false)
const compareError = ref('')

const STRATEGY_LABEL: Record<SchedulingStrategy, string> = {
  free_flow: '自由通行',
  tidal_window: '潮汐窗口调度',
  alternating_one_way: '单向交替通行'
}

onMounted(async () => {
  try {
    report.value = await api.getReport(id.value)
  } catch (e: any) {
    error.value = e.message || String(e)
  } finally {
    loading.value = false
  }
  try {
    allRuns.value = (await api.listRuns()).filter((r) => r.id !== id.value).reverse()
  } catch {
    /* ignore */
  }
})

async function loadCompareReport() {
  if (!compareRunId.value) {
    compareReport.value = null
    return
  }
  compareLoading.value = true
  compareError.value = ''
  try {
    compareReport.value = await api.getReport(compareRunId.value)
  } catch (e: any) {
    compareError.value = e.message || String(e)
    compareReport.value = null
  } finally {
    compareLoading.value = false
  }
}

const metricRows = computed(() => {
  if (!report.value) return []
  const m = report.value.metrics
  return [
    { label: '进港船次', value: m.throughputIn },
    { label: '出港船次', value: m.throughputOut },
    { label: '总吞吐量', value: m.totalThroughput },
    { label: '平均等待时间 (min)', value: m.avgWaitMinutes.toFixed(1) },
    { label: '最大等待时间 (min)', value: m.maxWaitMinutes },
    { label: '严重延误 ( >60min ) 船次', value: m.severeDelayCount },
    { label: '危险会遇次数', value: m.dangerousEncounters },
    { label: '碰撞预警次数', value: m.collisionWarnings }
  ]
})

const priorityColor: Record<string, string> = {
  高: 'text-glow-red border-glow-red/40',
  中: 'text-glow-amber border-glow-amber/40',
  低: 'text-glow-cyan border-glow-cyan/40'
}

const compareOption = computed(() => {
  if (!report.value || !compareReport.value) return null
  const cur = report.value.metrics
  const cmp = compareReport.value.metrics
  const curLabel = `Run #${id.value} · ${STRATEGY_LABEL[report.value.summary.strategy?.strategy] || '未知'}`
  const cmpLabel = `Run #${compareRunId.value} · ${STRATEGY_LABEL[compareReport.value.summary.strategy?.strategy] || '未知'}`
  return {
    backgroundColor: 'transparent',
    tooltip: { trigger: 'axis' },
    legend: {
      data: [curLabel, cmpLabel],
      textStyle: { color: '#94a3b8', fontSize: 11 },
      top: 0,
      right: 8
    },
    grid: { left: 60, right: 12, top: 36, bottom: 32 },
    xAxis: {
      type: 'category',
      data: ['总吞吐量 (艘)', '平均等待时间 (min)', '危险会遇次数'],
      axisLabel: { color: '#cbd5e1', fontSize: 11 }
    },
    yAxis: {
      type: 'value',
      axisLabel: { color: '#64748b', fontSize: 10 },
      splitLine: { lineStyle: { color: 'rgba(148,163,184,0.1)' } }
    },
    series: [
      {
        name: curLabel,
        type: 'bar',
        data: [cur.totalThroughput, cur.avgWaitMinutes, cur.dangerousEncounters],
        itemStyle: { color: '#00e5c7' },
        barWidth: '28%',
        label: { show: true, position: 'top', color: '#cbd5e1', fontSize: 10, formatter: (p: any) => Number(p.value).toFixed(p.dataIndex === 1 ? 1 : 0) }
      },
      {
        name: cmpLabel,
        type: 'bar',
        data: [cmp.totalThroughput, cmp.avgWaitMinutes, cmp.dangerousEncounters],
        itemStyle: { color: '#ffb547' },
        barWidth: '28%',
        label: { show: true, position: 'top', color: '#cbd5e1', fontSize: 10, formatter: (p: any) => Number(p.value).toFixed(p.dataIndex === 1 ? 1 : 0) }
      }
    ]
  }
})

function exportJSON() {
  if (!report.value) return
  const blob = new Blob([JSON.stringify(report.value, null, 2)], { type: 'application/json' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = `port-traffic-report-${id.value}.json`
  a.click()
  URL.revokeObjectURL(url)
}
</script>

<template>
  <div class="mx-auto max-w-5xl space-y-4 p-4">
    <header class="flex items-center justify-between">
      <div>
        <h2 class="text-lg font-semibold">仿真评估报告 · RUN #{{ id }}</h2>
        <p v-if="report" class="text-xs text-slate-500">
          {{ report.summary.durationMinutes }} 分钟 · 到达率 {{ report.summary.arrivalRate }} 艘/h · 种子 {{ report.summary.seed }} · 风 {{ report.summary.windSpeed }} 节 · 能见度 {{ report.summary.visibility }} 海里
          <span v-if="report.summary.strategy" class="ml-2 text-glow-cyan">[{{ STRATEGY_LABEL[report.summary.strategy.strategy] || report.summary.strategy.strategy }}]</span>
        </p>
      </div>
      <button class="btn" :disabled="!report" @click="exportJSON">导出 JSON</button>
    </header>

    <p v-if="loading" class="text-sm text-glow-amber animate-pulse">生成报告中…</p>
    <p v-if="error" class="text-sm text-glow-red">{{ error }}</p>

    <template v-if="report">
      <section class="panel p-4">
        <h3 class="panel-title mb-3">安全指标统计</h3>
        <div class="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <div v-for="r in metricRows" :key="r.label" class="rounded-md border border-glow-cyan/10 bg-navy-950/50 px-3 py-2">
            <div class="text-[10px] uppercase tracking-wider text-slate-500">{{ r.label }}</div>
            <div class="mt-1 font-mono text-lg text-glow-cyan">{{ r.value }}</div>
          </div>
        </div>
      </section>

      <section class="panel p-4">
        <h3 class="panel-title mb-3">策略对比分析</h3>
        <div class="mb-3 flex flex-wrap items-center gap-3">
          <div class="flex flex-col">
            <label class="label">选择对比仿真</label>
            <select v-model.number="compareRunId" class="input w-64" @change="loadCompareReport">
              <option :value="null">-- 选择历史仿真记录 --</option>
              <option v-for="r in allRuns" :key="r.id" :value="r.id">RUN #{{ r.id }} · {{ r.status }} · {{ new Date(r.startedAt).toLocaleString() }}</option>
            </select>
          </div>
          <span v-if="compareLoading" class="text-sm text-glow-amber animate-pulse">加载中…</span>
          <span v-if="compareError" class="text-sm text-glow-red">{{ compareError }}</span>
        </div>
        <div v-if="compareOption" class="h-80">
          <BaseChart :option="compareOption" height="320px" />
        </div>
        <div v-else class="rounded-md border border-dashed border-glow-cyan/20 p-8 text-center text-sm text-slate-500">
          选择另一个仿真记录进行策略对比,将以并排柱状图展示吞吐量、平均等待时间、危险会遇次数三项指标的差异。
        </div>
      </section>

      <div class="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <section class="panel p-4">
          <h3 class="panel-title mb-3">瓶颈航段识别 (TOP3)</h3>
          <div v-if="!report.bottlenecks.length" class="text-sm text-slate-500">无显著瓶颈。</div>
          <div v-for="b in report.bottlenecks" :key="b.segId" class="mb-2 flex items-center justify-between rounded-md border bg-navy-950/40 px-3 py-2" :class="priorityColor[b.priority] || 'border-glow-cyan/10'">
            <div>
              <div class="font-mono text-sm">{{ b.segId }} · 优先级 {{ b.priority }}</div>
              <div class="text-xs text-slate-500">平均拥堵 {{ (b.avgCongestion * 100).toFixed(0) }}% · 峰值 {{ (b.peakCongestion * 100).toFixed(0) }}%</div>
            </div>
            <span class="text-xs text-slate-400">扩容优先级 {{ b.rank }}</span>
          </div>
        </section>

        <section class="panel p-4">
          <h3 class="panel-title mb-3">调度建议</h3>
          <div v-if="!report.advice.length" class="text-sm text-glow-cyan">当前运行状况良好,无需额外调度。</div>
          <ul class="space-y-2">
            <li v-for="a in report.advice" :key="a.code" class="rounded-md border border-glow-amber/30 bg-glow-amber/5 px-3 py-2 text-sm">
              <span class="font-mono text-xs text-glow-amber">[{{ a.code }}]</span>
              <span class="ml-2 text-slate-200">{{ a.text }}</span>
            </li>
          </ul>
        </section>
      </div>

      <section class="panel p-4">
        <h3 class="panel-title mb-3">关键事件时间线</h3>
        <div v-if="!report.events.length" class="text-sm text-slate-500">本次仿真未记录关键事件。</div>
        <ol v-else class="relative space-y-2 border-l border-glow-red/30 pl-4">
          <li v-for="(e, i) in report.events" :key="i" class="text-sm">
            <span class="font-mono text-xs text-glow-cyan">{{ e.clock }}</span>
            <span class="mx-2 text-slate-300">{{ e.type }}</span>
            <span class="text-slate-400">{{ e.shipA }} ⇄ {{ e.shipB }}</span>
            <span class="ml-2 text-slate-500">{{ e.desc }}</span>
          </li>
        </ol>
      </section>

      <section class="panel p-4">
        <h3 class="panel-title mb-3">各航段平均拥堵度</h3>
        <div class="space-y-1.5">
          <div v-for="s in report.metrics.segmentCongestion" :key="s.segId" class="flex items-center gap-2 text-sm">
            <span class="w-10 font-mono text-glow-cyan">{{ s.segId }}</span>
            <div class="h-2 flex-1 overflow-hidden rounded bg-navy-950">
              <div class="h-full rounded" :class="s.avgCongestion > 0.7 ? 'bg-glow-red' : s.avgCongestion > 0.4 ? 'bg-glow-amber' : 'bg-glow-cyan'" :style="{ width: Math.min(100, s.avgCongestion * 100) + '%' }"></div>
            </div>
            <span class="w-12 text-right font-mono text-slate-400">{{ (s.avgCongestion * 100).toFixed(0) }}%</span>
          </div>
        </div>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { api } from '../api/client'
import type {
  BatchConflict,
  BatchStatus,
  ChannelStatus,
  ConflictError,
  CostPreview,
  DredgingBatch,
  OptimizeResult,
  SedimentStatus
} from '../api/types'
import GanttChart from '../components/GanttChart.vue'
import DepthSparkline from '../components/DepthSparkline.vue'
import DepthTrendModal from '../components/DepthTrendModal.vue'

const channels = ref<ChannelStatus[]>([])
const batches = ref<DredgingBatch[]>([])
const loading = ref(false)
const error = ref('')

const selectedIds = ref<string[]>([])
const batchName = ref('')
const startDate = ref(new Date().toISOString().slice(0, 10))
const durationDays = ref(7)
const targetDepth = ref(15.0)
const notes = ref('')
const costPreview = ref<CostPreview | null>(null)
const saving = ref(false)

const budget = ref<number>(5000)
const optimizeResult = ref<OptimizeResult | null>(null)
const optimizing = ref(false)

// Conflict detection
const conflictDialogVisible = ref(false)
const pendingConflicts = ref<BatchConflict[]>([])

// Depth trend modal
const trendModalVisible = ref(false)
const selectedChannel = ref<ChannelStatus | null>(null)

const STATUS_LABEL: Record<SedimentStatus, { label: string; cls: string }> = {
  normal:       { label: '正常',      cls: 'bg-emerald-500/20 text-emerald-300 border-emerald-400/40' },
  warning:      { label: '预警',      cls: 'bg-amber-500/20 text-amber-300 border-amber-400/40' },
  needs_dredge: { label: '需要疏浚',  cls: 'bg-rose-500/25 text-rose-300 border-rose-400/40' }
}

const BATCH_STATUS_LABEL: Record<BatchStatus, { label: string; cls: string }> = {
  planned:   { label: '计划中', cls: 'bg-sky-500/20 text-sky-300 border-sky-400/40' },
  ongoing:   { label: '进行中', cls: 'bg-fuchsia-500/20 text-fuchsia-300 border-fuchsia-400/40' },
  completed: { label: '已完成', cls: 'bg-slate-500/30 text-slate-300 border-slate-400/40' }
}

// ---- Batch progress tracking ----
function getBatchProgress(b: DredgingBatch): { percent: number; remainingDays: number; isOverdue: boolean; overdueDays: number } {
  if (b.status !== 'ongoing') {
    return { percent: 0, remainingDays: 0, isOverdue: false, overdueDays: 0 }
  }
  const start = new Date(b.actualStartDate || b.plannedStartDate)
  start.setHours(0, 0, 0, 0)
  const now = new Date()
  now.setHours(0, 0, 0, 0)
  const end = new Date(start)
  end.setDate(end.getDate() + b.estimatedDurationDays)

  const elapsedMs = now.getTime() - start.getTime()
  const totalMs = end.getTime() - start.getTime()
  let percent = totalMs > 0 ? (elapsedMs / totalMs) * 100 : 0
  percent = Math.max(0, Math.min(100, percent))

  const remainingMs = end.getTime() - now.getTime()
  const remainingDays = remainingMs > 0 ? Math.ceil(remainingMs / (1000 * 60 * 60 * 24)) : 0
  const isOverdue = remainingMs < 0
  const overdueDays = isOverdue ? Math.ceil(-remainingMs / (1000 * 60 * 60 * 24)) : 0

  return { percent: Math.round(percent), remainingDays, isOverdue, overdueDays }
}

// ---- Friendly error mapping ----
function friendlyError(e: any, fallback: string): string {
  const status: number | undefined = e?.status
  const raw: string | undefined = e?.raw
  const msg: string = (e?.message ?? '').toString().trim()

  if (msg && msg.length <= 60 && !msg.startsWith('{') && !msg.includes('SQLSTATE') && !msg.includes('ERROR:')) {
    return msg
  }
  switch (status) {
    case 400:
    case 422:
      return '请求参数错误，请检查输入后重试'
    case 401:
    case 403:
      return '没有操作权限，请重新登录或联系管理员'
    case 404:
      return '请求的资源不存在，可能已被删除'
    case 409: {
      try {
        const data = JSON.parse(raw || '{}')
        if (data?.conflicts?.length > 0) {
          return `疏浚时间冲突：${data.conflicts.length} 个批次与当前计划重叠`
        }
        return data?.message || '操作冲突，请刷新数据后重试'
      } catch {
        return '操作冲突，请刷新数据后重试'
      }
    }
    case 500:
    case 502:
    case 503:
      return '服务器错误，请稍后再试；若持续出现请联系技术支持'
  }
  if (raw && (raw.includes('SQLSTATE') || raw.includes('ERROR:'))) {
    return '服务器数据操作异常，请稍后再试或联系技术支持'
  }
  if (!status && typeof window !== 'undefined' && typeof fetch === 'function' && !navigator.onLine) {
    return '网络连接已断开，请检查网络后重试'
  }
  return fallback || '请求失败，请重试'
}

async function loadAll() {
  loading.value = true
  error.value = ''
  try {
    const [ch, ba] = await Promise.all([api.listChannels(), api.listBatches()])
    channels.value = ch
    batches.value = ba
  } catch (e: any) {
    error.value = friendlyError(e, '加载航道维护数据失败，请刷新页面重试')
  } finally {
    loading.value = false
  }
}

function toggleSelect(id: string) {
  const idx = selectedIds.value.indexOf(id)
  if (idx >= 0) selectedIds.value.splice(idx, 1)
  else selectedIds.value.push(id)
}

function selectAllUrgent() {
  selectedIds.value = channels.value
    .filter((c) => c.status === 'needs_dredge' || c.status === 'warning')
    .map((c) => c.segmentId)
}

function clearSelection() {
  selectedIds.value = []
}

async function refreshPreview() {
  if (selectedIds.value.length === 0 || targetDepth.value <= 0) {
    costPreview.value = null
    return
  }
  try {
    costPreview.value = await api.costPreview(selectedIds.value, targetDepth.value)
  } catch (e: any) {
    costPreview.value = null
    error.value = friendlyError(e, '成本预览失败，请检查目标水深设置是否合理')
  }
}

watch([selectedIds, targetDepth], refreshPreview, { deep: true })

// ---- Conflict-aware batch creation ----
async function tryCreateBatch(allowConflict = false) {
  if (!batchName.value.trim()) {
    error.value = '请输入批次名称'
    return
  }
  if (selectedIds.value.length === 0) {
    error.value = '请至少选择一条航道'
    return
  }

  if (!allowConflict) {
    try {
      const checkResult = await api.checkConflicts(
        selectedIds.value,
        new Date(startDate.value).toISOString(),
        durationDays.value
      )
      if (checkResult.hasConflict) {
        const hasOngoing = checkResult.conflicts.some(c => c.status === 'ongoing')
        if (hasOngoing) {
          error.value = `存在进行中的批次冲突，无法创建。请检查航道 ${checkResult.conflicts.find(c => c.status === 'ongoing')?.segmentId || ''}`
          return
        }
        pendingConflicts.value = checkResult.conflicts
        conflictDialogVisible.value = true
        return
      }
    } catch (e: any) {
      // If conflict check fails, proceed anyway and let backend handle it
    }
  }

  saving.value = true
  error.value = ''
  try {
    await api.createBatch({
      name: batchName.value.trim(),
      segmentIds: selectedIds.value,
      plannedStartDate: new Date(startDate.value).toISOString(),
      estimatedDurationDays: durationDays.value,
      targetDepth: targetDepth.value,
      notes: notes.value.trim(),
      allowConflict: true
    })
    batchName.value = ''
    selectedIds.value = []
    notes.value = ''
    costPreview.value = null
    conflictDialogVisible.value = false
    pendingConflicts.value = []
    await loadAll()
  } catch (e: any) {
    if (e?.status === 409) {
      try {
        const data = JSON.parse(e?.raw || '{}') as ConflictError
        if (data?.conflicts?.length > 0) {
          pendingConflicts.value = data.conflicts
          const hasOngoing = data.conflicts.some(c => c.status === 'ongoing')
          if (!hasOngoing) {
            conflictDialogVisible.value = true
          }
        }
      } catch {
        // ignore parse error
      }
    }
    error.value = friendlyError(e, '创建疏浚批次失败，请稍后再试')
  } finally {
    saving.value = false
  }
}

function cancelConflictDialog() {
  conflictDialogVisible.value = false
  pendingConflicts.value = []
}

function confirmConflictCreate() {
  tryCreateBatch(true)
}

// ---- Sparkline click ----
function openTrendModal(channel: ChannelStatus) {
  selectedChannel.value = channel
  trendModalVisible.value = true
}

async function startBatch(id: number) {
  error.value = ''
  try {
    await api.startBatch(id)
    await loadAll()
  } catch (e: any) {
    error.value = friendlyError(e, '启动批次失败，请稍后再试')
  }
}

async function completeBatch(id: number) {
  error.value = ''
  try {
    await api.completeBatch(id)
    await loadAll()
  } catch (e: any) {
    error.value = friendlyError(e, '完成批次失败，请稍后再试')
  }
}

async function delBatch(id: number) {
  if (!confirm('确定删除此疏浚批次？')) return
  error.value = ''
  try {
    await api.deleteBatch(id)
    await loadAll()
  } catch (e: any) {
    error.value = friendlyError(e, '删除批次失败，请稍后再试')
  }
}

async function runOptimize() {
  if (budget.value <= 0) return
  optimizing.value = true
  error.value = ''
  try {
    optimizeResult.value = await api.optimize(budget.value)
  } catch (e: any) {
    error.value = friendlyError(e, '优化建议计算失败，请稍后再试')
  } finally {
    optimizing.value = false
  }
}

const selectedCount = computed(() => selectedIds.value.length)

function fmtDate(s: string) {
  if (!s) return '—'
  const d = new Date(s)
  return d.toLocaleDateString('zh-CN')
}

function fmtConflictDate(s: string) {
  return new Date(s).toLocaleDateString('zh-CN')
}

function dismissError() {
  error.value = ''
}

onMounted(() => {
  loadAll()
})
</script>

<template>
  <div class="flex h-full w-full flex-col gap-3 p-3 text-slate-200">
    <div class="flex items-center justify-between">
      <div>
        <h1 class="text-xl font-semibold tracking-wide text-glow-cyan">航道维护规划</h1>
        <p class="mt-0.5 text-xs text-slate-400">评估淤积对通行能力的影响 · 制定疏浚计划 · 成本估算与优化</p>
      </div>
      <button
        class="rounded-md border border-glow-cyan/40 bg-navy-900/60 px-3 py-1.5 text-xs text-glow-cyan hover:bg-glow-cyan/10"
        @click="loadAll"
      >刷新数据</button>
    </div>

    <div
      v-if="error"
      class="flex items-start justify-between gap-3 rounded-md border border-rose-500/40 bg-rose-500/10 px-3 py-2 text-xs text-rose-200"
    >
      <div class="flex items-start gap-2">
        <span class="mt-0.5 text-base leading-none text-rose-400">⚠</span>
        <div>
          <div class="font-medium text-rose-200">操作失败</div>
          <div class="mt-0.5 text-rose-300/90">{{ error }}</div>
        </div>
      </div>
      <button
        class="rounded px-1.5 py-0.5 text-slate-400 hover:bg-rose-500/10 hover:text-rose-200"
        @click="dismissError"
      >×</button>
    </div>

    <div class="grid min-h-0 flex-1 grid-cols-12 gap-3">
      <!-- LEFT: Channel list with sparklines -->
      <div class="col-span-5 flex min-h-0 flex-col rounded-lg border border-slate-700/60 bg-navy-950/60">
        <div class="flex items-center justify-between border-b border-slate-700/50 px-3 py-2">
          <div class="text-sm font-medium text-slate-200">航道淤积状态</div>
          <div class="flex gap-2">
            <button class="rounded border border-slate-600/60 px-2 py-0.5 text-xs text-slate-300 hover:bg-slate-700/40" @click="selectAllUrgent">
              全选需疏浚
            </button>
            <button class="rounded border border-slate-600/60 px-2 py-0.5 text-xs text-slate-300 hover:bg-slate-700/40" @click="clearSelection">
              清空
            </button>
          </div>
        </div>
        <div class="min-h-0 flex-1 overflow-auto">
          <table class="w-full text-left text-xs">
            <thead class="sticky top-0 z-10 bg-navy-900/95 text-slate-400">
              <tr>
                <th class="w-8 px-2 py-2"></th>
                <th class="px-2 py-2">航道</th>
                <th class="px-2 py-2 text-right">当前水深</th>
                <th class="px-2 py-2 text-right">阈值</th>
                <th class="px-2 py-2 text-right">衰减率</th>
                <th class="px-2 py-2 text-right">剩余天数</th>
                <th class="px-2 py-2">状态</th>
                <th class="px-2 py-2 text-center">淤积趋势</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="c in channels"
                :key="c.segmentId"
                class="cursor-pointer border-t border-slate-800/60 transition hover:bg-slate-800/30"
                :class="{ 'bg-glow-cyan/5': selectedIds.includes(c.segmentId) }"
                @click="toggleSelect(c.segmentId)"
              >
                <td class="px-2 py-2">
                  <input
                    type="checkbox"
                    class="h-3.5 w-3.5 accent-cyan-500"
                    :checked="selectedIds.includes(c.segmentId)"
                    @click.stop
                    @change="toggleSelect(c.segmentId)"
                  />
                </td>
                <td class="px-2 py-2 font-mono text-slate-200">{{ c.segmentId }}</td>
                <td class="px-2 py-2 text-right font-mono text-slate-200">{{ c.currentEffectiveDepth.toFixed(2) }}m</td>
                <td class="px-2 py-2 text-right font-mono text-slate-400">{{ c.thresholdDepth.toFixed(2) }}m</td>
                <td class="px-2 py-2 text-right font-mono text-slate-400">{{ c.decayRate.toFixed(3) }}m/月</td>
                <td class="px-2 py-2 text-right font-mono" :class="c.daysToThreshold < 90 ? 'text-amber-300' : (c.daysToThreshold === 0 ? 'text-rose-300' : 'text-slate-300')">
                  {{ c.daysToThreshold === 0 ? '已触发' : c.daysToThreshold + '天' }}
                </td>
                <td class="px-2 py-2">
                  <span class="rounded border px-1.5 py-0.5 text-[10px]" :class="STATUS_LABEL[c.status].cls">
                    {{ STATUS_LABEL[c.status].label }}
                  </span>
                </td>
                <td class="px-2 py-2 text-center" @click.stop>
                  <div class="flex justify-center">
                    <DepthSparkline
                      :channel="c"
                      :width="120"
                      :height="28"
                      @click="openTrendModal(c)"
                    />
                  </div>
                </td>
              </tr>
              <tr v-if="loading && channels.length === 0">
                <td colspan="8" class="px-4 py-10 text-center text-slate-500">正在加载航道状态...</td>
              </tr>
              <tr v-else-if="channels.length === 0">
                <td colspan="8" class="px-4 py-10 text-center text-slate-500">暂无数据</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- RIGHT: Batch editor + gantt + batch list + optimizer -->
      <div class="col-span-7 flex min-h-0 flex-col gap-3">
        <div class="grid grid-cols-2 gap-3">
          <!-- Create batch -->
          <div class="rounded-lg border border-slate-700/60 bg-navy-950/60">
            <div class="border-b border-slate-700/50 px-3 py-2 text-sm font-medium text-slate-200">
              创建疏浚批次 <span class="ml-2 text-xs text-slate-500">已选 {{ selectedCount }} 条航道</span>
            </div>
            <div class="grid grid-cols-12 gap-2 p-3">
              <div class="col-span-12">
                <label class="mb-1 block text-xs text-slate-400">批次名称</label>
                <input
                  v-model="batchName"
                  type="text"
                  placeholder="例: 2026-Q3 主航道维护"
                  class="w-full rounded-md border border-slate-700 bg-navy-900 px-2.5 py-1.5 text-sm text-slate-100 outline-none focus:border-glow-cyan/60"
                />
              </div>
              <div class="col-span-6">
                <label class="mb-1 block text-xs text-slate-400">计划开始日期</label>
                <input
                  v-model="startDate"
                  type="date"
                  class="w-full rounded-md border border-slate-700 bg-navy-900 px-2.5 py-1.5 text-sm text-slate-100 outline-none focus:border-glow-cyan/60"
                />
              </div>
              <div class="col-span-6">
                <label class="mb-1 block text-xs text-slate-400">预计工期(天)</label>
                <input
                  v-model.number="durationDays"
                  type="number"
                  min="1"
                  class="w-full rounded-md border border-slate-700 bg-navy-900 px-2.5 py-1.5 text-sm text-slate-100 outline-none focus:border-glow-cyan/60"
                />
              </div>
              <div class="col-span-6">
                <label class="mb-1 block text-xs text-slate-400">疏浚目标水深(米)</label>
                <input
                  v-model.number="targetDepth"
                  type="number"
                  step="0.1"
                  min="0"
                  class="w-full rounded-md border border-slate-700 bg-navy-900 px-2.5 py-1.5 text-sm text-slate-100 outline-none focus:border-glow-cyan/60"
                />
              </div>
              <div class="col-span-6">
                <label class="mb-1 block text-xs text-slate-400">备注</label>
                <input
                  v-model="notes"
                  type="text"
                  placeholder="选填"
                  class="w-full rounded-md border border-slate-700 bg-navy-900 px-2.5 py-1.5 text-sm text-slate-100 outline-none focus:border-glow-cyan/60"
                />
              </div>
            </div>

            <div v-if="costPreview" class="border-t border-slate-700/50 bg-navy-900/40 px-3 py-2">
              <div class="mb-1 flex items-end justify-between">
                <div class="text-xs text-slate-400">成本预览</div>
                <div class="text-sm font-semibold text-amber-300">
                  总计 <span class="font-mono">¥{{ costPreview.totalCost.toFixed(2) }}</span>
                  <span class="ml-1 text-[10px] text-slate-400">万元</span>
                </div>
              </div>
            </div>

            <div class="border-t border-slate-700/50 px-3 py-2 text-right">
              <button
                :disabled="saving || selectedCount === 0 || !batchName.trim()"
                class="rounded-md bg-glow-cyan/80 px-4 py-1.5 text-sm font-medium text-navy-950 shadow transition hover:bg-glow-cyan disabled:cursor-not-allowed disabled:opacity-40"
                @click="tryCreateBatch(false)"
              >{{ saving ? '创建中...' : '创建批次' }}</button>
            </div>
          </div>

          <!-- Gantt chart -->
          <GanttChart :batches="batches" :channels="channels" class="h-[260px]" />
        </div>

        <!-- History batches with progress -->
        <div class="flex min-h-0 flex-1 flex-col rounded-lg border border-slate-700/60 bg-navy-950/60">
          <div class="border-b border-slate-700/50 px-3 py-2 text-sm font-medium text-slate-200">历史疏浚批次</div>
          <div class="min-h-0 flex-1 overflow-auto">
            <table class="w-full text-left text-xs">
              <thead class="sticky top-0 z-10 bg-navy-900/95 text-slate-400">
                <tr>
                  <th class="px-2 py-2">#</th>
                  <th class="px-2 py-2">名称</th>
                  <th class="px-2 py-2">状态</th>
                  <th class="px-2 py-2 text-right">开始</th>
                  <th class="px-2 py-2 text-right">工期</th>
                  <th class="px-2 py-2 text-right">进度</th>
                  <th class="px-2 py-2 text-right">目标水深</th>
                  <th class="px-2 py-2 text-right">费用</th>
                  <th class="px-2 py-2">航道</th>
                  <th class="px-2 py-2">操作</th>
                </tr>
              </thead>
              <tbody class="text-slate-300">
                <tr v-for="b in batches" :key="b.id" class="border-t border-slate-800/60">
                  <td class="px-2 py-2 font-mono text-slate-500">{{ b.id }}</td>
                  <td class="px-2 py-2 text-slate-200">{{ b.name }}</td>
                  <td class="px-2 py-2">
                    <span class="rounded border px-1.5 py-0.5 text-[10px]" :class="BATCH_STATUS_LABEL[b.status].cls">
                      {{ BATCH_STATUS_LABEL[b.status].label }}
                    </span>
                  </td>
                  <td class="px-2 py-2 text-right font-mono">{{ fmtDate(b.plannedStartDate) }}</td>
                  <td class="px-2 py-2 text-right font-mono">{{ b.estimatedDurationDays }}天</td>
                  <td class="px-2 py-2">
                    <template v-if="b.status === 'ongoing'">
                      <div class="flex items-center gap-2">
                        <div class="relative h-2 w-24 overflow-hidden rounded-full bg-slate-700/60">
                          <div
                            class="h-full rounded-full transition-all"
                            :class="getBatchProgress(b).isOverdue ? 'bg-rose-500' : 'bg-emerald-500'"
                            :style="{ width: getBatchProgress(b).percent + '%' }"
                          ></div>
                        </div>
                        <span
                          class="font-mono text-[10px]"
                          :class="getBatchProgress(b).isOverdue ? 'text-rose-400' : 'text-slate-400'"
                        >
                          <template v-if="getBatchProgress(b).isOverdue">
                            已超期 {{ getBatchProgress(b).overdueDays }} 天
                          </template>
                          <template v-else>
                            {{ getBatchProgress(b).percent }}% · 剩 {{ getBatchProgress(b).remainingDays }} 天
                          </template>
                        </span>
                      </div>
                    </template>
                    <span v-else class="text-slate-600">—</span>
                  </td>
                  <td class="px-2 py-2 text-right font-mono">{{ b.targetDepth.toFixed(2) }}m</td>
                  <td class="px-2 py-2 text-right font-mono text-amber-300">¥{{ b.totalCost.toFixed(1) }}</td>
                  <td class="px-2 py-2 font-mono text-slate-400">
                    {{ b.segments.map(s => s.segmentId).join(', ') }}
                  </td>
                  <td class="px-2 py-2">
                    <div class="flex gap-1.5">
                      <button
                        v-if="b.status === 'planned'"
                        class="rounded border border-sky-500/40 bg-sky-500/10 px-2 py-0.5 text-[11px] text-sky-300 hover:bg-sky-500/20"
                        @click="startBatch(b.id)"
                      >开始</button>
                      <button
                        v-if="b.status === 'ongoing'"
                        class="rounded border border-emerald-500/40 bg-emerald-500/10 px-2 py-0.5 text-[11px] text-emerald-300 hover:bg-emerald-500/20"
                        @click="completeBatch(b.id)"
                      >完成</button>
                      <button
                        v-if="b.status !== 'completed'"
                        class="rounded border border-rose-500/40 bg-rose-500/10 px-2 py-0.5 text-[11px] text-rose-300 hover:bg-rose-500/20"
                        @click="delBatch(b.id)"
                      >删除</button>
                    </div>
                  </td>
                </tr>
                <tr v-if="batches.length === 0">
                  <td colspan="10" class="px-4 py-8 text-center text-slate-500">暂无批次记录</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>

        <!-- Optimizer -->
        <div class="rounded-lg border border-slate-700/60 bg-navy-950/60">
          <div class="flex items-center justify-between border-b border-slate-700/50 px-3 py-2">
            <div class="text-sm font-medium text-slate-200">预算优化建议 · 贪心策略 (紧迫度/成本比)</div>
          </div>
          <div class="flex gap-3 p-3">
            <div class="flex flex-1 items-center gap-2">
              <label class="text-xs text-slate-400">年度预算上限(万元)</label>
              <input
                v-model.number="budget"
                type="number"
                min="0"
                class="w-36 rounded-md border border-slate-700 bg-navy-900 px-2.5 py-1.5 text-sm font-mono text-slate-100 outline-none focus:border-glow-cyan/60"
              />
              <button
                :disabled="optimizing || budget <= 0"
                class="rounded-md bg-fuchsia-500/80 px-4 py-1.5 text-sm font-medium text-white shadow transition hover:bg-fuchsia-500 disabled:opacity-40"
                @click="runOptimize"
              >{{ optimizing ? '计算中...' : '生成推荐' }}</button>
            </div>
            <div v-if="optimizeResult" class="flex items-center text-xs text-slate-400">
              预算: <span class="ml-1 font-mono text-slate-200">¥{{ optimizeResult.budget.toFixed(0) }}</span>
              · 预计花费: <span class="ml-1 font-mono text-amber-300">¥{{ optimizeResult.totalSpent.toFixed(2) }}</span>
              · 推荐 <span class="ml-1 font-mono text-fuchsia-300">{{ optimizeResult.recommendations.filter(r => !r.overBudget).length }}</span> 条
            </div>
          </div>
          <div v-if="optimizeResult" class="border-t border-slate-700/50">
            <table class="w-full text-xs">
              <thead class="bg-navy-900/60 text-slate-400">
                <tr>
                  <th class="px-2 py-1.5">序号</th>
                  <th class="px-2 py-1.5">航道</th>
                  <th class="px-2 py-1.5 text-right">剩余天数</th>
                  <th class="px-2 py-1.5 text-right">紧迫度</th>
                  <th class="px-2 py-1.5 text-right">紧迫度/成本</th>
                  <th class="px-2 py-1.5 text-right">当前→目标(m)</th>
                  <th class="px-2 py-1.5 text-right">花费(万元)</th>
                  <th class="px-2 py-1.5 text-right">累计花费</th>
                </tr>
              </thead>
              <tbody class="text-slate-300">
                <tr
                  v-for="r in optimizeResult.recommendations"
                  :key="r.rank"
                  class="border-t border-slate-800/50"
                  :class="{ 'opacity-40': r.overBudget, 'bg-emerald-500/5': !r.overBudget }"
                >
                  <td class="px-2 py-1.5 font-mono text-slate-500">{{ r.rank }}</td>
                  <td class="px-2 py-1.5 font-mono text-slate-200">
                    {{ r.segmentId }}
                    <span v-if="r.overBudget" class="ml-1 text-[10px] text-slate-500">(超预算)</span>
                  </td>
                  <td class="px-2 py-1.5 text-right font-mono" :class="r.daysLeft < 90 ? 'text-amber-300' : (r.daysLeft === 0 ? 'text-rose-300' : '')">
                    {{ r.daysLeft === 0 ? '已触发' : r.daysLeft }}
                  </td>
                  <td class="px-2 py-1.5 text-right font-mono text-fuchsia-300">{{ r.urgency.toFixed(4) }}</td>
                  <td class="px-2 py-1.5 text-right font-mono text-fuchsia-200">{{ r.urgencyCostRatio.toFixed(4) }}</td>
                  <td class="px-2 py-1.5 text-right font-mono">{{ r.currentDepth.toFixed(2) }} → {{ r.targetDepth.toFixed(2) }}</td>
                  <td class="px-2 py-1.5 text-right font-mono text-amber-300">{{ r.cost.toFixed(2) }}</td>
                  <td class="px-2 py-1.5 text-right font-mono" :class="r.overBudget ? 'text-rose-300 line-through' : 'text-amber-200'">
                    {{ r.cumulativeCost.toFixed(2) }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>

    <!-- Conflict Confirmation Dialog -->
    <Teleport to="body">
      <div
        v-if="conflictDialogVisible"
        class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm"
        @click.self="cancelConflictDialog"
      >
        <div class="flex w-[520px] max-w-[90vw] flex-col rounded-lg border border-amber-500/40 bg-navy-950 shadow-2xl">
          <div class="flex items-center justify-between border-b border-slate-700/50 px-4 py-3">
            <div class="flex items-center gap-2">
              <span class="text-lg text-amber-400">⚠</span>
              <h3 class="text-sm font-semibold text-amber-300">疏浚时间冲突警告</h3>
            </div>
            <button
              class="rounded px-2 py-1 text-slate-400 hover:bg-slate-700/40 hover:text-slate-200"
              @click="cancelConflictDialog"
            >✕</button>
          </div>

          <div class="px-4 py-3">
            <p class="mb-3 text-sm text-slate-300">
              以下批次与当前创建计划的时间段存在重叠，是否仍要创建？
            </p>
            <div class="max-h-[240px] overflow-auto rounded-md border border-slate-700/50 bg-navy-900/40">
              <table class="w-full text-[11px]">
                <thead class="bg-navy-900/80 text-slate-400">
                  <tr>
                    <th class="px-2 py-1.5 text-left">批次名称</th>
                    <th class="px-2 py-1.5 text-left">航道</th>
                    <th class="px-2 py-1.5 text-left">已有时间范围</th>
                    <th class="px-2 py-1.5 text-right">重叠天数</th>
                  </tr>
                </thead>
                <tbody class="text-slate-300">
                  <tr v-for="(c, i) in pendingConflicts" :key="i" class="border-t border-slate-800/50">
                    <td class="px-2 py-1.5 text-slate-200">{{ c.batchName }}</td>
                    <td class="px-2 py-1.5 font-mono">{{ c.segmentId }}</td>
                    <td class="px-2 py-1.5 font-mono text-slate-400">
                      {{ fmtConflictDate(c.existingStart) }} ~ {{ fmtConflictDate(c.existingEnd) }}
                    </td>
                    <td class="px-2 py-1.5 text-right font-mono text-amber-400">{{ c.overlapDays }} 天</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <div class="flex justify-end gap-2 border-t border-slate-700/50 px-4 py-3">
            <button
              class="rounded-md border border-slate-600/60 bg-navy-900/60 px-4 py-1.5 text-sm text-slate-300 hover:bg-slate-700/40"
              @click="cancelConflictDialog"
            >取消</button>
            <button
              class="rounded-md bg-amber-500/80 px-4 py-1.5 text-sm font-medium text-navy-950 shadow transition hover:bg-amber-500"
              @click="confirmConflictCreate"
            >仍要创建</button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Depth Trend Modal -->
    <DepthTrendModal
      :visible="trendModalVisible"
      :channel="selectedChannel"
      :batches="batches"
      @close="trendModalVisible = false"
    />
  </div>
</template>

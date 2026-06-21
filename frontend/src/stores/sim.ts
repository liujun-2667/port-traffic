import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { api, streamRun } from '../api/client'
import type { AppConfig, Frame, RunParams, TimelineEvent } from '../api/types'

export const useSimStore = defineStore('sim', () => {
  const config = ref<AppConfig | null>(null)
  const runId = ref<number | null>(null)
  const frame = ref<Frame | null>(null)
  const rate = ref(1)
  const playing = ref(false)
  const connecting = ref(false)
  const done = ref(false)
  const error = ref('')
  const throughputHist = ref<{ minute: number; in: number; out: number }[]>([])
  const waitMinutes = ref<number[]>([])
  const eventLog = ref<TimelineEvent[]>([])
  const seenEventKeys = ref<Set<string>>(new Set())

  let stopStream: (() => void) | null = null

  const kpi = computed(() => frame.value?.kpi ?? null)

  async function loadConfig() {
    config.value = await api.getConfig()
    return config.value
  }

  async function updateConfig(patch: { sim?: Partial<AppConfig['sim']>; weather?: Partial<AppConfig['weather']> }) {
    config.value = await api.putConfig(patch)
    return config.value
  }

  async function startRun(params: RunParams) {
    disconnect()
    frame.value = null
    throughputHist.value = []
    waitMinutes.value = []
    eventLog.value = []
    seenEventKeys.value = new Set()
    done.value = false
    error.value = ''
    connecting.value = true
    const { runId: id } = await api.startRun(params)
    runId.value = id
    playing.value = true
    rate.value = params.speedFactor || 1
    connect(id)
    return id
  }

  function connect(id: number) {
    connecting.value = true
    stopStream = streamRun(
      id,
      (f) => {
        frame.value = f
        connecting.value = false
        if (f.throughput && f.throughput.length) {
          throughputHist.value = f.throughput
        }
        if (f.events && f.events.length) {
          for (const ev of f.events) {
            const key = `${ev.minute}|${ev.type}|${ev.shipA}|${ev.shipB}|${ev.desc}`
            if (!seenEventKeys.value.has(key)) {
              seenEventKeys.value.add(key)
              eventLog.value.push(ev)
              if (eventLog.value.length > 200) {
                eventLog.value = eventLog.value.slice(eventLog.value.length - 200)
              }
            }
          }
        }
        if (f.done) {
          done.value = true
          playing.value = false
          disconnect()
        }
      },
      () => {
        error.value = '实时连接中断'
        connecting.value = false
      }
    )
  }

  async function control(action: 'pause' | 'resume' | 'set_rate' | 'reset', newRate?: number) {
    if (runId.value == null) return
    await api.controlRun(runId.value, action, newRate ?? rate.value)
    if (action === 'set_rate' && newRate) rate.value = newRate
    if (action === 'pause') playing.value = false
    if (action === 'resume') playing.value = true
  }

  function disconnect() {
    if (stopStream) {
      stopStream()
      stopStream = null
    }
    connecting.value = false
  }

  function setWaitDistribution(values: number[]) {
    waitMinutes.value = values
  }

  return {
    config,
    runId,
    frame,
    rate,
    playing,
    connecting,
    done,
    error,
    throughputHist,
    waitMinutes,
    eventLog,
    kpi,
    loadConfig,
    updateConfig,
    startRun,
    control,
    disconnect,
    setWaitDistribution
  }
})

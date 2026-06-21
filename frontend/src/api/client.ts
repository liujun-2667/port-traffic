import type {
  AppConfig,
  ChannelSediment,
  ChannelStatus,
  CostPreview,
  CreateBatchRequest,
  DredgingBatch,
  DualResult,
  Frame,
  OptimizeResult,
  Report,
  RunMeta,
  RunParams,
  ShipDetail,
  SinglePoint,
  TideResponse,
  TrajectoryRow,
  UpdateSedimentRequest
} from './types'

const BASE = '/api'

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...init
  })
  if (!res.ok) {
    const txt = await res.text().catch(() => '')
    let msg = ''
    try {
      const j = JSON.parse(txt)
      msg =
        j?.error ||
        j?.message ||
        j?.msg ||
        (typeof j?.detail === 'string' ? j.detail : '')
    } catch {
      msg = txt.trim()
    }
    if (!msg) msg = res.statusText
    const err: Error & { status?: number; raw?: string } = new Error(msg)
    err.status = res.status
    err.raw = txt
    throw err
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

export const api = {
  health: () => req<{ ok: boolean }>('/health'),
  getConfig: () => req<AppConfig>('/config'),
  putConfig: (body: { sim?: Partial<AppConfig['sim']>; weather?: Partial<AppConfig['weather']> }) =>
    req<AppConfig>('/config', { method: 'PUT', body: JSON.stringify(body) }),
  getTide: (hours = 24) => req<TideResponse>(`/tide?hours=${hours}`),
  startRun: (params: RunParams) =>
    req<{ runId: number }>('/sim/run', { method: 'POST', body: JSON.stringify(params) }),
  controlRun: (runId: number, action: string, rate = 0) =>
    req<{ ok: boolean }>(`/sim/${runId}/control`, {
      method: 'POST',
      body: JSON.stringify({ action, rate })
    }),
  getState: (runId: number) => req<Frame>(`/sim/${runId}/state`),
  listRuns: () => req<RunMeta[]>('/runs'),
  getRun: (runId: number) =>
    req<{ runId: number; live: boolean; frame?: Frame; meta?: RunMeta }>(`/runs/${runId}`),
  getTrajectory: (runId: number, from?: number, to?: number) => {
    const q = new URLSearchParams()
    if (from != null) q.set('from', String(from))
    if (to != null) q.set('to', String(to))
    const qs = q.toString()
    return req<TrajectoryRow[]>(`/runs/${runId}/trajectory${qs ? '?' + qs : ''}`)
  },
  getReport: (runId: number) => req<Report>(`/runs/${runId}/report`),
  sensitivitySingle: (param: string, from: number, to: number, step: number) =>
    req<SinglePoint[]>('/sensitivity/single', {
      method: 'POST',
      body: JSON.stringify({ param, from, to, step })
    }),
  sensitivityDual: (b: {
    paramX: string
    fromX: number
    toX: number
    stepX: number
    paramY: string
    fromY: number
    toY: number
    stepY: number
    metric: string
  }) => req<DualResult>('/sensitivity/dual', { method: 'POST', body: JSON.stringify(b) }),
  getShipDetail: (runId: number, shipId: string) =>
    req<ShipDetail>(`/sim/${runId}/ship/${encodeURIComponent(shipId)}`),

  // Dredging module
  listChannels: (date?: string) => {
    const qs = date ? `?date=${encodeURIComponent(date)}` : ''
    return req<ChannelStatus[]>(`/dredging/channels${qs}`)
  },
  getSediment: (segmentId: string) =>
    req<ChannelSediment>(`/dredging/channels/${encodeURIComponent(segmentId)}`),
  updateSediment: (segmentId: string, body: UpdateSedimentRequest) =>
    req<ChannelSediment>(`/dredging/channels/${encodeURIComponent(segmentId)}`, {
      method: 'PUT',
      body: JSON.stringify(body)
    }),
  costPreview: (segmentIds: string[], targetDepth: number) =>
    req<CostPreview>('/dredging/cost-preview', {
      method: 'POST',
      body: JSON.stringify({ segmentIds, targetDepth })
    }),
  createBatch: (body: CreateBatchRequest) =>
    req<DredgingBatch>('/dredging/batches', {
      method: 'POST',
      body: JSON.stringify(body)
    }),
  listBatches: () => req<DredgingBatch[]>('/dredging/batches'),
  getBatch: (batchId: number) => req<DredgingBatch>(`/dredging/batches/${batchId}`),
  startBatch: (batchId: number) =>
    req<{ ok: boolean }>(`/dredging/batches/${batchId}/start`, { method: 'POST' }),
  completeBatch: (batchId: number) =>
    req<{ ok: boolean }>(`/dredging/batches/${batchId}/complete`, { method: 'POST' }),
  deleteBatch: (batchId: number) =>
    req<{ ok: boolean }>(`/dredging/batches/${batchId}`, { method: 'DELETE' }),
  optimize: (annualBudget: number) =>
    req<OptimizeResult>('/dredging/optimize', {
      method: 'POST',
      body: JSON.stringify({ annualBudget })
    })
}

// Subscribe to the SSE frame stream for a run. Returns an unsubscribe function.
export function streamRun(runId: number, onFrame: (f: Frame) => void, onError?: (msg: string) => void) {
  const es = new EventSource(`${BASE}/sim/${runId}/stream`, { withCredentials: false })
  es.addEventListener('frame', (ev) => {
    try {
      onFrame(JSON.parse((ev as MessageEvent).data))
    } catch {
      /* ignore malformed frame */
    }
  })
  es.addEventListener('error', (ev) => {
    // Protocol (our custom) error payload from the server — close the source cleanly.
    const msg = (ev as MessageEvent).data
    if (typeof msg === 'string' && msg.length > 0) {
      if (onError) onError(msg)
    }
    es.close()
    // If the event came with no data, the connection broke: surface generic message.
    if (!msg || typeof msg !== 'string') {
      if (onError) onError('SSE connection closed')
    }
  })
  es.onerror = () => {
    es.close()
    if (onError) onError('SSE connection failed')
  }
  return () => es.close()
}

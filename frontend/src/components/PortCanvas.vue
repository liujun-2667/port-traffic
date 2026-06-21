<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { AppConfig, Frame, Ship, ShipType } from '../api/types'

const props = defineProps<{ config: AppConfig | null; frame: Frame | null }>()
const emit = defineEmits<{ (e: 'shipClick', ship: Ship): void }>()

const canvas = ref<HTMLCanvasElement | null>(null)
let ctx: CanvasRenderingContext2D | null = null
let raf = 0
let flashPhase = 0

const hoveredShip = ref<Ship | null>(null)
const hoverPos = ref<{ x: number; y: number }>({ x: 0, y: 0 })

const TYPE_COLOR: Record<ShipType, string> = {
  container: '#22c55e',
  bulk: '#3b82f6',
  tanker: '#ef4444',
  other: '#9ca3af'
}

const STATE_LABEL: Record<string, string> = {
  arrived: '已到达',
  waiting: '等待中',
  inbound: '进港',
  berthing: '靠泊中',
  working: '作业中',
  outbound: '出港',
  departed: '已离港',
  holding: '等待避让'
}

interface Transform {
  scale: number
  offX: number
  offY: number
  w: number
  h: number
}

function computeBounds(cfg: AppConfig) {
  let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity
  const push = (x: number, y: number) => {
    if (x < minX) minX = x
    if (y < minY) minY = y
    if (x > maxX) maxX = x
    if (y > maxY) maxY = y
  }
  for (const s of cfg.port.segments) { push(s.from.x, s.from.y); push(s.to.x, s.to.y) }
  for (const b of cfg.port.berths) push(b.position.x, b.position.y)
  for (const a of cfg.port.anchorages) push(a.position.x, a.position.y)
  for (const t of cfg.port.turningAreas) push(t.position.x, t.position.y)
  for (const z of cfg.port.encounterZones) push(z.position.x, z.position.y)
  if (!isFinite(minX)) return { minX: 0, minY: 0, maxX: 1, maxY: 1 }
  const pad = 200
  return { minX: minX - pad, minY: minY - pad, maxX: maxX + pad, maxY: maxY + pad }
}

function makeTransform(cfg: AppConfig, w: number, h: number): Transform {
  const b = computeBounds(cfg)
  const bw = b.maxX - b.minX
  const bh = b.maxY - b.minY
  const scale = Math.min(w / bw, h / bh)
  const offX = (w - bw * scale) / 2 - b.minX * scale
  const offY = (h - bh * scale) / 2 - b.minY * scale
  return { scale, offX, offY, w, h }
}

function toPx(t: Transform, x: number, y: number) {
  return { px: t.offX + x * t.scale, py: t.offY + (y) * t.scale }
}

function fromPx(t: Transform, px: number, py: number) {
  return { x: (px - t.offX) / t.scale, y: (py - t.offY) / t.scale }
}

function segHeading(cfg: AppConfig, ship: Ship) {
  const seg = cfg.port.segments.find((s) => s.id === ship.route[ship.routeIdx])
  if (!seg) return { hx: 1, hy: 0 }
  const dx = seg.to.x - seg.from.x
  const dy = seg.to.y - seg.from.y
  const l = Math.hypot(dx, dy) || 1
  return { hx: (dx / l) * ship.direction, hy: (dy / l) * ship.direction }
}

function draw() {
  const cv = canvas.value
  if (!cv || !ctx) return
  const dpr = window.devicePixelRatio || 1
  const rect = cv.getBoundingClientRect()
  if (cv.width !== rect.width * dpr || cv.height !== rect.height * dpr) {
    cv.width = rect.width * dpr
    cv.height = rect.height * dpr
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0)
  }
  const w = rect.width
  const h = rect.height
  ctx.clearRect(0, 0, w, h)
  ctx.fillStyle = '#0a1628'
  ctx.fillRect(0, 0, w, h)
  const cfg = props.config
  if (!cfg) return
  const t = makeTransform(cfg, w, h)
  const segCong = new Map<string, boolean>()
  if (props.frame) for (const sc of props.frame.segmentCongestion) segCong.set(sc.segId, sc.congested)

  for (const z of cfg.port.encounterZones) {
    const { px, py } = toPx(t, z.position.x, z.position.y)
    ctx.beginPath()
    ctx.arc(px, py, z.radius * t.scale, 0, Math.PI * 2)
    ctx.strokeStyle = 'rgba(255,181,71,0.35)'
    ctx.setLineDash([6, 5])
    ctx.lineWidth = 1.2
    ctx.stroke()
    ctx.setLineDash([])
    ctx.fillStyle = 'rgba(255,181,71,0.6)'
    ctx.font = '10px ui-monospace, monospace'
    ctx.fillText(z.id, px + 4, py - 4)
  }

  for (const seg of cfg.port.segments) {
    const a = toPx(t, seg.from.x, seg.from.y)
    const b = toPx(t, seg.to.x, seg.to.y)
    const halfW = (seg.width / 2) * t.scale
    const dx = b.px - a.px
    const dy = b.py - a.py
    const len = Math.hypot(dx, dy) || 1
    const nx = -dy / len
    const ny = dx / len
    ctx.beginPath()
    ctx.moveTo(a.px + nx * halfW, a.py + ny * halfW)
    ctx.lineTo(b.px + nx * halfW, b.py + ny * halfW)
    ctx.lineTo(b.px - nx * halfW, b.py - ny * halfW)
    ctx.lineTo(a.px - nx * halfW, a.py - ny * halfW)
    ctx.closePath()
    const congested = segCong.get(seg.id)
    ctx.fillStyle = congested ? 'rgba(255,181,71,0.28)' : 'rgba(59,130,246,0.22)'
    ctx.fill()
    ctx.strokeStyle = congested ? 'rgba(255,181,71,0.7)' : 'rgba(59,130,246,0.5)'
    ctx.lineWidth = 1
    ctx.stroke()
    const mx = (a.px + b.px) / 2
    const my = (a.py + b.py) / 2
    ctx.fillStyle = 'rgba(148,163,184,0.7)'
    ctx.font = '9px ui-monospace, monospace'
    ctx.fillText(seg.id, mx - 6, my - halfW - 4)
  }

  for (const ta of cfg.port.turningAreas) {
    const { px, py } = toPx(t, ta.position.x, ta.position.y)
    ctx.beginPath()
    ctx.arc(px, py, (ta.diameter / 2) * t.scale, 0, Math.PI * 2)
    ctx.strokeStyle = 'rgba(148,163,184,0.4)'
    ctx.setLineDash([3, 4])
    ctx.stroke()
    ctx.setLineDash([])
  }

  for (const an of cfg.port.anchorages) {
    const { px, py } = toPx(t, an.position.x, an.position.y)
    const r = Math.max(18, 40)
    ctx.beginPath()
    ctx.arc(px, py, r, 0, Math.PI * 2)
    ctx.fillStyle = 'rgba(15,29,53,0.8)'
    ctx.fill()
    ctx.strokeStyle = 'rgba(0,229,199,0.5)'
    ctx.setLineDash([5, 4])
    ctx.lineWidth = 1.2
    ctx.stroke()
    ctx.setLineDash([])
    ctx.fillStyle = '#00e5c7'
    ctx.font = '10px ui-monospace, monospace'
    const count = props.frame?.anchorage.count ?? an.currentCount
    ctx.fillText(`${an.id} ${count}/${an.capacity}`, px - 18, py + 3)
  }

  for (const berth of cfg.port.berths) {
    const { px, py } = toPx(t, berth.position.x, berth.position.y)
    const st = props.frame?.berths.find((b) => b.id === berth.id)
    ctx.fillStyle = TYPE_COLOR[(berth.type as ShipType)] || '#9ca3af'
    ctx.globalAlpha = st?.occupied ? 0.95 : 0.35
    ctx.fillRect(px - 7, py - 4, 14, 8)
    ctx.globalAlpha = 1
    ctx.strokeStyle = st?.occupied ? '#fff' : 'rgba(148,163,184,0.5)'
    ctx.lineWidth = 1
    ctx.strokeRect(px - 7, py - 4, 14, 8)
  }

  if (props.frame) {
    for (const ship of props.frame.ships) {
      if (ship.state === 'departed' || ship.state === 'arrived') continue
      const { px, py } = toPx(t, ship.position.x, ship.position.y)
      const color = TYPE_COLOR[ship.type] || '#9ca3af'
      const size = Math.max(4, Math.min(10, ship.length / 60))
      if (hoveredShip.value?.id === ship.id) {
        ctx.beginPath()
        ctx.arc(px, py, size + 6, 0, Math.PI * 2)
        ctx.strokeStyle = '#ffffff'
        ctx.lineWidth = 1.5
        ctx.stroke()
      }
      if (ship.state === 'working' || ship.state === 'berthing') {
        ctx.fillStyle = color
        ctx.fillRect(px - size / 2, py - size / 2, size, size)
      } else {
        const { hx, hy } = segHeading(cfg, ship)
        const ang = Math.atan2(hy, hx)
        ctx.save()
        ctx.translate(px, py)
        ctx.rotate(ang)
        ctx.beginPath()
        ctx.moveTo(size, 0)
        ctx.lineTo(-size * 0.7, size * 0.6)
        ctx.lineTo(-size * 0.7, -size * 0.6)
        ctx.closePath()
        ctx.fillStyle = color
        if (ship.state === 'holding') {
          ctx.globalAlpha = 0.5 + 0.5 * Math.abs(Math.sin(flashPhase))
        }
        ctx.fill()
        ctx.globalAlpha = 1
        ctx.restore()
      }
    }

    const flashOn = Math.sin(flashPhase) > 0
    for (const enc of props.frame.encounters) {
      if (!enc.dangerous) continue
      const sa = props.frame.ships.find((s) => s.id === enc.shipA)
      const sb = props.frame.ships.find((s) => s.id === enc.shipB)
      if (!sa || !sb) continue
      const a = toPx(t, sa.position.x, sa.position.y)
      const b = toPx(t, sb.position.x, sb.position.y)
      ctx.beginPath()
      ctx.moveTo(a.px, a.py)
      ctx.lineTo(b.px, b.py)
      ctx.strokeStyle = flashOn ? '#ff4d5e' : 'rgba(255,77,94,0.35)'
      ctx.lineWidth = 1.6
      ctx.setLineDash([5, 4])
      ctx.stroke()
      ctx.setLineDash([])
    }
  }

  drawLegend(ctx, w, h)
}

function drawLegend(c: CanvasRenderingContext2D, w: number, h: number) {
  const items: [string, string][] = [
    ['集装箱', '#22c55e'],
    ['散货', '#3b82f6'],
    ['油轮', '#ef4444'],
    ['其他', '#9ca3af']
  ]
  c.font = '10px ui-sans-serif, system-ui'
  let x = 12
  const y = h - 14
  for (const [label, color] of items) {
    c.fillStyle = color
    c.fillRect(x, y - 8, 10, 10)
    c.fillStyle = '#cbd5e1'
    c.fillText(label, x + 14, y)
    x += 60
  }
  c.fillStyle = '#ffb547'
  c.fillRect(x, y - 8, 10, 10)
  c.fillStyle = '#cbd5e1'
  c.fillText('拥堵航段', x + 14, y)
  x += 80
  c.fillStyle = '#ff4d5e'
  c.fillRect(x, y - 8, 10, 10)
  c.fillStyle = '#cbd5e1'
  c.fillText('危险会遇', x + 14, y)
}

function pickShip(clientX: number, clientY: number): Ship | null {
  const cv = canvas.value
  const cfg = props.config
  const frame = props.frame
  if (!cv || !cfg || !frame) return null
  const rect = cv.getBoundingClientRect()
  const px = clientX - rect.left
  const py = clientY - rect.top
  const t = makeTransform(cfg, rect.width, rect.height)
  const hitRadius = 14
  let closest: Ship | null = null
  let closestDist = Infinity
  for (const ship of frame.ships) {
    if (ship.state === 'departed' || ship.state === 'arrived') continue
    const s = toPx(t, ship.position.x, ship.position.y)
    const d = Math.hypot(px - s.px, py - s.py)
    if (d < hitRadius && d < closestDist) {
      closestDist = d
      closest = ship
    }
  }
  return closest
}

function handleMouseMove(ev: MouseEvent) {
  const ship = pickShip(ev.clientX, ev.clientY)
  hoveredShip.value = ship
  const cv = canvas.value
  if (cv && ship) {
    const rect = cv.getBoundingClientRect()
    hoverPos.value = { x: ev.clientX - rect.left, y: ev.clientY - rect.top }
  }
}

function handleClick(ev: MouseEvent) {
  const ship = pickShip(ev.clientX, ev.clientY)
  if (ship) {
    emit('shipClick', ship)
  }
}

function loop() {
  flashPhase += 0.18
  draw()
  raf = requestAnimationFrame(loop)
}

onMounted(() => {
  ctx = canvas.value?.getContext('2d') || null
  raf = requestAnimationFrame(loop)
  window.addEventListener('resize', draw)
})
onBeforeUnmount(() => {
  cancelAnimationFrame(raf)
  window.removeEventListener('resize', draw)
})
watch(() => props.frame, draw)
watch(() => props.config, draw)

const clockLabel = computed(() => props.frame?.clock ?? '--:--')
const minuteLabel = computed(() => `T+${props.frame?.minute ?? 0} min`)
</script>

<template>
  <div class="relative h-full w-full">
    <canvas
      ref="canvas"
      class="h-full w-full cursor-pointer"
      @mousemove="handleMouseMove"
      @click="handleClick"
    ></canvas>
    <div class="absolute right-3 top-3 rounded-md bg-navy-950/70 px-3 py-1.5 text-right font-mono text-xs text-glow-cyan backdrop-blur">
      <div>{{ clockLabel }}</div>
      <div class="text-slate-400">{{ minuteLabel }}</div>
    </div>

    <div
      v-if="hoveredShip"
      class="pointer-events-none absolute z-20 rounded-md border border-glow-cyan/30 bg-navy-950/95 px-3 py-2 text-xs shadow-xl backdrop-blur"
      :style="{ left: hoverPos.x + 12 + 'px', top: hoverPos.y + 12 + 'px' }"
    >
      <div class="mb-1 font-mono text-sm text-glow-cyan">{{ hoveredShip.id }}</div>
      <div class="space-y-0.5 text-slate-300">
        <div><span class="text-slate-500">类型:</span> {{ hoveredShip.type }}</div>
        <div><span class="text-slate-500">船长:</span> {{ hoveredShip.length.toFixed(0) }}m</div>
        <div><span class="text-slate-500">航速:</span> {{ hoveredShip.speedKn.toFixed(1) }} kn</div>
        <div><span class="text-slate-500">目标泊位:</span> {{ hoveredShip.targetBerth || '—' }}</div>
        <div><span class="text-slate-500">状态:</span> <span class="text-glow-cyan">{{ STATE_LABEL[hoveredShip.state] || hoveredShip.state }}</span></div>
      </div>
    </div>
  </div>
</template>

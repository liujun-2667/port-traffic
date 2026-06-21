// API types mirroring the Go backend JSON shapes.

export interface Point {
  x: number
  y: number
}

export interface Segment {
  id: string
  from: Point
  to: Point
  width: number
  baseDepth: number
  speedLimit: number
  maxTonnage: number
}

export interface Berth {
  id: string
  type: string
  maxTonnage: number
  position: Point
  branchSeg: string
}

export interface Anchorage {
  id: string
  capacity: number
  currentCount: number
  position: Point
}

export interface TurningArea {
  id: string
  diameter: number
  position: Point
}

export interface EncounterZone {
  id: string
  position: Point
  segmentIds: string[]
  radius: number
}

export interface PortModel {
  segments: Segment[]
  berths: Berth[]
  anchorages: Anchorage[]
  turningAreas: TurningArea[]
  encounterZones: EncounterZone[]
  bounds: [Point, Point]
}

export interface SimConfig {
  durationHours: number
  arrivalRate: number
  timeStepMinutes: number
  speedFactor: number
  seed: number
  safeSpacingShips: number
  encounterSafeRatio: number
}

export interface TideComponent {
  name: string
  amplitude: number
  phase: number
  speed: number
}

export interface TideConfig {
  datum: number
  meanSeaLevel: number
  components: TideComponent[]
  draftMargin: number
}

export interface DraftEntry {
  lenMin: number
  lenMax: number
  draft: number
  beamRatio: number
  dwt: number
}

export interface Maneuverability {
  turningRadius: number
  stopDistance: number
  accelRate: number
  decelRate: number
}

export interface TrafficConfig {
  typeWeights: Record<string, number>
  lengthMin: number
  lengthMax: number
  draftTable: DraftEntry[]
  maneuver: Record<string, Maneuverability>
  workDurationMinutes: Record<string, number>
  workJitter: number
}

export interface WeatherConfig {
  windSpeed: number
  visibility: number
  swell: number
}

export interface AppConfig {
  sim: SimConfig
  tide: TideConfig
  traffic: TrafficConfig
  weather: WeatherConfig
  port: PortModel
}

export type ShipType = 'container' | 'bulk' | 'tanker' | 'other'
export type ShipState =
  | 'arrived' | 'waiting' | 'inbound' | 'berthing'
  | 'working' | 'outbound' | 'departed' | 'holding'

export interface Ship {
  id: string
  type: ShipType
  length: number
  beam: number
  draft: number
  dwt: number
  targetBerth: string
  speedKn: number
  plannedSpeed: number
  maneuver: Maneuverability
  state: ShipState
  position: Point
  route: string[]
  routeIdx: number
  segOffset: number
  arrivalMinute: number
  enterMinute: number
  berthMinute: number
  workDuration: number
  waitMinutes: number
  direction: number
}

export interface Encounter {
  shipA: string
  shipB: string
  dcpa: number
  tcpa: number
  dangerous: boolean
  warning: boolean
  position: Point
  minute: number
}

export interface SegCong {
  segId: string
  congestion: number
  count: number
  capacity: number
  congested: boolean
}

export interface BerthState {
  id: string
  type: string
  occupied: boolean
  shipId: string
}

export interface AnchorageState {
  id: string
  count: number
  capacity: number
}

export interface KPI {
  inPort: number
  queueLength: number
  congestedSegments: number
  cumDangerous: number
  cumWarnings: number
  throughputIn: number
  throughputOut: number
  avgWait: number
  maxWait: number
}

export interface ThroughputPoint {
  minute: number
  in: number
  out: number
}

export interface Frame {
  minute: number
  clock: string
  done: boolean
  ships: Ship[]
  segmentCongestion: SegCong[] | null
  encounters: Encounter[] | null
  kpi: KPI
  throughput: ThroughputPoint[] | null
  tideLevel: number
  navigableDepth: number
  berths: BerthState[]
  anchorage: AnchorageState
  events: TimelineEvent[] | null
  strategy: StrategyConfig
}

export type SchedulingStrategy = 'free_flow' | 'tidal_window' | 'alternating_one_way'

export interface StrategyConfig {
  strategy: SchedulingStrategy
  tidalThresholdMeters: number
  oneWaySwitchMinutes: number
  oneWaySegments: string[]
}

export interface StateChange {
  minute: number
  clock: string
  state: string
  x: number
  y: number
}

export interface ShipDetail {
  shipId: string
  stateHistory: StateChange[]
  dangerousEncounters: TimelineEvent[]
}

export interface RunParams {
  durationHours: number
  arrivalRate: number
  seed: number
  windSpeed: number
  visibility: number
  speedFactor: number
  speedLimitScale: number
  strategy: StrategyConfig
}

export interface RunMeta {
  id: number
  paramsJson: number[] | string
  startedAt: string
  durationMinutes: number
  status: string
}

export interface TrajectoryRow {
  shipId: string
  minute: number
  x: number
  y: number
  state: string
  speed: number
}

export interface TimelineEvent {
  minute: number
  clock: string
  type: string
  shipA: string
  shipB: string
  desc: string
}

export interface Bottleneck {
  rank: number
  segId: string
  avgCongestion: number
  peakCongestion: number
  priority: string
}

export interface Advice {
  code: string
  text: string
}

export interface Summary {
  durationMinutes: number
  arrivalRate: number
  seed: number
  windSpeed: number
  visibility: number
  segmentCount: number
  berthCount: number
  strategy: StrategyConfig
}

export interface SegCongAvg {
  segId: string
  avgCongestion: number
  peakCongestion: number
}

export interface Metrics {
  totalThroughput: number
  throughputIn: number
  throughputOut: number
  avgWaitMinutes: number
  maxWaitMinutes: number
  severeDelayCount: number
  dangerousEncounters: number
  collisionWarnings: number
  segmentCongestion: SegCongAvg[]
}

export interface Report {
  summary: Summary
  metrics: Metrics
  events: TimelineEvent[]
  bottlenecks: Bottleneck[]
  advice: Advice[]
}

export interface SinglePoint {
  param: string
  value: number
  dangerous: number
  warning: number
  avgWait: number
  throughput: number
  congestion: number
}

export interface DualResult {
  paramX: string
  paramY: string
  x: number[]
  y: number[]
  matrix: number[][]
  metric: string
}

export interface TidePoint {
  t: number
  level: number
}

export interface TideResponse {
  series: TidePoint[]
  margin: number
  meanSeaLevel: number
}

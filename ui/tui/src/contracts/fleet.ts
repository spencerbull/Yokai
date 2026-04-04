export type DeviceRecord = {
  id: string
  label?: string
  host: string
  ssh_user?: string
  ssh_key?: string
  ssh_port?: number
  connection_type: string
  agent_port: number
  agent_token?: string
  gpu_type?: string
  tags?: string[]
  online: boolean
  tunnel_port: number
  tunnel_error?: string
}

export type DevicesResponse = {
  devices: DeviceRecord[]
}

export type CpuMetrics = {
  percent: number
}

export type RamMetrics = {
  used_mb: number
  total_mb: number
  percent: number
}

export type GpuMetrics = {
  index: number
  name: string
  utilization_percent: number
  vram_used_mb: number
  vram_total_mb: number
  temperature_c: number
  power_draw_w: number
  power_limit_w: number
  fan_percent: number
}

export type ContainerMetrics = {
  id: string
  name: string
  image?: string
  status?: string
  cpu_percent?: number
  memory_used_mb?: number
  gpu_memory_mb?: number
  uptime_seconds?: number
  ports?: Record<string, string>
  health?: string
  generation_tok_per_s?: number
  prompt_tok_per_s?: number
}

export type DeviceMetrics = {
  device_id: string
  timestamp: string
  online: boolean
  cpu?: CpuMetrics
  ram?: RamMetrics
  gpus?: GpuMetrics[]
  containers?: ContainerMetrics[]
}

export type MetricsResponse = Record<string, DeviceMetrics>

export type FleetDevice = {
  id: string
  label: string
  host: string
  online: boolean
  tunnelPort: number
  gpuType: string
  gpuName: string
  gpuCount: number
  activeGpuCount: number
  gpuUtilPercent: number
  gpuMemoryUsedMB: number
  gpuMemoryTotalMB: number
  cpuPercent: number
  ramPercent: number
  serviceCount: number
}

export type FleetService = {
  containerId: string
  serviceId: string
  name: string
  type: string
  model: string
  image: string
  status: string
  health: string
  deviceId: string
  deviceLabel: string
  deviceOnline: boolean
  port: number
  cpuPercent: number
  memoryUsedMB: number
  gpuMemoryMB: number
  uptimeSeconds: number
  generationTokPerSec: number
  promptTokPerSec: number
}

export type FleetSnapshot = {
  devices: FleetDevice[]
  services: FleetService[]
  totals: {
    devices: number
    onlineDevices: number
    services: number
    alertServices: number
    gpuCount: number
    activeGpuCount: number
    gpuMemoryUsedMB: number
    gpuMemoryTotalMB: number
    avgGpuUtilPercent: number
    avgCpuPercent: number
    avgRamPercent: number
  }
  updatedAt?: string
}

export type MetricSeries = {
  cpu: number[]
  ram: number[]
  gpu: number[]
  vram: number[]
}

export type FleetHistory = {
  fleet: MetricSeries
  devices: Record<string, MetricSeries>
}

export type LogTarget = {
  deviceId: string
  containerId: string
  serviceName: string
}

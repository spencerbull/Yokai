import type {
  ContainerMetrics,
  DeviceMetrics,
  DeviceRecord,
  FleetDevice,
  FleetService,
  FleetSnapshot,
  MetricsResponse,
} from "../../contracts/fleet"

export function normalizeFleetSnapshot(devices: DeviceRecord[], metrics: MetricsResponse): FleetSnapshot {
  const fleetDevices: FleetDevice[] = devices
    .map((device) => toFleetDevice(device, metrics[device.id]))
    .sort((left, right) => left.label.localeCompare(right.label))

  const fleetServices: FleetService[] = []
  for (const device of fleetDevices) {
    const deviceMetrics = metrics[device.id]
    for (const container of deviceMetrics?.containers ?? []) {
      fleetServices.push(toFleetService(device, container))
    }
  }

  fleetServices.sort(compareFleetServices)

  return {
    devices: fleetDevices,
    services: fleetServices,
    totals: {
      devices: fleetDevices.length,
      onlineDevices: fleetDevices.filter((device) => device.online).length,
      services: fleetServices.length,
      alertServices: fleetServices.filter(isAlertService).length,
      gpuCount: fleetDevices.reduce((sum, device) => sum + device.gpuCount, 0),
      activeGpuCount: fleetDevices.reduce((sum, device) => sum + device.activeGpuCount, 0),
      gpuMemoryUsedMB: fleetDevices.reduce((sum, device) => sum + device.gpuMemoryUsedMB, 0),
      gpuMemoryTotalMB: fleetDevices.reduce((sum, device) => sum + device.gpuMemoryTotalMB, 0),
      avgGpuUtilPercent: averageOf(
        fleetDevices.flatMap((device) => (device.gpuCount > 0 ? [device.gpuUtilPercent] : [])),
      ),
      avgCpuPercent: averageOf(
        fleetDevices.filter((device) => device.online).map((device) => device.cpuPercent),
      ),
      ramUsedMB: fleetDevices.reduce((sum, device) => sum + device.ramUsedMB, 0),
      ramTotalMB: fleetDevices.reduce((sum, device) => sum + device.ramTotalMB, 0),
      avgRamPercent: averageOf(
        fleetDevices.filter((device) => device.online).map((device) => device.ramPercent),
      ),
    },
    updatedAt: latestTimestamp(metrics),
  }
}

function toFleetDevice(device: DeviceRecord, metrics: DeviceMetrics | undefined): FleetDevice {
  const gpuStats = summarizeDeviceGpus(metrics?.gpus ?? [])

  return {
    id: device.id,
    label: device.label || device.id,
    host: device.host,
    online: device.online && (metrics?.online ?? true),
    tunnelPort: device.tunnel_port,
    gpuType: device.gpu_type ?? "",
    gpuName: gpuStats.name,
    gpuCount: gpuStats.count,
    activeGpuCount: gpuStats.activeCount,
    gpuUtilPercent: gpuStats.utilPercent,
    gpuMemoryUsedMB: gpuStats.memoryUsedMB,
    gpuMemoryTotalMB: gpuStats.memoryTotalMB,
    cpuPercent: metrics?.cpu?.percent ?? 0,
    ramUsedMB: metrics?.ram?.used_mb ?? 0,
    ramTotalMB: metrics?.ram?.total_mb ?? 0,
    ramPercent: metrics?.ram?.percent ?? 0,
    serviceCount: metrics?.containers?.length ?? 0,
  }
}

function toFleetService(device: FleetDevice, container: ContainerMetrics): FleetService {
  return {
    containerId: container.id,
    serviceId: inferServiceID(container.name, container.id),
    name: inferServiceName(container.name, container.id),
    type: inferServiceType(container.name, container.image),
    model: "",
    image: container.image ?? "",
    status: container.status ?? "running",
    health: container.health ?? "",
    deviceId: device.id,
    deviceLabel: device.label,
    deviceOnline: device.online,
    port: externalPort(container.ports),
    cpuPercent: container.cpu_percent ?? 0,
    memoryUsedMB: container.memory_used_mb ?? 0,
    gpuMemoryMB: container.gpu_memory_mb ?? 0,
    uptimeSeconds: container.uptime_seconds ?? 0,
    generationTokPerSec: container.generation_tok_per_s ?? 0,
    promptTokPerSec: container.prompt_tok_per_s ?? 0,
  }
}

function compareFleetServices(left: FleetService, right: FleetService) {
  const leftAlert = isAlertService(left)
  const rightAlert = isAlertService(right)
  if (leftAlert !== rightAlert) {
    return leftAlert ? -1 : 1
  }

  const deviceComparison = left.deviceLabel.localeCompare(right.deviceLabel)
  if (deviceComparison !== 0) {
    return deviceComparison
  }

  return left.name.localeCompare(right.name)
}

export function isAlertService(service: Pick<FleetService, "health" | "status">) {
  const status = (service.health || service.status || "").toLowerCase()
  switch (status) {
    case "":
    case "healthy":
    case "running":
    case "starting":
    case "created":
    case "restarting":
      return false
    default:
      return true
  }
}

export function isMonitoringService(service: Pick<FleetService, "name" | "type" | "image">) {
  const haystack = `${service.name} ${service.type} ${service.image}`.toLowerCase()
  return (
    haystack.startsWith("mon-") ||
    haystack.includes("monitoring") ||
    haystack.includes("prometheus") ||
    haystack.includes("grafana") ||
    haystack.includes("loki") ||
    haystack.includes("alloy")
  )
}

export function isAIService(service: Pick<FleetService, "name" | "type" | "image">) {
  return !isMonitoringService(service)
}

export function externalPort(ports: Record<string, string> | undefined) {
  if (!ports) {
    return 0
  }

  for (const external of Object.values(ports)) {
    const parsed = Number.parseInt(external, 10)
    if (Number.isFinite(parsed)) {
      return parsed
    }
  }

  return 0
}

export function inferServiceType(name: string, image?: string) {
  const haystack = `${name} ${image ?? ""}`.toLowerCase()
  if (haystack.includes("vllm")) {
    return "vllm"
  }
  if (haystack.includes("llama")) {
    return "llamacpp"
  }
  if (haystack.includes("comfyui") || haystack.includes("comfy")) {
    return "comfyui"
  }
  if (haystack.includes("prometheus") || haystack.includes("grafana")) {
    return "monitoring"
  }

  return "service"
}

function inferServiceID(name: string, fallback: string) {
  if (name.startsWith("yokai-")) {
    return name.slice("yokai-".length)
  }

  return fallback
}

function inferServiceName(name: string, fallback: string) {
  const serviceID = inferServiceID(name, fallback)
  return serviceID || fallback
}

function latestTimestamp(metrics: MetricsResponse) {
  let latest = ""

  for (const deviceMetrics of Object.values(metrics)) {
    if (!deviceMetrics?.timestamp) {
      continue
    }
    if (latest === "" || deviceMetrics.timestamp > latest) {
      latest = deviceMetrics.timestamp
    }
  }

  return latest || undefined
}

function summarizeDeviceGpus(gpus: NonNullable<DeviceMetrics["gpus"]>) {
  if (gpus.length === 0) {
    return {
      count: 0,
      activeCount: 0,
      name: "",
      utilPercent: 0,
      memoryUsedMB: 0,
      memoryTotalMB: 0,
    }
  }

  const totalUtil = gpus.reduce((sum, gpu) => sum + gpu.utilization_percent, 0)
  const memoryUsedMB = gpus.reduce((sum, gpu) => sum + gpu.vram_used_mb, 0)
  const memoryTotalMB = gpus.reduce((sum, gpu) => sum + gpu.vram_total_mb, 0)
  const activeCount = gpus.filter((gpu) => gpu.utilization_percent > 0 || gpu.vram_used_mb > 0).length

  return {
    count: gpus.length,
    activeCount,
    name: gpus[0]?.name ?? "",
    utilPercent: totalUtil / gpus.length,
    memoryUsedMB,
    memoryTotalMB,
  }
}

function averageOf(values: number[]) {
  if (values.length === 0) {
    return 0
  }

  return values.reduce((sum, value) => sum + value, 0) / values.length
}

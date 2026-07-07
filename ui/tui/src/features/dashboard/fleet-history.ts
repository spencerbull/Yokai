import type { FleetHistory, FleetSnapshot, MetricSeries } from "../../contracts/fleet"

const MAX_SAMPLES = 60

export const EMPTY_HISTORY: FleetHistory = {
  fleet: emptySeries(),
  devices: {},
}

export function appendFleetHistory(history: FleetHistory, snapshot: FleetSnapshot): FleetHistory {
  const nextDevices: Record<string, MetricSeries> = {}

  for (const device of snapshot.devices) {
    nextDevices[device.id] = appendSeries(history.devices[device.id], {
      cpu: device.cpuPercent,
      ram: device.ramPercent,
      gpu: device.gpuUtilPercent,
      vram: percentage(device.gpuMemoryUsedMB, device.gpuMemoryTotalMB),
    })
  }

  return {
    fleet: appendSeries(history.fleet, {
      cpu: snapshot.totals.avgCpuPercent,
      ram: snapshot.totals.avgRamPercent,
      gpu: snapshot.totals.avgGpuUtilPercent,
      vram: percentage(snapshot.totals.gpuMemoryUsedMB, snapshot.totals.gpuMemoryTotalMB),
    }),
    devices: nextDevices,
  }
}

function appendSeries(series: MetricSeries | undefined, sample: { cpu: number; gpu: number; ram: number; vram: number }): MetricSeries {
  const current = series ?? emptySeries()

  return {
    cpu: appendSample(current.cpu, sample.cpu),
    ram: appendSample(current.ram, sample.ram),
    gpu: appendSample(current.gpu, sample.gpu),
    vram: appendSample(current.vram, sample.vram),
  }
}

function appendSample(values: number[], nextValue: number) {
  const normalized = clampPercent(nextValue)
  if (values.length >= MAX_SAMPLES) {
    return [...values.slice(values.length - MAX_SAMPLES + 1), normalized]
  }

  return [...values, normalized]
}

function emptySeries(): MetricSeries {
  return {
    cpu: [],
    ram: [],
    gpu: [],
    vram: [],
  }
}

function percentage(used: number, total: number) {
  if (total <= 0) {
    return 0
  }

  return (used / total) * 100
}

function clampPercent(value: number) {
  if (!Number.isFinite(value)) {
    return 0
  }
  if (value < 0) {
    return 0
  }
  if (value > 100) {
    return 100
  }
  return value
}

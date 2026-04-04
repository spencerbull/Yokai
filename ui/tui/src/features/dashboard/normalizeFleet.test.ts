import { describe, expect, test } from "bun:test"

import type { DeviceRecord, MetricsResponse } from "../../contracts/fleet"
import { normalizeFleetSnapshot } from "./normalizeFleet"

describe("normalizeFleetSnapshot", () => {
  test("sorts alerting services first and enriches device labels", () => {
    const devices: DeviceRecord[] = [
      {
        id: "dev-a",
        label: "alpha",
        host: "10.0.0.1",
        connection_type: "tailscale",
        agent_port: 7474,
        online: true,
        tunnel_port: 17474,
      },
      {
        id: "dev-b",
        label: "beta",
        host: "10.0.0.2",
        connection_type: "tailscale",
        agent_port: 7474,
        online: true,
        tunnel_port: 27474,
      },
    ]

    const metrics: MetricsResponse = {
      "dev-a": {
        device_id: "dev-a",
        timestamp: "2026-04-04T02:00:00Z",
        online: true,
        cpu: { percent: 12 },
        ram: { used_mb: 1024, total_mb: 8192, percent: 12.5 },
        gpus: [],
        containers: [
          {
            id: "container-healthy",
            name: "yokai-healthy",
            status: "running",
            health: "healthy",
            generation_tok_per_s: 35.5,
          },
        ],
      },
      "dev-b": {
        device_id: "dev-b",
        timestamp: "2026-04-04T02:00:01Z",
        online: true,
        cpu: { percent: 20 },
        ram: { used_mb: 2048, total_mb: 8192, percent: 25 },
        gpus: [],
        containers: [
          {
            id: "container-alert",
            name: "yokai-alert",
            status: "exited",
          },
        ],
      },
    }

    const snapshot = normalizeFleetSnapshot(devices, metrics)

    expect(snapshot.totals.devices).toBe(2)
    expect(snapshot.totals.onlineDevices).toBe(2)
    expect(snapshot.totals.services).toBe(2)
    expect(snapshot.totals.alertServices).toBe(1)
    expect(snapshot.totals.avgCpuPercent).toBe(16)
    expect(snapshot.totals.avgRamPercent).toBe(18.75)
    expect(snapshot.updatedAt).toBe("2026-04-04T02:00:01Z")
    expect(snapshot.services[0].serviceId).toBe("alert")
    expect(snapshot.services[0].deviceLabel).toBe("beta")
    expect(snapshot.services[1].generationTokPerSec).toBe(35.5)
  })

  test("computes gpu totals and per-device gpu summary", () => {
    const devices: DeviceRecord[] = [
      {
        id: "dev-gpu",
        label: "gpu-box",
        host: "10.0.0.5",
        connection_type: "tailscale",
        agent_port: 7474,
        online: true,
        tunnel_port: 37474,
      },
    ]

    const metrics: MetricsResponse = {
      "dev-gpu": {
        device_id: "dev-gpu",
        timestamp: "2026-04-04T02:00:02Z",
        online: true,
        cpu: { percent: 5 },
        ram: { used_mb: 1024, total_mb: 4096, percent: 25 },
        gpus: [
          {
            index: 0,
            name: "NVIDIA RTX 4090",
            utilization_percent: 50,
            vram_used_mb: 8192,
            vram_total_mb: 24576,
            temperature_c: 40,
            power_draw_w: 200,
            power_limit_w: 450,
            fan_percent: 30,
          },
        ],
        containers: [],
      },
    }

    const snapshot = normalizeFleetSnapshot(devices, metrics)

    expect(snapshot.totals.gpuCount).toBe(1)
    expect(snapshot.totals.activeGpuCount).toBe(1)
    expect(snapshot.totals.gpuMemoryUsedMB).toBe(8192)
    expect(snapshot.totals.gpuMemoryTotalMB).toBe(24576)
    expect(snapshot.totals.avgGpuUtilPercent).toBe(50)
    expect(snapshot.devices[0].gpuName).toBe("NVIDIA RTX 4090")
    expect(snapshot.devices[0].gpuUtilPercent).toBe(50)
  })
})

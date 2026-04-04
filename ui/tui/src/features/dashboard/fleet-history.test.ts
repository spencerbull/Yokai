import { describe, expect, test } from "bun:test"

import type { FleetHistory, FleetSnapshot } from "../../contracts/fleet"
import { appendFleetHistory, EMPTY_HISTORY } from "./fleet-history"

describe("appendFleetHistory", () => {
  test("tracks fleet and per-device series", () => {
    const snapshot: FleetSnapshot = {
      devices: [
        {
          id: "dev-1",
          label: "alpha",
          host: "10.0.0.1",
          online: true,
          tunnelPort: 17474,
          gpuType: "nvidia",
          gpuName: "RTX 4090",
          gpuCount: 1,
          activeGpuCount: 1,
          gpuUtilPercent: 50,
          gpuMemoryUsedMB: 8192,
          gpuMemoryTotalMB: 24576,
          cpuPercent: 25,
          ramPercent: 60,
          serviceCount: 2,
        },
      ],
      services: [],
      totals: {
        devices: 1,
        onlineDevices: 1,
        services: 0,
        alertServices: 0,
        gpuCount: 1,
        activeGpuCount: 1,
        gpuMemoryUsedMB: 8192,
        gpuMemoryTotalMB: 24576,
        avgGpuUtilPercent: 50,
        avgCpuPercent: 25,
        avgRamPercent: 60,
      },
    }

    const history = appendFleetHistory(EMPTY_HISTORY, snapshot)

    expect(history.fleet.cpu).toEqual([25])
    expect(history.fleet.vram).toEqual([33.33333333333333])
    expect(history.devices["dev-1"]?.gpu).toEqual([50])
    expect(history.devices["dev-1"]?.ram).toEqual([60])
  })

  test("caps history to 60 samples and removes missing devices", () => {
    let history: FleetHistory = EMPTY_HISTORY

    for (let index = 0; index < 65; index += 1) {
      history = appendFleetHistory(history, {
        devices: [
          {
            id: "dev-1",
            label: "alpha",
            host: "10.0.0.1",
            online: true,
            tunnelPort: 17474,
            gpuType: "",
            gpuName: "",
            gpuCount: 0,
            activeGpuCount: 0,
            gpuUtilPercent: 0,
            gpuMemoryUsedMB: 0,
            gpuMemoryTotalMB: 0,
            cpuPercent: index,
            ramPercent: index,
            serviceCount: 0,
          },
        ],
        services: [],
        totals: {
          devices: 1,
          onlineDevices: 1,
          services: 0,
          alertServices: 0,
          gpuCount: 0,
          activeGpuCount: 0,
          gpuMemoryUsedMB: 0,
          gpuMemoryTotalMB: 0,
          avgGpuUtilPercent: 0,
          avgCpuPercent: index,
          avgRamPercent: index,
        },
      })
    }

    expect(history.fleet.cpu).toHaveLength(60)
    expect(history.devices["dev-1"]?.cpu[0]).toBe(5)

    history = appendFleetHistory(history, {
      devices: [],
      services: [],
      totals: {
        devices: 0,
        onlineDevices: 0,
        services: 0,
        alertServices: 0,
        gpuCount: 0,
        activeGpuCount: 0,
        gpuMemoryUsedMB: 0,
        gpuMemoryTotalMB: 0,
        avgGpuUtilPercent: 0,
        avgCpuPercent: 0,
        avgRamPercent: 0,
      },
    })

    expect(history.devices["dev-1"]).toBeUndefined()
  })
})

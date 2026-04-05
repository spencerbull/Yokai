import { startTransition, useEffect, useState } from "react"

import type { FleetHistory, FleetSnapshot } from "../../contracts/fleet"
import { getDevices, getMetrics } from "../../services/daemon-client"
import { appendFleetHistory, EMPTY_HISTORY } from "./fleet-history"
import { normalizeFleetSnapshot } from "./normalizeFleet"

type FleetState = {
  history: FleetHistory
  status: "loading" | "ready" | "error"
  snapshot: FleetSnapshot
  error?: string
}

const EMPTY_SNAPSHOT: FleetSnapshot = {
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
    ramUsedMB: 0,
    ramTotalMB: 0,
    avgRamPercent: 0,
  },
}

export function useFleetSnapshot(active: boolean) {
  const [state, setState] = useState<FleetState>({
    history: EMPTY_HISTORY,
    status: "loading",
    snapshot: EMPTY_SNAPSHOT,
  })

  useEffect(() => {
    if (!active) {
      return
    }

    let cancelled = false

    const poll = async () => {
      try {
        const [devicesResponse, metricsResponse] = await Promise.all([getDevices(), getMetrics()])
        const snapshot = normalizeFleetSnapshot(devicesResponse.devices, metricsResponse)

        if (cancelled) {
          return
        }

        startTransition(() => {
          setState((current) => ({
            error: undefined,
            history: appendFleetHistory(current.history, snapshot),
            status: "ready",
            snapshot,
          }))
        })
      } catch (cause) {
        if (cancelled) {
          return
        }

        setState((current) => ({
          history: current.history,
          status: current.snapshot.devices.length > 0 || current.snapshot.services.length > 0 ? "ready" : "error",
          snapshot: current.snapshot,
          error: cause instanceof Error ? cause.message : "failed to load fleet snapshot",
        }))
      }
    }

    void poll()
    const interval = setInterval(() => {
      void poll()
    }, 2000)

    return () => {
      cancelled = true
      clearInterval(interval)
    }
  }, [active])

  return state
}

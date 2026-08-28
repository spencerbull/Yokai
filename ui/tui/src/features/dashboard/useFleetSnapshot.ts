import { startTransition, useEffect, useRef, useState } from "react"

import type { FleetHistory, FleetSnapshot } from "../../contracts/fleet"
import type { DeploymentRecord } from "../../contracts/deploy"
import { getDeployments, getDevices, getMetrics } from "../../services/daemon-client"
import { appendFleetHistory, EMPTY_HISTORY } from "./fleet-history"
import { normalizeFleetSnapshot } from "./normalizeFleet"

type FleetState = {
  deployments: DeploymentRecord[]
  history: FleetHistory
  status: "loading" | "ready" | "error"
  snapshot: FleetSnapshot
  error?: string
}

type DeploymentPollResult =
  | { deployments: DeploymentRecord[]; error?: never }
  | { deployments?: never; error: string }

export function mergeDeploymentPoll(current: DeploymentRecord[], result: DeploymentPollResult) {
  if (result.error) {
    return { deployments: current, error: `deployment status unavailable: ${result.error}` }
  }
  return { deployments: result.deployments, error: undefined }
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
    deployments: [],
    history: EMPTY_HISTORY,
    status: "loading",
    snapshot: EMPTY_SNAPSHOT,
  })
  const pollRef = useRef<null | (() => Promise<void>)>(null)

  useEffect(() => {
    if (!active) {
      pollRef.current = null
      return
    }

    let cancelled = false

    const poll = async () => {
      try {
        const [devicesResponse, metricsResponse, deploymentResult] = await Promise.all([
          getDevices(),
          getMetrics(),
          getDeployments()
            .then((deployments): DeploymentPollResult => ({ deployments }))
            .catch((cause): DeploymentPollResult => ({ error: cause instanceof Error ? cause.message : "failed to load deployments" })),
        ])
        const snapshot = normalizeFleetSnapshot(devicesResponse.devices, metricsResponse)

        if (cancelled) {
          return
        }

        startTransition(() => {
          setState((current) => {
            const deploymentPoll = mergeDeploymentPoll(current.deployments, deploymentResult)
            return {
              deployments: deploymentPoll.deployments,
              error: deploymentPoll.error,
              history: appendFleetHistory(current.history, snapshot),
              status: "ready",
              snapshot,
            }
          })
        })
      } catch (cause) {
        if (cancelled) {
          return
        }

        setState((current) => ({
          deployments: current.deployments,
          history: current.history,
          status: current.snapshot.devices.length > 0 || current.snapshot.services.length > 0 ? "ready" : "error",
          snapshot: current.snapshot,
          error: cause instanceof Error ? cause.message : "failed to load fleet snapshot",
        }))
      }
    }

    pollRef.current = poll

    void poll()
    const interval = setInterval(() => {
      void poll()
    }, 2000)

    return () => {
      cancelled = true
      if (pollRef.current === poll) {
        pollRef.current = null
      }
      clearInterval(interval)
    }
  }, [active])

  return {
    ...state,
    refresh() {
      if (!active || !pollRef.current) {
        return
      }
      void pollRef.current()
    },
  }
}

import { useEffect, useMemo, useState } from "react"

import type { FleetService, LogTarget } from "../../contracts/fleet"
import { compactActionError, confirmForAction, runDashboardAction, type DashboardActionKind, type DashboardConfirm, type DashboardNotice } from "./dashboard-actions"
import { useLogsStream } from "../logs/useLogsStream"
import { useFleetSnapshot } from "./useFleetSnapshot"

type KeyLike = {
  name: string
}

export function useDashboardController(active: boolean, terminalWidth: number, terminalHeight: number) {
  const fleet = useFleetSnapshot(active)
  const [selectedContainerId, setSelectedContainerId] = useState<string | null>(null)
  const [logsTarget, setLogsTarget] = useState<LogTarget | null>(null)
  const [confirm, setConfirm] = useState<DashboardConfirm | null>(null)
  const [notice, setNotice] = useState<DashboardNotice | null>(null)
  const [pendingAction, setPendingAction] = useState<DashboardActionKind | null>(null)

  useEffect(() => {
    if (!notice) {
      return
    }

    const timeout = setTimeout(() => {
      setNotice((current) => (current === notice ? null : current))
    }, 4500)

    return () => {
      clearTimeout(timeout)
    }
  }, [notice])

  useEffect(() => {
    const services = fleet.snapshot.services
    if (services.length === 0) {
      setSelectedContainerId(null)
      return
    }

    if (!selectedContainerId || !services.some((service) => service.containerId === selectedContainerId)) {
      setSelectedContainerId(services[0].containerId)
    }
  }, [fleet.snapshot.services, selectedContainerId])

  const selectedService = useMemo(() => {
    return fleet.snapshot.services.find((service) => service.containerId === selectedContainerId) ?? null
  }, [fleet.snapshot.services, selectedContainerId])

  const selectedIndex = useMemo(() => {
    if (!selectedService) {
      return -1
    }
    return fleet.snapshot.services.findIndex((service) => service.containerId === selectedService.containerId)
  }, [fleet.snapshot.services, selectedService])

  const logsViewportLines = estimateLogViewportLines(terminalWidth, terminalHeight)
  const logs = useLogsStream(logsTarget, logsViewportLines, active)

  return {
    ...fleet,
    confirm,
    logs,
    logsOpen: logs.target !== null,
    notice,
    pendingAction,
    selectedIndex,
    selectedService,
    selectService(containerId: string) {
      const service = fleet.snapshot.services.find((entry) => entry.containerId === containerId)
      setSelectedContainerId(containerId)
      if (!service) {
        return
      }
      setLogsTarget({
        deviceId: service.deviceId,
        containerId: service.containerId,
        serviceName: service.name,
      })
    },
    handleKey(key: KeyLike) {
      if (confirm) {
        switch (key.name) {
          case "y":
          case "return":
          case "enter":
            void executeAction(confirm.action)
            return true
          case "n":
          case "escape":
            setConfirm(null)
            return true
          default:
            return true
        }
      }

      if (key.name === "escape" && logs.target) {
        setLogsTarget(null)
        logs.close()
        return true
      }

      switch (key.name) {
        case "up":
        case "k":
          moveSelection(-1)
          return true
        case "down":
        case "j":
          moveSelection(1)
          return true
        case "l":
        case "return":
        case "enter":
          return openSelectedServiceLogs()
        case "s":
          return queueAction("stop")
        case "r":
          return queueAction("restart")
        case "t":
          return queueAction("test")
        case "x":
          return queueAction("delete")
        case "f":
          if (logs.target) {
            logs.toggleFollow()
            return true
          }
          return false
        case "pageup":
          if (logs.target) {
            logs.pageUp()
            return true
          }
          return false
        case "pagedown":
          if (logs.target) {
            logs.pageDown()
            return true
          }
          return false
        case "home":
          if (logs.target) {
            logs.scrollHome()
            return true
          }
          return false
        case "end":
          if (logs.target) {
            logs.scrollEnd()
            return true
          }
          return false
        default:
          return false
      }
    },
  }

  function moveSelection(delta: number) {
    const services = fleet.snapshot.services
    if (services.length === 0) {
      return
    }

    const currentIndex = selectedIndex === -1 ? 0 : selectedIndex
    const nextIndex = (currentIndex + delta + services.length) % services.length
    setSelectedContainerId(services[nextIndex].containerId)
  }

  function queueAction(action: DashboardActionKind) {
    if (!selectedService || pendingAction) {
      return false
    }

    const confirmPrompt = confirmForAction(action, selectedService)
    if (confirmPrompt) {
      setConfirm(confirmPrompt)
      return true
    }

    void executeAction(action)
    return true
  }

  async function executeAction(action: DashboardActionKind) {
    if (!selectedService || pendingAction) {
      return
    }

    const service = selectedService
    setConfirm(null)
    setPendingAction(action)
    setNotice(null)

    try {
      const result = await runDashboardAction(action, service)
      if (action === "delete" && logsTarget?.containerId === service.containerId) {
        setLogsTarget(null)
      }
      setNotice(result)
    } catch (error) {
      setNotice({
        level: "error",
        message: `${capitalizeAction(action)} failed: ${compactActionError(action, error)}`,
      })
    } finally {
      setPendingAction(null)
    }
  }

  function openSelectedServiceLogs() {
    if (!selectedService) {
      return false
    }

    setLogsTarget({
      deviceId: selectedService.deviceId,
      containerId: selectedService.containerId,
      serviceName: selectedService.name,
    })
    return true
  }
}

function estimateLogViewportLines(terminalWidth: number, terminalHeight: number) {
  if (terminalWidth >= 120) {
    return Math.max(6, Math.floor((terminalHeight - 14) / 2))
  }

  return Math.max(6, Math.floor((terminalHeight - 18) / 3))
}

export type DashboardController = ReturnType<typeof useDashboardController>

function capitalizeAction(action: DashboardActionKind) {
  return action.charAt(0).toUpperCase() + action.slice(1)
}

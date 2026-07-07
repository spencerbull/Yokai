import { useEffect, useMemo, useState } from "react"

import type { FleetService, LogTarget } from "../../contracts/fleet"
import { useLogsStream } from "../logs/useLogsStream"
import { compactActionError, confirmForAction, runDashboardAction, type DashboardActionKind, type DashboardConfirm, type DashboardNotice } from "./dashboard-actions"
import { isAIService, isMonitoringService } from "./normalizeFleet"
import { useFleetSnapshot } from "./useFleetSnapshot"

type KeyLike = {
  name: string
  shift?: boolean
}

type OverviewSection = "ai" | "monitoring"
type ServiceSection = "actions" | "inspector" | "device"
type ServiceAction = "logs" | DashboardActionKind

const SERVICE_ACTIONS: ServiceAction[] = ["logs", "restart", "stop", "test", "delete"]

export function useDashboardController(active: boolean, terminalWidth: number, terminalHeight: number) {
  const fleet = useFleetSnapshot(active)
  const [selectedContainerId, setSelectedContainerId] = useState<string | null>(null)
  const [logsTarget, setLogsTarget] = useState<LogTarget | null>(null)
  const [viewMode, setViewMode] = useState<"overview" | "service" | "logs">("overview")
  const [overviewSection, setOverviewSection] = useState<OverviewSection>("ai")
  const [serviceSection, setServiceSection] = useState<ServiceSection>("device")
  const [serviceActionIndex, setServiceActionIndex] = useState(0)
  const [confirm, setConfirm] = useState<DashboardConfirm | null>(null)
  const [notice, setNotice] = useState<DashboardNotice | null>(null)
  const [pendingAction, setPendingAction] = useState<DashboardActionKind | null>(null)

  const aiServices = useMemo(() => fleet.snapshot.services.filter(isAIService), [fleet.snapshot.services])
  const monitoringServices = useMemo(() => fleet.snapshot.services.filter(isMonitoringService), [fleet.snapshot.services])
  const currentOverviewServices = overviewSection === "ai" ? aiServices : monitoringServices

  useEffect(() => {
    if (!notice) {
      return
    }

    const timeout = setTimeout(() => {
      setNotice((current) => (current === notice ? null : current))
    }, 4500)

    return () => clearTimeout(timeout)
  }, [notice])

  useEffect(() => {
    const services = fleet.snapshot.services
    if (services.length === 0) {
      setSelectedContainerId(null)
      setViewMode("overview")
      return
    }

    if (!selectedContainerId || !services.some((service) => service.containerId === selectedContainerId)) {
      setSelectedContainerId(services[0].containerId)
    }

    if (viewMode !== "overview" && selectedContainerId && !services.some((service) => service.containerId === selectedContainerId)) {
      setViewMode("overview")
      setLogsTarget(null)
    }
  }, [fleet.snapshot.services, selectedContainerId, viewMode])

  useEffect(() => {
    if (overviewSection === "ai" && aiServices.length === 0 && monitoringServices.length > 0) {
      setOverviewSection("monitoring")
      return
    }
    if (overviewSection === "monitoring" && monitoringServices.length === 0 && aiServices.length > 0) {
      setOverviewSection("ai")
    }
  }, [aiServices.length, monitoringServices.length, overviewSection])

  useEffect(() => {
    if (viewMode !== "service") {
      setServiceSection("actions")
      setServiceActionIndex(0)
    }
  }, [viewMode])

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
  const logs = useLogsStream(logsTarget, logsViewportLines, active && viewMode === "logs")

  return {
    ...fleet,
    aiServices,
    confirm,
    currentOverviewServices,
    logs,
    logsOpen: logs.target !== null,
    monitoringServices,
    notice,
    overviewSection,
    pendingAction,
    selectedIndex,
    selectedService,
    serviceActionIndex,
    serviceSection,
    setServiceActionIndex,
    viewMode,
    closeServiceView() {
      setViewMode("overview")
      setLogsTarget(null)
    },
    openLogs() {
      return openLogsForSelectedService()
    },
    openService(containerId?: string) {
      const targetId = containerId ?? selectedContainerId
      if (!targetId) {
        return false
      }

      const service = fleet.snapshot.services.find((entry) => entry.containerId === targetId)
      if (!service) {
        return false
      }

      setSelectedContainerId(service.containerId)
      setViewMode("service")
      setServiceSection("actions")
      setServiceActionIndex(0)
      setLogsTarget(null)
      setOverviewSection(isMonitoringService(service) ? "monitoring" : "ai")
      return true
    },
    selectService(containerId: string) {
      setSelectedContainerId(containerId)
      const service = fleet.snapshot.services.find((entry) => entry.containerId === containerId)
      if (service) {
        setOverviewSection(isMonitoringService(service) ? "monitoring" : "ai")
      }
    },
    runServiceAction(action: ServiceAction) {
      switch (action) {
        case "logs":
          return openLogsForSelectedService()
        case "stop":
        case "restart":
        case "test":
        case "delete":
          return queueAction(action)
        default:
          return false
      }
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

      switch (viewMode) {
        case "service":
          return handleServiceKey(key)
        case "logs":
          return handleLogsKey(key)
        default:
          return handleOverviewKey(key)
      }
    },
  }

  function handleOverviewKey(key: KeyLike) {
    switch (key.name) {
      case "tab":
        switchOverviewSection(key.shift ? -1 : 1)
        return true
      case "up":
      case "k":
        moveSelectionWithinOverview(-1)
        return true
      case "down":
      case "j":
        moveSelectionWithinOverview(1)
        return true
      case "return":
      case "enter":
      case "l":
        return openSelectedService()
      default:
        return false
    }
  }

  function switchOverviewSection(delta: number) {
    const nextSection = nextOverviewSection(overviewSection, aiServices.length, monitoringServices.length, delta)
    setOverviewSection(nextSection)
    const targetServices = nextSection === "ai" ? aiServices : monitoringServices
    if (targetServices.length > 0) {
      setSelectedContainerId(targetServices[0].containerId)
    }
  }

  function handleServiceKey(key: KeyLike) {
    if (key.name === "L" || (key.shift && key.name === "l")) {
      return openLogsForSelectedService()
    }

    switch (key.name) {
      case "escape":
        setViewMode("overview")
        setLogsTarget(null)
        return true
      case "tab":
        setServiceSection((current) => nextServiceSection(current, key.shift ? -1 : 1))
        return true
      case "left":
      case "h":
        if (serviceSection === "actions") {
          setServiceActionIndex((current) => (current + SERVICE_ACTIONS.length - 1) % SERVICE_ACTIONS.length)
          return true
        }
        return false
      case "right":
      case "l":
        if (serviceSection === "actions") {
          setServiceActionIndex((current) => (current + 1) % SERVICE_ACTIONS.length)
          return true
        }
        return false
      case "return":
      case "enter":
        if (serviceSection === "actions") {
          return runServiceSectionAction()
        }
        return false
      case "s":
        return queueAction("stop")
      case "r":
        return queueAction("restart")
      case "t":
        return queueAction("test")
      case "x":
        return queueAction("delete")
      default:
        return false
    }
  }

  function handleLogsKey(key: KeyLike) {
    switch (key.name) {
      case "escape":
        setViewMode("service")
        return true
      case "f":
        logs.toggleFollow()
        return true
      case "pageup":
        logs.pageUp()
        return true
      case "pagedown":
        logs.pageDown()
        return true
      case "home":
        logs.scrollHome()
        return true
      case "end":
        logs.scrollEnd()
        return true
      default:
        return false
    }
  }

  function moveSelection(delta: number) {
    const services = fleet.snapshot.services
    if (services.length === 0) {
      return
    }

    const currentIndex = selectedIndex === -1 ? 0 : selectedIndex
    const nextIndex = (currentIndex + delta + services.length) % services.length
    const nextService = services[nextIndex]
    setSelectedContainerId(nextService.containerId)
    setOverviewSection(isMonitoringService(nextService) ? "monitoring" : "ai")
    if (viewMode === "logs") {
      setLogsTarget({
        deviceId: nextService.deviceId,
        containerId: nextService.containerId,
        serviceName: nextService.name,
      })
    }
  }

  function moveSelectionWithinOverview(delta: number) {
    const services = currentOverviewServices
    if (services.length === 0) {
      return
    }

    const currentIndex = services.findIndex((service) => service.containerId === selectedContainerId)
    const safeIndex = currentIndex === -1 ? 0 : currentIndex
    const nextIndex = (safeIndex + delta + services.length) % services.length
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
      if (action === "delete") {
        setSelectedContainerId(null)
        setLogsTarget(null)
        setViewMode("overview")
      }
      setNotice(result)
      fleet.refresh()
    } catch (error) {
      setNotice({
        level: "error",
        message: `${capitalizeAction(action)} failed: ${compactActionError(action, error)}`,
      })
    } finally {
      setPendingAction(null)
    }
  }

  function openSelectedService() {
    if (!selectedService) {
      return false
    }

    setViewMode("service")
    setServiceSection("actions")
    setServiceActionIndex(0)
    setLogsTarget(null)
    return true
  }

  function openLogsForSelectedService() {
    if (!selectedService) {
      return false
    }

    setViewMode("logs")
    setLogsTarget({
      deviceId: selectedService.deviceId,
      containerId: selectedService.containerId,
      serviceName: selectedService.name,
    })
    return true
  }

  function runServiceSectionAction() {
    const action = SERVICE_ACTIONS[serviceActionIndex]
    return action ? (action === "logs" ? openLogsForSelectedService() : queueAction(action)) : false
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

function nextOverviewSection(current: OverviewSection, aiCount: number, monitoringCount: number, delta: number) {
  const sections = [
    ...(aiCount > 0 ? (["ai"] as const) : []),
    ...(monitoringCount > 0 ? (["monitoring"] as const) : []),
  ]
  if (sections.length === 0) {
    return current
  }
  const index = sections.findIndex((section) => section === current)
  const safeIndex = index === -1 ? 0 : index
  return sections[(safeIndex + delta + sections.length) % sections.length]
}

function nextServiceSection(current: ServiceSection, delta: number) {
  const sections: ServiceSection[] = ["actions", "inspector", "device"]
  const index = sections.findIndex((section) => section === current)
  return sections[(index + delta + sections.length) % sections.length]
}

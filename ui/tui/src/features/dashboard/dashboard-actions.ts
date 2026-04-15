import { removeContainer, restartContainer, stopContainer, testContainer } from "../../services/daemon-client"
import type { FleetService } from "../../contracts/fleet"

export type DashboardActionKind = "stop" | "restart" | "test" | "delete"

export type DashboardNotice = {
  level: "info" | "success" | "warning" | "error"
  message: string
}

export type DashboardConfirm = {
  action: DashboardActionKind
  message: string
}

type DashboardActionClient = {
  removeContainer: typeof removeContainer
  restartContainer: typeof restartContainer
  stopContainer: typeof stopContainer
  testContainer: typeof testContainer
}

const defaultClient: DashboardActionClient = {
  removeContainer,
  restartContainer,
  stopContainer,
  testContainer,
}

export async function runDashboardAction(action: DashboardActionKind, service: FleetService, client: DashboardActionClient = defaultClient) {
  switch (action) {
    case "stop": {
      await client.stopContainer(service.deviceId, service.containerId)
      return {
        level: "success" as const,
        message: `Stopped ${service.name}`,
      }
    }
    case "restart": {
      await client.restartContainer(service.deviceId, service.containerId)
      return {
        level: "info" as const,
        message: `Restarting ${service.name}`,
      }
    }
    case "test": {
      const result = await client.testContainer(service.deviceId, service.containerId)
      return {
        level: "success" as const,
        message: result.message?.trim() || `Service test passed for ${service.name}`,
      }
    }
    case "delete": {
      const result = await client.removeContainer(service.deviceId, service.containerId)
      const removedServices = result.removed_services ?? 0
      const removedLabel = removedServices === 1 ? "config entry" : "config entries"
      return {
        level: "success" as const,
        message:
          removedServices > 0
            ? `Deleted ${service.name} and removed ${removedServices} ${removedLabel}`
            : `Deleted ${service.name}`,
      }
    }
  }
}

export function confirmForAction(action: DashboardActionKind, service: FleetService): DashboardConfirm | null {
  switch (action) {
    case "stop":
      return {
        action,
        message: `Stop service "${service.name}"? [Y]es / [N]o`,
      }
    case "delete":
      return {
        action,
        message: `Delete service "${service.name}"? This stops and removes the container. [Y]es / [N]o`,
      }
    default:
      return null
  }
}

export function compactActionError(action: DashboardActionKind, error: unknown) {
  const message = error instanceof Error ? error.message : `unknown ${action} error`
  if (action === "test") {
    if (message.includes("status 404") || message.toLowerCase().includes("page not found")) {
      return "Service test route unavailable; restart the daemon and re-bootstrap the device agent."
    }
  }

  const normalized = message.replace(/\s+/g, " ").trim()
  if (normalized.length <= 120) {
    return normalized
  }
  return `${normalized.slice(0, 117)}...`
}

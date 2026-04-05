import type { FleetService } from "../../contracts/fleet"
import { useTheme } from "../../theme/context"
import { isAlertService } from "./normalizeFleet"

type ServiceListPaneProps = {
  services: FleetService[]
  selectedIndex: number
  terminalHeight: number
  terminalWidth: number
}

export function ServiceListPane(props: ServiceListPaneProps) {
  const theme = useTheme()
  const visibleRows = Math.max(6, props.terminalHeight - 15)
  const [start, end] = windowRange(props.services.length, props.selectedIndex, visibleRows)
  const rows = props.services.slice(start, end)

  return (
    <box
      flexGrow={1}
      border
      borderStyle="single"
      borderColor={theme.colors.border}
      backgroundColor={theme.colors.panelMuted}
      padding={1}
      flexDirection="column"
      gap={1}
    >
      <text fg={theme.colors.text}>
        <strong>Services</strong> <span fg={theme.colors.textSubtle}>[{props.services.length}]</span>
      </text>
      <text fg={theme.colors.textSubtle}>{headerLine(props.terminalWidth)}</text>

      {rows.length === 0 ? (
        <text fg={theme.colors.textSubtle}>No Yokai services are running yet.</text>
      ) : (
        rows.map((service, index) => {
          const absoluteIndex = start + index
          const selected = absoluteIndex === props.selectedIndex
          const statusColor = isAlertService(service)
            ? theme.colors.danger
            : service.deviceOnline
              ? theme.colors.success
              : theme.colors.textSubtle

          return (
            <text key={service.containerId} fg={selected ? theme.colors.selectionText : theme.colors.textMuted} bg={selected ? theme.colors.selectionBackground : theme.colors.panelMuted}>
              <span fg={selected ? theme.colors.selectionText : statusColor}>{selected ? "▌" : healthGlyph(service)}</span>{" "}
              {rowLine(service, props.terminalWidth)}
            </text>
          )
        })
      )}

      <text fg={theme.colors.textSubtle}>Arrows/J/K move selection. Press L to open logs for the selected service.</text>
    </box>
  )
}

function headerLine(terminalWidth: number) {
  if (terminalWidth < 120) {
    return `${pad("Service", 18)} ${pad("Device", 12)} ${pad("State", 10)} ${pad("Tok/s", 7)}`
  }

  return `${pad("Service", 20)} ${pad("Device", 14)} ${pad("State", 10)} ${pad("GPU", 8)} ${pad("Tok/s", 7)}`
}

function rowLine(service: FleetService, terminalWidth: number) {
  if (terminalWidth < 120) {
    return `${pad(truncate(service.name, 18), 18)} ${pad(truncate(service.deviceLabel, 12), 12)} ${pad(truncate(service.health || service.status, 10), 10)} ${pad(formatRate(service.generationTokPerSec), 7)}`
  }

  return `${pad(truncate(service.name, 20), 20)} ${pad(truncate(service.deviceLabel, 14), 14)} ${pad(truncate(service.health || service.status, 10), 10)} ${pad(formatMemory(service.gpuMemoryMB), 8)} ${pad(formatRate(service.generationTokPerSec), 7)}`
}

function healthGlyph(service: Pick<FleetService, "health" | "status" | "deviceOnline">) {
  if (!service.deviceOnline) {
    return "○"
  }
  if (isAlertService({ health: service.health, status: service.status } as Pick<FleetService, "health" | "status">)) {
    return "!"
  }
  return "●"
}

function pad(value: string, width: number) {
  return value.padEnd(width, " ")
}

function truncate(value: string, width: number) {
  if (value.length <= width) {
    return value
  }
  if (width <= 1) {
    return value.slice(0, width)
  }
  return `${value.slice(0, width - 1)}…`
}

function formatRate(value: number) {
  if (value <= 0) {
    return "-"
  }
  return value >= 100 ? `${value.toFixed(0)}` : `${value.toFixed(1)}`
}

function formatMemory(value: number) {
  if (value <= 0) {
    return "-"
  }
  if (value >= 1024) {
    return `${(value / 1024).toFixed(1)}G`
  }
  return `${value}M`
}

function windowRange(total: number, selectedIndex: number, visibleRows: number) {
  if (total <= visibleRows) {
    return [0, total] as const
  }

  const half = Math.floor(visibleRows / 2)
  let start = Math.max(0, selectedIndex - half)
  let end = start + visibleRows

  if (end > total) {
    end = total
    start = Math.max(0, end - visibleRows)
  }

  return [start, end] as const
}

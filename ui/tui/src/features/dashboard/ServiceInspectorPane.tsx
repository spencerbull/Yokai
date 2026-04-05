import type { FleetSnapshot, FleetService } from "../../contracts/fleet"
import { useTheme } from "../../theme/context"

type ServiceInspectorPaneProps = {
  pendingAction?: string | null
  snapshot: FleetSnapshot
  service: FleetService | null
}

export function ServiceInspectorPane(props: ServiceInspectorPaneProps) {
  const theme = useTheme()

  return (
    <box
      border
      borderStyle="single"
      borderColor={theme.colors.border}
      backgroundColor={theme.colors.panelMuted}
      padding={1}
      flexDirection="column"
      gap={1}
    >
      <text fg={theme.colors.text}>
        <strong>Inspector</strong>
      </text>

      {props.service ? (
        <>
          <text fg={theme.colors.text}><strong>{props.service.name}</strong></text>
          <text fg={theme.colors.textMuted}>{props.service.deviceLabel} · {props.service.type}</text>
          <MetricLine label="State" value={props.service.health || props.service.status || "running"} />
          <MetricLine label="Image" value={props.service.image || "unknown"} />
          <MetricLine label="Port" value={props.service.port > 0 ? `${props.service.port}` : "-"} />
          <MetricLine label="CPU" value={`${props.service.cpuPercent.toFixed(1)}%`} />
          <MetricLine label="RAM" value={formatMemory(props.service.memoryUsedMB)} />
          <MetricLine label="GPU" value={formatMemory(props.service.gpuMemoryMB)} />
          <MetricLine label="Output" value={formatRate(props.service.generationTokPerSec, "tok/s")} />
          <MetricLine label="Prefill" value={formatRate(props.service.promptTokPerSec, "tok/s")} />
          <MetricLine label="Uptime" value={formatUptime(props.service.uptimeSeconds)} />
          <text fg={theme.colors.textSubtle}>Actions: S stop · R restart · T test · X delete</text>
          {props.pendingAction ? <text fg={theme.colors.accent}>Pending: {props.pendingAction}</text> : null}
        </>
      ) : (
        <text fg={theme.colors.textSubtle}>No service selected.</text>
      )}

      <text fg={theme.colors.textSubtle}>
        Fleet: {props.snapshot.totals.onlineDevices}/{props.snapshot.totals.devices} devices online · {props.snapshot.totals.services} services · {props.snapshot.totals.alertServices} alerts
      </text>
    </box>
  )
}

function MetricLine(props: { label: string; value: string }) {
  const theme = useTheme()

  return (
    <text fg={theme.colors.textMuted}>
      <span fg={theme.colors.textSubtle}>{props.label}:</span> {props.value}
    </text>
  )
}

function formatMemory(value: number) {
  if (value <= 0) {
    return "-"
  }
  if (value >= 1024) {
    return `${(value / 1024).toFixed(1)} GB`
  }
  return `${value} MB`
}

function formatRate(value: number, unit: string) {
  if (value <= 0) {
    return "-"
  }
  return `${value.toFixed(1)} ${unit}`
}

function formatUptime(seconds: number) {
  if (seconds <= 0) {
    return "-"
  }

  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (hours > 0) {
    return `${hours}h ${minutes}m`
  }
  return `${minutes}m`
}

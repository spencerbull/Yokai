import type { ReactNode } from "react"

import type { FleetHistory, FleetSnapshot } from "../../contracts/fleet"
import { MetricViz } from "./UtilizationViz"
import { useTheme } from "../../theme/context"

type FleetOverviewPanelsProps = {
  history: FleetHistory
  snapshot: FleetSnapshot
  terminalWidth: number
}

export function FleetOverviewPanels(props: FleetOverviewPanelsProps) {
  const wide = props.terminalWidth >= 120
  const panelWidth = wide
    ? Math.max(30, Math.floor((props.terminalWidth - 26) / 2))
    : Math.max(28, props.terminalWidth - 18)
  const vizWidth = Math.max(22, panelWidth - 8)

  if (wide) {
    return (
      <box flexDirection="row" gap={1}>
        <FleetPanel title="AI Fleet">
          <MetricViz history={props.history.fleet.gpu} label="GPU" value={props.snapshot.totals.avgGpuUtilPercent} width={vizWidth} />
          <MetricViz history={props.history.fleet.vram} label="VRAM" value={memoryPercent(props.snapshot.totals.gpuMemoryUsedMB, props.snapshot.totals.gpuMemoryTotalMB)} width={vizWidth} />
          <OverviewLine label="GPUs" value={`${props.snapshot.totals.activeGpuCount} active / ${props.snapshot.totals.gpuCount} total`} />
          <OverviewLine label="Free" value={`${formatMemory(Math.max(0, props.snapshot.totals.gpuMemoryTotalMB - props.snapshot.totals.gpuMemoryUsedMB))} available`} />
        </FleetPanel>
        <FleetPanel title="Fleet Runtime">
          <MetricViz history={props.history.fleet.cpu} label="CPU" value={props.snapshot.totals.avgCpuPercent} width={vizWidth} />
          <MetricViz history={props.history.fleet.ram} label="RAM" value={props.snapshot.totals.avgRamPercent} width={vizWidth} />
          <OverviewLine label="Nodes" value={`${props.snapshot.totals.onlineDevices} online · ${Math.max(0, props.snapshot.totals.devices - props.snapshot.totals.onlineDevices)} offline`} />
          <OverviewLine label="Svc" value={`${props.snapshot.totals.services} total · ${props.snapshot.totals.alertServices} alert(s)`} />
        </FleetPanel>
      </box>
    )
  }

  return (
    <box flexDirection="column" gap={1}>
      <FleetPanel title="AI Fleet">
        <MetricViz history={props.history.fleet.gpu} label="GPU" value={props.snapshot.totals.avgGpuUtilPercent} width={vizWidth} />
        <MetricViz history={props.history.fleet.vram} label="VRAM" value={memoryPercent(props.snapshot.totals.gpuMemoryUsedMB, props.snapshot.totals.gpuMemoryTotalMB)} width={vizWidth} />
        <OverviewLine label="GPUs" value={`${props.snapshot.totals.activeGpuCount} active / ${props.snapshot.totals.gpuCount} total`} />
      </FleetPanel>
      <FleetPanel title="Fleet Runtime">
        <MetricViz history={props.history.fleet.cpu} label="CPU" value={props.snapshot.totals.avgCpuPercent} width={vizWidth} />
        <MetricViz history={props.history.fleet.ram} label="RAM" value={props.snapshot.totals.avgRamPercent} width={vizWidth} />
        <OverviewLine label="Nodes" value={`${props.snapshot.totals.onlineDevices} online · ${Math.max(0, props.snapshot.totals.devices - props.snapshot.totals.onlineDevices)} offline`} />
        <OverviewLine label="Svc" value={`${props.snapshot.totals.services} total · ${props.snapshot.totals.alertServices} alert(s)`} />
      </FleetPanel>
    </box>
  )
}

function FleetPanel(props: { title: string; children: ReactNode }) {
  const theme = useTheme()

  return (
    <box
      flexGrow={1}
      minHeight={7}
      border
      borderStyle="single"
      borderColor={theme.colors.border}
      backgroundColor={theme.colors.panelMuted}
      padding={1}
      flexDirection="column"
      gap={1}
    >
      <text fg={theme.colors.text}>
        <strong>{props.title}</strong>
      </text>
      {props.children}
    </box>
  )
}

function OverviewLine(props: { label: string; value: string }) {
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
    return `${(value / 1024).toFixed(1)}G`
  }
  return `${value}M`
}

function memoryPercent(usedMB: number, totalMB: number) {
  if (totalMB <= 0) {
    return 0
  }
  return (usedMB / totalMB) * 100
}

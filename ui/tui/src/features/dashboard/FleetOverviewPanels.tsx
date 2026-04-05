import type { ReactNode } from "react"

import type { FleetHistory, FleetSnapshot } from "../../contracts/fleet"
import { MetricViz } from "./UtilizationViz"
import { useTheme } from "../../theme/context"

type FleetOverviewPanelsProps = {
  contentWidth: number
  history: FleetHistory
  snapshot: FleetSnapshot
}

export function FleetOverviewPanels(props: FleetOverviewPanelsProps) {
  const wide = props.contentWidth >= 96
  const panelWidth = wide
    ? Math.max(28, Math.floor((props.contentWidth - 1) / 2) - 2)
    : Math.max(24, props.contentWidth - 6)
  const wideVizWidth = Math.max(12, Math.floor((panelWidth - 7) / 2))
  const stackedVizWidth = Math.max(18, panelWidth - 8)

  if (wide) {
    return (
      <box flexDirection="row" gap={1}>
        <FleetPanel title="AI Fleet">
          <box flexDirection="row" gap={1}>
            <MetricViz compact height={3} history={props.history.fleet.gpu} label="GPU" value={props.snapshot.totals.avgGpuUtilPercent} width={wideVizWidth} />
            <MetricViz compact detail={memoryDetail(props.snapshot.totals.gpuMemoryUsedMB, props.snapshot.totals.gpuMemoryTotalMB)} height={3} history={props.history.fleet.vram} label="VRAM" value={memoryPercent(props.snapshot.totals.gpuMemoryUsedMB, props.snapshot.totals.gpuMemoryTotalMB)} width={wideVizWidth} />
          </box>
          <OverviewLine label="GPUs" value={`${props.snapshot.totals.activeGpuCount} active / ${props.snapshot.totals.gpuCount} total`} />
          <OverviewLine label="Free" value={`${formatMemory(Math.max(0, props.snapshot.totals.gpuMemoryTotalMB - props.snapshot.totals.gpuMemoryUsedMB))} available`} />
        </FleetPanel>
        <FleetPanel title="Fleet Runtime">
          <box flexDirection="row" gap={1}>
            <MetricViz compact height={3} history={props.history.fleet.cpu} label="CPU" value={props.snapshot.totals.avgCpuPercent} width={wideVizWidth} />
            <MetricViz compact detail={memoryDetail(props.snapshot.totals.ramUsedMB, props.snapshot.totals.ramTotalMB)} height={3} history={props.history.fleet.ram} label="RAM" value={props.snapshot.totals.avgRamPercent} width={wideVizWidth} />
          </box>
          <OverviewLine label="Nodes" value={`${props.snapshot.totals.onlineDevices} online · ${Math.max(0, props.snapshot.totals.devices - props.snapshot.totals.onlineDevices)} offline`} />
          <OverviewLine label="Svc" value={`${props.snapshot.totals.services} total · ${props.snapshot.totals.alertServices} alert(s)`} />
        </FleetPanel>
      </box>
    )
  }

  return (
    <box flexDirection="column" gap={1}>
      <FleetPanel title="AI Fleet">
        <MetricViz history={props.history.fleet.gpu} label="GPU" value={props.snapshot.totals.avgGpuUtilPercent} width={stackedVizWidth} />
        <MetricViz detail={memoryDetail(props.snapshot.totals.gpuMemoryUsedMB, props.snapshot.totals.gpuMemoryTotalMB)} history={props.history.fleet.vram} label="VRAM" value={memoryPercent(props.snapshot.totals.gpuMemoryUsedMB, props.snapshot.totals.gpuMemoryTotalMB)} width={stackedVizWidth} />
        <OverviewLine label="GPUs" value={`${props.snapshot.totals.activeGpuCount} active / ${props.snapshot.totals.gpuCount} total`} />
      </FleetPanel>
      <FleetPanel title="Fleet Runtime">
        <MetricViz history={props.history.fleet.cpu} label="CPU" value={props.snapshot.totals.avgCpuPercent} width={stackedVizWidth} />
        <MetricViz detail={memoryDetail(props.snapshot.totals.ramUsedMB, props.snapshot.totals.ramTotalMB)} history={props.history.fleet.ram} label="RAM" value={props.snapshot.totals.avgRamPercent} width={stackedVizWidth} />
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
      border
      borderStyle="single"
      borderColor={theme.colors.border}
      backgroundColor={theme.colors.panelMuted}
      paddingX={1}
      paddingY={0}
      flexDirection="column"
      gap={0}
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

function memoryDetail(usedMB: number, totalMB: number) {
  if (totalMB <= 0) {
    return "-"
  }
  return `${roundGiB(usedMB)} / ${roundGiB(totalMB)} GiB`
}

function roundGiB(valueMB: number) {
  return Math.round(valueMB / 1024)
}

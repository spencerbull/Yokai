import type { FleetDevice, FleetHistory, FleetService } from "../../contracts/fleet"
import { MarqueeText } from "../shared/MarqueeText"
import { MetricViz, seriesForDevice } from "./UtilizationViz"
import { useTheme } from "../../theme/context"

type DeviceOverviewPaneProps = {
  compact?: boolean
  devices: FleetDevice[]
  history: FleetHistory
  panelWidth?: number
  selectedService: FleetService | null
}

export function DeviceOverviewPane(props: DeviceOverviewPaneProps) {
  const theme = useTheme()
  const selectedDevice =
    props.devices.find((device) => device.id === props.selectedService?.deviceId) ?? props.devices[0] ?? null
  const selectedHistory = selectedDevice ? seriesForDevice(props.history.devices, selectedDevice.id) : null
  const compact = props.compact ?? false
  const panelWidth = Math.max(22, props.panelWidth ?? (compact ? 38 : 64))
  const compactVizWidth = Math.max(12, Math.floor((panelWidth - 7) / 2))
  const fullVizWidth = Math.max(20, panelWidth - 8)

  return (
    <box
      border
      borderStyle="single"
      borderColor={theme.colors.border}
      backgroundColor={theme.colors.panelMuted}
      padding={1}
      flexDirection="column"
      gap={1}
      minHeight={compact ? 11 : 10}
    >
      <text fg={theme.colors.text}>
        <strong>Per Device</strong>
      </text>

      <box flexDirection="row" gap={1}>
        {props.devices.length === 0 ? (
          <text fg={theme.colors.textSubtle}>No devices configured.</text>
        ) : (
          props.devices.map((device) => {
            const selected = device.id === selectedDevice?.id
            return (
              <box
                key={device.id}
                border
                borderStyle={selected ? "double" : "single"}
                borderColor={selected ? theme.colors.borderStrong : theme.colors.border}
                backgroundColor={selected ? theme.colors.selectionBackground : theme.colors.panel}
                paddingX={1}
                minWidth={12}
              >
                <text fg={selected ? theme.colors.selectionText : theme.colors.textMuted}>
                  {device.online ? "●" : "○"} {truncate(device.label, 10)}
                </text>
              </box>
            )
          })
        )}
      </box>

      {selectedDevice ? (
        <>
          <text fg={theme.colors.text}>
            <strong>{selectedDevice.label}</strong>
            <span fg={theme.colors.textSubtle}> · {selectedDevice.host}</span>
          </text>
          {compact ? (
            <>
              <text fg={theme.colors.textSubtle}>
                {selectedDevice.online ? "online" : "offline"} · {selectedDevice.serviceCount} services
              </text>
              {selectedDevice.gpuCount > 0 ? (
                <box flexDirection="row">
                  <text fg={theme.colors.textSubtle}>GPU </text>
                  <MarqueeText fg={theme.colors.textMuted} text={selectedDevice.gpuName || "GPU"} width={26} active />
                </box>
              ) : (
                <text fg={theme.colors.textSubtle}>CPU-only device</text>
              )}
              <box flexDirection="row" gap={1}>
                <box flexDirection="column" flexGrow={1} gap={1}>
                  {selectedHistory ? <MetricViz compact height={2} history={selectedHistory.cpu} label="CPU" value={selectedDevice.cpuPercent} width={compactVizWidth} /> : null}
                  {selectedHistory ? <MetricViz compact height={2} history={selectedHistory.ram} label="RAM" value={selectedDevice.ramPercent} width={compactVizWidth} /> : null}
                </box>
                <box flexDirection="column" flexGrow={1} gap={1}>
                  {selectedDevice.gpuCount > 0 && selectedHistory ? <MetricViz compact height={2} history={selectedHistory.gpu} label="GPU" value={selectedDevice.gpuUtilPercent} width={compactVizWidth} /> : null}
                  {selectedDevice.gpuCount > 0 && selectedHistory ? <MetricViz compact height={2} history={selectedHistory.vram} label="VRAM" value={memoryPercent(selectedDevice.gpuMemoryUsedMB, selectedDevice.gpuMemoryTotalMB)} width={compactVizWidth} /> : null}
                </box>
              </box>
              <text fg={theme.colors.textSubtle}>
                VRAM {formatMemoryPair(selectedDevice.gpuMemoryUsedMB, selectedDevice.gpuMemoryTotalMB)}
              </text>
            </>
          ) : (
            <>
              {selectedHistory ? <MetricViz history={selectedHistory.cpu} label="CPU" value={selectedDevice.cpuPercent} width={fullVizWidth} /> : null}
              {selectedHistory ? <MetricViz history={selectedHistory.ram} label="RAM" value={selectedDevice.ramPercent} width={fullVizWidth} /> : null}
              {selectedDevice.gpuCount > 0 && selectedHistory ? <MetricViz history={selectedHistory.gpu} label="GPU" value={selectedDevice.gpuUtilPercent} width={fullVizWidth} /> : null}
              {selectedDevice.gpuCount > 0 && selectedHistory ? <MetricViz history={selectedHistory.vram} label="VRAM" value={memoryPercent(selectedDevice.gpuMemoryUsedMB, selectedDevice.gpuMemoryTotalMB)} width={fullVizWidth} /> : null}
              <DeviceLine label="GPU" value={selectedDevice.gpuName || "none"} marquee />
              <DeviceLine label="GUtil" value={selectedDevice.gpuCount > 0 ? `${selectedDevice.gpuUtilPercent.toFixed(0)}%` : "-"} />
              <DeviceLine label="VRAM" value={formatMemoryPair(selectedDevice.gpuMemoryUsedMB, selectedDevice.gpuMemoryTotalMB)} />
              <DeviceLine label="CPU" value={`${selectedDevice.cpuPercent.toFixed(0)}%`} />
              <DeviceLine label="RAM" value={`${selectedDevice.ramPercent.toFixed(0)}%`} />
              <DeviceLine label="Services" value={`${selectedDevice.serviceCount}`} />
            </>
          )}
        </>
      ) : null}
    </box>
  )
}

function DeviceLine(props: { label: string; marquee?: boolean; value: string }) {
  const theme = useTheme()

  if (props.marquee) {
    return (
      <box flexDirection="row">
        <text fg={theme.colors.textMuted}>
          <span fg={theme.colors.textSubtle}>{props.label}:</span>{" "}
        </text>
        <MarqueeText fg={theme.colors.textMuted} text={props.value} width={32} active />
      </box>
    )
  }

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

function formatMemoryPair(usedMB: number, totalMB: number) {
  if (totalMB <= 0) {
    return "-"
  }
  return `${formatMemory(usedMB)}/${formatMemory(totalMB)}`
}

function memoryPercent(usedMB: number, totalMB: number) {
  if (totalMB <= 0) {
    return 0
  }
  return (usedMB / totalMB) * 100
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

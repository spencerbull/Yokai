import { DeviceSetupOverlays } from "./DeviceSetupOverlays"
import { DeviceOverviewPane } from "../dashboard/DeviceOverviewPane"
import { ServiceCategoryPane } from "../dashboard/ServiceCategoryPane"
import { isAIService, isMonitoringService } from "../dashboard/normalizeFleet"
import { useTheme } from "../../theme/context"
import type { DevicesController } from "./useDevicesController"

type DevicesRouteProps = {
  controller: DevicesController
}

export function DevicesRoute(props: DevicesRouteProps) {
  const theme = useTheme()

  if (props.controller.viewMode === "detail") {
    return <DeviceDetailRoute controller={props.controller} />
  }

  return (
    <box flexDirection="column" gap={1} flexGrow={1}>
      {props.controller.notice ? <Banner color={noticeColor(theme, props.controller.notice.level)}>{props.controller.notice.message}</Banner> : null}
      {props.controller.error ? <Banner color={theme.colors.warning}>{props.controller.error}</Banner> : null}

      <box flexDirection="row" gap={1} flexGrow={1}>
        <box
          flexBasis="46%"
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
            <strong>Devices</strong> <span fg={theme.colors.textSubtle}>[{props.controller.devices.length}]</span>
          </text>
          {props.controller.devices.length === 0 ? (
            <text fg={theme.colors.textSubtle}>No devices configured yet. Press A to start the add-device wizard.</text>
          ) : (
            props.controller.devices.map((device) => {
              const selected = device.id === props.controller.selectedDevice?.id
              const color = device.online ? theme.colors.success : theme.colors.warning
              return (
                <box key={device.id} focusable backgroundColor={theme.colors.panelMuted} onMouseDown={() => props.controller.openDeviceView(device.id)}>
                  <text fg={selected ? theme.colors.text : theme.colors.textMuted}>
                    <span fg={selected ? theme.colors.accent : color}>{selected ? "▌" : device.online ? "●" : "○"}</span>{" "}
                    {truncate(device.label || device.id, 18)}
                    <span fg={theme.colors.textSubtle}> · {truncate(device.connection_type || "manual", 10)} · {device.agent_port}</span>
                  </text>
                </box>
              )
            })
          )}
          <text fg={theme.colors.textSubtle}>Actions: Enter open view · A add · E edit · T test · U upgrade · Shift+T bulk test · Shift+U bulk upgrade · X remove · R refresh</text>
        </box>

        <box
          width={48}
          minWidth={40}
          border
          borderStyle="single"
          borderColor={theme.colors.border}
          backgroundColor={theme.colors.panelMuted}
          padding={1}
          flexDirection="column"
          gap={1}
        >
          <text fg={theme.colors.text}>
            <strong>Details</strong>
          </text>

          {props.controller.selectedDevice ? (
            <>
              <DetailLine label="Label" value={props.controller.selectedDevice.label || props.controller.selectedDevice.id} />
              <DetailLine label="ID" value={props.controller.selectedDevice.id} />
              <DetailLine label="Host" value={props.controller.selectedDevice.host} />
              <DetailLine label="SSH User" value={props.controller.selectedDevice.ssh_user || "-"} />
              <DetailLine label="SSH Key" value={props.controller.selectedDevice.ssh_key || "-"} />
              <DetailLine label="SSH Port" value={`${props.controller.selectedDevice.ssh_port ?? 22}`} />
              <DetailLine label="Agent Port" value={`${props.controller.selectedDevice.agent_port}`} />
              <DetailLine label="Agent Token" value={props.controller.selectedDevice.agent_token ? "configured" : "missing"} />
              <DetailLine label="Tags" value={props.controller.selectedDevice.tags?.join(", ") || "-"} />
              <DetailLine label="Tunnel" value={props.controller.selectedDevice.online ? `online on ${props.controller.selectedDevice.tunnel_port}` : "offline"} />
              {props.controller.selectedDevice.tunnel_error ? <DetailError title="Tunnel Error" value={props.controller.selectedDevice.tunnel_error} /> : null}
              <text fg={theme.colors.textSubtle}>Use T to test, U to upgrade, Shift+T to test all, and Shift+U to upgrade all.</text>
            </>
          ) : (
            <text fg={theme.colors.textSubtle}>No device selected.</text>
          )}
        </box>
      </box>

      {props.controller.deleteCandidate ? (
        <Banner color={theme.colors.warning}>
          Remove device "{props.controller.deleteCandidate.label || props.controller.deleteCandidate.id}" from local config? [Y]es / [N]o
        </Banner>
      ) : null}

      {props.controller.pendingAction ? <Banner color={theme.colors.accent}>Running {props.controller.pendingAction}...</Banner> : null}

      <DeviceSetupOverlays controller={props.controller} />
    </box>
  )
}

function DeviceDetailRoute(props: { controller: DevicesController }) {
  const theme = useTheme()
  const device = props.controller.selectedDevice
  const fleetDevice = props.controller.selectedFleetDevice
  const services = props.controller.selectedServices
  const aiServices = services.filter(isAIService)
  const monitoringServices = services.filter(isMonitoringService)

  if (!device) {
    return <text fg={theme.colors.textSubtle}>No device selected.</text>
  }

  return (
    <scrollbox height={28} style={scrollboxStyle(theme)}>
      <box flexDirection="column" gap={1} paddingRight={1}>
        {props.controller.notice ? <Banner color={noticeColor(theme, props.controller.notice.level)}>{props.controller.notice.message}</Banner> : null}
        {props.controller.pendingAction ? <Banner color={theme.colors.accent}>Running {props.controller.pendingAction}...</Banner> : null}

        <box border borderStyle="single" borderColor={theme.colors.borderStrong} backgroundColor={theme.colors.panelMuted} padding={1} flexDirection="column" gap={1}>
          <text fg={theme.colors.text}><strong>{device.label || device.id}</strong></text>
          <text fg={theme.colors.textMuted}>{device.host} · {device.connection_type || "manual"} · {device.online ? "online" : "offline"}</text>
          <text fg={theme.colors.textSubtle}>Esc back · J/K switch device · T test · U upgrade · G open Grafana · X remove</text>
        </box>

        <box flexDirection="row" gap={1}>
          <box flexBasis="52%" flexGrow={1} flexDirection="column" gap={1}>
            <DeviceOverviewPane
              devices={fleetDevice ? [fleetDevice] : []}
              history={props.controller.fleetHistory}
              panelWidth={52}
              selectedDeviceId={fleetDevice?.id}
              selectedService={null}
            />
          </box>
          <box width={44} minWidth={38} border borderStyle="single" borderColor={theme.colors.border} backgroundColor={theme.colors.panelMuted} padding={1} flexDirection="column" gap={1}>
            <text fg={theme.colors.text}><strong>Details</strong></text>
            <DetailLine label="ID" value={device.id} />
            <DetailLine label="Host" value={device.host} />
            <DetailLine label="SSH User" value={device.ssh_user || "-"} />
            <DetailLine label="SSH Key" value={device.ssh_key || "-"} />
            <DetailLine label="SSH Port" value={`${device.ssh_port ?? 22}`} />
            <DetailLine label="Agent Port" value={`${device.agent_port}`} />
            <DetailLine label="Tags" value={device.tags?.join(", ") || "-"} />
            <DetailLine label="Services" value={`${services.length}`} />
            <DetailLine label="AI" value={`${aiServices.length}`} />
            <DetailLine label="Monitoring" value={`${monitoringServices.length}`} />
            {device.tunnel_error ? <DetailError title="Tunnel Error" value={device.tunnel_error} /> : null}
            {monitoringServices.some((service) => service.name.includes("grafana") || service.image.includes("grafana")) ? (
              <ActionText keys="G" onSelect={props.controller.openGrafana}>Open Grafana</ActionText>
            ) : null}
          </box>
        </box>

        <ServiceCategoryPane
          title="AI Services"
          services={aiServices}
          rowWidth={88}
          selectedContainerId={undefined}
          emptyText="No AI services are running on this device."
          maxRows={Math.max(4, Math.min(8, aiServices.length || 4))}
        />
        <ServiceCategoryPane
          title="Monitoring Services"
          services={monitoringServices}
          rowWidth={88}
          selectedContainerId={undefined}
          emptyText="No monitoring services are running on this device."
          maxRows={Math.max(4, Math.min(8, monitoringServices.length || 4))}
        />

        <DeviceSetupOverlays controller={props.controller} />
      </box>
    </scrollbox>
  )
}

function scrollboxStyle(theme: ReturnType<typeof useTheme>) {
  return {
    rootOptions: { backgroundColor: theme.colors.panel },
    wrapperOptions: { backgroundColor: theme.colors.panel },
    viewportOptions: { backgroundColor: theme.colors.panel },
    contentOptions: { backgroundColor: theme.colors.panel },
    scrollbarOptions: {
      trackOptions: {
        foregroundColor: theme.colors.borderStrong,
        backgroundColor: theme.colors.panelMuted,
      },
    },
  }
}

function Banner(props: { children: string; color: string }) {
  const theme = useTheme()

  return (
    <box border borderStyle="single" borderColor={props.color} backgroundColor={theme.colors.panelMuted} paddingX={1}>
      <text fg={props.color}>{props.children}</text>
    </box>
  )
}

function DetailLine(props: { label: string; value: string }) {
  const theme = useTheme()

  return (
    <text fg={theme.colors.textMuted}>
      <span fg={theme.colors.textSubtle}>{props.label}:</span> {truncate(props.value, 34)}
    </text>
  )
}

function DetailError(props: { title: string; value: string }) {
  const theme = useTheme()

  return (
    <box border borderStyle="single" borderColor={theme.colors.warning} backgroundColor={theme.colors.panel} padding={1} flexDirection="column">
      <text fg={theme.colors.warning}><strong>{props.title}</strong></text>
      <text fg={theme.colors.textMuted}>{truncate(props.value, 110)}</text>
    </box>
  )
}

function ActionText(props: { children: string; keys: string; onSelect: () => void }) {
  const theme = useTheme()
  return (
    <text fg={theme.colors.accent} onMouseDown={props.onSelect}>
      [{props.keys}] {props.children}
    </text>
  )
}

function noticeColor(theme: ReturnType<typeof useTheme>, level: "info" | "success" | "warning" | "error") {
  switch (level) {
    case "success":
      return theme.colors.success
    case "warning":
      return theme.colors.warning
    case "error":
      return theme.colors.danger
    default:
      return theme.colors.accent
  }
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

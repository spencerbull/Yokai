import { AddDeviceSourceModal } from "./AddDeviceSourceModal"
import { DeviceEditorModal } from "./DeviceEditorModal"
import { SSHImportModal } from "./SSHImportModal"
import { TailscaleImportModal } from "./TailscaleImportModal"
import { useTheme } from "../../theme/context"
import type { DevicesController } from "./useDevicesController"

type DevicesRouteProps = {
  controller: DevicesController
}

export function DevicesRoute(props: DevicesRouteProps) {
  const theme = useTheme()

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
                <text key={device.id} fg={selected ? theme.colors.selectionText : theme.colors.textMuted} bg={selected ? theme.colors.selectionBackground : theme.colors.panelMuted}>
                  <span fg={selected ? theme.colors.selectionText : color}>{selected ? "▌" : device.online ? "●" : "○"}</span>{" "}
                  {truncate(device.label || device.id, 18)}
                  <span fg={selected ? theme.colors.selectionText : theme.colors.textSubtle}> · {truncate(device.connection_type || "manual", 10)} · {device.agent_port}</span>
                </text>
              )
            })
          )}
          <text fg={theme.colors.textSubtle}>Actions: A add wizard · E edit · T test · X remove · R refresh</text>
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
              <text fg={theme.colors.textSubtle}>Select a device with Up/Down, then use the action keys above.</text>
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

      {props.controller.addSource ? (
        <AddDeviceSourceModal
          selectedIndex={props.controller.addSource.selectedIndex}
          onSelect={props.controller.selectAddSource}
        />
      ) : null}

      {props.controller.editor ? (
        <DeviceEditorModal field={props.controller.editor.field} form={props.controller.editor.form} onChange={props.controller.setEditorValue} />
      ) : null}

      {props.controller.importer ? (
        <SSHImportModal error={props.controller.importer.error} hosts={props.controller.importer.hosts} loading={props.controller.importer.loading} selectedIndex={props.controller.importer.selectedIndex} />
      ) : null}

      {props.controller.tailscaleImporter ? (
        <TailscaleImportModal
          error={props.controller.tailscaleImporter.error}
          peers={props.controller.tailscaleImporter.peers}
          query={props.controller.tailscaleImporter.query}
          selectedIndex={props.controller.tailscaleImporter.selectedIndex}
          showTagHelp={props.controller.tailscaleImporter.showTagHelp}
          status={props.controller.tailscaleImporter.status}
          visiblePeers={props.controller.visiblePeers}
          onQueryChange={props.controller.setTailscaleQuery}
        />
      ) : null}
    </box>
  )
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

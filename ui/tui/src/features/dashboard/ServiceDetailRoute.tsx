import type { ReactNode } from "react"

import { DashboardActionBanner } from "./DashboardActionBanner"
import { DeviceOverviewPane } from "./DeviceOverviewPane"
import { LogsPane } from "../logs/LogsPane"
import { ServiceInspectorPane } from "./ServiceInspectorPane"
import { useTheme } from "../../theme/context"
import type { DashboardController } from "./useDashboardController"

type ServiceDetailRouteProps = {
  contentHeight: number
  contentWidth: number
  controller: DashboardController
  terminalHeight: number
}

const ACTIONS: Array<{ action: "logs" | "stop" | "restart" | "test" | "delete"; label: string }> = [
  { action: "logs", label: "View Logs" },
  { action: "restart", label: "Restart" },
  { action: "stop", label: "Stop" },
  { action: "test", label: "Test" },
  { action: "delete", label: "Delete" },
]

export function ServiceDetailRoute(props: ServiceDetailRouteProps) {
  const theme = useTheme()
  const service = props.controller.selectedService
  const wide = props.contentWidth >= 110
  const viewportHeight = Math.max(12, props.contentHeight)
  const rightColumnWidth = wide ? Math.max(34, Math.min(44, Math.floor(props.contentWidth * 0.3))) : props.contentWidth
  const leftColumnWidth = wide ? Math.max(40, props.contentWidth - rightColumnWidth - 1) : props.contentWidth
  const positionLabel = props.controller.selectedIndex >= 0 ? `${props.controller.selectedIndex + 1} of ${props.controller.snapshot.services.length}` : null

  if (!service) {
    return <text fg={theme.colors.textSubtle}>No service selected.</text>
  }

  return (
    <scrollbox height={viewportHeight} style={scrollboxStyle(theme)}>
      <box flexDirection="column" gap={1} paddingRight={1}>
        <DashboardActionBanner confirm={props.controller.confirm} notice={props.controller.notice} pendingAction={props.controller.pendingAction} />

        <box border borderStyle="single" borderColor={theme.colors.borderStrong} backgroundColor={theme.colors.panelMuted} padding={1} flexDirection="column" gap={1}>
          <box flexDirection={wide ? "row" : "column"} justifyContent="space-between" gap={1}>
            <box flexDirection="column" gap={0}>
              <text fg={theme.colors.text}><strong>{service.name}</strong></text>
              <text fg={theme.colors.textMuted}>{service.deviceLabel} · {service.type} · {service.health || service.status || "running"}{positionLabel ? ` · ${positionLabel}` : ""}</text>
            </box>
            <text fg={theme.colors.textSubtle}>Esc back to dashboard</text>
          </box>
          <FocusFrame focused={props.controller.serviceSection === "actions"}>
            <ActionPanel controller={props.controller} />
          </FocusFrame>
          <text fg={theme.colors.textSubtle}>Actions first: Shift+L opens logs · S stop · R restart · T test · X delete · Left/Right choose action · Enter runs · Tab cycles panels</text>
        </box>

        {wide ? (
          <box flexDirection="row" gap={1}>
            <box width={leftColumnWidth} minWidth={36} flexDirection="column" gap={1}>
              <FocusFrame focused={props.controller.serviceSection === "inspector"}>
                <ServiceInspectorPane pendingAction={props.controller.pendingAction} service={service} snapshot={props.controller.snapshot} />
              </FocusFrame>
            </box>
            <box width={rightColumnWidth} minWidth={34} flexDirection="column" gap={1}>
              <FocusFrame focused={props.controller.serviceSection === "device"}>
                <DeviceOverviewPane compact panelWidth={rightColumnWidth - 8} devices={props.controller.snapshot.devices} history={props.controller.history} selectedService={service} />
              </FocusFrame>
            </box>
          </box>
        ) : (
          <box flexDirection="column" gap={1}>
            <FocusFrame focused={props.controller.serviceSection === "inspector"}>
              <ServiceInspectorPane pendingAction={props.controller.pendingAction} service={service} snapshot={props.controller.snapshot} />
            </FocusFrame>
            <FocusFrame focused={props.controller.serviceSection === "device"}>
              <DeviceOverviewPane compact panelWidth={props.contentWidth - 8} devices={props.controller.snapshot.devices} history={props.controller.history} selectedService={service} />
            </FocusFrame>
          </box>
        )}
      </box>
    </scrollbox>
  )
}

export function ServiceLogsRoute(props: { contentHeight: number; controller: DashboardController; terminalWidth: number }) {
  const theme = useTheme()
  const service = props.controller.selectedService
  const routeHeight = Math.max(10, props.contentHeight)
  const logsHeight = Math.max(8, routeHeight - 8)

  return (
    <box flexDirection="column" gap={1} height={routeHeight}>
      <DashboardActionBanner confirm={props.controller.confirm} notice={props.controller.notice} pendingAction={props.controller.pendingAction} />
      <box border borderStyle="single" borderColor={theme.colors.borderStrong} backgroundColor={theme.colors.panelMuted} padding={1} flexDirection="column" gap={1}>
        <text fg={theme.colors.text}><strong>Logs</strong>{service ? ` · ${service.name}` : ""}</text>
        <text fg={theme.colors.textSubtle}>Esc back to service view · PgUp/PgDn scroll · F toggle follow</text>
      </box>
      <LogsContainer controller={props.controller} viewportHeight={logsHeight} viewportWidth={props.terminalWidth} />
    </box>
  )
}

function LogsContainer(props: { controller: DashboardController; viewportHeight: number; viewportWidth: number }) {
  const theme = useTheme()
  return (
    <box height={props.viewportHeight} border borderStyle="single" borderColor={theme.colors.border} backgroundColor={theme.colors.panelMuted} padding={1} flexDirection="column" gap={1}>
      <text fg={theme.colors.text}><strong>Log stream</strong></text>
      <box height={Math.max(6, props.viewportHeight - 3)}>
        {/* kept in a nested box so the logs pane owns the main content area cleanly */}
        <LogsPane logs={props.controller.logs} showHeader={false} viewportHeight={Math.max(6, props.viewportHeight - 3)} viewportWidth={props.viewportWidth - 4} />
      </box>
    </box>
  )
}

function ActionPanel(props: { controller: DashboardController }) {
  const theme = useTheme()

  return (
    <box border borderStyle="single" borderColor={theme.colors.border} backgroundColor={theme.colors.panelMuted} padding={1} flexDirection="column" gap={1}>
      <text fg={theme.colors.text}><strong>Service Actions</strong></text>
      <box flexDirection="row" gap={1} flexWrap="wrap">
        {ACTIONS.map((item, index) => {
          const active = props.controller.serviceActionIndex === index
          return (
            <box
              key={item.action}
              border
              borderStyle={active ? "double" : "single"}
              borderColor={active ? theme.colors.borderStrong : theme.colors.border}
              backgroundColor={theme.colors.panel}
              paddingX={1}
              onMouseDown={() => {
                props.controller.setServiceActionIndex(index)
                props.controller.runServiceAction(item.action)
              }}
            >
              <text fg={active ? theme.colors.accent : theme.colors.textMuted}>{active ? `▸ ${item.label}` : item.label}</text>
            </box>
          )
        })}
      </box>
      <text fg={theme.colors.textSubtle}>Logs is first. Left/Right choose an action and Enter runs it.</text>
    </box>
  )
}

function FocusFrame(props: { children: ReactNode; focused: boolean }) {
  const theme = useTheme()
  return (
    <box border borderStyle={props.focused ? "double" : "single"} borderColor={props.focused ? theme.colors.borderStrong : theme.colors.border} backgroundColor={theme.colors.panelMuted} padding={0}>
      {props.children}
    </box>
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

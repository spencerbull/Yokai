import { useTheme } from "../../theme/context"
import type { DashboardController } from "./useDashboardController"
import { DashboardActionBanner } from "./DashboardActionBanner"
import { DashboardStatusBanner } from "./DashboardStatusBanner"
import { DeviceOverviewPane } from "./DeviceOverviewPane"
import { FleetOverviewPanels } from "./FleetOverviewPanels"
import { isAIService, isMonitoringService } from "./normalizeFleet"
import { ServiceCategoryPane } from "./ServiceCategoryPane"
import { ServiceDetailRoute, ServiceLogsRoute } from "./ServiceDetailRoute"

type DashboardRouteProps = {
  contentHeight: number
  contentWidth: number
  controller: DashboardController
  terminalHeight: number
}

export function DashboardRoute(props: DashboardRouteProps) {
  const theme = useTheme()
  const wide = props.contentWidth >= 110
  const aiServices = props.controller.snapshot.services.filter(isAIService)
  const monitoringServices = props.controller.snapshot.services.filter(isMonitoringService)
  const categoryRows = wide ? Math.max(4, Math.min(6, Math.floor((props.terminalHeight - 26) / 2))) : 6
  const viewportHeight = Math.max(14, props.terminalHeight - (wide ? 20 : 18))
  const rightColumnWidth = wide ? Math.max(34, Math.min(48, Math.floor(props.contentWidth * 0.3))) : props.contentWidth
  const leftColumnWidth = wide ? Math.max(36, props.contentWidth - rightColumnWidth - 1) : props.contentWidth
  const serviceRowWidth = Math.max(24, leftColumnWidth - 8)

  if (props.controller.status === "error" && props.controller.snapshot.services.length === 0 && props.controller.snapshot.devices.length === 0) {
    return <text fg={theme.colors.danger}>{props.controller.error ?? "Failed to load dashboard data."}</text>
  }

  if (props.controller.status === "loading" && props.controller.snapshot.services.length === 0 && props.controller.snapshot.devices.length === 0) {
    return <text fg={theme.colors.textSubtle}>Loading fleet snapshot from the daemon...</text>
  }

  if (props.controller.viewMode === "service") {
    return <ServiceDetailRoute contentHeight={props.contentHeight} contentWidth={props.contentWidth} controller={props.controller} terminalHeight={props.terminalHeight} />
  }

  if (props.controller.viewMode === "logs") {
    return <ServiceLogsRoute contentHeight={props.contentHeight} controller={props.controller} terminalWidth={props.contentWidth} />
  }

  if (wide) {
    return (
      <scrollbox
        height={viewportHeight}
        style={scrollboxStyle(theme)}
      >
        <box flexDirection="column" gap={1} paddingRight={1}>
          <DashboardActionBanner confirm={props.controller.confirm} notice={props.controller.notice} pendingAction={props.controller.pendingAction} />
          <DashboardStatusBanner error={props.controller.error} snapshot={props.controller.snapshot} />
          <FleetOverviewPanels contentWidth={props.contentWidth} history={props.controller.history} snapshot={props.controller.snapshot} />
          <box flexDirection="row" gap={1}>
            <box width={leftColumnWidth} minWidth={36} flexDirection="column" gap={1}>
              <ServiceCategoryPane
                title="AI Services"
                services={aiServices}
                onSelect={props.controller.openService}
                rowWidth={serviceRowWidth}
                selectedContainerId={props.controller.selectedService?.containerId}
                sectionFocused={props.controller.overviewSection === "ai"}
                emptyText="No AI services are running."
                maxRows={categoryRows}
              />
              <ServiceCategoryPane
                title="Monitoring Services"
                services={monitoringServices}
                onSelect={props.controller.openService}
                rowWidth={serviceRowWidth}
                selectedContainerId={props.controller.selectedService?.containerId}
                sectionFocused={props.controller.overviewSection === "monitoring"}
                emptyText="No monitoring services are running."
                maxRows={categoryRows}
              />
            </box>
            <box width={rightColumnWidth} minWidth={34} border borderStyle="single" borderColor={theme.colors.border} backgroundColor={theme.colors.panelMuted} padding={1} flexDirection="column" justifyContent="center" gap={1}>
              <text fg={theme.colors.text}><strong>Service View</strong></text>
              <text fg={theme.colors.textMuted}>Tab selects AI vs Monitoring sections. J/K moves within that section. Enter or click opens the dedicated service view.</text>
              <text fg={theme.colors.textSubtle}>The service view shows device utilization charts, inspector details, actions, and a full-screen logs view.</text>
            </box>
          </box>
        </box>
      </scrollbox>
    )
  }

  return (
    <scrollbox height={viewportHeight} style={scrollboxStyle(theme)}>
      <box flexDirection="column" gap={1} paddingRight={1}>
        <DashboardActionBanner confirm={props.controller.confirm} notice={props.controller.notice} pendingAction={props.controller.pendingAction} />
        <DashboardStatusBanner error={props.controller.error} snapshot={props.controller.snapshot} />
        <FleetOverviewPanels contentWidth={props.contentWidth} history={props.controller.history} snapshot={props.controller.snapshot} />
        <DeviceOverviewPane panelWidth={Math.max(24, props.contentWidth - 8)} devices={props.controller.snapshot.devices} history={props.controller.history} selectedService={props.controller.selectedService} />
        <ServiceCategoryPane
          title="AI Services"
          services={aiServices}
          onSelect={props.controller.openService}
          rowWidth={Math.max(24, props.contentWidth - 8)}
          selectedContainerId={props.controller.selectedService?.containerId}
          sectionFocused={props.controller.overviewSection === "ai"}
          emptyText="No AI services are running."
          maxRows={categoryRows}
        />
        <ServiceCategoryPane
          title="Monitoring Services"
          services={monitoringServices}
          onSelect={props.controller.openService}
          rowWidth={Math.max(24, props.contentWidth - 8)}
          selectedContainerId={props.controller.selectedService?.containerId}
          sectionFocused={props.controller.overviewSection === "monitoring"}
          emptyText="No monitoring services are running."
          maxRows={categoryRows}
        />
        <box border borderStyle="single" borderColor={theme.colors.border} backgroundColor={theme.colors.panelMuted} padding={1} flexDirection="column" gap={1}>
          <text fg={theme.colors.text}><strong>Service View</strong></text>
          <text fg={theme.colors.textMuted}>Tab switches between sections. Enter on a selected service opens the dedicated service view.</text>
        </box>
      </box>
    </scrollbox>
  )
}

function scrollboxStyle(theme: ReturnType<typeof useTheme>) {
  return {
    rootOptions: {
      backgroundColor: theme.colors.panel,
    },
    wrapperOptions: {
      backgroundColor: theme.colors.panel,
    },
    viewportOptions: {
      backgroundColor: theme.colors.panel,
    },
    contentOptions: {
      backgroundColor: theme.colors.panel,
    },
    scrollbarOptions: {
      trackOptions: {
        foregroundColor: theme.colors.borderStrong,
        backgroundColor: theme.colors.panelMuted,
      },
    },
  }
}

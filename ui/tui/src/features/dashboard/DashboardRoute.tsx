import { LogsPane } from "../logs/LogsPane"
import { useTheme } from "../../theme/context"
import type { DashboardController } from "./useDashboardController"
import { DeviceOverviewPane } from "./DeviceOverviewPane"
import { DashboardActionBanner } from "./DashboardActionBanner"
import { DashboardStatusBanner } from "./DashboardStatusBanner"
import { FleetOverviewPanels } from "./FleetOverviewPanels"
import { isAIService, isMonitoringService } from "./normalizeFleet"
import { ServiceInspectorPane } from "./ServiceInspectorPane"
import { ServiceCategoryPane } from "./ServiceCategoryPane"

type DashboardRouteProps = {
  controller: DashboardController
  terminalHeight: number
  terminalWidth: number
}

export function DashboardRoute(props: DashboardRouteProps) {
  const theme = useTheme()
  const wide = props.terminalWidth >= 120
  const aiServices = props.controller.snapshot.services.filter(isAIService)
  const monitoringServices = props.controller.snapshot.services.filter(isMonitoringService)
  const categoryRows = wide ? Math.max(4, Math.min(6, Math.floor((props.terminalHeight - 26) / 2))) : 6
  const viewportHeight = Math.max(14, props.terminalHeight - (wide ? 20 : 18))

  if (props.controller.status === "error" && props.controller.snapshot.services.length === 0 && props.controller.snapshot.devices.length === 0) {
    return <text fg={theme.colors.danger}>{props.controller.error ?? "Failed to load dashboard data."}</text>
  }

  if (props.controller.status === "loading" && props.controller.snapshot.services.length === 0 && props.controller.snapshot.devices.length === 0) {
    return <text fg={theme.colors.textSubtle}>Loading fleet snapshot from the daemon...</text>
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
          <FleetOverviewPanels history={props.controller.history} snapshot={props.controller.snapshot} terminalWidth={props.terminalWidth} />
          <box flexDirection="row" gap={1}>
            <box flexBasis="58%" flexGrow={1} minWidth={46} flexDirection="column" gap={1}>
              <ServiceCategoryPane
                title="AI Services"
                services={aiServices}
                onSelect={props.controller.selectService}
                rowWidth={48}
                selectedContainerId={props.controller.selectedService?.containerId}
                emptyText="No AI services are running."
                maxRows={categoryRows}
              />
              <ServiceCategoryPane
                title="Monitoring Services"
                services={monitoringServices}
                onSelect={props.controller.selectService}
                rowWidth={48}
                selectedContainerId={props.controller.selectedService?.containerId}
                emptyText="No monitoring services are running."
                maxRows={categoryRows}
              />
            </box>
            <box width={44} minWidth={38} flexDirection="column" gap={1}>
              <DeviceOverviewPane compact panelWidth={40} devices={props.controller.snapshot.devices} history={props.controller.history} selectedService={props.controller.selectedService} />
              <ServiceInspectorPane pendingAction={props.controller.pendingAction} service={props.controller.selectedService} snapshot={props.controller.snapshot} />
              <LogsPane logs={props.controller.logs} terminalHeight={props.terminalHeight} terminalWidth={props.terminalWidth} />
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
        <FleetOverviewPanels history={props.controller.history} snapshot={props.controller.snapshot} terminalWidth={props.terminalWidth} />
        <DeviceOverviewPane panelWidth={Math.max(30, props.terminalWidth - 16)} devices={props.controller.snapshot.devices} history={props.controller.history} selectedService={props.controller.selectedService} />
        <ServiceCategoryPane
          title="AI Services"
          services={aiServices}
          onSelect={props.controller.selectService}
          rowWidth={Math.max(30, props.terminalWidth - 16)}
          selectedContainerId={props.controller.selectedService?.containerId}
          emptyText="No AI services are running."
          maxRows={categoryRows}
        />
        <ServiceCategoryPane
          title="Monitoring Services"
          services={monitoringServices}
          onSelect={props.controller.selectService}
          rowWidth={Math.max(30, props.terminalWidth - 16)}
          selectedContainerId={props.controller.selectedService?.containerId}
          emptyText="No monitoring services are running."
          maxRows={categoryRows}
        />
        <ServiceInspectorPane pendingAction={props.controller.pendingAction} service={props.controller.selectedService} snapshot={props.controller.snapshot} />
        <LogsPane logs={props.controller.logs} terminalHeight={props.terminalHeight} terminalWidth={props.terminalWidth} />
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

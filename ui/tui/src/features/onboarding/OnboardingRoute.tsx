import { ADD_DEVICE_SOURCES } from "../devices/AddDeviceSourceModal"
import { DeviceSetupOverlays } from "../devices/DeviceSetupOverlays"
import type { DevicesController } from "../devices/useDevicesController"
import { useTheme } from "../../theme/context"

type OnboardingRouteProps = {
  contentWidth: number
  controller: DevicesController
}

export function OnboardingRoute(props: OnboardingRouteProps) {
  const theme = useTheme()
  const cardWidth = Math.max(20, Math.floor((props.contentWidth - 4) / 3))
  const wide = props.contentWidth >= 96

  return (
    <box flexDirection="column" gap={1} flexGrow={1} justifyContent="center">
      {props.controller.notice ? <Banner color={noticeColor(theme, props.controller.notice.level)}>{props.controller.notice.message}</Banner> : null}
      {props.controller.error ? <Banner color={theme.colors.warning}>{props.controller.error}</Banner> : null}
      {props.controller.pendingAction ? <Banner color={theme.colors.accent}>Running {props.controller.pendingAction}...</Banner> : null}

      <box
        border
        borderStyle="rounded"
        borderColor={theme.colors.borderStrong}
        backgroundColor={theme.colors.panelMuted}
        padding={2}
        flexDirection="column"
        gap={1}
      >
        <text fg={theme.colors.text}>
          <strong>Add your first GPU device</strong>
        </text>
        <text fg={theme.colors.textMuted}>
          Yokai will connect to the device, run preflight checks, deploy the Yokai agent, and then bring the host into the dashboard automatically.
        </text>
        <text fg={theme.colors.textSubtle}>
          Press <span fg={theme.colors.textMuted}>Enter</span> or <span fg={theme.colors.textMuted}>A</span> to choose a connection source.
        </text>
      </box>

      <box flexDirection={wide ? "row" : "column"} gap={1}>
        {ADD_DEVICE_SOURCES.map((source, index) => (
          <box
            key={source.title}
            width={wide ? cardWidth : "100%"}
            border
            borderStyle="single"
            borderColor={theme.colors.border}
            backgroundColor={theme.colors.panelMuted}
            padding={1}
            flexDirection="column"
            gap={1}
            onMouseDown={() => props.controller.selectAddSource(index)}
          >
            <text fg={theme.colors.text}>
              <strong>{index + 1}. {source.title}</strong>
            </text>
            <text fg={theme.colors.textMuted}>{source.description}</text>
          </box>
        ))}
      </box>

      <box
        border
        borderStyle="single"
        borderColor={theme.colors.border}
        backgroundColor={theme.colors.panel}
        padding={1}
        flexDirection="column"
        gap={0}
      >
        <text fg={theme.colors.text}>
          <strong>What happens next</strong>
        </text>
        <text fg={theme.colors.textSubtle}>1. Choose Manual, SSH Config, or Tailscale.</text>
        <text fg={theme.colors.textSubtle}>2. Review SSH credentials and connection details.</text>
        <text fg={theme.colors.textSubtle}>3. Yokai bootstraps the remote agent and adds the device automatically.</text>
      </box>

      <DeviceSetupOverlays controller={props.controller} />
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

import type { FleetSnapshot } from "../../contracts/fleet"
import { useTheme } from "../../theme/context"

type DashboardStatusBannerProps = {
  error?: string
  snapshot: FleetSnapshot
}

export function DashboardStatusBanner(props: DashboardStatusBannerProps) {
  const theme = useTheme()

  if (props.error) {
    return <Banner color={theme.colors.warning}>{props.error}</Banner>
  }

  if (props.snapshot.totals.devices > 0 && props.snapshot.totals.onlineDevices === 0) {
    return (
      <Banner color={theme.colors.warning}>
        No live fleet data yet. The daemon sees {props.snapshot.totals.devices} configured device(s), but 0 tunnels are online.
        Check SSH auth or ssh-agent, then restart the daemon.
      </Banner>
    )
  }

  if (props.snapshot.totals.devices === 0) {
    return <Banner color={theme.colors.textSubtle}>No devices are configured yet. Run onboarding or add a device first.</Banner>
  }

  return null
}

function Banner(props: { color: string; children: string }) {
  const theme = useTheme()

  return (
    <box
      border
      borderStyle="single"
      borderColor={props.color}
      backgroundColor={theme.colors.panelMuted}
      paddingX={1}
      paddingY={0}
    >
      <text fg={props.color}>{props.children}</text>
    </box>
  )
}

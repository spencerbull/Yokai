import type { AppRouteId } from "../../app/routes"
import { useTheme } from "../../theme/context"

type RoutePlaceholderProps = {
  routeId: AppRouteId
  summary: string
  status: string
}

export function RoutePlaceholder(props: RoutePlaceholderProps) {
  const theme = useTheme()

  return (
    <box flexDirection="column" gap={1}>
      <text fg={theme.colors.text}>
        <strong>{labelForRoute(props.routeId)}</strong>
      </text>
      <text fg={theme.colors.textMuted}>{props.summary}</text>
      <text fg={theme.colors.textSubtle}>Status: {props.status}</text>
      <text fg={theme.colors.textSubtle}>
        This route is still scaffolded while the migration focuses on the shell,
        dashboard, logs, and theme system.
      </text>
    </box>
  )
}

function labelForRoute(routeId: AppRouteId) {
  switch (routeId) {
    case "devices":
      return "Devices"
    case "deploy":
      return "Deploy"
    case "settings":
      return "Settings"
    default:
      return "Dashboard"
  }
}

import type { DashboardConfirm, DashboardNotice } from "./dashboard-actions"
import { useTheme } from "../../theme/context"

type DashboardActionBannerProps = {
  confirm: DashboardConfirm | null
  notice: DashboardNotice | null
  pendingAction: string | null
}

export function DashboardActionBanner(props: DashboardActionBannerProps) {
  const theme = useTheme()

  if (props.confirm) {
    return (
      <Banner borderColor={theme.colors.warning} textColor={theme.colors.warning}>
        {props.confirm.message}
      </Banner>
    )
  }

  if (props.pendingAction) {
    return (
      <Banner borderColor={theme.colors.accent} textColor={theme.colors.textMuted}>
        Running {props.pendingAction} action...
      </Banner>
    )
  }

  if (!props.notice) {
    return null
  }

  return (
    <Banner borderColor={colorForNotice(theme, props.notice.level)} textColor={colorForNotice(theme, props.notice.level)}>
      {props.notice.message}
    </Banner>
  )
}

function Banner(props: { borderColor: string; textColor: string; children: string }) {
  const theme = useTheme()

  return (
    <box
      border
      borderStyle="single"
      borderColor={props.borderColor}
      backgroundColor={theme.colors.panelMuted}
      paddingX={1}
      paddingY={0}
    >
      <text fg={props.textColor}>{props.children}</text>
    </box>
  )
}

function colorForNotice(theme: ReturnType<typeof useTheme>, level: DashboardNotice["level"]) {
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

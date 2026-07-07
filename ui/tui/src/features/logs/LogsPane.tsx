import { useTheme } from "../../theme/context"
import type { LogsStreamState } from "./useLogsStream"

type LogsPaneProps = {
  logs: LogsStreamState
  showHeader?: boolean
  viewportHeight: number
  viewportWidth: number
}

export function LogsPane(props: LogsPaneProps) {
  const theme = useTheme()
  const showHeader = props.showHeader ?? true
  const metaLines = props.logs.target
    ? (showHeader ? 3 : 1) + (props.logs.error ? 1 : 0)
    : (showHeader ? 2 : 1)
  const visibleLines = Math.max(4, props.viewportHeight - metaLines)
  const start = props.logs.follow ? Math.max(0, props.logs.lines.length - visibleLines) : props.logs.offset
  const end = Math.min(props.logs.lines.length, start + visibleLines)
  const visible = props.logs.lines.slice(start, end)
  const contentHeight = Math.max(4, props.viewportHeight - metaLines + 1)

  return (
    <box
      height={props.viewportHeight}
      border
      borderStyle="single"
      borderColor={theme.colors.border}
      backgroundColor={theme.colors.panelMuted}
      padding={1}
      flexDirection="column"
      gap={1}
      flexGrow={1}
    >
      {showHeader ? (
        <text fg={theme.colors.text}>
          <strong>Logs</strong>
          {props.logs.target ? <span fg={theme.colors.textSubtle}> [{props.logs.target.serviceName}]</span> : null}
        </text>
      ) : null}

      {props.logs.target ? (
        <>
          <text fg={theme.colors.textSubtle}>
            {statusLabel(props.logs.status, props.logs.follow)} · PgUp/PgDn scroll · F follow
          </text>

          {props.logs.error ? <text fg={theme.colors.danger}>{props.logs.error}</text> : null}

          <scrollbox height={contentHeight} style={scrollboxStyle(theme)}>
            <box flexDirection="column">
              {visible.length === 0 ? (
                <text fg={theme.colors.textSubtle}>{emptyStateMessage(props.logs.status)}</text>
              ) : (
                visible.map((line, index) => (
                  <text key={`${start + index}-${line}`} fg={theme.colors.textMuted}>
                    <span fg={theme.colors.textSubtle}>{String(start + index + 1).padStart(4, " ")}</span> {truncate(line, logLineWidth(props.viewportWidth))}
                  </text>
                ))
              )}
            </box>
          </scrollbox>
        </>
      ) : (
        <text fg={theme.colors.textSubtle}>Open a service first, then use the top action bar or Shift+L to open a contextual live log stream.</text>
      )}
    </box>
  )
}

function statusLabel(status: string, follow: boolean) {
  const followLabel = follow ? "FOLLOW" : "MANUAL"

  switch (status) {
    case "connecting":
      return `connecting · ${followLabel}`
    case "open":
      return `streaming · ${followLabel}`
    case "error":
      return `error · ${followLabel}`
    default:
      return `idle · ${followLabel}`
  }
}

function emptyStateMessage(status: string) {
  switch (status) {
    case "connecting":
      return "Connecting to the daemon log stream..."
    case "error":
      return "Log stream ended with an error."
    default:
      return "No log lines received yet."
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

function logLineWidth(terminalWidth: number) {
  if (terminalWidth >= 120) {
    return Math.max(48, terminalWidth - 12)
  }
  return Math.max(24, terminalWidth - 16)
}

function scrollboxStyle(theme: ReturnType<typeof useTheme>) {
  return {
    rootOptions: { backgroundColor: theme.colors.panelMuted },
    wrapperOptions: { backgroundColor: theme.colors.panelMuted },
    viewportOptions: { backgroundColor: theme.colors.panelMuted },
    contentOptions: { backgroundColor: theme.colors.panelMuted },
    scrollbarOptions: {
      trackOptions: {
        foregroundColor: theme.colors.borderStrong,
        backgroundColor: theme.colors.panel,
      },
    },
  }
}

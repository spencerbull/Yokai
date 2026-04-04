import { useTheme } from "../../theme/context"
import type { LogsStreamState } from "./useLogsStream"

type LogsPaneProps = {
  logs: LogsStreamState
  terminalWidth: number
  terminalHeight: number
}

export function LogsPane(props: LogsPaneProps) {
  const theme = useTheme()
  const visibleLines = Math.max(6, props.terminalWidth >= 120 ? Math.floor((props.terminalHeight - 14) / 2) : Math.floor((props.terminalHeight - 18) / 3))
  const start = props.logs.follow ? Math.max(0, props.logs.lines.length - visibleLines) : props.logs.offset
  const end = Math.min(props.logs.lines.length, start + visibleLines)
  const visible = props.logs.lines.slice(start, end)

  return (
    <box
      border
      borderStyle="single"
      borderColor={theme.colors.border}
      backgroundColor={theme.colors.panelMuted}
      padding={1}
      flexDirection="column"
      gap={1}
      flexGrow={1}
    >
      <text fg={theme.colors.text}>
        <strong>Logs</strong>
        {props.logs.target ? <span fg={theme.colors.textSubtle}> [{props.logs.target.serviceName}]</span> : null}
      </text>

      {props.logs.target ? (
        <>
          <text fg={theme.colors.textSubtle}>
            {statusLabel(props.logs.status, props.logs.follow)} · PgUp/PgDn scroll · F follow · Esc close
          </text>

          {props.logs.error ? <text fg={theme.colors.danger}>{props.logs.error}</text> : null}

          {visible.length === 0 ? (
            <text fg={theme.colors.textSubtle}>{emptyStateMessage(props.logs.status)}</text>
          ) : (
            visible.map((line, index) => (
              <text key={`${start + index}-${line}`} fg={theme.colors.textMuted}>
                <span fg={theme.colors.textSubtle}>{String(start + index + 1).padStart(4, " ")}</span> {truncate(line, logLineWidth(props.terminalWidth))}
              </text>
            ))
          )}
        </>
      ) : (
        <text fg={theme.colors.textSubtle}>Press L on a selected service to open a contextual live log stream.</text>
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
    return 36
  }
  return Math.max(24, terminalWidth - 16)
}

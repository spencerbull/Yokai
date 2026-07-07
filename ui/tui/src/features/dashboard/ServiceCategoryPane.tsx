import type { FleetService } from "../../contracts/fleet"
import { MarqueeText } from "../shared/MarqueeText"
import { useTheme } from "../../theme/context"
import { isAlertService } from "./normalizeFleet"

type ServiceCategoryPaneProps = {
  onSelect?: (containerId: string) => void
  rowWidth?: number
  sectionFocused?: boolean
  services: FleetService[]
  selectedContainerId?: string
  title: string
  emptyText: string
  maxRows: number
}

export function ServiceCategoryPane(props: ServiceCategoryPaneProps) {
  const theme = useTheme()
  const rowWidth = props.rowWidth ?? 52
  const selectedIndex = props.selectedContainerId
    ? props.services.findIndex((service) => service.containerId === props.selectedContainerId)
    : -1
  const [start, end] = windowRange(props.services.length, selectedIndex, props.maxRows)
  const rows = props.services.slice(start, end)

  return (
    <box
      border
      borderStyle={props.sectionFocused ? "double" : "single"}
      borderColor={props.sectionFocused ? theme.colors.borderStrong : theme.colors.border}
      backgroundColor={theme.colors.panelMuted}
      padding={1}
      flexDirection="column"
      gap={1}
      minHeight={props.maxRows + 4}
    >
      <text fg={theme.colors.text}>
        <strong>{props.title}</strong> <span fg={theme.colors.textSubtle}>[{props.services.length}]</span>
      </text>

      {rows.length === 0 ? (
        <text fg={theme.colors.textSubtle}>{props.emptyText}</text>
      ) : (
        rows.map((service) => {
          const selected = service.containerId === props.selectedContainerId
          const color = isAlertService(service) ? theme.colors.danger : theme.colors.success

          return (
            <box
              key={service.containerId}
              focusable
              flexDirection="row"
              backgroundColor={theme.colors.panelMuted}
              onMouseDown={() => props.onSelect?.(service.containerId)}
            >
              <text fg={selected ? theme.colors.accent : color}>
                {selected ? "▌" : service.deviceOnline ? "●" : "○"}
              </text>
              <text fg={selected ? theme.colors.accent : theme.colors.textMuted}>
                {" "}
              </text>
              <MarqueeText
                active={selected}
                bg={theme.colors.panelMuted}
                fg={selected ? theme.colors.text : theme.colors.textMuted}
                text={service.name}
                width={Math.max(10, rowWidth - 28)}
              />
              <text fg={theme.colors.textSubtle}>
                {` · ${truncate(service.deviceLabel, 12)} · ${truncate(service.health || service.status, 10)}`}
              </text>
            </box>
          )
        })
      )}

      {props.services.length > rows.length ? (
        <text fg={theme.colors.textSubtle}>Showing {rows.length} of {props.services.length}</text>
      ) : null}
    </box>
  )
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

function windowRange(total: number, selectedIndex: number, visibleRows: number) {
  if (total <= visibleRows) {
    return [0, total] as const
  }

  const safeSelected = selectedIndex === -1 ? 0 : selectedIndex
  const half = Math.floor(visibleRows / 2)
  let start = Math.max(0, safeSelected - half)
  let end = start + visibleRows

  if (end > total) {
    end = total
    start = Math.max(0, end - visibleRows)
  }

  return [start, end] as const
}

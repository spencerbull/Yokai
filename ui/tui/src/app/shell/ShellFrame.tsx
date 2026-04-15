import type { ReactNode } from "react"

import { useTheme } from "../../theme/context"
import { getShellFrameWidth } from "./layout"
import type { AppRouteId } from "../routes"

type RouteDefinition = {
  id: AppRouteId
  label: string
  status: string
  summary: string
}

type KeymapItem = {
  keys: string
  description: string
}

type ShellFrameProps = {
  activeRoute: AppRouteId
  keymap: readonly KeymapItem[]
  mainContent: ReactNode
  onGoHome: () => void
  onSelectRoute: (routeId: AppRouteId) => void
  routes: readonly RouteDefinition[]
  shellNotice?: string
  terminalHeight: number
  terminalWidth: number
}

export function ShellFrame(props: ShellFrameProps) {
  const theme = useTheme()
  const frameWidth = getShellFrameWidth(props.terminalWidth)
  const wide = frameWidth >= 110
  const activeRoute = routeForActive(props.routes, props.activeRoute)
  const footerWidth = Math.max(20, frameWidth - 28)

  return (
    <box
      width="100%"
      height="100%"
      flexDirection="column"
      paddingX={1}
      paddingY={1}
      alignItems="center"
      backgroundColor={theme.colors.background}
    >
      <box
        flexDirection="column"
        width={frameWidth}
        flexGrow={1}
        gap={1}
      >
        <box
          flexDirection="row"
          gap={1}
        >
          <box
            focusable
            border
            borderStyle="single"
            borderColor={theme.colors.borderStrong}
            backgroundColor={theme.colors.panelMuted}
            paddingX={2}
            paddingY={0}
            onMouseDown={props.onGoHome}
          >
            <text fg={theme.colors.accent}>G. Home</text>
          </box>
          {props.routes.map((route, index) => {
            const selected = route.id === props.activeRoute
            return (
              <box
                key={route.id}
                focusable
                border
                borderStyle={selected ? "double" : "single"}
                borderColor={selected ? theme.colors.borderStrong : theme.colors.border}
                backgroundColor={theme.colors.panelMuted}
                paddingX={2}
                paddingY={0}
                onMouseDown={() => props.onSelectRoute(route.id)}
              >
                <text fg={selected ? theme.colors.accent : theme.colors.textMuted}>
                  {selected ? `▸ ${index + 1}. ${route.label}` : `${index + 1}. ${route.label}`}
                </text>
              </box>
            )
          })}
        </box>

        <box
          flexGrow={1}
          border
          borderStyle="rounded"
          borderColor={theme.colors.border}
          backgroundColor={theme.colors.panel}
          padding={1}
          flexDirection="column"
          gap={1}
        >
          <box flexDirection={wide ? "row" : "column"} justifyContent="space-between" gap={1}>
            <box flexDirection="column" gap={0}>
              <text fg={theme.colors.text}>
                <strong>{activeRoute.label}</strong>
              </text>
              <text fg={theme.colors.textSubtle}>{truncate(activeRoute.summary, frameWidth - 12)}</text>
            </box>

            <box alignItems={wide ? "flex-end" : "flex-start"} flexDirection="column" gap={0}>
              <text fg={theme.colors.accent}>█▄█ █▀█ █▄▀ ▄▀█ █</text>
              <text fg={theme.colors.textMuted}>░█░ █▄█ █░█ █▀█ █</text>
            </box>
          </box>

          <box flexGrow={1} flexDirection="column">
            {props.mainContent}
          </box>
        </box>

        <box
          border
          borderStyle="rounded"
          borderColor={theme.colors.border}
          backgroundColor={theme.colors.panelMuted}
          paddingX={1}
          flexDirection="column"
          gap={props.shellNotice ? 0 : 0}
        >
          {props.shellNotice ? <text fg={theme.colors.warning}>{props.shellNotice}</text> : null}
          <box flexDirection="row" justifyContent="space-between">
            <text fg={theme.colors.textSubtle}>{truncate(renderKeymap(props.keymap), footerWidth)}</text>
            <text fg={theme.colors.textSubtle}>OpenTUI React + Bun</text>
          </box>
        </box>
      </box>
    </box>
  )
}

function routeForActive(routes: readonly RouteDefinition[], activeRoute: AppRouteId) {
  return (
    routes.find((route) => route.id === activeRoute) ?? {
      id: "dashboard" as AppRouteId,
      label: "Dashboard",
      status: "Active",
      summary: "Yokai OpenTUI shell",
    }
  )
}

function renderKeymap(keymap: readonly KeymapItem[]) {
  return keymap.map((item) => `${item.keys} ${item.description}`).join("  |  ")
}

function truncate(value: string, width: number) {
  if (width <= 0 || value.length <= width) {
    return value
  }
  if (width <= 1) {
    return value.slice(0, width)
  }
  return `${value.slice(0, width - 1)}…`
}

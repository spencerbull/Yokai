import type { ReactNode } from "react"

import { useTheme } from "../../theme/context"
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
  backendUrl: string
  keymap: readonly KeymapItem[]
  mainContent: ReactNode
  routes: readonly RouteDefinition[]
  terminalHeight: number
  terminalWidth: number
}

export function ShellFrame(props: ShellFrameProps) {
  const theme = useTheme()
  const frameWidth = Math.max(40, Math.min(Math.max(0, props.terminalWidth - 2), 156))
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
          border
          borderStyle="rounded"
          borderColor={theme.colors.borderStrong}
          backgroundColor={theme.colors.panel}
          padding={1}
          flexDirection={wide ? "row" : "column"}
          justifyContent="space-between"
          alignItems={wide ? "center" : "flex-start"}
        >
          <box flexDirection="column" gap={0}>
            <text fg={theme.colors.text}>
              <strong>Yokai</strong> <span fg={theme.colors.textMuted}>OpenTUI</span>
            </text>
            <text fg={theme.colors.textSubtle}>daemon-backed orchestration frontend</text>
          </box>

          <box flexDirection={wide ? "row" : "column"} gap={1}>
            <HeaderPill label="Backend" value={displayBackend(props.backendUrl)} />
            <HeaderPill label="Theme" value={sessionThemeSource(theme.source, theme.themeName)} />
            <HeaderPill label="SSH" value={process.env.SSH_AUTH_SOCK ? "agent ready" : "agent missing"} tone={process.env.SSH_AUTH_SOCK ? "success" : "warning"} />
          </box>
        </box>

        <box
          flexDirection="row"
          gap={1}
        >
          {props.routes.map((route, index) => {
            const selected = route.id === props.activeRoute
            return (
              <box
                key={route.id}
                border
                borderStyle={selected ? "double" : "single"}
                borderColor={selected ? theme.colors.borderStrong : theme.colors.border}
                backgroundColor={selected ? theme.colors.selectionBackground : theme.colors.panelMuted}
                paddingX={2}
                paddingY={0}
              >
                <text fg={selected ? theme.colors.selectionText : theme.colors.textMuted}>
                  {index + 1}. {route.label}
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

            <box flexDirection={wide ? "row" : "column"} gap={1}>
              <HeaderPill label="Slice" value={activeRoute.status} />
              <HeaderPill label="Viewport" value={`${props.terminalWidth}x${props.terminalHeight}`} />
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
          flexDirection="row"
          justifyContent="space-between"
        >
          <text fg={theme.colors.textSubtle}>{truncate(renderKeymap(props.keymap), footerWidth)}</text>
          <text fg={theme.colors.textSubtle}>OpenTUI React + Bun</text>
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

function HeaderPill(props: { label: string; tone?: "default" | "success" | "warning"; value: string }) {
  const theme = useTheme()
  const color =
    props.tone === "success"
      ? theme.colors.success
      : props.tone === "warning"
        ? theme.colors.warning
        : theme.colors.border

  return (
    <box border borderStyle="single" borderColor={color} backgroundColor={theme.colors.panelMuted} paddingX={1}>
      <text fg={theme.colors.textMuted}>
        <span fg={theme.colors.textSubtle}>{props.label}:</span> {props.value}
      </text>
    </box>
  )
}

function renderKeymap(keymap: readonly KeymapItem[]) {
  return keymap.map((item) => `${item.keys} ${item.description}`).join("  |  ")
}

function sessionThemeSource(source: string, themeName?: string) {
  if (source === "omarchy") {
    return themeName ? `omarchy (${themeName})` : "omarchy"
  }

  if (source === "terminal") {
    return "terminal"
  }

  if (source === "override") {
    return "manual dark"
  }

  return "default"
}

function displayBackend(url: string) {
  return url.replace(/^https?:\/\//, "")
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

import { useState } from "react"

import { useKeyboard, useRenderer, useTerminalDimensions } from "@opentui/react"

import { DAEMON_URL } from "../config"
import { DashboardRoute } from "../features/dashboard/DashboardRoute"
import { useDashboardController } from "../features/dashboard/useDashboardController"
import { DevicesRoute } from "../features/devices/DevicesRoute"
import { useDevicesController } from "../features/devices/useDevicesController"
import { RoutePlaceholder } from "../features/shared/RoutePlaceholder"
import { SettingsView } from "../features/settings/SettingsView"
import { ThemeProvider, useTheme } from "../theme/context"
import { GLOBAL_KEYMAP } from "./keymap"
import { APP_ROUTES, type AppRouteId } from "./routes"
import { ShellFrame } from "./shell/ShellFrame"

export function App() {
  return (
    <ThemeProvider>
      <AppShell />
    </ThemeProvider>
  )
}

function AppShell() {
  const renderer = useRenderer()
  const { width, height } = useTerminalDimensions()
  const theme = useTheme()
  const [activeRoute, setActiveRoute] = useState<AppRouteId>("dashboard")
  const dashboard = useDashboardController(activeRoute === "dashboard", width, height)
  const devices = useDevicesController(activeRoute === "devices")

  useKeyboard((key) => {
    if (activeRoute === "dashboard" && dashboard.handleKey(key)) {
      return
    }

    if (activeRoute === "devices" && devices.handleKey(key)) {
      return
    }

    if (key.name === "escape") {
      renderer.destroy()
      return
    }

    if (key.name === "tab") {
      setActiveRoute((current) => shiftRoute(current, key.shift ? -1 : 1))
      return
    }

    if (["1", "2", "3", "4"].includes(key.name)) {
      const route = APP_ROUTES[Number(key.name) - 1]
      if (route) {
        setActiveRoute(route.id)
        return
      }
    }

    if (activeRoute === "settings") {
      if (key.name === "a") {
        theme.setPreference("auto")
        return
      }

      if (key.name === "d") {
        theme.setPreference("dark")
      }
    }
  })

  const active = APP_ROUTES.find((route) => route.id === activeRoute) ?? APP_ROUTES[0]
  const mainContent =
    activeRoute === "dashboard" ? (
      <DashboardRoute controller={dashboard} terminalHeight={height} terminalWidth={width} />
    ) : activeRoute === "devices" ? (
      <DevicesRoute controller={devices} />
    ) : activeRoute === "settings" ? (
      <SettingsView />
    ) : (
      <RoutePlaceholder routeId={active.id} status={active.status} summary={active.summary} />
    )

  return (
    <ShellFrame
      activeRoute={active.id}
      backendUrl={DAEMON_URL}
      keymap={GLOBAL_KEYMAP}
      mainContent={mainContent}
      routes={APP_ROUTES}
      terminalHeight={height}
      terminalWidth={width}
    />
  )
}

function shiftRoute(current: AppRouteId, delta: number): AppRouteId {
  const currentIndex = APP_ROUTES.findIndex((route) => route.id === current)
  if (currentIndex === -1) {
    return APP_ROUTES[0].id
  }

  const nextIndex = (currentIndex + delta + APP_ROUTES.length) % APP_ROUTES.length
  return APP_ROUTES[nextIndex].id
}

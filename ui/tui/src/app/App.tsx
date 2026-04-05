import { useEffect, useState } from "react"

import { useKeyboard, useRenderer, useTerminalDimensions } from "@opentui/react"

import { DAEMON_URL } from "../config"
import { DashboardRoute } from "../features/dashboard/DashboardRoute"
import { useDashboardController } from "../features/dashboard/useDashboardController"
import { DeployRoute } from "../features/deploy/DeployRoute"
import { useDeployController } from "../features/deploy/useDeployController"
import { DevicesRoute } from "../features/devices/DevicesRoute"
import { useDevicesController } from "../features/devices/useDevicesController"
import { LandingRoute } from "../features/landing/LandingRoute"
import { OnboardingRoute } from "../features/onboarding/OnboardingRoute"
import { RoutePlaceholder } from "../features/shared/RoutePlaceholder"
import { SettingsRoute } from "../features/settings/SettingsRoute"
import { useSettingsController } from "../features/settings/useSettingsController"
import { ThemeProvider, useTheme } from "../theme/context"
import { keymapForRoute } from "./keymap"
import { APP_ROUTES, type AppRouteId } from "./routes"
import { getShellContentHeight, getShellContentWidth } from "./shell/layout"
import { ShellFrame } from "./shell/ShellFrame"

type AppMode = "home" | "app"

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
  const contentWidth = getShellContentWidth(width)
  const contentHeight = getShellContentHeight(height)
  const theme = useTheme()
  const [appMode, setAppMode] = useState<AppMode>("home")
  const [activeRoute, setActiveRoute] = useState<AppRouteId>("dashboard")
  const [landingIndex, setLandingIndex] = useState(0)
  const [exitArmed, setExitArmed] = useState<null | "ctrlc" | "escape">(null)
  const dashboard = useDashboardController(activeRoute === "dashboard", width, height)
  const devices = useDevicesController(true)
  const onboardingVisible = devices.status !== "ready" || devices.devices.length === 0
  const homeVisible = !onboardingVisible && appMode === "home"
  const deploy = useDeployController(activeRoute === "deploy" && !onboardingVisible, () => setActiveRoute("dashboard"))
  const settings = useSettingsController(activeRoute === "settings" && !onboardingVisible, theme)

  useEffect(() => {
    if (!exitArmed) {
      return
    }

    const timeout = setTimeout(() => {
      setExitArmed(null)
    }, 1600)

    return () => clearTimeout(timeout)
  }, [exitArmed])

  useKeyboard((key) => {
    if (homeVisible) {
      switch (key.name) {
        case "up":
        case "k":
          setLandingIndex((current) => Math.max(0, current - 1))
          return
        case "down":
        case "j":
          setLandingIndex((current) => Math.min(APP_ROUTES.length - 1, current + 1))
          return
        case "return":
        case "enter":
          activateRoute(APP_ROUTES[landingIndex]?.id ?? "dashboard")
          return
        case "1":
        case "2":
        case "3":
        case "4":
          activateRoute(APP_ROUTES[Number(key.name) - 1]?.id ?? "dashboard")
          return
      }
    }

    if (!onboardingVisible && key.name === "g") {
      activateHome()
      return
    }

    if (onboardingVisible && devices.hasOverlayOpen) {
      if (devices.handleKey(key)) {
        return
      }
      return
    }

    if (onboardingVisible) {
      switch (key.name) {
        case "a":
        case "return":
        case "enter":
          devices.openAddWizard()
          return
        case "1":
        case "2":
        case "3":
          devices.selectAddSource(Number(key.name) - 1)
          return
        case "r":
          return
      }
    }

    if (activeRoute === "dashboard") {
      if (dashboard.handleKey(key)) {
        return
      }
    }

    if (!onboardingVisible && activeRoute === "devices") {
      if (devices.handleKey(key)) {
        return
      }
      if (devices.hasOverlayOpen) {
        return
      }
    }

    if (!onboardingVisible && activeRoute === "deploy") {
      if (deploy.handleKey(key)) {
        return
      }
      if (deploy.locksGlobalNav) {
        return
      }
    }

    if (!onboardingVisible && activeRoute === "settings") {
      if (settings.handleKey(key)) {
        return
      }
      if (settings.locksGlobalNav) {
        return
      }
    }

    if (key.ctrl && key.name === "c") {
      if (exitArmed === "ctrlc") {
        renderer.destroy()
        return
      }
      setExitArmed("ctrlc")
      return
    }

    if (key.name === "escape") {
      if (exitArmed === "escape") {
        renderer.destroy()
        return
      }
      setExitArmed("escape")
      return
    }

    setExitArmed(null)

    if (key.name === "tab") {
      activateRoute(shiftRoute(activeRoute, key.shift ? -1 : 1))
      return
    }

    if (["1", "2", "3", "4"].includes(key.name)) {
      const route = APP_ROUTES[Number(key.name) - 1]
      if (route) {
        activateRoute(route.id)
        return
      }
    }

  })

  const active = APP_ROUTES.find((route) => route.id === activeRoute) ?? APP_ROUTES[0]
  const mainContent =
    onboardingVisible ? (
      <OnboardingRoute contentWidth={contentWidth} controller={devices} />
    ) : homeVisible ? (
      <LandingRoute
        onActivate={activateRoute}
        onSelectIndex={setLandingIndex}
        routes={APP_ROUTES}
        selectedIndex={landingIndex}
        terminalHeight={height}
        terminalWidth={width}
      />
    ) : activeRoute === "dashboard" ? (
      <DashboardRoute contentHeight={contentHeight} contentWidth={contentWidth} controller={dashboard} terminalHeight={height} />
    ) : activeRoute === "devices" ? (
      <DevicesRoute controller={devices} />
    ) : activeRoute === "deploy" ? (
      <DeployRoute controller={deploy} terminalHeight={height} />
    ) : activeRoute === "settings" ? (
      <SettingsRoute controller={settings} />
    ) : (
      <RoutePlaceholder routeId={active.id} status={active.status} summary={active.summary} />
    )

  const currentKeymap = keymapForRoute({
    activeRoute,
    dashboardMode: dashboard.viewMode,
    deployStep: deploy.step,
    landingVisible: homeVisible,
    onboardingVisible,
  })

  if (homeVisible || onboardingVisible) {
    return mainContent
  }

  return (
    <ShellFrame
      activeRoute={active.id}
      keymap={currentKeymap}
      mainContent={mainContent}
      onGoHome={activateHome}
      onSelectRoute={activateRoute}
      routes={APP_ROUTES}
      shellNotice={
        exitArmed === "escape"
          ? "Press Esc again to quit"
          : exitArmed === "ctrlc"
            ? "Press Ctrl+C again to quit"
            : undefined
      }
      terminalHeight={height}
      terminalWidth={width}
    />
  )

  function activateRoute(routeId: AppRouteId) {
    setAppMode("app")
    setActiveRoute(routeId)
    const index = APP_ROUTES.findIndex((route) => route.id === routeId)
    if (index >= 0) {
      setLandingIndex(index)
    }
  }

  function activateHome() {
    setAppMode("home")
    const index = APP_ROUTES.findIndex((route) => route.id === activeRoute)
    if (index >= 0) {
      setLandingIndex(index)
    }
  }
}

function shiftRoute(current: AppRouteId, delta: number): AppRouteId {
  const currentIndex = APP_ROUTES.findIndex((route) => route.id === current)
  if (currentIndex === -1) {
    return APP_ROUTES[0].id
  }

  const nextIndex = (currentIndex + delta + APP_ROUTES.length) % APP_ROUTES.length
  return APP_ROUTES[nextIndex].id
}

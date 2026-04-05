import type { AppRouteId } from "./routes"

export type KeymapItem = {
  keys: string
  description: string
}

const BASE_NAV: readonly KeymapItem[] = [
  { keys: "G", description: "home" },
  { keys: "Tab", description: "next section" },
  { keys: "Shift+Tab", description: "previous section" },
  { keys: "1-4", description: "jump to section" },
  { keys: "Esc Esc", description: "quit" },
]

const ONBOARDING_KEYS: readonly KeymapItem[] = [
  { keys: "A/Enter", description: "choose source" },
  { keys: "1-3", description: "pick source" },
]

const LANDING_KEYS: readonly KeymapItem[] = [
  { keys: "↑/↓", description: "move selection" },
  { keys: "Enter", description: "open route" },
  { keys: "1-4", description: "jump to route" },
  { keys: "Esc Esc", description: "quit" },
]

const DASHBOARD_KEYS: readonly KeymapItem[] = [
  { keys: "Tab", description: "next section" },
  { keys: "J/K", description: "select service" },
  { keys: "Enter", description: "open service" },
]

const DEVICES_KEYS: readonly KeymapItem[] = [
  { keys: "A", description: "add device" },
  { keys: "E", description: "edit device" },
  { keys: "T/U", description: "test/upgrade" },
  { keys: "Shift+T/U", description: "bulk ops" },
  { keys: "X", description: "remove device" },
]

const SETTINGS_KEYS: readonly KeymapItem[] = [
  { keys: "A/D", description: "theme" },
  { keys: "H", description: "HF token" },
  { keys: "P", description: "defaults" },
  { keys: "V/O/L/K/X", description: "tool toggles" },
  { keys: "C", description: "configure tools" },
]

export function keymapForRoute(input: {
  activeRoute: AppRouteId
  dashboardMode?: "overview" | "service" | "logs"
  deployStep?: string
  landingVisible?: boolean
  onboardingVisible: boolean
}) {
  if (input.onboardingVisible) {
    return [...BASE_NAV, ...ONBOARDING_KEYS]
  }

  if (input.landingVisible) {
    return [...BASE_NAV, ...LANDING_KEYS]
  }

  switch (input.activeRoute) {
    case "dashboard":
      return [...BASE_NAV, ...dashboardKeys(input.dashboardMode)]
    case "devices":
      return [...BASE_NAV, ...DEVICES_KEYS]
    case "deploy":
      return [...BASE_NAV, ...deployKeys(input.deployStep)]
    case "settings":
      return [...BASE_NAV, ...SETTINGS_KEYS]
    default:
      return BASE_NAV
  }
}

function dashboardKeys(mode?: "overview" | "service" | "logs"): readonly KeymapItem[] {
  switch (mode) {
    case "service":
      return [
        { keys: "Tab", description: "next section" },
        { keys: "J/K", description: "switch service" },
        { keys: "←/→", description: "action select" },
        { keys: "Enter", description: "run action" },
        { keys: "Esc", description: "back" },
      ]
    case "logs":
      return [
        { keys: "PgUp/Dn", description: "scroll logs" },
        { keys: "F", description: "toggle follow" },
        { keys: "Esc", description: "back" },
      ]
    default:
      return DASHBOARD_KEYS
  }
}

function deployKeys(step?: string): readonly KeymapItem[] {
  switch (step) {
    case "config":
      return [
        { keys: "Tab", description: "next field" },
        { keys: "B", description: "apply BKC" },
        { keys: "M", description: "run hf-mem" },
        { keys: "F", description: "apply flags" },
        { keys: "Enter", description: "continue" },
      ]
    case "model":
      return [
        { keys: "J/K", description: "select model" },
        { keys: "Enter", description: "continue" },
      ]
    case "device":
      return [
        { keys: "J/K", description: "select device" },
        { keys: "Enter", description: "continue" },
      ]
    case "review":
      return [
        { keys: "Tab/←/→", description: "select action" },
        { keys: "Enter", description: "run selected" },
      ]
    default:
      return [
        { keys: "1-3/Enter", description: "advance wizard" },
      ]
  }
}

import type { OmarchyTheme, ThemeColors, ThemeMode, ThemePreference, ThemeSource } from "./types"

const DARK_THEME: ThemeColors = {
  background: "#0f172a",
  panel: "#111827",
  panelMuted: "#0b1220",
  border: "#334155",
  borderStrong: "#60a5fa",
  text: "#f8fafc",
  textMuted: "#cbd5e1",
  textSubtle: "#94a3b8",
  accent: "#2563eb",
  accentContrast: "#ffffff",
  selectionBackground: "#1d4ed8",
  selectionText: "#ffffff",
  success: "#16a34a",
  warning: "#d97706",
  danger: "#dc2626",
}

const LIGHT_THEME: ThemeColors = {
  background: "#f8fafc",
  panel: "#ffffff",
  panelMuted: "#f1f5f9",
  border: "#cbd5e1",
  borderStrong: "#2563eb",
  text: "#0f172a",
  textMuted: "#334155",
  textSubtle: "#64748b",
  accent: "#2563eb",
  accentContrast: "#ffffff",
  selectionBackground: "#dbeafe",
  selectionText: "#1e3a8a",
  success: "#16a34a",
  warning: "#d97706",
  danger: "#dc2626",
}

export function normalizeThemePreference(value: string | undefined): ThemePreference {
  if (value === "dark") {
    return "dark"
  }

  return "auto"
}

export function resolveTheme(input: {
  preference: ThemePreference
  terminalMode: ThemeMode | null
  omarchyTheme: OmarchyTheme | null
}) {
  if (input.preference === "dark") {
    return {
      preference: input.preference,
      resolvedMode: "dark" as ThemeMode,
      source: "override" as ThemeSource,
      colors: DARK_THEME,
    }
  }

  if (input.omarchyTheme) {
    return {
      preference: input.preference,
      resolvedMode: themeModeFromHex(input.omarchyTheme.background),
      source: "omarchy" as ThemeSource,
      themeName: input.omarchyTheme.name,
      colors: colorsFromOmarchy(input.omarchyTheme),
    }
  }

  if (input.terminalMode === "light") {
    return {
      preference: input.preference,
      resolvedMode: "light" as ThemeMode,
      source: "terminal" as ThemeSource,
      colors: LIGHT_THEME,
    }
  }

  if (input.terminalMode === "dark") {
    return {
      preference: input.preference,
      resolvedMode: "dark" as ThemeMode,
      source: "terminal" as ThemeSource,
      colors: DARK_THEME,
    }
  }

  return {
    preference: input.preference,
    resolvedMode: "dark" as ThemeMode,
    source: "default" as ThemeSource,
    colors: DARK_THEME,
  }
}

function colorsFromOmarchy(theme: OmarchyTheme): ThemeColors {
  const panel = blendHex(theme.background, theme.foreground, 0.11)
  const panelMuted = blendHex(theme.background, theme.foreground, 0.06)
  const border = blendHex(theme.background, theme.foreground, 0.2)
  const textMuted = blendHex(theme.background, theme.foreground, 0.78)
  const textSubtle = blendHex(theme.background, theme.foreground, 0.58)

  return {
    background: theme.background,
    panel,
    panelMuted,
    border,
    borderStrong: theme.accent,
    text: theme.foreground,
    textMuted,
    textSubtle,
    accent: theme.accent,
    accentContrast: theme.color15 ?? theme.selectionForeground,
    selectionBackground: theme.accent,
    selectionText: theme.color15 ?? theme.selectionForeground,
    success: theme.color10 ?? theme.color2 ?? theme.accent,
    warning: theme.color12 ?? theme.color10 ?? theme.accent,
    danger: theme.color11 ?? theme.color9 ?? theme.color3 ?? theme.accent,
  }
}

function blendHex(baseHex: string, mixHex: string, ratio: number) {
  const base = parseHex(baseHex)
  const mix = parseHex(mixHex)
  if (!base || !mix) {
    return baseHex
  }

  const safeRatio = Math.max(0, Math.min(1, ratio))
  const blended = base.map((channel, index) => {
    return Math.round(channel + (mix[index] - channel) * safeRatio)
  })

  return `#${blended.map((value) => value.toString(16).padStart(2, "0")).join("")}`
}

function themeModeFromHex(hex: string): ThemeMode {
  const channels = parseHex(hex)
  if (!channels) {
    return "dark"
  }

  const [red, green, blue] = channels
  const luminance = (0.2126 * red + 0.7152 * green + 0.0722 * blue) / 255

  return luminance > 0.6 ? "light" : "dark"
}

function parseHex(hex: string) {
  const normalized = hex.trim().replace(/^#/, "")
  if (!/^[0-9a-fA-F]{6}$/.test(normalized)) {
    return null
  }

  return [
    Number.parseInt(normalized.slice(0, 2), 16),
    Number.parseInt(normalized.slice(2, 4), 16),
    Number.parseInt(normalized.slice(4, 6), 16),
  ] as const
}

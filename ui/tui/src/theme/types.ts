export type ThemePreference = "auto" | "dark"

export type ThemeMode = "dark" | "light"

export type ThemeSource = "override" | "omarchy" | "terminal" | "default"

export type ThemeColors = {
  background: string
  panel: string
  panelMuted: string
  border: string
  borderStrong: string
  text: string
  textMuted: string
  textSubtle: string
  accent: string
  accentContrast: string
  selectionBackground: string
  selectionText: string
  success: string
  warning: string
  danger: string
}

export type ThemeState = {
  preference: ThemePreference
  resolvedMode: ThemeMode
  source: ThemeSource
  themeName?: string
  colors: ThemeColors
  isSaving: boolean
  error?: string
  setPreference: (preference: ThemePreference) => void
}

export type OmarchyTheme = {
  name?: string
  accent: string
  foreground: string
  background: string
  selectionForeground: string
  selectionBackground: string
  color0?: string
  color2?: string
  color3?: string
  color4?: string
  color8?: string
  color9?: string
  color10?: string
  color11?: string
  color12?: string
  color14?: string
  color15?: string
}

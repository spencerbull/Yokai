import { useTheme } from "../../theme/context"

export function SettingsView() {
  const theme = useTheme()

  return (
    <box flexDirection="column" gap={1}>
      <text fg={theme.colors.text}>
        <strong>Themes</strong>
      </text>
      <text fg={theme.colors.textMuted}>
        Choose whether Yokai should follow the terminal and Omarchy palette automatically
        or stay on Yokai's built-in dark theme.
      </text>

      <box flexDirection="row" gap={1}>
        <ThemeChoice
          active={theme.preference === "auto"}
          description={autoDescription(theme.source, theme.themeName)}
          keys="A"
          label="Auto"
          onSelect={() => theme.setPreference("auto")}
        />
        <ThemeChoice
          active={theme.preference === "dark"}
          description="Use Yokai's built-in dark palette regardless of terminal or Omarchy theme."
          keys="D"
          label="Dark"
          onSelect={() => theme.setPreference("dark")}
        />
      </box>

      <text fg={theme.colors.textSubtle}>Resolved mode: {theme.resolvedMode}</text>
      <text fg={theme.colors.textSubtle}>Theme source: {sourceLabel(theme.source, theme.themeName)}</text>

      {theme.isSaving ? <text fg={theme.colors.warning}>Saving theme preference...</text> : null}
      {theme.error ? <text fg={theme.colors.danger}>{theme.error}</text> : null}
    </box>
  )
}

type ThemeChoiceProps = {
  active: boolean
  description: string
  keys: string
  label: string
  onSelect: () => void
}

function ThemeChoice(props: ThemeChoiceProps) {
  const theme = useTheme()

  return (
    <box
      width={30}
      minHeight={7}
      border
      borderStyle={props.active ? "double" : "single"}
      borderColor={props.active ? theme.colors.borderStrong : theme.colors.border}
      backgroundColor={props.active ? theme.colors.selectionBackground : theme.colors.panelMuted}
      padding={1}
      flexDirection="column"
      gap={1}
      onMouseDown={props.onSelect}
    >
      <text fg={props.active ? theme.colors.selectionText : theme.colors.text}>
        <strong>{props.label}</strong> <span fg={props.active ? theme.colors.selectionText : theme.colors.textSubtle}>[{props.keys}]</span>
      </text>
      <text fg={props.active ? theme.colors.selectionText : theme.colors.textMuted}>{props.description}</text>
    </box>
  )
}

function autoDescription(source: string, themeName?: string) {
  if (source === "omarchy") {
    return `Follow Omarchy automatically${themeName ? ` using ${themeName}` : ""}.`
  }

  if (source === "terminal") {
    return "Follow terminal-reported light or dark mode automatically."
  }

  return "Prefer Omarchy or terminal auto-detection and fall back to Yokai dark mode."
}

function sourceLabel(source: string, themeName?: string) {
  switch (source) {
    case "omarchy":
      return themeName ? `omarchy (${themeName})` : "omarchy"
    case "terminal":
      return "terminal"
    case "override":
      return "manual dark override"
    default:
      return "built-in default"
  }
}

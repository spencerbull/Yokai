import type { CliRenderer } from "@opentui/core"
import type { EventEmitter } from "node:events"

import { themeModeFromHex } from "./resolve"
import type { ThemeMode } from "./types"

type ThemeRenderer = Pick<CliRenderer, "themeMode" | "getPalette" | "clearPaletteCache">
  & Pick<EventEmitter, "on" | "off">

// OpenTUI 0.1.96 reports DEC 2031 notifications through themeMode, but does
// not derive it from OSC colors. Query those colors for older terminals.
export function watchTerminalTheme(renderer: ThemeRenderer, onChange: (mode: ThemeMode) => void) {
  let stopped = false
  let querying = false
  let reportedMode = renderer.themeMode

  const handleMode = (mode: ThemeMode) => {
    reportedMode = mode
    onChange(mode)
  }
  const queryBackground = async () => {
    if (stopped || querying || reportedMode !== null) return
    querying = true
    try {
      renderer.clearPaletteCache()
      const colors = await renderer.getPalette({ timeout: 700 })
      // A DEC notification received during the query takes precedence.
      if (!stopped && reportedMode === null && colors.defaultBackground) {
        const mode = themeModeFromHex(colors.defaultBackground)
        if (mode) onChange(mode)
      }
    } catch {
      // Unsupported terminals keep the resolver's default; focus retries.
    } finally {
      querying = false
    }
  }
  const handleFocus = () => { void queryBackground() }

  renderer.on("theme_mode", handleMode)
  renderer.on("focus", handleFocus)
  // Re-read after subscribing so detection between render and effect is kept.
  reportedMode = renderer.themeMode
  if (reportedMode !== null) onChange(reportedMode)
  else void queryBackground()

  return () => {
    stopped = true
    renderer.off("theme_mode", handleMode)
    renderer.off("focus", handleFocus)
  }
}

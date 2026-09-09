import { expect, spyOn, test } from "bun:test"
import { act } from "react"
import { RGBA } from "@opentui/core"
import { testRender } from "@opentui/react/test-utils"

import * as themeContext from "../../theme/context"
import { resolveTheme } from "../../theme/resolve"
import type { DevicesController } from "../devices/useDevicesController"
import { OnboardingRoute } from "./OnboardingRoute"

for (const mode of ["light", "dark"] as const) {
  test(`onboarding paints the full ${mode} background even when the daemon is offline`, async () => {
    const theme = {
      ...resolveTheme({ preference: "auto", terminalMode: mode, omarchyTheme: null }),
      isSaving: false, setPreference() {},
    }
    const themeSpy = spyOn(themeContext, "useTheme").mockReturnValue(theme)
    const controller = {
      error: "Unable to connect. Is the computer able to access the url?",
      selectAddSource() {},
    } as DevicesController
    let setup: Awaited<ReturnType<typeof testRender>> | undefined
    try {
      setup = await testRender(<OnboardingRoute contentWidth={120} controller={controller} />, { width: 160, height: 50 })
      await setup.renderOnce()
      const frame = setup.captureSpans()
      expect(setup.captureCharFrame()).toContain("Add your first GPU device")
      expect(frame.lines[0].spans[0].bg.equals(RGBA.fromHex(theme.colors.background))).toBe(true)
      for (const line of frame.lines) {
        for (const span of line.spans) expect(span.bg.a).toBe(1)
      }
      const title = frame.lines.flatMap((line) => line.spans).find((span) => span.text.includes("Add your first GPU device"))
      expect(title!.fg.equals(RGBA.fromHex(theme.colors.text))).toBe(true)
      expect(title!.bg.equals(RGBA.fromHex(theme.colors.panelMuted))).toBe(true)
    } finally {
      if (setup) await act(async () => setup!.renderer.destroy())
      themeSpy.mockRestore()
    }
  })
}

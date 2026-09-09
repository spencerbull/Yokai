import { describe, expect, spyOn, test } from "bun:test"
import { act, useState } from "react"
import { RGBA } from "@opentui/core"
import { testRender } from "@opentui/react/test-utils"

import * as themeContext from "../../theme/context"
import { resolveTheme } from "../../theme/resolve"
import type { ThemeMode, ThemeState } from "../../theme/types"
import { SettingsRoute } from "./SettingsRoute"
import { useSettingsController, type SettingsController } from "./useSettingsController"

function themeFor(mode: ThemeMode): ThemeState {
  return {
    ...resolveTheme({ preference: "auto", terminalMode: mode, omarchyTheme: null }),
    isSaving: false,
    setPreference() {},
  }
}

describe("settings input theme colors", () => {
  for (const initialMode of ["light", "dark"] as const) {
    for (const editor of ["defaults", "hf"] as const) {
      test(`${editor} editor follows theme changes and focus from ${initialMode}`, async () => {
        let theme = themeFor(initialMode)
        const themeSpy = spyOn(themeContext, "useTheme").mockImplementation(() => theme)
        let controller!: SettingsController
        let refresh!: () => void

        function Harness() {
          const [, setRevision] = useState(0)
          refresh = () => setRevision((revision) => revision + 1)
          // Exercise the real editor behavior without fetching daemon settings.
          controller = useSettingsController(false, theme)
          return <SettingsRoute controller={controller} terminalHeight={50} />
        }

        let setup: Awaited<ReturnType<typeof testRender>> | undefined
        try {
          setup = await testRender(<Harness />, { width: 160, height: 50 })
          await act(async () => {
            if (editor === "defaults") controller.openDefaultsEditor()
            else controller.openHFEditor()
          })
          const values = editor === "defaults"
            ? ["test-vllm-image", "test-sglang-image", "test-llama-image", "test-comfy-image"]
            : ["hf_test_fixture"]
          await act(async () => {
            if (editor === "defaults") {
              for (const [index, field] of (["vllm", "sglang", "llama", "comfy"] as const).entries()) {
                controller.saveDefaultsValue(field, values[index])
              }
            } else controller.setHFTokenValue(values[0])
          })

          // Keep the same input instances through both directions of a theme change.
          const modes: ThemeMode[] = [initialMode, initialMode === "light" ? "dark" : "light", initialMode]
          for (const mode of modes) {
            theme = themeFor(mode)
            await act(async () => refresh())
            for (let focus = 0; focus < values.length; focus++) {
              await setup.renderOnce()
              const spans = setup.captureSpans().lines.flatMap((line) => line.spans)
              for (const value of values) {
                const span = spans.find((candidate) => candidate.text.includes(value))
                expect(span).toBeDefined()
                expect(span!.fg.equals(RGBA.fromHex(theme.colors.text))).toBe(true)
                expect(span!.bg.equals(RGBA.fromHex(theme.colors.panelMuted))).toBe(true)
              }
              if (editor === "defaults") {
                await act(async () => { controller.handleKey({ name: "tab" }) })
              }
            }
          }
        } finally {
          if (setup) await act(async () => setup!.renderer.destroy())
          themeSpy.mockRestore()
        }
      })
    }
  }
})

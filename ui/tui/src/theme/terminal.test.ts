import { describe, expect, mock, test } from "bun:test"
import { EventEmitter } from "node:events"
import type { CliRenderer } from "@opentui/core"

import { watchTerminalTheme } from "./terminal"
import type { ThemeMode } from "./types"

type Palette = Awaited<ReturnType<CliRenderer["getPalette"]>>
function palette(defaultBackground: string | null): Palette {
  return {
    palette: [], defaultBackground, defaultForeground: null, cursorColor: null,
    mouseForeground: null, mouseBackground: null, tekForeground: null,
    tekBackground: null, highlightBackground: null, highlightForeground: null,
  }
}

class Terminal extends EventEmitter {
  themeMode: ThemeMode | null = null
  clearPaletteCache = mock(() => {})
  getPalette = mock(async () => palette("#f5f0e8"))
}

describe("terminal theme detection", () => {
  test("detects a light OSC background without theme notifications and refreshes on focus", async () => {
    const terminal = new Terminal()
    const changed = mock((_mode: ThemeMode) => {})
    const stop = watchTerminalTheme(terminal, changed)
    try {
      await terminal.getPalette.mock.results[0].value
      expect(changed).toHaveBeenLastCalledWith("light")
      terminal.getPalette.mockResolvedValue(palette("#101010"))
      terminal.emit("focus")
      await terminal.getPalette.mock.results[1].value
      expect(changed).toHaveBeenLastCalledWith("dark")
      expect(terminal.clearPaletteCache).toHaveBeenCalledTimes(2)
    } finally { stop() }
    expect(terminal.listenerCount("theme_mode")).toBe(0)
    expect(terminal.listenerCount("focus")).toBe(0)
  })

  test("uses an already detected mode immediately", () => {
    const terminal = new Terminal()
    terminal.themeMode = "light"
    const changed = mock((_mode: ThemeMode) => {})
    const stop = watchTerminalTheme(terminal, changed)
    try {
      expect(changed).toHaveBeenCalledWith("light")
      expect(terminal.getPalette).not.toHaveBeenCalled()
    } finally { stop() }
  })

  for (const event of ["notification", "stop"] as const) {
    test(`ignores a pending OSC result after ${event}`, async () => {
      const terminal = new Terminal()
      const pending = Promise.withResolvers<Palette>()
      terminal.getPalette.mockReturnValue(pending.promise)
      const changed = mock((_mode: ThemeMode) => {})
      const stop = watchTerminalTheme(terminal, changed)
      try {
        if (event === "stop") stop()
        else terminal.emit("theme_mode", "dark")
        pending.resolve(palette("#ffffff"))
        await pending.promise
        if (event === "stop") expect(changed).not.toHaveBeenCalled()
        else {
          expect(changed).toHaveBeenCalledTimes(1)
          expect(changed).toHaveBeenLastCalledWith("dark")
        }
      } finally { stop() }
    })
  }

  for (const background of [null, "invalid"] as const) {
    test(`keeps the default when the background is ${background}`, async () => {
      const terminal = new Terminal()
      terminal.getPalette.mockResolvedValue(palette(background))
      const changed = mock((_mode: ThemeMode) => {})
      const stop = watchTerminalTheme(terminal, changed)
      try {
        await terminal.getPalette.mock.results[0].value
        expect(changed).not.toHaveBeenCalled()
      } finally { stop() }
    })
  }
})

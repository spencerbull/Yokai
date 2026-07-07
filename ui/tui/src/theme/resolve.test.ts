import { describe, expect, test } from "bun:test"

import { parseOmarchyTheme } from "./omarchy"
import { normalizeThemePreference, resolveTheme } from "./resolve"

describe("normalizeThemePreference", () => {
  test("keeps dark preference", () => {
    expect(normalizeThemePreference("dark")).toBe("dark")
  })

  test("falls back to auto for legacy theme names", () => {
    expect(normalizeThemePreference("tokyonight")).toBe("auto")
  })
})

describe("parseOmarchyTheme", () => {
  test("parses key Omarchy palette values", () => {
    const parsed = parseOmarchyTheme({
      name: "matte-black",
      colors: `accent = "#e68e0d"
foreground = "#bebebe"
background = "#121212"
selection_foreground = "#bebebe"
selection_background = "#333333"
color0 = "#333333"
color10 = "#FFC107"`,
    })

    expect(parsed).not.toBeNull()
    expect(parsed?.name).toBe("matte-black")
    expect(parsed?.accent).toBe("#e68e0d")
    expect(parsed?.background).toBe("#121212")
    expect(parsed?.selectionBackground).toBe("#333333")
    expect(parsed?.color10).toBe("#FFC107")
  })
})

describe("resolveTheme", () => {
  test("prefers dark override over Omarchy and terminal mode", () => {
    const result = resolveTheme({
      preference: "dark",
      terminalMode: "light",
      omarchyTheme: {
        name: "matte-black",
        accent: "#e68e0d",
        foreground: "#bebebe",
        background: "#121212",
        selectionForeground: "#bebebe",
        selectionBackground: "#333333",
      },
    })

    expect(result.source).toBe("override")
    expect(result.resolvedMode).toBe("dark")
  })

  test("uses Omarchy palette in auto mode when available", () => {
    const result = resolveTheme({
      preference: "auto",
      terminalMode: "light",
      omarchyTheme: {
        name: "matte-black",
        accent: "#e68e0d",
        foreground: "#bebebe",
        background: "#121212",
        selectionForeground: "#bebebe",
        selectionBackground: "#333333",
      },
    })

    expect(result.source).toBe("omarchy")
    expect(result.themeName).toBe("matte-black")
    expect(result.resolvedMode).toBe("dark")
    expect(result.colors.background).toBe("#121212")
  })
})

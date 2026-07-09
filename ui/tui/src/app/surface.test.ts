import { describe, expect, test } from "bun:test"

import { resolveAppSurface } from "./surface"

describe("resolveAppSurface", () => {
  test("shows the Yokai home screen before devices are configured", () => {
    expect(resolveAppSurface("home", "loading", 0)).toBe("home")
    expect(resolveAppSurface("home", "ready", 0)).toBe("home")
  })

  test("opens onboarding after selecting a route without devices", () => {
    expect(resolveAppSurface("app", "loading", 0)).toBe("onboarding")
    expect(resolveAppSurface("app", "ready", 0)).toBe("onboarding")
  })

  test("opens the selected route when devices are available", () => {
    expect(resolveAppSurface("app", "ready", 1)).toBe("route")
  })
})

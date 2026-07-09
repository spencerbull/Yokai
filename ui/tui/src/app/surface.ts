export type AppMode = "home" | "app"

export type AppSurface = "home" | "onboarding" | "route"

export function resolveAppSurface(appMode: AppMode, devicesStatus: string, deviceCount: number): AppSurface {
  if (appMode === "home") {
    return "home"
  }
  if (devicesStatus !== "ready" || deviceCount === 0) {
    return "onboarding"
  }
  return "route"
}

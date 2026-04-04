import { createContext, useContext, useEffect, useState, type ReactNode } from "react"

import { useRenderer } from "@opentui/react"

import { patchSettings, getSettings } from "../services/daemon-client"
import { loadOmarchyTheme, watchOmarchyTheme } from "./omarchy"
import { normalizeThemePreference, resolveTheme } from "./resolve"
import type { ThemeMode, ThemePreference, ThemeState } from "./types"

const ThemeContext = createContext<ThemeState | null>(null)

export function ThemeProvider(props: { children: ReactNode }) {
  const renderer = useRenderer()
  const [preference, setPreferenceState] = useState<ThemePreference>("auto")
  const [terminalMode, setTerminalMode] = useState<ThemeMode | null>(renderer.themeMode ?? null)
  const [omarchyTheme, setOmarchyTheme] = useState<Awaited<ReturnType<typeof loadOmarchyTheme>>>(null)
  const [isSaving, setIsSaving] = useState(false)
  const [error, setError] = useState<string>()

  useEffect(() => {
    const handler = (mode: ThemeMode) => setTerminalMode(mode)
    renderer.on("theme_mode", handler)

    return () => {
      renderer.off("theme_mode", handler)
    }
  }, [renderer])

  useEffect(() => {
    let cancelled = false

    const load = async () => {
      try {
        const settings = await getSettings()
        if (!cancelled) {
          setPreferenceState(normalizeThemePreference(settings.preferences.theme))
        }
      } catch {
        if (!cancelled) {
          setPreferenceState("auto")
        }
      }
    }

    void load()

    return () => {
      cancelled = true
    }
  }, [])

  useEffect(() => {
    let cancelled = false
    let stopWatching = async () => undefined

    const load = async () => {
      const next = await loadOmarchyTheme()
      if (!cancelled) {
        setOmarchyTheme(next)
      }
    }

    void load()

    const startWatching = async () => {
      stopWatching = await watchOmarchyTheme(() => {
        void load()
      })
    }

    void startWatching()

    return () => {
      cancelled = true
      void stopWatching()
    }
  }, [])

  const resolved = resolveTheme({
    preference,
    terminalMode,
    omarchyTheme,
  })

  const value: ThemeState = {
    preference,
    resolvedMode: resolved.resolvedMode,
    source: resolved.source,
    themeName: resolved.themeName,
    colors: resolved.colors,
    isSaving,
    error,
    setPreference: (nextPreference) => {
      if (nextPreference === preference || isSaving) {
        return
      }

      const previous = preference
      setPreferenceState(nextPreference)
      setIsSaving(true)
      setError(undefined)

      void patchSettings({
        preferences: {
          theme: nextPreference,
        },
      })
        .then(() => {
          setIsSaving(false)
        })
        .catch((cause: unknown) => {
          setPreferenceState(previous)
          setIsSaving(false)
          setError(cause instanceof Error ? cause.message : "failed to save theme preference")
        })
    },
  }

  return <ThemeContext.Provider value={value}>{props.children}</ThemeContext.Provider>
}

export function useTheme() {
  const context = useContext(ThemeContext)
  if (!context) {
    throw new Error("useTheme must be used within ThemeProvider")
  }

  return context
}

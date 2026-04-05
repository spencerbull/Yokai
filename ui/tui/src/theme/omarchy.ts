import { watch } from "node:fs"
import { readFile } from "node:fs/promises"
import path from "node:path"

import type { OmarchyTheme } from "./types"

const OMARCHY_CURRENT_DIR = path.join(configHome(), "omarchy", "current")
const OMARCHY_THEME_NAME = path.join(OMARCHY_CURRENT_DIR, "theme.name")
const OMARCHY_COLORS = path.join(OMARCHY_CURRENT_DIR, "theme", "colors.toml")

export async function loadOmarchyTheme() {
  const [name, colors] = await Promise.all([
    readOptionalText(OMARCHY_THEME_NAME),
    readOptionalText(OMARCHY_COLORS),
  ])

  if (!colors) {
    return null
  }

  return parseOmarchyTheme({
    colors,
    name: name?.trim() || undefined,
  })
}

export async function watchOmarchyTheme(onChange: () => void) {
  try {
    const watcher = watch(OMARCHY_CURRENT_DIR, (_eventType, filename) => {
      const next = filename?.toString() ?? ""
      if (next === "" || next === "theme" || next === "theme.name") {
        onChange()
      }
    })

    return async () => {
      watcher.close()
    }
  } catch {
    return async () => undefined
  }
}

export function parseOmarchyTheme(input: { colors: string; name?: string }): OmarchyTheme | null {
  const values = parseSimpleToml(input.colors)
  const background = values.background
  const foreground = values.foreground

  if (!background || !foreground) {
    return null
  }

  return {
    name: input.name,
    accent: values.accent ?? values.color4 ?? values.color12 ?? "#f59e0b",
    foreground,
    background,
    selectionForeground: values.selection_foreground ?? foreground,
    selectionBackground: values.selection_background ?? values.color0 ?? background,
    color0: values.color0,
    color2: values.color2,
    color3: values.color3,
    color4: values.color4,
    color8: values.color8,
    color9: values.color9,
    color10: values.color10,
    color11: values.color11,
    color12: values.color12,
    color14: values.color14,
    color15: values.color15,
  }
}

async function readOptionalText(filePath: string) {
  try {
    return await readFile(filePath, "utf8")
  } catch {
    return null
  }
}

function configHome() {
  const xdg = process.env.XDG_CONFIG_HOME
  if (xdg) {
    return xdg
  }

  const home = process.env.HOME
  if (home) {
    return path.join(home, ".config")
  }

  return path.join(process.cwd(), ".config")
}

function parseSimpleToml(content: string) {
  const values: Record<string, string> = {}

  for (const rawLine of content.split("\n")) {
    const line = rawLine.trim()
    if (!line || line.startsWith("#")) {
      continue
    }

    const separator = line.indexOf("=")
    if (separator === -1) {
      continue
    }

    const key = line.slice(0, separator).trim()
    const value = line.slice(separator + 1).trim().replace(/^"|"$/g, "")
    if (key && value) {
      values[key] = value
    }
  }

  return values
}

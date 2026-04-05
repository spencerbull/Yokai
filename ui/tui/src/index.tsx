import { createCliRenderer } from "@opentui/core"
import { createRoot } from "@opentui/react"

import { App } from "./app/App"

const renderer = await createCliRenderer({
  exitOnCtrlC: false,
  useMouse: true,
})

createRoot(renderer).render(<App />)

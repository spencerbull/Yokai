import { createCliRenderer } from "@opentui/core"
import { createRoot } from "@opentui/react"

import { App } from "./app/App"

async function main() {
  const renderer = await createCliRenderer({
    exitOnCtrlC: false,
    useMouse: true,
  })

  createRoot(renderer).render(<App />)
}

main().catch((error) => {
  console.error(error)
  process.exitCode = 1
})

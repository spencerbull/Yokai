import { describe, expect, test } from "bun:test"

import type { FleetService } from "../../contracts/fleet"
import { compactActionError, confirmForAction } from "./dashboard-actions"

const SERVICE: FleetService = {
  containerId: "cont-1",
  serviceId: "svc-1",
  name: "llama-serve",
  type: "vllm",
  model: "",
  image: "vllm/vllm-openai:latest",
  status: "running",
  health: "healthy",
  deviceId: "dev-1",
  deviceLabel: "alpha",
  deviceOnline: true,
  port: 8000,
  cpuPercent: 2,
  memoryUsedMB: 1024,
  gpuMemoryMB: 2048,
  uptimeSeconds: 60,
  generationTokPerSec: 12,
  promptTokPerSec: 50,
}

describe("confirmForAction", () => {
  test("requires confirmation for stop and delete only", () => {
    expect(confirmForAction("stop", SERVICE)?.message).toContain("Stop service")
    expect(confirmForAction("delete", SERVICE)?.message).toContain("Delete service")
    expect(confirmForAction("restart", SERVICE)).toBeNull()
    expect(confirmForAction("test", SERVICE)).toBeNull()
  })
})

describe("compactActionError", () => {
  test("maps test 404s to a helpful hint", () => {
    expect(compactActionError("test", new Error("daemon returned status 404 page not found"))).toContain(
      "Service test route unavailable",
    )
  })

  test("truncates long errors", () => {
    const message = compactActionError("delete", new Error("x".repeat(200)))
    expect(message.length).toBeLessThanOrEqual(120)
    expect(message.endsWith("...")).toBeTrue()
  })
})

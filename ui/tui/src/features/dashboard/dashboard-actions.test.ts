import { describe, expect, test } from "bun:test"

import type { FleetService } from "../../contracts/fleet"
import { compactActionError, confirmForAction, runDashboardAction } from "./dashboard-actions"

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

describe("runDashboardAction", () => {
  test("calls stop and returns a success notice", async () => {
    const calls: string[] = []
    const result = await runDashboardAction("stop", SERVICE, {
      stopContainer: async (deviceId, containerId) => {
        calls.push(`stop:${deviceId}:${containerId}`)
        return { status: "stopped" }
      },
      restartContainer: async () => ({ status: "restarting" }),
      testContainer: async () => ({ message: "ok", service_type: "vllm" }),
      removeContainer: async () => ({ status: "removed" }),
    })

    expect(calls).toEqual(["stop:dev-1:cont-1"])
    expect(result).toEqual({ level: "success", message: "Stopped llama-serve" })
  })

  test("calls restart and returns an info notice", async () => {
    const calls: string[] = []
    const result = await runDashboardAction("restart", SERVICE, {
      stopContainer: async () => ({ status: "stopped" }),
      restartContainer: async (deviceId, containerId) => {
        calls.push(`restart:${deviceId}:${containerId}`)
        return { status: "restarting" }
      },
      testContainer: async () => ({ message: "ok", service_type: "vllm" }),
      removeContainer: async () => ({ status: "removed" }),
    })

    expect(calls).toEqual(["restart:dev-1:cont-1"])
    expect(result).toEqual({ level: "info", message: "Restarting llama-serve" })
  })

  test("uses daemon test message when present", async () => {
    const result = await runDashboardAction("test", SERVICE, {
      stopContainer: async () => ({ status: "stopped" }),
      restartContainer: async () => ({ status: "restarting" }),
      testContainer: async () => ({ message: "Health check passed", service_type: "vllm" }),
      removeContainer: async () => ({ status: "removed" }),
    })

    expect(result).toEqual({ level: "success", message: "Health check passed" })
  })

  test("falls back to a default test message when the daemon response is blank", async () => {
    const result = await runDashboardAction("test", SERVICE, {
      stopContainer: async () => ({ status: "stopped" }),
      restartContainer: async () => ({ status: "restarting" }),
      testContainer: async () => ({ message: "   ", service_type: "vllm" }),
      removeContainer: async () => ({ status: "removed" }),
    })

    expect(result).toEqual({ level: "success", message: "Service test passed for llama-serve" })
  })

  test("reports removed config entries on delete", async () => {
    const result = await runDashboardAction("delete", SERVICE, {
      stopContainer: async () => ({ status: "stopped" }),
      restartContainer: async () => ({ status: "restarting" }),
      testContainer: async () => ({ message: "ok", service_type: "vllm" }),
      removeContainer: async () => ({ status: "removed", removed_services: 2 }),
    })

    expect(result).toEqual({ level: "success", message: "Deleted llama-serve and removed 2 config entries" })
  })

  test("reports plain delete success when no config entries were removed", async () => {
    const result = await runDashboardAction("delete", SERVICE, {
      stopContainer: async () => ({ status: "stopped" }),
      restartContainer: async () => ({ status: "restarting" }),
      testContainer: async () => ({ message: "ok", service_type: "vllm" }),
      removeContainer: async () => ({ status: "removed", removed_services: 0 }),
    })

    expect(result).toEqual({ level: "success", message: "Deleted llama-serve" })
  })
})

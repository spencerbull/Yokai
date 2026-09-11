import { describe, expect, test } from "bun:test"

import type { DeployBKC, DeployForm } from "../../contracts/deploy"
import { DaemonRequestError, readDaemonError } from "../../services/daemon-client"
import { buildClusterDeploymentRequest, buildDeployRequest, deploymentHistoryWarning, deploymentRecoveryNotice, updateHistory, validateClusterDeploymentForm } from "./useDeployController"

const form: DeployForm = {
  deviceId: "spark-a",
  extraArgs: "",
  image: "ignored-for-cluster",
  model: "LibertAIDAI/GLM-5.3-Flash-NVFP4",
  ggufVariant: "",
  ggufFiles: [],
  name: "glm53",
  port: "8000",
  workload: "sglang",
  headDeviceId: "spark-a",
  headFabricAddress: "192.168.201.1",
  headServiceAddress: "100.96.0.20",
  workerDeviceId: "spark-b",
  workerFabricAddress: "192.168.201.2",
  idempotencyKey: "glm53-canary-1",
  localModelPath: "/srv/models/glm53-snapshot",
  apiKey: "-transient-secret",
  headObservedContainerId: "head-old-container",
  workerObservedContainerId: "1234567890ab",
}

const bkc: DeployBKC = {
  id: "glm-5-3-flash-nvfp4-dual-gb10",
  name: "GLM dual GB10",
  workload: "sglang",
  model_id: form.model,
  image: "pinned@sha256:digest",
  port: "8000",
  extra_args: "sglang serve --tp-size 2",
  env: {},
  volumes: {},
  plugins: [],
  runtime: {},
  description: "cluster",
  match_type: "exact",
  source: "test",
  notes: [],
  multi_device: {
    world_size: 2,
    tp_size: 2,
    backend: "torch_distributed",
    gpus_per_node: 1,
    rendezvous_port: 25000,
    service_port: 8000,
    model_revision: "revision",
    roles: [{ name: "head", rank: 0, api: true }, { name: "worker", rank: 1, api: false }],
    required_capabilities: ["deployments.v1"],
  },
}

describe("cluster deployment bindings", () => {
  test("builds exact head and worker bindings with optional snapshot", () => {
    expect(validateClusterDeploymentForm(form, bkc)).toBeNull()
    expect(buildClusterDeploymentRequest(form, bkc)).toEqual({
      bkc_id: bkc.id,
      idempotency_key: "glm53-canary-1",
      bindings: [
		{ role: "head", device_id: "spark-a", fabric_address: "192.168.201.1", service_address: "100.96.0.20", service_port: 8000, observed_container_id: "head-old-container" },
		{ role: "worker", device_id: "spark-b", fabric_address: "192.168.201.2", observed_container_id: "1234567890ab" },
      ],
      local_model_path: "/srv/models/glm53-snapshot",
	  api_key: "-transient-secret",
    })
  })

  test("rejects duplicate devices and legacy single-device routing", () => {
    expect(validateClusterDeploymentForm({ ...form, workerDeviceId: "spark-a" }, bkc)).toContain("distinct")
    expect(validateClusterDeploymentForm({ ...form, headServiceAddress: "0.0.0.0" }, bkc)).toContain("non-loopback")
    expect(() => buildDeployRequest(form, bkc)).toThrow("grouped deployments API")
	 expect(validateClusterDeploymentForm({ ...form, apiKey: "" }, bkc)).toContain("API key")
  })

  test("preserves failed create deployment recovery details in the daemon error", async () => {
    const deployment = {
      id: "dep-recovery",
      bkc_id: bkc.id,
      idempotency_key: "key",
      state: "rollback_failed" as const,
      generation: 1,
      previous_generation: 0,
      bindings: [],
      rollback: { attempted: true, errors: ["restore previous head failed"] },
      created_at: "2026-08-28T00:00:00Z",
      updated_at: "2026-08-28T00:01:00Z",
    }
    const error = await readDaemonError(new Response(JSON.stringify({ error: "deployment_dependency", message: "cutover failed", deployment }), { status: 502 }))
    expect(error).toBeInstanceOf(DaemonRequestError)
    expect(error.deployment).toEqual(deployment)
    expect(deploymentRecoveryNotice(deployment)).toContain("dep-recovery is rollback_failed")
    expect(deploymentRecoveryNotice(deployment)).toContain("restore previous head failed")
  })

  test("treats grouped history failure as a nonfatal successful deployment warning", () => {
	const deployment = {
	  id: "dep-running", bkc_id: bkc.id, idempotency_key: "key", state: "running" as const,
	  generation: 1, previous_generation: 0, bindings: [], created_at: "2026-08-28T00:00:00Z", updated_at: "2026-08-28T00:01:00Z",
	}
	expect(deploymentHistoryWarning(deployment, new Error("disk full"))).toBe("Deployment dep-running is running; deploy history was not saved: disk full")
  })
})

describe("updateHistory null-safety", () => {
  const form: DeployForm = {
    deviceId: "dev-a",
    extraArgs: "",
    image: "vllm/vllm-openai:latest",
    model: "",
    ggufVariant: "",
    ggufFiles: [],
    name: "",
    port: "8000",
    workload: "vllm",
    headDeviceId: "",
    headFabricAddress: "",
    headServiceAddress: "",
    workerDeviceId: "",
    workerFabricAddress: "",
    idempotencyKey: "",
    localModelPath: "",
    apiKey: "",
    headObservedContainerId: "",
    workerObservedContainerId: "",
  }

  test("tolerates a null history document (images/models null) without throwing", () => {
    // Regression: a fresh daemon (no history.json) or a hand-edited file can
    // marshal history as null / { images: null, models: null }, which broke the
    // TUI deploy screen with `null is not iterable`.
    const settings = { history: { images: null, models: null } }
    expect(() => updateHistory(settings as never, form)).not.toThrow()
    const result = updateHistory(settings as never, form)
    expect(result).toEqual({ images: ["vllm/vllm-openai:latest"], models: [] })
  })

  test("tolerates a missing history field entirely", () => {
    const settings = {}
    expect(() => updateHistory(settings as never, form)).not.toThrow()
    expect(updateHistory(settings as never, form)).toEqual({ images: ["vllm/vllm-openai:latest"], models: [] })
  })

  test("prepends to existing history and caps at 20", () => {
    const settings = { history: { images: ["a", "b"], models: ["m1"] } }
    const result = updateHistory(settings as never, { ...form, image: "latest", model: "m1" })
    expect(result.images[0]).toBe("latest")
    expect(result.images).toContain("a")
    expect(result.models[0]).toBe("m1")
  })
})

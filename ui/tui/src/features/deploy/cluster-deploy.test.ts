import { describe, expect, test } from "bun:test"

import type { DeployBKC, DeployForm } from "../../contracts/deploy"
import { normalizeSettingsDocument } from "../../contracts/settings"
import { DaemonRequestError, readDaemonError } from "../../services/daemon-client"
import { buildClusterDeploymentRequest, buildDeployRequest, deploymentHistoryWarning, deploymentRecoveryNotice, nextConfigField, updateHistory, validateClusterDeploymentForm } from "./useDeployController"

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
  headFabricInterface: "",
  headFabricHCA: "",
  headFabricGIDIndex: "",
  headServiceAddress: "100.96.0.20",
  workerDeviceId: "spark-b",
  workerFabricAddress: "192.168.201.2",
  workerFabricInterface: "",
  workerFabricHCA: "",
  workerFabricGIDIndex: "",
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

const fabricBKC: DeployBKC = {
  ...bkc,
  id: "qwen-like-fabric-required",
  name: "Qwen-like fabric-required BKC",
  workload: "vllm",
  model_id: "example/Qwen-Fabric-Required",
  port: "8888",
  multi_device: {
    ...bkc.multi_device!,
    backend: "vllm_multi_node",
    service_port: 8888,
    requires_fabric_config: true,
  },
}

const fabricForm: DeployForm = {
  ...form,
  workload: "vllm",
  model: fabricBKC.model_id,
  port: "8888",
  headFabricInterface: "enp1s0f0np0",
  headFabricHCA: "rocep1s0f0",
  headFabricGIDIndex: "3",
  workerFabricInterface: "enp1s0f1np1",
  workerFabricHCA: "rocep1s0f1",
  workerFabricGIDIndex: "4",
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

  test("serializes explicit per-role fabric settings for fabric-required BKCs", () => {
    expect(validateClusterDeploymentForm(fabricForm, fabricBKC)).toBeNull()
    expect(buildClusterDeploymentRequest(fabricForm, fabricBKC)).toEqual({
      bkc_id: fabricBKC.id,
      idempotency_key: "glm53-canary-1",
      bindings: [
        {
          role: "head",
          device_id: "spark-a",
          fabric_address: "192.168.201.1",
          fabric_interface: "enp1s0f0np0",
          fabric_hca: "rocep1s0f0",
          fabric_gid_index: 3,
          service_address: "100.96.0.20",
          service_port: 8888,
          observed_container_id: "head-old-container",
        },
        {
          role: "worker",
          device_id: "spark-b",
          fabric_address: "192.168.201.2",
          fabric_interface: "enp1s0f1np1",
          fabric_hca: "rocep1s0f1",
          fabric_gid_index: 4,
          observed_container_id: "1234567890ab",
        },
      ],
      local_model_path: "/srv/models/glm53-snapshot",
      api_key: "-transient-secret",
    })
  })

  test("reports each missing fabric-required value before submitting", () => {
    const requiredCases: Array<[keyof DeployForm, string]> = [
      ["headFabricInterface", "Head fabric interface is required"],
      ["headFabricHCA", "Head fabric HCA is required"],
      ["headFabricGIDIndex", "Head fabric GID index is required"],
      ["workerFabricInterface", "Worker fabric interface is required"],
      ["workerFabricHCA", "Worker fabric HCA is required"],
      ["workerFabricGIDIndex", "Worker fabric GID index is required"],
    ]
    for (const [field, message] of requiredCases) {
      expect(validateClusterDeploymentForm({ ...fabricForm, [field]: "" }, fabricBKC)).toBe(message)
    }
  })

  test("requires fabric GID indices to be integers in the backend's 0-255 range", () => {
    expect(validateClusterDeploymentForm({ ...fabricForm, headFabricGIDIndex: "3.5" }, fabricBKC)).toBe("Head fabric GID index must be an integer between 0 and 255")
    expect(validateClusterDeploymentForm({ ...fabricForm, headFabricGIDIndex: "-1" }, fabricBKC)).toBe("Head fabric GID index must be an integer between 0 and 255")
    expect(validateClusterDeploymentForm({ ...fabricForm, workerFabricGIDIndex: "256" }, fabricBKC)).toBe("Worker fabric GID index must be an integer between 0 and 255")
    expect(validateClusterDeploymentForm({ ...fabricForm, headFabricGIDIndex: "0", workerFabricGIDIndex: "255" }, fabricBKC)).toBeNull()
  })

  test("includes fabric controls in keyboard focus only when the BKC requires them", () => {
    expect(nextConfigField("headFabric", "vllm", fabricBKC, 1)).toBe("headFabricInterface")
    expect(nextConfigField("headFabricInterface", "vllm", fabricBKC, 1)).toBe("headFabricHCA")
    expect(nextConfigField("headFabricHCA", "vllm", fabricBKC, 1)).toBe("headFabricGIDIndex")
    expect(nextConfigField("workerFabric", "vllm", fabricBKC, 1)).toBe("workerFabricInterface")
    expect(nextConfigField("headFabric", "sglang", bkc, 1)).toBe("headService")
    expect(nextConfigField("workerFabric", "sglang", bkc, 1)).toBe("workerObserved")
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
    headFabricInterface: "",
    headFabricHCA: "",
    headFabricGIDIndex: "",
    headServiceAddress: "",
    workerDeviceId: "",
    workerFabricAddress: "",
    workerFabricInterface: "",
    workerFabricHCA: "",
    workerFabricGIDIndex: "",
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

describe("normalizeSettingsDocument", () => {
  test("coerces null history images/models to empty arrays", () => {
    const out = normalizeSettingsDocument({ history: { images: null, models: null } } as never)
    expect(out.history.images).toEqual([])
    expect(out.history.models).toEqual([])
  })

  test("coerces a missing history field to empty arrays", () => {
    const out = normalizeSettingsDocument({} as never)
    expect(out.history).toEqual({ images: [], models: [] })
  })

  test("preserves existing history arrays", () => {
    const out = normalizeSettingsDocument({ history: { images: ["a"], models: ["m"] } } as never)
    expect(out.history.images).toEqual(["a"])
    expect(out.history.models).toEqual(["m"])
  })
})

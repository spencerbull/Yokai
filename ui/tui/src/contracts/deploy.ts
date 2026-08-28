export type WorkloadType = "vllm" | "sglang" | "llamacpp" | "comfyui"

export type HFModel = {
  id: string
  author: string
  likes: number
  downloads: number
  tags: string[]
  pipeline_tag: string
}

export type DeployForm = {
  deviceId: string
  extraArgs: string
  image: string
  model: string
  ggufVariant: string
  ggufFiles: string[]
  name: string
  port: string
  workload: WorkloadType
  headDeviceId: string
  headFabricAddress: string
  headServiceAddress: string
  workerDeviceId: string
  workerFabricAddress: string
  idempotencyKey: string
  localModelPath: string
  apiKey: string
  headObservedContainerId: string
  workerObservedContainerId: string
}

export type GGUFFile = {
  rfilename: string
  SizeMB?: number
}

export type GGUFVariant = {
  quantization: string
  shards: GGUFFile[]
  shard_count: number
  total_size_mb: number
  primary: string
}

export type GGUFVariantsResponse = {
  model: string
  variants: GGUFVariant[]
}

export type DeployRequest = {
  bkc_id?: string
  device_id: string
  service_type: WorkloadType
  image: string
  name: string
  model: string
  gguf_variant?: string
  gguf_files?: string[]
  ports: Record<string, string>
  env: Record<string, string>
  gpu_ids: string
  extra_args: string
  volumes: Record<string, string>
  plugins: string[]
  runtime: {
    ipc_mode?: string
    shm_size?: string
    ulimits?: Record<string, string>
  }
}

export type DeployResult = {
  container_id: string
  status: string
  ports: Record<string, string>
}

export type DeployBKC = {
  id: string
  name: string
  workload: string
  model_id: string
  image: string
  port: string
  extra_args: string
  env: Record<string, string>
  volumes: Record<string, string>
  plugins: string[]
  runtime: {
    ipc_mode?: string
    shm_size?: string
    ulimits?: Record<string, string>
  }
  description: string
  match_type: "exact" | "suggested"
  source: string
  notes: string[]
  warning?: string
  target_devices?: string[]
  min_vram_gb_per_gpu?: number
  min_gpu_count?: number
  quantization?: string
  arch?: string
  multi_device?: MultiDeviceMetadata
}

export type MultiDeviceMetadata = {
  world_size: number
  tp_size: number
  backend: "torch_distributed"
  gpus_per_node: number
  rendezvous_port: number
  service_port: number
  model_revision: string
  runtime_patches?: RuntimePatchMetadata[]
  roles: Array<{ name: "head" | "worker"; rank: number; api: boolean }>
  required_capabilities: string[]
}

export type RuntimePatchProvenance = {
  label: string
  source_path: string
  original_sha256: string
  patched_sha256: string
}

export type RuntimePatchMetadata = RuntimePatchProvenance & {
  original_line: string
  replacement_line: string
}

export type DeploymentBinding = {
  role: "head" | "worker"
  device_id: string
  fabric_address: string
  service_address?: string
  service_port?: number
  observed_container_id?: string
}

export type DeploymentCreateRequest = {
  bkc_id: string
  idempotency_key: string
  bindings: DeploymentBinding[]
  local_model_path?: string
  api_key: string
}

export type DeploymentMember = {
  role: "head" | "worker"
  rank: number
  device_id: string
  fabric_address: string
  service_address?: string
  service_port?: number
  container_id: string
  name?: string
  status: string
  ownership: "managed" | "adopted" | "observed"
  generation: number
}

export type DeploymentRecord = {
  id: string
  bkc_id: string
  idempotency_key: string
  state: "pending" | "starting" | "running" | "stopping" | "stopped" | "failed" | "rolled_back" | "rollback_failed"
  phase?: string
  generation: number
  previous_generation: number
  bindings: DeploymentBinding[]
  members?: DeploymentMember[]
  previous_members?: DeploymentMember[]
  uses_local_model_snapshot?: boolean
  runtime_patches?: RuntimePatchProvenance[]
  error?: string
  rollback?: { attempted: boolean; removed_candidates?: string[]; restarted_containers?: string[]; errors?: string[] }
  last_test?: { ok: boolean; metrics_ready: boolean; message: string; model?: string; tested_at: string }
  created_at: string
  updated_at: string
}

export type VLLMMemoryEstimate = {
  applied_tp_default: boolean
  context_length: number
  gpu_count: number
  kv_cache_gb: number
  min_vram_gb: number
  overhead_gb: number
  required_per_gpu_gb: number
  tensor_parallel: number
  utilization: number
  weights_gb: number
}

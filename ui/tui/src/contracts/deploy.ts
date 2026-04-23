export type WorkloadType = "vllm" | "llamacpp" | "comfyui"

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

import { startTransition, useEffect, useMemo, useState } from "react"

import type { DeploymentCreateRequest, DeploymentRecord, DeployBKC, DeployForm, GGUFVariant, HFModel, VLLMMemoryEstimate, WorkloadType } from "../../contracts/deploy"
import type { DeviceRecord } from "../../contracts/fleet"
import type { SettingsDocument } from "../../contracts/settings"
import { createDeployment, DaemonRequestError, deployService, getDeployBKCs, getDevices, getGGUFVariants, getHFModels, getSettings, getVLLMMemoryEstimate, putDeployHistory } from "../../services/daemon-client"

type DeployStep = "workload" | "device" | "image" | "model" | "variant" | "config" | "review"
type ConfigField = "port" | "extraArgs" | "bkcAction" | "contextLength" | "overheadGB" | "hfmemCalculate" | "hfmemApply" | "headDevice" | "headFabric" | "headService" | "headObserved" | "workerDevice" | "workerFabric" | "workerObserved" | "idempotencyKey" | "localModelPath" | "apiKey"
type ReviewAction = "back" | "deploy"

type DeployNotice = {
  level: "info" | "success" | "warning" | "error"
  message: string
}

type VLLMHelperState = {
  contextLength: string
  estimate: VLLMMemoryEstimate | null
  error?: string
  loading: boolean
  overheadGB: string
}

type KeyLike = {
  name: string
  shift?: boolean
}

const STEPS: DeployStep[] = ["workload", "device", "image", "model", "variant", "config", "review"]
const WORKLOADS: WorkloadType[] = ["vllm", "sglang", "llamacpp", "comfyui"]

const EMPTY_SETTINGS: SettingsDocument = {
  hf: { configured: false, source: "none" },
  preferences: {
    theme: "auto",
    default_vllm_image: "",
    default_sglang_image: "",
    default_llama_image: "",
    default_comfyui_image: "",
  },
  history: { images: [], models: [] },
  integrations: {
    vscode: { available: false, configured: false },
    opencode: { available: false, configured: false },
    openclaw: { available: false, configured: false },
  },
}

export function useDeployController(active: boolean, onComplete: (notice?: DeployNotice) => void) {
  const [status, setStatus] = useState<"loading" | "ready" | "error">("loading")
  const [devices, setDevices] = useState<DeviceRecord[]>([])
  const [settings, setSettings] = useState<SettingsDocument>(EMPTY_SETTINGS)
  const [form, setForm] = useState<DeployForm>(emptyForm())
  const [step, setStep] = useState<DeployStep>("workload")
  const [cursor, setCursor] = useState(0)
  const [configField, setConfigField] = useState<ConfigField>("port")
  const [extraArgsEditing, setExtraArgsEditing] = useState(false)
  const [reviewAction, setReviewAction] = useState<ReviewAction>("back")
  const [notice, setNotice] = useState<DeployNotice | null>(null)
  const [pendingAction, setPendingAction] = useState<string | null>(null)
  const [recoveryDeployment, setRecoveryDeployment] = useState<DeploymentRecord | null>(null)
  const [modelResults, setModelResults] = useState<HFModel[]>([])
  const [searchError, setSearchError] = useState<string>()
  const [bkcs, setBkcs] = useState<DeployBKC[]>([])
  const [bkcIndex, setBkcIndex] = useState(0)
  const [appliedBKCId, setAppliedBKCId] = useState("")
  const [ggufVariants, setGGUFVariants] = useState<GGUFVariant[]>([])
  const [ggufLoading, setGGUFLoading] = useState(false)
  const [ggufError, setGGUFError] = useState<string>()
  const [vllmHelper, setVLLMHelper] = useState<VLLMHelperState>({
    contextLength: "32768",
    estimate: null,
    loading: false,
    overheadGB: "1.5",
  })
  const bkc = bkcs[bkcIndex] ?? null
  const activeBKC = bkc && appliedBKCId === bkc.id ? bkc : null

  useEffect(() => {
    if (!active) {
      return
    }

    let cancelled = false
    const load = async () => {
      try {
        const [deviceResponse, settingsDoc] = await Promise.all([getDevices(), getSettings()])
        if (cancelled) {
          return
        }
        startTransition(() => {
          setDevices(deviceResponse.devices)
          setSettings(settingsDoc)
          setStatus("ready")
          setForm((current) => applyDefaults(current, settingsDoc))
        })
      } catch (cause) {
        if (cancelled) {
          return
        }
        setStatus("error")
        setNotice({ level: "error", message: cause instanceof Error ? cause.message : "failed to load deploy defaults" })
      }
    }

    void load()
    return () => {
      cancelled = true
    }
  }, [active])

  useEffect(() => {
    if (!active || step !== "model") {
      return
    }
    if (form.workload === "comfyui") {
      setModelResults([])
      setSearchError(undefined)
      return
    }

    const query = form.model.trim()
    if (query.length < 2) {
      setModelResults([])
      setSearchError(undefined)
      return
    }

    let cancelled = false
    const timeout = setTimeout(() => {
      void getHFModels(query, form.workload)
        .then((results) => {
          if (!cancelled) {
            setModelResults(results)
            setSearchError(undefined)
          }
        })
        .catch((cause) => {
          if (!cancelled) {
            setModelResults([])
            setSearchError(cause instanceof Error ? cause.message : "HF search failed")
          }
        })
    }, 250)

    return () => {
      cancelled = true
      clearTimeout(timeout)
    }
  }, [active, form.model, form.workload, step])

  useEffect(() => {
    if (!active) {
      return
    }
    if (form.workload !== "vllm" && form.workload !== "sglang" && form.workload !== "llamacpp") {
      setBkcs([])
      setBkcIndex(0)
      return
    }
    const model = form.model.trim()
    if (model === "") {
      setBkcs([])
      setBkcIndex(0)
      return
    }

    let cancelled = false
    void getDeployBKCs(form.workload, model, form.deviceId || undefined)
      .then((configs) => {
        if (!cancelled) {
          setBkcs(configs)
          setBkcIndex(0)
        }
      })
      .catch(() => {
        if (!cancelled) {
          setBkcs([])
          setBkcIndex(0)
        }
      })

    return () => {
      cancelled = true
    }
  }, [active, form.deviceId, form.model, form.workload])

  useEffect(() => {
    if (!active) {
      return
    }
    if (form.workload === "comfyui" || form.workload === "sglang") {
      setGGUFVariants([])
      setGGUFError(undefined)
      setGGUFLoading(false)
      return
    }
    const model = form.model.trim()
    if (model === "") {
      setGGUFVariants([])
      setGGUFError(undefined)
      setGGUFLoading(false)
      return
    }

    // Debounce so rapid typing on the Model step doesn't hammer the Hub
    // API. Matches the 250ms debounce used by the model-search effect.
    let cancelled = false
    setGGUFLoading(true)
    setGGUFError(undefined)
    const timeout = setTimeout(() => {
      void getGGUFVariants(model)
        .then((variants) => {
          if (cancelled) return
          setGGUFVariants(variants)
          setGGUFLoading(false)
        })
        .catch((cause) => {
          if (cancelled) return
          setGGUFVariants([])
          setGGUFError(cause instanceof Error ? cause.message : "failed to fetch GGUF variants")
          setGGUFLoading(false)
        })
    }, 250)

    return () => {
      cancelled = true
      clearTimeout(timeout)
    }
  }, [active, form.model, form.workload])

  useEffect(() => {
    if (!notice) {
      return
    }
    const timeout = setTimeout(() => {
      setNotice((current) => (current === notice ? null : current))
    }, 4500)
    return () => clearTimeout(timeout)
  }, [notice])

  const deviceOptions = useMemo(() => devices.map((device) => ({
    id: device.id,
    label: device.label || device.id,
    secondary: device.host,
  })), [devices])

  function goToVariantStep() {
    // Prime the loading flag so the Variant step shows a spinner on its very
    // first frame instead of briefly flashing the "no GGUF files" empty
    // state. The fetch effect (watching form.model) will take over as soon
    // as it fires and either populate variants or clear the flag.
    if (form.model.trim() !== "") {
      setGGUFLoading(true)
    }
    setStep("variant")
    setCursor(0)
  }

  function advanceFromModel() {
    if (form.workload === "comfyui" || form.workload === "sglang") {
      setStep("config")
      setCursor(0)
      return
    }
    // Always route GGUF-capable workloads through the Variant step. The step
    // renders a spinner while variants are being fetched and falls back to
    // an explicit "no GGUF files in this repo, press Enter to continue"
    // message when the repo has none — so the user always sees the variant
    // mapping when one exists, rather than being silently skipped past.
    goToVariantStep()
  }

  function selectVariantByIndex(index: number) {
    const variant = ggufVariants[index]
    if (!variant) {
      return
    }
    const files = variant.shards.map((shard) => shard.rfilename)
    setForm((current) => ({
      ...current,
      ggufVariant: variant.quantization,
      ggufFiles: files,
    }))
    setStep("config")
    setCursor(0)
  }

  return {
    activeBKC,
    availableBKC: bkc,
    availableBKCCount: bkcs.length,
    availableBKCIndex: bkcIndex,
    applyBKC() {
      applyBKCToForm()
    },
    selectBKC(direction: -1 | 1) {
      selectBKCByOffset(direction)
    },
    ggufVariants,
    ggufLoading,
    ggufError,
    configField,
    cursor,
    calculateVLLMMemory: () => void calculateVLLMMemory(),
    deviceOptions,
    form,
    extraArgsEditing,
    hasAppliedBKC: Boolean(bkc && appliedBKCId === bkc.id),
    isClusterBKC: Boolean(activeBKC?.multi_device),
    locksGlobalNav: step === "image" || step === "model" || step === "variant" || step === "config",
    modelResults,
    notice,
    pendingAction,
    recoveryDeployment,
    openRecoveryDeployment() {
      onComplete()
    },
    reviewAction,
    searchError,
    settings,
    status,
    step,
    stepIndex: STEPS.findIndex((entry) => entry === step),
    applyVLLMMemory() {
      if (vllmHelper.estimate) {
        applyVLLMEstimate(vllmHelper.estimate)
      }
    },
    updateVLLMHelper(field: "contextLength" | "overheadGB", value: string) {
      setVLLMHelper((current) => ({
        ...current,
        [field]: value,
      }))
    },
    vllmHelper,
    handleKey(key: KeyLike) {
      switch (step) {
        case "workload":
          return handleWorkloadKey(key)
        case "device":
          return handleDeviceKey(key)
        case "image":
          return handleImageKey(key)
        case "model":
          return handleModelKey(key)
        case "variant":
          return handleVariantKey(key)
        case "config":
          return handleConfigKey(key)
        case "review":
          return handleReviewKey(key)
        default:
          return false
      }
    },
    selectDevice(deviceId: string) {
      setForm((current) => ({ ...current, deviceId }))
      setStep("image")
      setCursor(0)
    },
    selectReviewAction(action: ReviewAction) {
      setReviewAction(action)
    },
    selectModel(modelId: string) {
      setForm((current) => ({ ...current, model: modelId, ggufVariant: "", ggufFiles: [] }))
      setSearchError(undefined)
      setModelResults([])
      if (form.workload === "comfyui" || form.workload === "sglang") {
        setStep("config")
        setCursor(0)
        return
      }
      // Prime the Variant step with a spinner on first frame so the user
      // doesn't see an empty-state flash before the fetch effect fires.
      setGGUFLoading(true)
      setStep("variant")
      setCursor(0)
    },
    selectVariant(index: number) {
      selectVariantByIndex(index)
    },
    selectWorkload(workload: WorkloadType) {
      setForm((current) => applyWorkloadDefaults(current, settings, workload))
      setStep("device")
      setCursor(0)
    },
    setValue<K extends keyof DeployForm>(field: K, value: DeployForm[K]) {
      setForm((current) => ({ ...current, [field]: value }))
    },
    submitDeploy: () => void submitDeploy(),
    closeExtraArgsEditor(value: string) {
      setForm((current) => ({ ...current, extraArgs: value }))
      setExtraArgsEditing(false)
    },
    openExtraArgsEditor() {
      setExtraArgsEditing(true)
    },
  }

  function handleWorkloadKey(key: KeyLike) {
    switch (key.name) {
      case "up":
      case "left":
      case "k":
        setCursor((current) => Math.max(0, current - 1))
        return true
      case "down":
      case "right":
      case "j":
        setCursor((current) => Math.min(WORKLOADS.length - 1, current + 1))
        return true
      case "1":
      case "2":
      case "3":
      case "4":
        selectWorkloadByIndex(Number(key.name) - 1)
        return true
      case "return":
      case "enter":
        selectWorkloadByIndex(cursor)
        return true
      default:
        return false
    }
  }

  function handleDeviceKey(key: KeyLike) {
    switch (key.name) {
      case "escape":
        setStep("workload")
        return true
      case "up":
      case "k":
        setCursor((current) => Math.max(0, current - 1))
        return true
      case "down":
      case "j":
        setCursor((current) => Math.min(Math.max(0, deviceOptions.length - 1), current + 1))
        return true
      case "return":
      case "enter":
        if (deviceOptions[cursor]) {
          setForm((current) => ({ ...current, deviceId: deviceOptions[cursor].id }))
          setStep("image")
          setCursor(0)
        }
        return true
      default:
        return false
    }
  }

  function handleImageKey(key: KeyLike) {
    switch (key.name) {
      case "escape":
        setStep("device")
        return true
      case "return":
      case "enter":
        if (form.image.trim() === "") {
          setNotice({ level: "error", message: "Image is required" })
          return true
        }
        setStep(form.workload === "comfyui" ? "config" : "model")
        setCursor(0)
        return true
      default:
        return false
    }
  }

  function handleModelKey(key: KeyLike) {
    switch (key.name) {
      case "escape":
        setStep("image")
        return true
      case "up":
      case "k":
        setCursor((current) => Math.max(0, current - 1))
        return true
      case "down":
      case "j":
        setCursor((current) => Math.min(Math.max(0, modelResults.length - 1), current + 1))
        return true
      case "return":
      case "enter":
        if (modelResults.length > 0 && cursor < modelResults.length) {
          setForm((current) => ({ ...current, model: modelResults[cursor].id, ggufVariant: "", ggufFiles: [] }))
        }
        if (form.model.trim() === "") {
          setNotice({ level: "error", message: "Model is required" })
          return true
        }
        advanceFromModel()
        return true
      default:
        return false
    }
  }

  function handleVariantKey(key: KeyLike) {
    switch (key.name) {
      case "escape":
        setStep("model")
        return true
      case "up":
      case "k":
        setCursor((current) => Math.max(0, current - 1))
        return true
      case "down":
      case "j":
        setCursor((current) => Math.min(Math.max(0, ggufVariants.length - 1), current + 1))
        return true
      case "s":
        // Skip variant selection (deploy without pre-downloading GGUF files).
        setForm((current) => ({ ...current, ggufVariant: "", ggufFiles: [] }))
        setStep("config")
        setCursor(0)
        return true
      case "return":
      case "enter":
        if (ggufVariants.length === 0) {
          setStep("config")
          setCursor(0)
          return true
        }
        selectVariantByIndex(cursor)
        return true
      default:
        return false
    }
  }

  function handleConfigKey(key: KeyLike) {
    if (extraArgsEditing) {
      return false
    }

    switch (key.name) {
      case "escape":
        if (form.workload === "comfyui") {
          setStep("image")
        } else if (ggufVariants.length > 0) {
          setStep("variant")
        } else {
          setStep("model")
        }
        return true
      case "tab":
        setConfigField((current) => nextConfigField(current, form.workload, Boolean(activeBKC?.multi_device), key.shift ? -1 : 1))
        return true
      case "b":
        if (bkc) {
          applyBKCToForm()
          return true
        }
        return false
      case "m":
        if (form.workload === "vllm") {
          void calculateVLLMMemory()
          return true
        }
        return false
      case "f":
        if (form.workload === "vllm" && vllmHelper.estimate) {
          applyVLLMEstimate(vllmHelper.estimate)
          return true
        }
        return false
      case "left":
      case "h":
        if (configField === "bkcAction" && bkcs.length > 1) {
          selectBKCByOffset(-1)
          return true
        }
        return false
      case "right":
      case "l":
        if (configField === "bkcAction" && bkcs.length > 1) {
          selectBKCByOffset(1)
          return true
        }
        return false
      case "return":
      case "enter":
        if (configField === "extraArgs") {
          setExtraArgsEditing(true)
          return true
        }
        if (configField === "bkcAction" && bkc) {
          applyBKCToForm()
          return true
        }
        if (configField === "hfmemCalculate" && form.workload === "vllm") {
          void calculateVLLMMemory()
          return true
        }
        if (configField === "hfmemApply" && form.workload === "vllm" && vllmHelper.estimate) {
          applyVLLMEstimate(vllmHelper.estimate)
          return true
        }
        if (activeBKC?.multi_device) {
          const clusterValidation = validateClusterDeploymentForm(form, activeBKC)
          if (clusterValidation) {
            setNotice({ level: "error", message: clusterValidation })
            return true
          }
        } else if (!Number.isFinite(Number.parseInt(form.port.trim(), 10))) {
          setNotice({ level: "error", message: "Port must be a number" })
          return true
        }
        setReviewAction("back")
        setStep("review")
        return true
      default:
        return false
    }
  }

  function handleReviewKey(key: KeyLike) {
    switch (key.name) {
      case "escape":
        setStep("config")
        return true
      case "left":
      case "h":
        setReviewAction("back")
        return true
      case "right":
      case "l":
      case "tab":
        setReviewAction((current) => (current === "back" ? "deploy" : "back"))
        return true
      case "return":
      case "enter":
        if (reviewAction === "deploy") {
          void submitDeploy()
        } else {
          setStep("config")
        }
        return true
      default:
        return false
    }
  }

  function selectWorkloadByIndex(index: number) {
    const workload = WORKLOADS[index]
    if (!workload) {
      return
    }
    setForm((current) => applyWorkloadDefaults(current, settings, workload))
    setStep("device")
    setCursor(0)
  }

  async function submitDeploy() {
    const validation = activeBKC?.multi_device ? validateClusterDeploymentForm(form, activeBKC) : validateDeployForm(form)
    if (validation) {
      setNotice({ level: "error", message: validation })
      return
    }

    setPendingAction("deploying service")
    setRecoveryDeployment(null)
    try {
      let completionNotice: DeployNotice
      let successfulDeployment: DeploymentRecord | null = null
      if (activeBKC?.multi_device) {
        const deployment = await createDeployment(buildClusterDeploymentRequest(form, activeBKC))
        successfulDeployment = deployment
        setForm((current) => ({ ...current, apiKey: "" }))
        completionNotice = {
          level: deployment.state === "running" ? "success" : deployment.state === "stopped" ? "info" : "warning",
          message: `Deployment ${deployment.id} is ${deployment.state}`,
        }
      } else {
        await deployService(buildDeployRequest(form, activeBKC))
        completionNotice = { level: "success", message: `Deployed ${form.name}` }
      }
      try {
        await putDeployHistory(updateHistory(settings, form))
      } catch (historyCause) {
        if (!successfulDeployment) {
          throw historyCause
        }
        setRecoveryDeployment(successfulDeployment)
        const warning = { level: "warning" as const, message: deploymentHistoryWarning(successfulDeployment, historyCause) }
        setNotice(warning)
        onComplete(warning)
        return
      }
      setNotice(completionNotice)
      onComplete()
    } catch (cause) {
      if (cause instanceof DaemonRequestError && cause.deployment) {
        setRecoveryDeployment(cause.deployment)
        setNotice({ level: "error", message: `${cause.message}. ${deploymentRecoveryNotice(cause.deployment)}` })
      } else {
        setNotice({ level: "error", message: cause instanceof Error ? cause.message : "deploy failed" })
      }
    } finally {
      if (activeBKC?.multi_device) {
        setForm((current) => ({ ...current, apiKey: "" }))
      }
      setPendingAction(null)
    }
  }

  async function calculateVLLMMemory() {
    if (form.workload !== "vllm") {
      return
    }
    if (!form.deviceId) {
      setNotice({ level: "error", message: "Select a device first" })
      return
    }
    if (!form.model.trim()) {
      setNotice({ level: "error", message: "Select a model first" })
      return
    }

    setVLLMHelper((current) => ({ ...current, loading: true, error: undefined, estimate: null }))
    try {
      const estimate = await getVLLMMemoryEstimate({
        context_length: Number.parseInt(vllmHelper.contextLength, 10),
        device_id: form.deviceId,
        model: form.model.trim(),
        overhead_gb: vllmHelper.overheadGB,
        extra_args: estimateArgsSource(form.extraArgs, bkc),
      })
      setVLLMHelper((current) => ({ ...current, loading: false, estimate }))
    } catch (cause) {
      setVLLMHelper((current) => ({
        ...current,
        error: cause instanceof Error ? cause.message : "hf-mem estimate failed",
        loading: false,
      }))
    }
  }

  function applyVLLMEstimate(estimate: VLLMMemoryEstimate) {
    setForm((current) => ({
      ...current,
      extraArgs: formatArgsForEditor(applyVLLMFlags(current.extraArgs, estimate)),
    }))
    setVLLMHelper((current) => ({
      ...current,
      contextLength: `${estimate.context_length}`,
      error: "Applied recommended vLLM flags",
    }))
  }

  function applyBKCToForm() {
    if (!bkc) {
      return
    }
    setAppliedBKCId(bkc.id)
    setForm((current) => ({
      ...current,
      model: bkc.model_id,
      image: bkc.image,
      port: bkc.port,
      extraArgs: formatArgsForEditor(bkc.extra_args),
      headDeviceId: bkc.multi_device ? current.deviceId : current.headDeviceId,
    }))
    const maxModelLen = argValue(bkc.extra_args, "--max-model-len")
    setConfigField(bkc.multi_device ? "headDevice" : "extraArgs")
    setVLLMHelper((current) => ({
      ...current,
      contextLength: maxModelLen || current.contextLength,
      error: `Applied BKC: ${bkc.name}`,
      estimate: null,
    }))
  }

  function selectBKCByOffset(direction: -1 | 1) {
    if (bkcs.length < 2) {
      return
    }
    setBkcIndex((current) => (current + direction + bkcs.length) % bkcs.length)
  }
}

export function deploymentHistoryWarning(deployment: DeploymentRecord, cause: unknown): string {
  const detail = cause instanceof Error ? cause.message : "history persistence failed"
  return `Deployment ${deployment.id} is ${deployment.state}; deploy history was not saved: ${detail}`
}

function emptyForm(): DeployForm {
  return {
    deviceId: "",
    extraArgs: "",
    image: "",
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
}

function applyDefaults(form: DeployForm, settings: SettingsDocument) {
  return applyWorkloadDefaults(form, settings, form.workload)
}

function applyWorkloadDefaults(form: DeployForm, settings: SettingsDocument, workload: WorkloadType): DeployForm {
  const resetGGUF = workload !== form.workload
  switch (workload) {
    case "llamacpp":
      return {
        ...form,
        image: form.workload === workload && form.image ? form.image : settings.preferences.default_llama_image,
        name: defaultName(workload, form.model),
        port: "8080",
        extraArgs: workload === form.workload ? form.extraArgs : "",
        ggufVariant: resetGGUF ? "" : form.ggufVariant,
        ggufFiles: resetGGUF ? [] : form.ggufFiles,
        workload,
      }
    case "comfyui":
      return {
        ...form,
        image: form.workload === workload && form.image ? form.image : settings.preferences.default_comfyui_image,
        model: "",
        name: defaultName(workload, "comfyui"),
        port: "8188",
        extraArgs: workload === form.workload ? form.extraArgs : "",
        ggufVariant: "",
        ggufFiles: [],
        workload,
      }
    case "sglang":
      return {
        ...form,
        image: form.workload === workload && form.image ? form.image : settings.preferences.default_sglang_image,
        name: defaultName(workload, form.model),
        port: "30000",
        extraArgs: workload === form.workload ? form.extraArgs : "",
        ggufVariant: "",
        ggufFiles: [],
        workload,
      }
    default:
      return {
        ...form,
        image: form.workload === workload && form.image ? form.image : settings.preferences.default_vllm_image,
        name: defaultName(workload, form.model),
        port: "8000",
        extraArgs: workload === form.workload ? form.extraArgs : "",
        ggufVariant: resetGGUF ? "" : form.ggufVariant,
        ggufFiles: resetGGUF ? [] : form.ggufFiles,
        workload,
      }
  }
}

function defaultName(workload: WorkloadType, model: string) {
  const seed = model.trim() || workload
  return `${workload}-${seed}`.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "").slice(0, 40)
}

function validateDeployForm(form: DeployForm) {
  if (!form.deviceId) return "Select a device"
  if (!form.image.trim()) return "Image is required"
  if (form.workload !== "comfyui" && !form.model.trim()) return "Model is required"
  const port = Number.parseInt(form.port.trim(), 10)
  if (!Number.isFinite(port) || port < 1 || port > 65535) return "Port must be between 1 and 65535"
  return null
}

export function buildDeployRequest(form: DeployForm, bkc: DeployBKC | null) {
  if (bkc?.multi_device) {
    throw new Error("multi-device BKCs must use the grouped deployments API")
  }
  const port = form.port.trim()
  return {
    bkc_id: bkc?.id,
    device_id: form.deviceId,
    service_type: form.workload,
    image: form.image.trim(),
    name: defaultName(form.workload, form.model),
    model: form.workload === "comfyui" ? "" : form.model.trim(),
    gguf_variant: form.ggufVariant || undefined,
    gguf_files: form.ggufFiles.length > 0 ? [...form.ggufFiles] : undefined,
    ports: { [port]: port },
    env: cloneMap(bkc?.env),
    gpu_ids: "all",
    extra_args: normalizeArgs(form.extraArgs),
    volumes: cloneMap(bkc?.volumes),
    plugins: bkc?.plugins ? [...bkc.plugins] : [],
    runtime: cloneRuntime(bkc?.runtime),
  }
}

export function buildClusterDeploymentRequest(form: DeployForm, bkc: DeployBKC): DeploymentCreateRequest {
  if (!bkc.multi_device) {
    throw new Error("BKC is not a multi-device deployment")
  }
  return {
    bkc_id: bkc.id,
    idempotency_key: form.idempotencyKey.trim(),
    bindings: [
	  { role: "head", device_id: form.headDeviceId.trim(), fabric_address: form.headFabricAddress.trim(), service_address: form.headServiceAddress.trim(), service_port: bkc.multi_device.service_port, observed_container_id: form.headObservedContainerId.trim() || undefined },
	  { role: "worker", device_id: form.workerDeviceId.trim(), fabric_address: form.workerFabricAddress.trim(), observed_container_id: form.workerObservedContainerId.trim() || undefined },
    ],
    local_model_path: form.localModelPath.trim() || undefined,
    api_key: form.apiKey,
  }
}

export function validateClusterDeploymentForm(form: DeployForm, bkc: DeployBKC) {
  if (!bkc.multi_device) return "Select and apply a multi-device BKC"
  if (!form.headDeviceId.trim() || !form.workerDeviceId.trim()) return "Both head and worker device IDs are required"
  if (form.headDeviceId.trim() === form.workerDeviceId.trim()) return "Head and worker devices must be distinct"
  if (!isIPAddress(form.headFabricAddress) || !isIPAddress(form.workerFabricAddress)) return "Both fabric addresses must be valid explicit IPs"
  if (form.headFabricAddress.trim() === form.workerFabricAddress.trim()) return "Head and worker fabric addresses must be distinct"
  if (!isExplicitServiceIPAddress(form.headServiceAddress)) return "Head service address must be an explicit non-loopback IP"
  if (form.headServiceAddress.trim() === form.headFabricAddress.trim() || form.headServiceAddress.trim() === form.workerFabricAddress.trim()) return "Head service address must be distinct from private fabric addresses"
  if (bkc.multi_device.service_port < 1 || bkc.multi_device.service_port > 65535 || bkc.port !== String(bkc.multi_device.service_port)) return "BKC head client/monitor port metadata is inconsistent"
  if (!form.idempotencyKey.trim()) return "Idempotency key is required"
	if (!form.apiKey) return "API key is required"
  if (form.localModelPath.trim() && !form.localModelPath.trim().startsWith("/")) return "Local model path must be absolute"
  return null
}

function isIPAddress(value: string) {
  const input = value.trim()
  if (/^(?:\d{1,3}\.){3}\d{1,3}$/.test(input)) {
    return input.split(".").every((part) => Number(part) >= 0 && Number(part) <= 255)
  }
  return input.includes(":") && /^[0-9a-f:]+$/i.test(input)
}

function isExplicitServiceIPAddress(value: string) {
  const input = value.trim().toLowerCase()
  if (!isIPAddress(input)) return false
  if (input === "0.0.0.0" || input === "::" || input === "::1" || input.startsWith("127.")) return false
  return !input.startsWith("ff")
}

function updateHistory(settings: SettingsDocument, form: DeployForm) {
  const images = [form.image.trim(), ...settings.history.images].filter(Boolean)
  const models = form.workload === "comfyui" ? settings.history.models : [form.model.trim(), ...settings.history.models].filter(Boolean)
  return {
    images: dedupe(images).slice(0, 20),
    models: dedupe(models).slice(0, 20),
  }
}

function dedupe(values: string[]) {
  const seen = new Set<string>()
  const result: string[] = []
  for (const value of values) {
    if (seen.has(value)) continue
    seen.add(value)
    result.push(value)
  }
  return result
}

export type DeployController = ReturnType<typeof useDeployController>

export function deploymentRecoveryNotice(deployment: DeploymentRecord) {
  const rollbackErrors = deployment.rollback?.errors?.filter(Boolean) ?? []
  const rollback = rollbackErrors.length > 0 ? ` Rollback needs attention: ${rollbackErrors.join("; ")}.` : ""
  return `Deployment ${deployment.id} is ${deployment.state}.${rollback} Open its dashboard record to inspect or retry recovery.`
}

function nextConfigField(current: ConfigField, workload: WorkloadType, cluster: boolean, delta: number) {
  const fields: ConfigField[] = cluster
    ? ["bkcAction", "headDevice", "headFabric", "headService", "headObserved", "workerDevice", "workerFabric", "workerObserved", "idempotencyKey", "localModelPath", "apiKey"]
    : workload === "vllm"
    ? ["port", "extraArgs", "bkcAction", "contextLength", "overheadGB", "hfmemCalculate", "hfmemApply"]
    : ["port", "extraArgs", "bkcAction"]
  const index = fields.findIndex((field) => field === current)
  return fields[(index + delta + fields.length) % fields.length]
}

function applyVLLMFlags(extraArgs: string, estimate: VLLMMemoryEstimate) {
  let args = extraArgs
  args = setOrReplaceArg(args, "--gpu-memory-utilization", estimate.utilization.toFixed(2))
  args = setOrReplaceArg(args, "--max-model-len", `${estimate.context_length}`)
  if (!argValue(args, "--tensor-parallel-size")) {
    args = setOrReplaceArg(args, "--tensor-parallel-size", `${estimate.tensor_parallel}`)
  }
  return normalizeArgs(args)
}

function normalizeArgs(args: string) {
  return args.trim().replace(/\s+/g, " ")
}

function formatArgsForEditor(args: string) {
  const tokens = args.trim().split(/\s+/).filter(Boolean)
  if (tokens.length === 0) {
    return ""
  }

  const lines: string[] = []
  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index]
    if (token.startsWith("-") && !token.includes("=") && index + 1 < tokens.length && !tokens[index + 1].startsWith("-")) {
      lines.push(`${token} ${tokens[index + 1]}`)
      index += 1
      continue
    }
    lines.push(token)
  }
  return lines.join("\n")
}

function argValue(args: string, flag: string) {
  const tokens = args.trim().split(/\s+/).filter(Boolean)
  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index]
    if (token === flag) {
      return tokens[index + 1] ?? ""
    }
    const prefix = `${flag}=`
    if (token.startsWith(prefix)) {
      return token.slice(prefix.length)
    }
  }
  return ""
}

function setOrReplaceArg(args: string, flag: string, value: string) {
  const tokens = args.trim().split(/\s+/).filter(Boolean)
  const next: string[] = []
  let replaced = false
  for (let index = 0; index < tokens.length; index += 1) {
    const token = tokens[index]
    if (token === flag) {
      next.push(flag, value)
      replaced = true
      if (index + 1 < tokens.length) {
        index += 1
      }
      continue
    }
    const prefix = `${flag}=`
    if (token.startsWith(prefix)) {
      next.push(flag, value)
      replaced = true
      continue
    }
    next.push(token)
  }
  if (!replaced) {
    next.push(flag, value)
  }
  return next.join(" ")
}

function cloneMap(src?: Record<string, string>) {
  return src ? { ...src } : {}
}

function cloneRuntime(src?: DeployBKC["runtime"]) {
  return {
    ipc_mode: src?.ipc_mode,
    shm_size: src?.shm_size,
    ulimits: src?.ulimits ? { ...src.ulimits } : {},
  }
}

function estimateArgsSource(extraArgs: string, bkc: DeployBKC | null) {
  if (argValue(extraArgs, "--kv-cache-dtype")) {
    return extraArgs
  }
  if (!bkc) {
    return extraArgs
  }
  const kvCacheDType = argValue(bkc.extra_args, "--kv-cache-dtype")
  if (!kvCacheDType) {
    return extraArgs
  }
  return normalizeArgs(`${extraArgs} --kv-cache-dtype ${kvCacheDType}`)
}

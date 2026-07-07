import type { TextareaRenderable } from "@opentui/core"
import { useKeyboard } from "@opentui/react"
import { useRef, type ReactNode } from "react"

import { useTheme } from "../../theme/context"
import type { DeployController } from "./useDeployController"

type DeployRouteProps = {
  controller: DeployController
  terminalHeight?: number
}

const STEP_LABELS = ["Workload", "Device", "Image", "Model", "Variant", "Config", "Deploy"]

export function DeployRoute(props: DeployRouteProps) {
  const theme = useTheme()
  const currentStep = STEP_LABELS[props.controller.stepIndex] ?? STEP_LABELS[0]
  const viewportHeight = Math.max(14, (props.terminalHeight ?? 40) - 18)

  return (
    <scrollbox height={viewportHeight} style={scrollboxStyle(theme)}>
      <box flexDirection="column" gap={1} flexGrow={1} paddingRight={1}>
      {props.controller.notice ? <Banner color={noticeColor(theme, props.controller.notice.level)}>{props.controller.notice.message}</Banner> : null}
      {props.controller.pendingAction ? <Banner color={theme.colors.accent}>Running {props.controller.pendingAction}...</Banner> : null}

      <box flexDirection="row" gap={1}>
        {STEP_LABELS.map((label, index) => {
          const active = index === props.controller.stepIndex
          const complete = index < props.controller.stepIndex
          return (
            <box
              key={label}
              border
              borderStyle={active ? "double" : "single"}
              borderColor={active ? theme.colors.borderStrong : complete ? theme.colors.success : theme.colors.border}
              backgroundColor={theme.colors.panelMuted}
              paddingX={1}
            >
              <text fg={active ? theme.colors.accent : complete ? theme.colors.success : theme.colors.textMuted}>{active ? `▸ ${index + 1}. ${label}` : `${index + 1}. ${label}`}</text>
            </box>
          )
        })}
      </box>

      <box flexDirection="row" gap={1} flexGrow={1}>
        <box flexBasis="58%" flexGrow={1} border borderStyle="single" borderColor={theme.colors.border} backgroundColor={theme.colors.panelMuted} padding={1} flexDirection="column" gap={1}>
          <text fg={theme.colors.text}><strong>{currentStep}</strong></text>
          {renderStep(props.controller)}
        </box>

        <box width={42} minWidth={38} border borderStyle="single" borderColor={theme.colors.border} backgroundColor={theme.colors.panelMuted} padding={1} flexDirection="column" gap={1}>
          <text fg={theme.colors.text}><strong>Summary</strong></text>
          <Line label="Workload" value={props.controller.form.workload} />
          <Line label="Device" value={props.controller.form.deviceId || "-"} />
          <Line label="Image" value={props.controller.form.image || "-"} />
          <Line label="Model" value={props.controller.form.workload === "comfyui" ? "n/a" : props.controller.form.model || "-"} />
          <Line label="Variant" value={props.controller.form.workload === "comfyui" ? "n/a" : variantSummary(props.controller.form.ggufVariant, props.controller.form.ggufFiles.length)} />
          <Line label="Port" value={props.controller.form.port || "-"} />
          <Line label="Args" value={firstArgLine(props.controller.form.extraArgs) || "-"} />
          <text fg={theme.colors.textSubtle}>Use Esc to go back a step.</text>
        </box>
      </box>
      </box>
    </scrollbox>
  )
}

function renderStep(controller: DeployController) {
  switch (controller.step) {
    case "workload":
      return <WorkloadStep controller={controller} />
    case "device":
      return <DeviceStep controller={controller} />
    case "image":
      return <ImageStep controller={controller} />
    case "model":
      return <ModelStep controller={controller} />
    case "variant":
      return <VariantStep controller={controller} />
    case "config":
      return <ConfigStep controller={controller} />
    default:
      return <ReviewStep controller={controller} />
  }
}

function WorkloadStep(props: { controller: DeployController }) {
  const theme = useTheme()
  const options = [
    { id: "vllm", label: "vLLM", description: "OpenAI-compatible server for text generation" },
    { id: "llamacpp", label: "llama.cpp", description: "GGUF inference server for efficient local deploys" },
    { id: "comfyui", label: "ComfyUI", description: "Node-based image workflow server" },
  ] as const

  return (
    <box flexDirection="column" gap={1}>
      <text fg={theme.colors.textMuted}>Choose the workload type you want to deploy.</text>
      {options.map((option, index) => {
        const highlighted = index === props.controller.cursor
        const selected = props.controller.form.workload === option.id
        return (
          <box key={option.id} border borderStyle={selected ? "double" : "single"} borderColor={selected ? theme.colors.borderStrong : highlighted ? theme.colors.accent : theme.colors.border} backgroundColor={theme.colors.panel} padding={1} flexDirection="column" onMouseDown={() => props.controller.selectWorkload(option.id)}>
            <text fg={selected ? theme.colors.accent : theme.colors.text}><strong>{selected ? `▸ ${index + 1}. ${option.label}` : `${index + 1}. ${option.label}`}</strong></text>
            <text fg={highlighted ? theme.colors.textMuted : theme.colors.textSubtle}>{option.description}</text>
          </box>
        )
      })}
      <text fg={theme.colors.textSubtle}>Arrow keys or 1-3 choose. Enter continues.</text>
    </box>
  )
}

function DeviceStep(props: { controller: DeployController }) {
  const theme = useTheme()
  if (props.controller.deviceOptions.length === 0) {
    return <text fg={theme.colors.warning}>No devices are configured. Add a device first.</text>
  }

  return (
    <box flexDirection="column" gap={1}>
      {props.controller.deviceOptions.map((device, index) => {
        const selected = device.id === props.controller.form.deviceId
        const highlighted = index === props.controller.cursor
        return (
          <box key={device.id} border borderStyle={selected ? "double" : "single"} borderColor={selected ? theme.colors.borderStrong : highlighted ? theme.colors.accent : theme.colors.border} backgroundColor={theme.colors.panel} padding={1} flexDirection="column" onMouseDown={() => props.controller.selectDevice(device.id)}>
            <text fg={selected ? theme.colors.accent : theme.colors.text}><strong>{selected ? `▸ ${device.label}` : device.label}</strong></text>
            <text fg={highlighted ? theme.colors.textMuted : theme.colors.textSubtle}>{device.secondary}</text>
          </box>
        )
      })}
      <text fg={theme.colors.textSubtle}>Arrow keys choose. Enter continues.</text>
    </box>
  )
}

function ImageStep(props: { controller: DeployController }) {
  const theme = useTheme()
  const history = props.controller.settings.history.images.slice(0, 5)

  return (
    <box flexDirection="column" gap={1}>
      <text fg={theme.colors.textMuted}>Enter the Docker image or pick a recent/default one.</text>
      <Field label="Docker image" active>
        <input value={props.controller.form.image} onInput={(value) => props.controller.setValue("image", value)} focused width={52} backgroundColor={theme.colors.panel} textColor={theme.colors.text} focusedBackgroundColor={theme.colors.panel} cursorColor={theme.colors.accent} placeholder="vllm/vllm-openai:latest" />
      </Field>
      <text fg={theme.colors.textSubtle}>Recent images:</text>
      {history.map((image) => (
        <ActionChip key={image} onSelect={() => props.controller.setValue("image", image)}>{image}</ActionChip>
      ))}
      <text fg={theme.colors.textSubtle}>Enter continues.</text>
    </box>
  )
}

function ModelStep(props: { controller: DeployController }) {
  const theme = useTheme()
  const history = props.controller.settings.history.models.slice(0, 5)
  if (props.controller.form.workload === "comfyui") {
    return <text fg={theme.colors.textMuted}>ComfyUI does not require model selection. Press Enter to continue.</text>
  }

  return (
    <box flexDirection="column" gap={1}>
      <text fg={theme.colors.textMuted}>Enter a Hugging Face model ID. Matching models are searched through the daemon.</text>
      <Field label="Model ID" active>
        <input value={props.controller.form.model} onInput={(value) => props.controller.setValue("model", value)} focused width={52} backgroundColor={theme.colors.panel} textColor={theme.colors.text} focusedBackgroundColor={theme.colors.panel} cursorColor={theme.colors.accent} placeholder="meta-llama/Llama-3.1-8B-Instruct" />
      </Field>
      {props.controller.searchError ? <text fg={theme.colors.warning}>{props.controller.searchError}</text> : null}
      {props.controller.modelResults.slice(0, 4).map((model, index) => (
        <ActionChip key={model.id} active={index === props.controller.cursor} onSelect={() => props.controller.selectModel(model.id)}>{model.id}</ActionChip>
      ))}
      <text fg={theme.colors.textSubtle}>Recent models:</text>
      {history.map((model) => (
        <ActionChip key={model} onSelect={() => props.controller.setValue("model", model)}>{model}</ActionChip>
      ))}
    </box>
  )
}

function VariantStep(props: { controller: DeployController }) {
  const theme = useTheme()
  const variants = props.controller.ggufVariants

  if (props.controller.ggufLoading) {
    return <text fg={theme.colors.textMuted}>Looking up GGUF variants for {props.controller.form.model}...</text>
  }

  if (props.controller.ggufError) {
    return (
      <box flexDirection="column" gap={1}>
        <text fg={theme.colors.warning}>Could not list GGUF variants: {props.controller.ggufError}</text>
        <text fg={theme.colors.textSubtle}>Press Enter or S to skip and continue without a pre-downloaded variant.</text>
      </box>
    )
  }

  if (variants.length === 0) {
    return (
      <box flexDirection="column" gap={1}>
        <text fg={theme.colors.textMuted}>No GGUF files were found in this repo.</text>
        <text fg={theme.colors.textSubtle}>Press Enter to continue. The container will load the model directly.</text>
      </box>
    )
  }

  return (
    <box flexDirection="column" gap={1}>
      <text fg={theme.colors.textMuted}>Choose a GGUF quantization. All shards for the selected variant are downloaded to the device before the container starts.</text>
      {variants.map((variant, index) => {
        const highlighted = index === props.controller.cursor
        const selected = props.controller.form.ggufVariant === variant.quantization
        const sizeGB = (variant.total_size_mb / 1024).toFixed(1)
        const shardLabel = variant.shard_count > 1 ? `${variant.shard_count} shards` : "single file"
        return (
          <box
            key={`${variant.quantization}-${variant.primary}`}
            border
            borderStyle={selected ? "double" : "single"}
            borderColor={selected ? theme.colors.borderStrong : highlighted ? theme.colors.accent : theme.colors.border}
            backgroundColor={theme.colors.panel}
            padding={1}
            flexDirection="column"
            onMouseDown={() => props.controller.selectVariant(index)}
          >
            <text fg={selected ? theme.colors.accent : theme.colors.text}>
              <strong>{highlighted ? `▸ ${variant.quantization}` : variant.quantization}</strong>
              <span fg={theme.colors.textMuted}> · {sizeGB} GB · {shardLabel}</span>
            </text>
            <text fg={theme.colors.textSubtle}>{variant.primary}</text>
          </box>
        )
      })}
      <text fg={theme.colors.textSubtle}>Arrow keys or j/k choose. Enter selects. S skips. Esc returns to Model.</text>
    </box>
  )
}

function ConfigStep(props: { controller: DeployController }) {
  const theme = useTheme()
  const bkc = props.controller.availableBKC
  const helper = props.controller.vllmHelper

  return (
    <box flexDirection="column" gap={1}>
      <Field label="Port" active={props.controller.configField === "port"}>
        <input value={props.controller.form.port} onInput={(value) => props.controller.setValue("port", value)} focused={props.controller.configField === "port"} width={12} backgroundColor={theme.colors.panel} textColor={theme.colors.text} focusedBackgroundColor={theme.colors.panel} cursorColor={theme.colors.accent} placeholder="8000" />
      </Field>
      <Field label="Extra args" active={props.controller.configField === "extraArgs"}>
        <box minHeight={6} paddingY={0} flexDirection="column" justifyContent="center">
          {formatArgsPreview(props.controller.form.extraArgs).length > 0 ? (
            formatArgsPreview(props.controller.form.extraArgs).map((line, index) => (
              <text key={`arg-${index}`} fg={theme.colors.textMuted}>{line}</text>
            ))
          ) : (
            <text fg={theme.colors.textSubtle}>--max-model-len 32768</text>
          )}
        </box>
      </Field>

      <box border borderStyle="single" borderColor={bkc ? props.controller.hasAppliedBKC ? theme.colors.success : bkc.match_type === "suggested" ? theme.colors.warning : theme.colors.border : theme.colors.border} backgroundColor={theme.colors.panel} padding={1} flexDirection="column" gap={1}>
        <text fg={theme.colors.text}><strong>BKC</strong></text>
        {bkc ? (
          <>
            <text fg={theme.colors.textMuted}>{bkc.name}</text>
            <text fg={theme.colors.textSubtle}>{bkc.description}</text>
            {bkc.match_type === "suggested" && bkc.warning ? <text fg={theme.colors.warning}>{bkc.warning}</text> : null}
            {bkc.notes.map((note, index) => (
              <text key={`${bkc.id}-${index}`} fg={theme.colors.textSubtle}>• {note}</text>
            ))}
            <ActionChip active={props.controller.configField === "bkcAction"} onSelect={props.controller.applyBKC}>{props.controller.hasAppliedBKC ? "Reapply BKC" : bkc.match_type === "suggested" ? "Apply suggested BKC" : "Apply BKC"}</ActionChip>
            {props.controller.hasAppliedBKC ? <text fg={theme.colors.success}>BKC active. You can still override image, port, and extra args before deploying.</text> : null}
          </>
        ) : props.controller.form.model.trim() !== "" ? (
          <text fg={theme.colors.textSubtle}>No BKC found for the current model.</text>
        ) : (
          <text fg={theme.colors.textSubtle}>Select a model to check for a BKC preset.</text>
        )}
      </box>

      {props.controller.form.workload === "vllm" ? (
        <box border borderStyle="single" borderColor={theme.colors.border} backgroundColor={theme.colors.panel} padding={1} flexDirection="column" gap={1}>
          <text fg={theme.colors.text}><strong>vLLM Memory Helper</strong></text>
          <text fg={theme.colors.textMuted}>Estimate `--gpu-memory-utilization` and related flags using `hf-mem` plus the selected device GPU VRAM.</text>
          <box flexDirection="row" gap={1}>
            <Field label="Context length" active={props.controller.configField === "contextLength"}>
              <input value={helper.contextLength} onInput={(value) => props.controller.updateVLLMHelper("contextLength", value)} focused={props.controller.configField === "contextLength"} width={14} backgroundColor={theme.colors.panel} textColor={theme.colors.text} focusedBackgroundColor={theme.colors.panel} cursorColor={theme.colors.accent} placeholder="32768" />
            </Field>
            <Field label="Overhead (GB)" active={props.controller.configField === "overheadGB"}>
              <input value={helper.overheadGB} onInput={(value) => props.controller.updateVLLMHelper("overheadGB", value)} focused={props.controller.configField === "overheadGB"} width={10} backgroundColor={theme.colors.panel} textColor={theme.colors.text} focusedBackgroundColor={theme.colors.panel} cursorColor={theme.colors.accent} placeholder="1.5" />
            </Field>
          </box>
          <box flexDirection="row" gap={1}>
            <ActionChip active={props.controller.configField === "hfmemCalculate"} onSelect={props.controller.calculateVLLMMemory}>{helper.loading ? "Calculating..." : "Calculate with hf-mem"}</ActionChip>
            {helper.estimate ? <ActionChip active={props.controller.configField === "hfmemApply"} onSelect={props.controller.applyVLLMMemory}>Apply recommended flags</ActionChip> : null}
          </box>
          {helper.error ? <text fg={helper.error.startsWith("Applied") ? theme.colors.success : theme.colors.warning}>{helper.error}</text> : null}
          {!helper.loading && !helper.estimate && !helper.error ? <text fg={theme.colors.textSubtle}>No estimate yet. Run `hf-mem` after selecting a model and device.</text> : null}
          {helper.estimate ? (
            <>
              <text fg={theme.colors.textSubtle}>Weights: {helper.estimate.weights_gb.toFixed(2)} GB · KV cache: {helper.estimate.kv_cache_gb.toFixed(2)} GB · Overhead: {helper.estimate.overhead_gb.toFixed(2)} GB</text>
              <text fg={theme.colors.textSubtle}>GPUs: {helper.estimate.gpu_count} × {helper.estimate.min_vram_gb.toFixed(1)} GB min · TP: {helper.estimate.tensor_parallel}{helper.estimate.applied_tp_default ? " (defaulted from GPU count)" : ""}</text>
              <text fg={theme.colors.textSubtle}>Per-GPU need: {helper.estimate.required_per_gpu_gb.toFixed(2)} GB · Recommended `--gpu-memory-utilization {helper.estimate.utilization.toFixed(2)}`</text>
            </>
          ) : null}
        </box>
      ) : null}

      <text fg={theme.colors.textSubtle}>Tab switches fields. Enter edits args or continues. B applies BKC. M runs hf-mem. F applies recommended flags.</text>

      {props.controller.extraArgsEditing ? (
        <ExtraArgsEditorModal
          initialValue={props.controller.form.extraArgs}
          onClose={props.controller.closeExtraArgsEditor}
        />
      ) : null}
    </box>
  )
}

function ExtraArgsEditorModal(props: { initialValue: string; onClose: (value: string) => void }) {
  const theme = useTheme()
  const textareaRef = useRef<TextareaRenderable>(null)

  useKeyboard((key) => {
    if (key.name === "escape") {
      props.onClose(textareaRef.current?.plainText ?? props.initialValue)
    }
  })

  return (
    <box position="absolute" left={0} top={0} width="100%" height="100%" justifyContent="center" alignItems="center">
      <box width={80} border borderStyle="double" borderColor={theme.colors.borderStrong} backgroundColor={theme.colors.panel} padding={1} flexDirection="column" gap={1}>
        <text fg={theme.colors.text}><strong>Edit extra args</strong></text>
        <text fg={theme.colors.textSubtle}>One flag per line. Esc saves and closes. Deploy will flatten whitespace before submission.</text>
        <box border borderStyle="single" borderColor={theme.colors.borderStrong} backgroundColor={theme.colors.panelMuted} paddingX={1} paddingY={0}>
          <textarea
            ref={textareaRef}
            initialValue={props.initialValue}
            focused
            width={68}
            height={10}
            backgroundColor={theme.colors.panelMuted}
            textColor={theme.colors.text}
            focusedBackgroundColor={theme.colors.panelMuted}
            focusedTextColor={theme.colors.text}
            placeholder="--max-model-len 32768\n--tensor-parallel-size 2"
          />
        </box>
      </box>
    </box>
  )
}

function ReviewStep(props: { controller: DeployController }) {
  const theme = useTheme()
  return (
    <box flexDirection="column" gap={1}>
      <text fg={theme.colors.textMuted}>Review the deploy summary and explicitly select Deploy to submit the request.</text>
      <text fg={theme.colors.textSubtle}>The daemon will persist the service config and deploy to the selected device agent.</text>
      <box flexDirection="row" gap={1}>
        <ActionChip active={props.controller.reviewAction === "back"} onSelect={() => {
          if (props.controller.reviewAction === "back") {
            props.controller.handleKey({ name: "enter" })
            return
          }
          props.controller.selectReviewAction("back")
        }}>Back</ActionChip>
        <ActionChip active={props.controller.reviewAction === "deploy"} onSelect={() => {
          if (props.controller.reviewAction === "deploy") {
            props.controller.submitDeploy()
            return
          }
          props.controller.selectReviewAction("deploy")
        }}>Deploy now</ActionChip>
      </box>
      <text fg={theme.colors.textSubtle}>Left/Right or Tab select Back vs Deploy. Enter activates the selected action.</text>
    </box>
  )
}

function Card(props: { children: ReactNode; title: string }) {
  const theme = useTheme()
  return (
    <box flexGrow={1} border borderStyle="single" borderColor={theme.colors.border} backgroundColor={theme.colors.panelMuted} padding={1} flexDirection="column" gap={1}>
      <text fg={theme.colors.text}><strong>{props.title}</strong></text>
      {props.children}
    </box>
  )
}

function Field(props: { active: boolean; children: ReactNode; label: string }) {
  const theme = useTheme()
  return (
    <box flexDirection="column" gap={0}>
      <text fg={props.active ? theme.colors.borderStrong : theme.colors.textSubtle}>{props.label}</text>
      <box border borderStyle={props.active ? "double" : "single"} borderColor={props.active ? theme.colors.borderStrong : theme.colors.border} backgroundColor={theme.colors.panel} paddingX={1}>
        {props.children}
      </box>
    </box>
  )
}

function Line(props: { label: string; value: string }) {
  const theme = useTheme()
  return (
    <text fg={theme.colors.textMuted}>
      <span fg={theme.colors.textSubtle}>{props.label}:</span> {props.value}
    </text>
  )
}

function ActionChip(props: { active?: boolean; children: string; onSelect: () => void }) {
  const theme = useTheme()
  return (
    <box border borderStyle={props.active ? "double" : "single"} borderColor={props.active ? theme.colors.borderStrong : theme.colors.border} backgroundColor={theme.colors.panel} paddingX={1} onMouseDown={props.onSelect}>
      <text fg={props.active ? theme.colors.accent : theme.colors.textMuted}>{props.active ? `▸ ${props.children}` : props.children}</text>
    </box>
  )
}

function Banner(props: { children: string; color: string }) {
  const theme = useTheme()
  return (
    <box border borderStyle="single" borderColor={props.color} backgroundColor={theme.colors.panelMuted} paddingX={1}>
      <text fg={props.color}>{props.children}</text>
    </box>
  )
}

function scrollboxStyle(theme: ReturnType<typeof useTheme>) {
  return {
    rootOptions: { backgroundColor: theme.colors.panelMuted },
    wrapperOptions: { backgroundColor: theme.colors.panelMuted },
    viewportOptions: { backgroundColor: theme.colors.panelMuted },
    contentOptions: { backgroundColor: theme.colors.panelMuted },
    scrollbarOptions: {
      trackOptions: {
        foregroundColor: theme.colors.borderStrong,
        backgroundColor: theme.colors.panel,
      },
    },
  }
}

function formatArgsPreview(args: string) {
  const lines = args.split(/\n+/).map((line) => line.trim()).filter(Boolean)
  return lines.length > 0 ? lines : [args.trim()]
}

function firstArgLine(args: string) {
  return formatArgsPreview(args)[0] ?? ""
}

function variantSummary(quant: string, shardCount: number) {
  if (!quant) {
    return "-"
  }
  if (shardCount > 1) {
    return `${quant} (${shardCount} shards)`
  }
  return quant
}

function noticeColor(theme: ReturnType<typeof useTheme>, level: "info" | "success" | "warning" | "error") {
  switch (level) {
    case "success":
      return theme.colors.success
    case "warning":
      return theme.colors.warning
    case "error":
      return theme.colors.danger
    default:
      return theme.colors.accent
  }
}

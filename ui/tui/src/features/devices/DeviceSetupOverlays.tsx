import type { DevicesController } from "./useDevicesController"
import { MonitoringPromptModal } from "./MonitoringPromptModal"
import { AddDeviceSourceModal } from "./AddDeviceSourceModal"
import { DeviceEditorModal } from "./DeviceEditorModal"
import { SSHImportModal } from "./SSHImportModal"
import { TailscaleImportModal } from "./TailscaleImportModal"

type DeviceSetupOverlaysProps = {
  controller: DevicesController
}

export function DeviceSetupOverlays(props: DeviceSetupOverlaysProps) {
  return (
    <>
      {props.controller.addSource ? (
        <AddDeviceSourceModal
          selectedIndex={props.controller.addSource.selectedIndex}
          onSelect={props.controller.selectAddSource}
        />
      ) : null}

      {props.controller.editor ? (
        <DeviceEditorModal field={props.controller.editor.field} form={props.controller.editor.form} onChange={props.controller.setEditorValue} />
      ) : null}

      {props.controller.monitoringPrompt ? <MonitoringPromptModal /> : null}

      {props.controller.importer ? (
        <SSHImportModal error={props.controller.importer.error} hosts={props.controller.importer.hosts} loading={props.controller.importer.loading} selectedIndex={props.controller.importer.selectedIndex} />
      ) : null}

      {props.controller.tailscaleImporter ? (
        <TailscaleImportModal
          error={props.controller.tailscaleImporter.error}
          peers={props.controller.tailscaleImporter.peers}
          query={props.controller.tailscaleImporter.query}
          selectedIndex={props.controller.tailscaleImporter.selectedIndex}
          showTagHelp={props.controller.tailscaleImporter.showTagHelp}
          status={props.controller.tailscaleImporter.status}
          visiblePeers={props.controller.visiblePeers}
          onQueryChange={props.controller.setTailscaleQuery}
        />
      ) : null}
    </>
  )
}

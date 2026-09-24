// Hosts the authenticated Go Mode voice controls when mddb advertises a voice gateway.

import { onCleanup, onMount } from "solid-js";
import VoiceOverlay, { defaultVoiceOverlayMessages, type VoiceOverlayMessages } from "@maruel/gomode/web/VoiceOverlay";
import { configureMcpClient } from "@maruel/gomode/web/McpClient";
import { configureVoiceGateway, voiceSession } from "@maruel/gomode/web/VoiceSession";
import { notifications } from "@maruel/gomode/web/notifications";
import { useAuth } from "../contexts/AuthContext";
import { useI18n } from "../i18n";
import type { VoiceEndpoints } from "./BrowserVoiceShell";
import styles from "./VoicePanel.module.css";

export default function VoicePanel(props: { endpoints: VoiceEndpoints }) {
  const { token } = useAuth();
  const { t } = useI18n();
  const messages = (): VoiceOverlayMessages => ({
    assistant: t("voice.assistant") || defaultVoiceOverlayMessages.assistant,
    cancel: t("voice.cancel") || defaultVoiceOverlayMessages.cancel,
    cancelConnection: t("voice.cancelConnection") || defaultVoiceOverlayMessages.cancelConnection,
    clearTranscript: t("voice.clearTranscript") || defaultVoiceOverlayMessages.clearTranscript,
    connect: t("voice.connect") || defaultVoiceOverlayMessages.connect,
    connectionFailed: t("voice.connectionFailed") || defaultVoiceOverlayMessages.connectionFailed,
    endSession: t("voice.endSession") || defaultVoiceOverlayMessages.endSession,
    listening: t("voice.listening") || defaultVoiceOverlayMessages.listening,
    microphone: t("voice.microphone") || defaultVoiceOverlayMessages.microphone,
    mute: t("voice.mute") || defaultVoiceOverlayMessages.mute,
    muted: t("voice.muted") || defaultVoiceOverlayMessages.muted,
    reconnecting: t("voice.reconnecting") || defaultVoiceOverlayMessages.reconnecting,
    retry: t("voice.retry") || defaultVoiceOverlayMessages.retry,
    signaling: t("voice.signaling") || defaultVoiceOverlayMessages.signaling,
    speaker: t("voice.speaker") || defaultVoiceOverlayMessages.speaker,
    speaking: t("voice.speaking") || defaultVoiceOverlayMessages.speaking,
    transcript: t("voice.transcript") || defaultVoiceOverlayMessages.transcript,
    transcriptPlaceholder: t("voice.transcriptPlaceholder") || defaultVoiceOverlayMessages.transcriptPlaceholder,
    unmute: t("voice.unmute") || defaultVoiceOverlayMessages.unmute,
    voiceAssistant: t("voice.voiceAssistant") || defaultVoiceOverlayMessages.voiceAssistant,
    waitingForServer: t("voice.waitingForServer") || defaultVoiceOverlayMessages.waitingForServer,
    settingUpWebRTC: t("voice.settingUpWebRTC") || defaultVoiceOverlayMessages.settingUpWebRTC,
    you: t("voice.you") || defaultVoiceOverlayMessages.you,
  });

  onMount(() => {
    configureMcpClient(props.endpoints.mcpEndpoint, "mddb-frontend", token);
    configureVoiceGateway(props.endpoints.gatewayURL, null, token);
  });
  onCleanup(() => {
    voiceSession.disconnect();
    notifications.setVoiceActive(false);
  });

  return (
    <div class={styles.shell}>
      <VoiceOverlay messages={messages} />
    </div>
  );
}

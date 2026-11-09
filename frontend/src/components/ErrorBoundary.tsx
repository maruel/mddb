// Application error boundary with localized retry, reload, and diagnostic recovery actions.

import { createSignal, ErrorBoundary as SolidErrorBoundary, type ParentComponent } from "solid-js";
import { useI18n } from "../i18n";
import { Button } from "./shared";
import styles from "./ErrorBoundary.module.css";

interface ErrorFallbackProps {
  error: unknown;
  reset: () => void;
}

function ErrorFallback(props: ErrorFallbackProps) {
  const { t } = useI18n();
  const [copyState, setCopyState] = createSignal<"idle" | "copied" | "failed">("idle");
  const diagnostic = () => {
    const error = props.error;
    return error instanceof Error ? `${error.name}: ${error.message}\n${error.stack ?? ""}`.trim() : String(error);
  };

  const copyDiagnostic = async () => {
    // eslint-disable-next-line @typescript-eslint/no-unnecessary-condition -- Browser capabilities and DOM state can differ from their declared types.
    if (!navigator.clipboard) {
      setCopyState("failed");
      return;
    }
    try {
      await navigator.clipboard.writeText(diagnostic());
      setCopyState("copied");
    } catch {
      setCopyState("failed");
    }
  };

  return (
    <div class={styles.errorBoundary}>
      <div class={styles.alertContent} role="alert" aria-live="assertive" aria-atomic="true">
        <h2>{t("recovery.title") || "Something went wrong"}</h2>
        <p class={styles.errorMessage}>{t("recovery.message") || "The workspace encountered an unexpected problem."}</p>
        <details class={styles.errorDetails}>
          <summary>{t("recovery.technicalDetails") || "Technical details"}</summary>
          <pre data-testid="error-diagnostic">{diagnostic()}</pre>
        </details>
        <div class={styles.actions}>
          <Button variant="primary" onClick={props.reset}>
            {t("recovery.retry") || "Try again"}
          </Button>
          <Button variant="secondary" onClick={() => window.location.reload()}>
            {t("recovery.reload") || "Reload app"}
          </Button>
          <Button variant="secondary" onClick={copyDiagnostic}>
            {t("recovery.copyDetails") || "Copy diagnostic details"}
          </Button>
        </div>
      </div>
      <p class={styles.copyStatus} role="status" aria-live="polite">
        {copyState() === "copied"
          ? t("recovery.copied") || "Diagnostic details copied"
          : copyState() === "failed"
            ? t("recovery.copyFailed") || "Could not copy diagnostic details"
            : ""}
      </p>
    </div>
  );
}

export const AppErrorBoundary: ParentComponent = (props) => {
  return (
    <SolidErrorBoundary fallback={(err: unknown, reset) => <ErrorFallback error={err} reset={reset} />}>
      {props.children}
    </SolidErrorBoundary>
  );
};

export default AppErrorBoundary;

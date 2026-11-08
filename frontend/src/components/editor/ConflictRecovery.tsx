// Presents draft/saved comparisons, explicit overlap choices, and editable conditional-save recovery.

import { createEffect, createMemo, createSignal, For, Show, untrack } from "solid-js";
import { useEditor, type RecoverySession } from "../../contexts/EditorContext";
import { useI18n } from "../../i18n";
import type { MergePart } from "../../contexts/editorMerge";
import { Dialog } from "../shared/Dialog";
import styles from "./ConflictRecovery.module.css";

export function ConflictRecovery(props: { onSaved: () => void }) {
  const editor = useEditor();
  const { t } = useI18n();
  return (
    <Show when={editor.recovery() || editor.recoveryLoading() || editor.recoveryError()}>
      <Dialog
        ariaLabel={t("editor.compareChanges")}
        class={styles.dialog}
        onClose={editor.closeRecovery}
        dismissOnEscape={() => !editor.recovery() || !editor.recoveryLoading()}
        dismissOnBackdrop={false}
      >
        <h2>{t("editor.compareChanges")}</h2>
        <Show when={editor.recoveryError()}>
          <p role="alert">{editor.recoveryError()}</p>
        </Show>
        <Show when={editor.recoveryLoading()}>
          <p role="status">{t("editor.recoveryWorking")}</p>
        </Show>
        <Show when={editor.recovery()} keyed>
          {(session) => <Comparison session={session} onSaved={props.onSaved} />}
        </Show>
        <Show when={!editor.recovery()}>
          <div class={styles.actions}>
            <button onClick={editor.closeRecovery}>{t("common.cancel")}</button>
            <button onClick={editor.compareChanges} disabled={editor.recoveryLoading()}>
              {t("editor.loadLatest")}
            </button>
          </div>
        </Show>
      </Dialog>
    </Show>
  );
}

function Comparison(props: { session: RecoverySession; onSaved: () => void }) {
  const editor = useEditor();
  const { t } = useI18n();
  const [choices, setChoices] = createSignal<Record<string, "local" | "saved">>({});
  const [previousResult, setPreviousResult] = createSignal(untrack(() => props.session.previousResult ?? null));
  let edited = false;
  const choose = (id: string, side: "local" | "saved") => {
    if (edited) setPreviousResult({ title: resultTitle(), content: resultContent() });
    edited = false;
    setChoices((current) => ({ ...current, [id]: side }));
  };
  const parts = () => [
    ...props.session.titleMerge.parts.map((part, i) => ({ part, id: `title-${i}`, title: true })),
    ...props.session.contentMerge.parts.map((part, i) => ({ part, id: `content-${i}`, title: false })),
  ];
  const conflicts = createMemo(() => parts().filter((entry) => !("text" in entry.part)));
  const resolved = () => conflicts().every((entry) => choices()[entry.id] !== undefined);
  function renderPart(part: MergePart, id: string): string {
    if ("text" in part) return part.text;
    return choices()[id] === "saved" ? part.saved : part.local;
  }
  const [resultTitle, setResultTitle] = createSignal("");
  const [resultContent, setResultContent] = createSignal("");
  createEffect(() => {
    setResultTitle(props.session.titleMerge.parts.map((part, i) => renderPart(part, `title-${i}`)).join(""));
    setResultContent(props.session.contentMerge.parts.map((part, i) => renderPart(part, `content-${i}`)).join(""));
  });
  return (
    <>
      <p>{t("editor.compareHelp")}</p>
      <Show when={previousResult()}>
        {(previous) => (
          <label class={styles.previous}>
            {t("editor.previousRecovery")}
            <textarea readOnly value={`# ${previous().title}\n\n${previous().content}`} />
          </label>
        )}
      </Show>
      <div class={styles.comparison}>
        <section>
          <h3>{t("editor.yourDraft")}</h3>
          <strong>{props.session.local.title}</strong>
          <pre>{props.session.local.content}</pre>
        </section>
        <section>
          <h3>{t("editor.latestSaved")}</h3>
          <strong>{props.session.saved.title}</strong>
          <pre>{props.session.saved.content}</pre>
        </section>
      </div>
      <details>
        <summary>{t("editor.acknowledgedBase")}</summary>
        <strong>{props.session.base.title}</strong>
        <pre class={styles.base}>{props.session.base.content}</pre>
      </details>
      <Show when={props.session.contentMerge.manual || props.session.titleMerge.manual}>
        <p role="status">{t("editor.manualRecovery")}</p>
      </Show>
      <Show when={conflicts().length > 0} fallback={<p role="status">{t("editor.independentChanges")}</p>}>
        <p>{t("editor.overlapHelp")}</p>
        <For each={conflicts()}>
          {(entry, index) => (
            <fieldset class={styles.conflict} disabled={editor.recoveryLoading()}>
              <legend>
                {entry.title ? t("editor.titlePlaceholder") : t("editor.overlap")} {index() + 1}
              </legend>
              <div class={styles.comparison}>
                <label>
                  <input
                    type="radio"
                    name={entry.id}
                    checked={choices()[entry.id] === "local"}
                    onChange={() => choose(entry.id, "local")}
                  />
                  {t("editor.useDraft")}
                  <pre>{"local" in entry.part ? entry.part.local : ""}</pre>
                </label>
                <label>
                  <input
                    type="radio"
                    name={entry.id}
                    checked={choices()[entry.id] === "saved"}
                    onChange={() => choose(entry.id, "saved")}
                  />
                  {t("editor.useSaved")}
                  <pre>{"saved" in entry.part ? entry.part.saved : ""}</pre>
                </label>
              </div>
            </fieldset>
          )}
        </For>
      </Show>
      <fieldset class={styles.result} disabled={!resolved() || editor.recoveryLoading()}>
        <legend>{t("editor.recoveredResult")}</legend>
        <label>
          {t("editor.titlePlaceholder")}
          <input
            value={resultTitle()}
            onInput={(e) => {
              edited = true;
              setResultTitle(e.currentTarget.value);
            }}
          />
        </label>
        <label>
          {t("editor.recoveredBody")}
          <textarea
            value={resultContent()}
            onInput={(e) => {
              edited = true;
              setResultContent(e.currentTarget.value);
            }}
          />
        </label>
      </fieldset>
      <div class={styles.actions}>
        <button onClick={editor.closeRecovery} disabled={editor.recoveryLoading()}>
          {t("common.cancel")}
        </button>
        <button onClick={editor.copyDraft}>{t("editor.copyDraft")}</button>
        <button
          disabled={!resolved() || editor.recoveryLoading()}
          onClick={async () => {
            if (await editor.saveRecovery(props.session, { title: resultTitle(), content: resultContent() }))
              props.onSaved();
          }}
        >
          {t("editor.saveRecovered")}
        </button>
      </div>
    </>
  );
}

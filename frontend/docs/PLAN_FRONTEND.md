# MDDB Frontend Interaction Foundation

Improve MDDB's workspace UI by adopting CAIC's proven interaction patterns—shared controls, keyboard-safe navigation, responsive layout state, and clear operational feedback—while preserving MDDB's document and table workflows.

## Phase 1 — productivity-navigation: Discoverable workspace shortcuts

- **Depends on:** workspace-navigation, feedback-recovery
- **Scope:** Shortcut reference and workspace-level keyboard navigation or quick switching.
- **Preserve:** ProseMirror and native input shortcuts take precedence while editing.
- **Verify:** Shortcuts are discoverable, do not intercept editor text entry, restore focus after overlays, and are covered by representative keyboard E2E tests.

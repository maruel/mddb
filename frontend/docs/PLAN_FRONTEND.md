# MDDB Frontend Interaction Foundation

Improve MDDB's workspace UI by adopting CAIC's proven interaction patterns—shared controls, keyboard-safe navigation, responsive layout state, and clear operational feedback—while preserving MDDB's document and table workflows.

## Phase 1 — responsive-shell: Reliable layout changes across breakpoints

- **Depends on:** workspace-navigation
- **Scope:** Workspace and settings sidebar state, mobile backdrops, workspace-creation modal state, and their breakpoint behavior.
- **Preserve:** Desktop sidebar collapse behavior, mobile overlay behavior, and non-dismissible first-workspace setup only when the workspace state requires it.
- **Verify:** Resizing from desktop to mobile and back produces the intended sidebar state without obscuring content; Escape and backdrop dismissal work; browser tests cover both resize directions and a touch-sized viewport.

## Phase 2 — feedback-recovery: Clear save, connection, and failure feedback

- **Depends on:** interaction-primitives
- **Scope:** Editor save state, event-stream/connectivity status, transient operation feedback, and error-boundary recovery.
- **Preserve:** Autosave, external-change protection, error localization, and existing notification data.
- **Verify:** Save and connection states are visible and announced appropriately; transient failures do not reflow the workspace; the error fallback offers retry, reload, and copyable diagnostic context; unit and browser tests cover success, failure, and recovery.

## Phase 3 — visual-system: Semantic UI tokens and interaction polish

- **Depends on:** interaction-primitives
- **Scope:** Shared semantic color, surface, state, and elevation tokens; migrated component styles.
- **Preserve:** Current light-theme appearance, contrast, component ownership, and data-driven field colors.
- **Verify:** Component styles consume semantic tokens except for intentional data colors; focus, hover, disabled, warning, and danger states remain visibly distinct; run CSS validation, `make lint build test`, and visual/mobile E2E coverage.

## Phase 4 — productivity-navigation: Discoverable workspace shortcuts

- **Depends on:** workspace-navigation, feedback-recovery
- **Scope:** Shortcut reference and workspace-level keyboard navigation or quick switching.
- **Preserve:** ProseMirror and native input shortcuts take precedence while editing.
- **Verify:** Shortcuts are discoverable, do not intercept editor text entry, restore focus after overlays, and are covered by representative keyboard E2E tests.

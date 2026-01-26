# Frontend Bugs and Issues

This document tracks potential bugs and issues found during code review while increasing test coverage.

## Issues

### 1. TableBoard Group Filtering Logic
**File:** `src/components/TableBoard.tsx:52-55`

**Description:** The filtering logic for groups compares option names with group names, but groups are keyed by `opt.id`. If an option's `id` differs from its `name`, this could cause unexpected behavior where groups with records are hidden.

**Note:** This works correctly when `opt.id` matches `opt.name`, but could be fragile if they differ.

---

### 2. Potential XSS via Markdown HTML
**File:** `src/components/MarkdownPreview.tsx:9-13`

**Description:** The markdown-it configuration has `html: true`, which allows raw HTML in markdown content. Combined with `innerHTML={html()}`, this could allow XSS attacks if user-provided markdown is rendered.

**Note:** This may be intentional for advanced use cases. Consider documenting the security implications or adding sanitization.

---

### 3. Error Handling Pattern in switchOrg
**File:** `src/App.tsx:104-125`

**Description:** The `switchOrg` function catches errors and sets an error message but then re-throws the error. This double error handling pattern could lead to confusion about who handles the error.

**Note:** This is intentional for some callers that need to handle the error, but the pattern could be cleaner.

---

### 4. Missing onUpdateRecord in Table View Components
**File:** `src/components/TableGrid.tsx`, `TableGallery.tsx`, `TableBoard.tsx`

**Description:** Unlike `TableTable`, the Grid, Gallery, and Board views don't have `onUpdateRecord` support. Users cannot edit records inline in these views.

**Note:** This may be intentional UX decision, but it's worth noting for feature parity.

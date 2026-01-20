# Frontend Bugs and Issues

This document tracks potential bugs and issues found during code review while increasing test coverage.

## Critical Issues

### 1. Checkbox Input Boolean Conversion Bug
**File:** `src/components/DatabaseTable.tsx:94-100`

**Description:** The checkbox input uses `Boolean(initialValue)` but `initialValue` is a string from `editValue()`. When editing a checkbox cell, if the value is the string `"false"`, `Boolean("false")` returns `true` because any non-empty string is truthy.

**Code:**
```tsx
case PropertyTypeCheckbox:
  return (
    <input
      type="checkbox"
      checked={Boolean(initialValue)}  // BUG: "false" string becomes true
      onChange={(e) => handleCellChange(String(e.target.checked))}
      class={styles.input}
    />
  );
```

**Expected Behavior:** `"false"` string should result in an unchecked checkbox.

**Suggested Fix:** Parse the string value properly:
```tsx
checked={initialValue === 'true' || initialValue === true}
```

---

### 2. Nested Node Type Detection Fails
**File:** `src/App.tsx:840-841, 867-868`

**Description:** When checking node type to determine which view to show (document vs database), the code uses `nodes().find((n) => n.id === selectedNodeId())` which only searches top-level nodes, not nested children. This means nested database or document nodes will not be found, and the wrong view type may be displayed.

**Code:**
```tsx
when={nodes().find((n) => n.id === selectedNodeId())?.type !== 'database'}
```

**Expected Behavior:** Should recursively search through all nodes including children.

**Suggested Fix:** Create a recursive node finder utility:
```tsx
const findNodeById = (nodes: Node[], id: string): Node | undefined => {
  for (const node of nodes) {
    if (node.id === id) return node;
    if (node.children) {
      const found = findNodeById(node.children.filter(Boolean) as Node[], id);
      if (found) return found;
    }
  }
  return undefined;
};
```

---

## Medium Priority Issues

### 3. Variable Shadowing in Auth Effect
**File:** `src/App.tsx:186`

**Description:** Inside the `createEffect` for loading the user, a variable `t` is assigned which shadows the `t` translation function from `useI18n()`. While this works correctly, it reduces code clarity and could cause confusion.

**Code:**
```tsx
createEffect(() => {
  // ...
  const t = token();  // Shadows t() from useI18n()
  const u = user();
  if (t && !u) {
    // ...
  }
});
```

**Suggested Fix:** Rename the local variable to `tok` or `currentToken`.

---

### 4. DatabaseBoard Group Filtering Logic
**File:** `src/components/DatabaseBoard.tsx:52-55`

**Description:** The filtering logic for groups compares option names with group names, but groups are keyed by `opt.id`. If an option's `id` differs from its `name`, this could cause unexpected behavior where groups with records are hidden.

**Code:**
```tsx
const optionNames = (col.options || []).map((opt) => opt.name);
return Object.values(grouped).filter(
  (g) => g.records.length > 0 || optionNames.includes(g.name)
);
```

**Note:** This works correctly when `opt.id` matches `opt.name`, but could be fragile if they differ.

---

## Low Priority / Code Quality Issues

### 5. Potential XSS via Markdown HTML
**File:** `src/components/MarkdownPreview.tsx:9-13`

**Description:** The markdown-it configuration has `html: true`, which allows raw HTML in markdown content. Combined with `innerHTML={html()}`, this could allow XSS attacks if user-provided markdown is rendered. This is an intentional feature for power users but should be documented.

**Code:**
```tsx
const md = new MarkdownIt({
  html: true,  // Allows raw HTML
  linkify: true,
  typographer: true,
});
```

**Note:** This may be intentional for advanced use cases. Consider documenting the security implications or adding sanitization.

---

### 6. Missing Error Handling in switchOrg
**File:** `src/App.tsx:104-125`

**Description:** The `switchOrg` function catches errors and sets an error message but then re-throws the error. This double error handling pattern could lead to confusion about who handles the error.

**Code:**
```tsx
} catch (err) {
  setError(`${t('errors.failedToSwitch')}: ${err}`);
  throw err; // Propagate error for callers
}
```

**Note:** This is intentional for some callers that need to handle the error, but the pattern could be cleaner.

---

### 7. Inconsistent Delete Button Symbols
**File:** Multiple components

**Description:** Delete buttons use inconsistent symbols across components:
- `DatabaseTable.tsx`: Uses `✕` (multiplication sign)
- `DatabaseGrid.tsx`: Uses `×` (multiplication sign, different character)
- `DatabaseGallery.tsx`: Uses `✕`
- `DatabaseBoard.tsx`: Uses `×`

**Suggested Fix:** Standardize on one symbol or use an icon component.

---

### 8. Missing onUpdateRecord in Database View Components
**File:** `src/components/DatabaseGrid.tsx`, `DatabaseGallery.tsx`, `DatabaseBoard.tsx`

**Description:** Unlike `DatabaseTable`, the Grid, Gallery, and Board views don't have `onUpdateRecord` support. Users cannot edit records inline in these views.

**Note:** This may be intentional UX decision, but it's worth noting for feature parity.

---

## Test Coverage Notes

The following areas have new test coverage:
- `src/i18n/index.test.tsx` - I18n provider, hook, and error translation
- `src/components/MarkdownPreview.test.tsx` - Markdown rendering and asset URL rewriting
- `src/components/Auth.test.tsx` - Login/register forms, OAuth, error handling
- `src/components/DatabaseTable.test.tsx` - Table rendering, editing, CRUD operations
- `src/components/DatabaseGrid.test.tsx` - Grid view rendering
- `src/components/DatabaseGallery.test.tsx` - Gallery view and image detection
- `src/components/DatabaseBoard.test.tsx` - Board view and grouping logic
- `src/utils/debounce.test.ts` - Additional debounce edge cases

### Areas Still Needing Coverage
- `src/App.tsx` - Main app logic (complex, may need integration tests)
- `src/components/SidebarNode.tsx` - Tree rendering
- `src/components/WorkspaceSettings.tsx` - Settings management
- `src/components/Onboarding.tsx` - Onboarding flow
- `src/components/Privacy.tsx`, `Terms.tsx` - Static pages
- `src/components/PWAInstallBanner.tsx` - PWA functionality

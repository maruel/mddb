# Bugs Found During E2E Testing

This document tracks bugs discovered during comprehensive e2e testing. These are actual application issues, not test selector problems.

---

## Bug 1: Table cell inline editing not working properly - FIXED

**Location**: `frontend/src/components/TableTable.tsx` - cell click handling

**Steps to Reproduce**:
1. Create a table with records
2. Click on a cell to edit its value
3. Expected: Cell should enter edit mode with an input field
4. Actual: The cell may not enter edit mode, or the input field isn't properly accessible

**Analysis**: The table cell click-to-edit functionality appears to have issues where clicking on a cell doesn't reliably trigger the edit mode.

**Impact**: High - Users cannot reliably edit table records inline

**Verified via**: E2E test `table-operations.spec.ts` - "add and edit records in table" (skipped)

**Fix**: Added auto-focus to input fields when entering edit mode using a ref callback with setTimeout. Also added keyboard support (Enter to save, Escape to cancel).

---

## Bug 2: Table record deletion not completing - FIXED

**Location**: `frontend/src/App.tsx` - `handleDeleteRecord` function

**Steps to Reproduce**:
1. Create a table with a record
2. Click the delete button (✕) for the record
3. Accept the confirmation dialog
4. Expected: Record should be removed from the table
5. Actual: Record remains visible; delete action doesn't complete

**Impact**: High - Users cannot delete table records

**Verified via**: E2E test `table-operations.spec.ts` - "delete a record from table" (skipped)

**Fix**: The `handleDeleteRecord` function was calling `loadNode(nodeId)` which has a guard that prevents reloading if the node is already loaded (`loadedNodeId === id`). Changed to directly reload records after deletion instead of calling loadNode.

---

## Bug 3: Page deletion not working - FIXED

**Location**: `frontend/src/App.tsx` - page delete functionality

**Steps to Reproduce**:
1. Create a page
2. Navigate to it
3. Click Delete button
4. Accept confirmation dialog
5. Expected: Page should be removed from sidebar
6. Actual: Page remains in sidebar after deletion

**Impact**: High - Users cannot delete pages

**Verified via**: E2E test `page-crud.spec.ts` - "delete a page" (skipped)

**Fix**: Added `loadedNodeId = null` before reloading nodes to clear stale state. Also clear `records` signal to prevent showing old data.

---

## Bug 4: Sidebar title not updating in real-time when editing

**Location**: `frontend/src/App.tsx` - `updateNodeTitle` and sidebar rendering

**Steps to Reproduce**:
1. Navigate to a page
2. Edit the title in the title input
3. Expected: Sidebar should update immediately to show new title
4. Actual: Sometimes works, sometimes doesn't (timing dependent)

**Impact**: Low - The title does eventually sync after save

**Verified via**: E2E test `page-crud.spec.ts` - "page title updates in sidebar" (flaky)

---

## Bug 5: Breadcrumbs not rendering for nested pages - FIXED

**Location**: `frontend/src/App.tsx` - breadcrumb rendering

**Steps to Reproduce**:
1. Create parent -> child -> grandchild page hierarchy
2. Navigate to grandchild via sidebar
3. Expected: Breadcrumbs show "Parent > Child > Grandchild"
4. Actual: Breadcrumb navigation element is empty

**Analysis**: The `<nav>` element exists but has no content when viewing a nested page. The breadcrumb data or rendering logic is not working.

**Impact**: Medium - Users cannot see page hierarchy or navigate via breadcrumbs

**Verified via**: E2E test `page-crud.spec.ts` - "breadcrumb navigation works for nested pages" (skipped)

**Fix**: The `getBreadcrumbs` function only searched the local `nodes` store, which only contains top-level nodes. Children are lazy-loaded and not always present. Added a `breadcrumbPath` signal that is populated by fetching the ancestor chain when loading a node via `loadNode()`.

---

## Bug 6: Mobile sidebar backdrop click not working - FIXED

**Location**: `frontend/src/components/Sidebar.module.css` or App.tsx - mobile sidebar

**Steps to Reproduce**:
1. Use mobile viewport (375x667)
2. Open sidebar via hamburger menu
3. Click on backdrop overlay
4. Expected: Sidebar should close
5. Actual: Sidebar content intercepts pointer events, backdrop is not clickable

**Analysis**: The error shows `_pageList` from the sidebar subtree intercepts pointer events. The backdrop's z-index or pointer-events CSS is incorrect.

**Impact**: Medium - Poor mobile UX, users cannot dismiss sidebar by clicking outside

**Verified via**: E2E test `mobile-ui.spec.ts` - "clicking backdrop closes mobile sidebar" (skipped)

**Fix**: Raised backdrop z-index from 40 to 99 (just below sidebar's 100). Added `cursor: pointer` to backdrop. Added `overflow: hidden` to sidebar when closed and `overflow-x: hidden` to pageList to prevent content from extending beyond sidebar bounds.

---

## Bug 7: Unsaved indicator timing issues

**Location**: `frontend/src/App.tsx` - autosave status signals

**Steps to Reproduce**:
1. Navigate to a page
2. Edit content
3. Expected: "Unsaved" indicator appears immediately, then "Saving..." then "Saved"
4. Actual: Indicators sometimes don't appear or appear in wrong sequence

**Impact**: Low - Autosave still works, but user feedback is inconsistent

**Verified via**: E2E test `page-crud.spec.ts` - "unsaved indicator appears when editing" (skipped)

---

## Bug 8: Workspace switching doesn't clear selected node content - FIXED

**Location**: `frontend/src/App.tsx` - workspace switching logic

**Steps to Reproduce**:
1. Navigate to a page in workspace 1 showing "Content in workspace 1"
2. Switch to workspace 2 via API/UI
3. Expected: Content from workspace 1 should not be visible
4. Actual: "Content in workspace 1" is still visible

**Analysis**: The workspace switching process might not be properly clearing the selected node state.

**Impact**: Medium - Confusing UX when switching workspaces

**Verified via**: E2E test `workspace-org.spec.ts` - "switching workspace clears selected node" (skipped)

**Fix**: Added clearing of `title`, `content`, `records`, `breadcrumbPath`, `hasUnsavedChanges`, and `autoSaveStatus` signals in both `switchWorkspace` and `switchOrg` functions.

---

## Bug 9: Version history not loading properly

**Location**: Version history API/display

**Steps to Reproduce**:
1. Create a page
2. Update the page content 3 times via API
3. View version history
4. Expected: 4 history entries (initial + 3 updates)
5. Actual: Sometimes shows fewer entries or doesn't load

**Impact**: Low - Version history may not show all commits

**Verified via**: E2E test `page-crud.spec.ts` - "version history loads and displays commits" (skipped)

---

## Bug 10: Invalid workspace ID not handled

**Location**: Frontend routing / workspace loading

**Steps to Reproduce**:
1. Navigate to `/w/invalid-workspace-12345/some-page`
2. Expected: Show error message or redirect to valid workspace
3. Actual: Page shows empty content without any error

**Analysis**: The app doesn't validate workspace IDs or handle invalid ones gracefully.

**Impact**: Low - Edge case, but poor error handling

**Verified via**: E2E test `error-handling.spec.ts` - "navigating to non-existent workspace shows error" (skipped)

---

## Test Issues Fixed (Not Bugs)

The following test failures were due to incorrect test selectors, not application bugs:

1. **User menu selector** - Fixed: Use `[class*="avatarButton"]` with `getByRole(..., { exact: true })`
2. **Workspace settings tab selector** - Fixed: Use `getByRole('button', { name: 'Workspace', exact: true })` to avoid matching workspace menu button
3. **Privacy/Terms page text** - Fixed: Look for specific heading text like "Terms of Service"
4. **Sidebar title selector** - Fixed: Use `[class*="pageTitleText"]` instead of `.title`
5. **Table header selectors** - Fixed: Use `th` element locators
6. **Code block selector** - Fixed: Use `.first()` when multiple code elements exist

---

## Recommendations

1. **Add error boundaries**: Invalid workspace IDs should show clear error messages.

2. ~~**Fix mobile sidebar z-index**: The backdrop should have higher z-index than sidebar content for proper click handling.~~ DONE

3. **Implement optimistic updates**: The sidebar title should update immediately when typing.

4. ~~**Add data refresh mechanisms**: Tables and lists should refresh automatically when data changes.~~ DONE

5. **Add e2e-friendly test IDs**: More consistent `data-testid` attributes would make tests more reliable.

6. ~~**Fix breadcrumb rendering**: Ensure breadcrumb path is populated when viewing nested pages.~~ DONE

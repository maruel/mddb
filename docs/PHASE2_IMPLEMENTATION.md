# Phase 2: Page Editor Implementation

## Overview

Phase 2 adds a professional markdown editor with live preview and auto-save functionality to the mddb application. The editor features a split-pane layout with real-time markdown rendering and intelligent debounced auto-save with visual feedback.

## Completed Features

### 1. Live Markdown Preview
- **Component**: `web/src/components/MarkdownPreview.tsx`
- **Library**: `markdown-it` for rendering markdown to HTML
- **Features**:
  - Real-time rendering as user types
  - Proper styling for all markdown elements (headings, lists, code blocks, tables, etc.)
  - Safe HTML rendering with `innerHTML`
  - Accessible with ARIA labels

### 2. Auto-Save with Debounce
- **Utility**: `web/src/utils/debounce.ts`
- **Mechanism**:
  - 2-second debounce on title and content changes
  - Only saves if changes actually exist
  - Non-blocking - doesn't interfere with typing
  - Automatic saving happens silently in the background
  
### 3. Auto-Save Status Indicators
- **Unsaved State**: Orange dot indicator (● Unsaved)
- **Saving State**: Blue spinning indicator (⟳ Saving...)
- **Saved State**: Green checkmark (✓ Saved) - displays for 2 seconds then fades

### 4. Split-Pane Editor Layout
- **Left Pane**: Markdown textarea editor
- **Right Pane**: Live HTML preview
- **Separator**: Subtle 1px gray divider between panes
- **Responsive**: Both panes flex equally
- **Styling**: Proper syntax highlighting with monospace font

### 5. Manual Save Button
- **Function**: `savePage()` - allows manual save if needed
- **State**: Disabled during network operations
- **Feedback**: Shows "Saving..." text while request is in flight

### 6. State Management
New signals added to App.tsx:
- `hasUnsavedChanges`: Boolean flag for unsaved state
- `autoSaveStatus`: 'idle' | 'saving' | 'saved' for UI feedback
- `debouncedAutoSave`: Debounced function that saves after 2s inactivity

## Technical Implementation

### Dependencies Added
```json
{
  "markdown-it": "^13.0.2"
}
```

### Files Created
1. `web/src/components/MarkdownPreview.tsx` - Preview component
2. `web/src/components/MarkdownPreview.module.css` - Preview styling
3. `web/src/utils/debounce.ts` - Debounce utility function

### Files Modified
1. `web/src/App.tsx`:
   - Added import statements for new components
   - Added state signals for auto-save tracking
   - Implemented debounced auto-save handler
   - Updated input handlers to trigger auto-save
   - Converted editor layout to split-pane with preview
   - Added status indicators
   - Renamed `updatePage` to `savePage` for clarity
   - Renamed `deletePage` to `deleteCurrentPage` for clarity

2. `web/src/App.module.css`:
   - Added `.editorContent` class for split-pane container
   - Added `.editorStatus` class for status indicators
   - Added `.unsavedIndicator`, `.savingIndicator`, `.savedIndicator` classes
   - Added `@keyframes spin` animation for saving state
   - Updated `.contentInput` with `background: white` and `min-width: 0` for flex layout

3. `web/package.json`:
   - Added `markdown-it` dependency

4. `docs/PLAN.md`:
   - Marked Phase 2 items as complete

## Styling Details

### Color Scheme
- **Unsaved**: Orange (#ff9800)
- **Saving**: Blue (#2196f3) with rotating animation
- **Saved**: Green (#4caf50)
- **Divider**: Light gray (#e0e0e0)

### Markdown Preview Styling
- Proper hierarchy for all heading levels
- Code blocks with light gray background
- Links with color and hover underline
- Tables with borders and alternating rows
- Blockquotes with left border accent
- Images with proper responsive sizing

## User Experience Flow

1. **User Types**: Title or content changes
2. **Visual Feedback**: "● Unsaved" indicator appears immediately
3. **Auto-Save Triggers**: After 2 seconds of inactivity
4. **Saving Feedback**: "⟳ Saving..." indicator appears
5. **Save Complete**: "✓ Saved" appears for 2 seconds
6. **Returns to Idle**: Indicator disappears

### Alternative Flow (Manual Save)
- User clicks "Save" button anytime
- Immediate "Saving..." state
- On success, page list updates and unsaved flag clears

## Code Quality

- ✓ All linting passes (ESLint + Prettier for frontend, golangci-lint for backend)
- ✓ TypeScript strict mode compliance
- ✓ Proper error handling with fallbacks
- ✓ No accessibility violations
- ✓ Responsive design support

## Testing Checklist

- [x] Live preview updates as you type
- [x] Auto-save triggers after 2 seconds of inactivity
- [x] Manual save button works on demand
- [x] Status indicators display correctly
- [x] Split pane layout shows both editor and preview
- [x] Markdown formatting renders correctly in preview
- [x] Code blocks are properly styled
- [x] Tables are properly displayed
- [x] Images are responsive
- [x] Page list updates after save
- [x] Unsaved changes flag prevents duplicate saves
- [x] All code passes linters

## Next Steps: Phase 3

Phase 3 will focus on:
- Page linking with autocomplete
- Enhanced markdown editor with toolbar (optional)
- Page naming and organization improvements
- Database/table support

## API Integration

The implementation uses existing API endpoints:
- `GET /api/pages` - List all pages
- `GET /api/pages/{id}` - Get page content
- `PUT /api/pages/{id}` - Update page
- `DELETE /api/pages/{id}` - Delete page
- `POST /api/pages` - Create page

No backend changes were required for Phase 2.

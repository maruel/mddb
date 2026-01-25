# Frontend Development Guidelines

## Frontend Development (SolidJS)

### Code Organization

- Components in `frontend/src/components/`
- Global state in `frontend/src/stores/` (if needed) or Context
- **CSS Modules**: Each `.tsx` file should have a corresponding `.module.css` file in the same directory. Import styles as `import styles from './ComponentName.module.css'`. This keeps component logic and styling colocated and prevents global CSS pollution.

### Build & Distribution

mddb uses `go:embed` to include the frontend in the mddb binary created from backend/cmd/mddb:

```bash
# Build frontend + Go binary with embedded frontend
make build-all

# Result: ./mddb (single executable, self-contained)
```

The compiled `../backend/frontend/dist/` folder is tracked in git for reproducible builds.

### Development Workflow

**Frontend development** (live reload):
```bash
make frontend-dev
# Frontend at http://localhost:5173 (proxies API to :8080)
```

**Backend + embedded frontend** (for testing embedded binary):
```bash
make frontend-build   # Build frontend once
make build            # Build Go binary
./mddb                # Run with embedded frontend
```

## Internationalization (i18n)

**All user-visible strings must be localized.**

### Adding New Strings

1. **Add the key to `src/i18n/types.ts`** in the appropriate section:
   ```typescript
   // In the Dictionary interface
   mySection: {
     existingKey: string;
     newKey: string;  // Add your new key
   };
   ```

2. **Add translations to ALL dictionary files**:
   - `src/i18n/dictionaries/en.ts` (English - required)
   - `src/i18n/dictionaries/fr.ts` (French)
   - `src/i18n/dictionaries/de.ts` (German)
   - `src/i18n/dictionaries/es.ts` (Spanish)

3. **Use the `t()` function in components**:
   ```tsx
   import { useI18n } from '../i18n';

   function MyComponent() {
     const { t } = useI18n();
     return <button>{t('mySection.newKey')}</button>;
   }
   ```

### Dictionary Structure

- `common.*` - Reusable strings (loading, save, cancel, delete)
- `app.*` - App-level UI (title, navigation, footer links)
- `auth.*` - Authentication forms
- `editor.*` - Document editor
- `welcome.*` - Welcome/empty states
- `onboarding.*` - Onboarding wizard
- `settings.*` - Settings panels
- `table.*` - Table views
- `errors.*` - Error messages (keyed by ErrorCode for API errors)
- `success.*` - Success messages

### Guidelines

- Never hardcode user-visible strings in components
- Use `t('key') || 'fallback'` for placeholders/titles that need string type
- Error messages from API use `translateError(code)` helper
- Keep translations concise - UI space is limited
- Test with longer languages (German) to catch overflow issues

## Code Quality & Linting

**All code must pass linting before commits.**

### Frontend (ESLint + Prettier)

Configured in root `eslint.config.js` and `.prettierrc`. Enforces strict equality, no-unused-vars, and consistent formatting (single quotes, 2 spaces).

## Useful Resources

- [SolidJS Docs](https://docs.solidjs.com)
- [solid-primitives/i18n](https://github.com/solidjs-community/solid-primitives/tree/main/packages/i18n)

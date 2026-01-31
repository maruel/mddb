import eslint from '@eslint/js';
import tseslint from 'typescript-eslint';
import solid from 'eslint-plugin-solid/configs/typescript';
import globals from 'globals';

// Shared TypeScript rules
const sharedRules = {
  'no-unused-vars': 'off',
  '@typescript-eslint/no-unused-vars': [
    'error',
    {
      argsIgnorePattern: '^_',
      varsIgnorePattern: '^_',
    },
  ],
  '@typescript-eslint/no-explicit-any': 'error',
  '@typescript-eslint/no-non-null-assertion': 'error',
  '@typescript-eslint/consistent-type-imports': [
    'error',
    { prefer: 'type-imports' },
  ],
  'no-shadow': 'off',
  '@typescript-eslint/no-shadow': ['error', { hoist: 'all' }],
  'no-console': [
    'error',
    {
      allow: ['warn', 'error'],
    },
  ],
  'no-debugger': 'error',
  eqeqeq: ['error', 'always'],
  'no-var': 'error',
  'prefer-const': 'error',
  'prefer-arrow-callback': 'error',
  'object-shorthand': 'error',
};

export default tseslint.config(
  eslint.configs.recommended,
  ...tseslint.configs.recommended,
  // Frontend (SolidJS) files
  {
    files: ['frontend/**/*.{ts,tsx}', 'sdk/**/*.ts'],
    ...solid,
    languageOptions: {
      globals: {
        ...globals.browser,
      },
      parserOptions: {
        project: './tsconfig.json',
      },
    },
    rules: {
      ...sharedRules,
      // Enforce all solid rules as errors
      'solid/components-return-once': 'error',
      'solid/event-handlers': 'error',
      'solid/imports': 'error',
      'solid/jsx-no-duplicate-props': 'error',
      'solid/jsx-no-script-url': 'error',
      'solid/jsx-no-undef': ['error', { typescriptEnabled: true }],
      'solid/jsx-uses-vars': 'error',
      'solid/no-array-handlers': 'error',
      'solid/no-destructure': 'error',
      'solid/no-innerhtml': 'error',
      'solid/no-react-deps': 'error',
      'solid/no-react-specific-props': 'error',
      'solid/no-unknown-namespaces': 'error',
      'solid/prefer-for': 'error',
      'solid/reactivity': 'error',
      'solid/self-closing-comp': 'error',
      'solid/style-prop': 'error',
    },
  },
  // Unit tests (allow non-null assertions like e2e tests)
  {
    files: ['frontend/**/*.test.{ts,tsx}'],
    rules: {
      '@typescript-eslint/no-non-null-assertion': 'off',
    },
  },
  // E2E tests (Playwright)
  {
    files: ['e2e/**/*.ts', 'playwright.config.ts'],
    languageOptions: {
      globals: {
        ...globals.node,
      },
      parserOptions: {
        project: './tsconfig.e2e.json',
      },
    },
    rules: {
      ...sharedRules,
      'no-console': 'off', // Allow console in e2e tests
      '@typescript-eslint/no-non-null-assertion': 'off', // Common in tests after expect assertions
    },
  },
  {
    ignores: [
      'frontend/src/*.gen.ts',
      'frontend/dist/**',
      'sdk/types.gen.ts', // Generated file
      'e2e/*.cjs', // CommonJS scripts
    ],
  }
);

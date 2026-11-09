// ESLint configuration for TypeScript and SolidJS.
import eslint from "@eslint/js";
import tseslint from "typescript-eslint";
import solid from "eslint-plugin-solid/configs/typescript";
import globals from "globals";
import prettier from "eslint-config-prettier";

// Shared TypeScript rules
const sharedRules = {
  "no-unused-vars": "off",
  "@typescript-eslint/no-unused-vars": [
    "error",
    {
      argsIgnorePattern: "^_",
      varsIgnorePattern: "^_",
    },
  ],
  "@typescript-eslint/no-explicit-any": "error",
  "@typescript-eslint/no-non-null-assertion": "error",
  "@typescript-eslint/consistent-type-imports": ["error", { prefer: "type-imports" }],
  "no-shadow": "off",
  "@typescript-eslint/no-shadow": ["error", { hoist: "all" }],
  "no-console": [
    "error",
    {
      allow: ["warn", "error"],
    },
  ],
  "no-debugger": "error",
  eqeqeq: ["error", "always"],
  "no-var": "error",
  "prefer-const": "error",
  "prefer-arrow-callback": "error",
  "object-shorthand": "error",
};

export default tseslint.config(
  eslint.configs.recommended,
  ...tseslint.configs.recommended,
  // Frontend (SolidJS) files
  {
    files: ["frontend/**/*.{ts,tsx}"],
    ...solid,
    languageOptions: {
      globals: {
        ...globals.browser,
      },
      parserOptions: {
        project: "./tsconfig.json",
      },
    },
    rules: {
      ...sharedRules,
      "@typescript-eslint/no-misused-spread": "error",
      // Enforce all solid rules as errors
      "solid/components-return-once": "error",
      "solid/event-handlers": "error",
      "solid/imports": "error",
      "solid/jsx-no-duplicate-props": "error",
      "solid/jsx-no-script-url": "error",
      "solid/jsx-no-undef": ["error", { typescriptEnabled: true }],
      "solid/jsx-uses-vars": "error",
      "solid/no-array-handlers": "error",
      "solid/no-destructure": "error",
      "solid/no-innerhtml": "error",
      "solid/no-react-deps": "error",
      "solid/no-react-specific-props": "error",
      "solid/no-unknown-namespaces": "error",
      "solid/prefer-for": "error",
      "solid/reactivity": "error",
      "solid/self-closing-comp": "error",
      "solid/style-prop": "error",
    },
  },
  {
    files: ["frontend/**/*.tsx"],
    // Solid assigns ref variables during rendering.
    rules: { "no-unassigned-vars": "off" },
  },
  {
    files: ["frontend/src/**/*.{ts,tsx}"],
    ignores: ["**/*.test.{ts,tsx}", "**/*.bench.{ts,tsx}"],
    rules: {
      "@typescript-eslint/await-thenable": "error",
      "@typescript-eslint/no-floating-promises": "error",
      "@typescript-eslint/no-base-to-string": "error",
      "@typescript-eslint/restrict-template-expressions": ["error", { allowNumber: true }],
      "@typescript-eslint/require-await": "error",
      "@typescript-eslint/no-misused-promises": ["error", { checksVoidReturn: { attributes: false } }],
      "@typescript-eslint/no-unnecessary-type-assertion": "error",
      "@typescript-eslint/no-unnecessary-condition": "error",
      "@typescript-eslint/no-unsafe-argument": "error",
      "@typescript-eslint/no-unsafe-assignment": "error",
      "@typescript-eslint/no-unsafe-call": "error",
      "@typescript-eslint/no-unsafe-member-access": "error",
      "@typescript-eslint/no-unsafe-return": "error",
      "@typescript-eslint/prefer-promise-reject-errors": "error",
      "@typescript-eslint/switch-exhaustiveness-check": ["error", { considerDefaultExhaustiveForUnions: true }],
      "@typescript-eslint/use-unknown-in-catch-callback-variable": "error",
      "@typescript-eslint/no-array-delete": "error",
      "@typescript-eslint/no-deprecated": "error",
      "@typescript-eslint/no-for-in-array": "error",
      "@typescript-eslint/only-throw-error": "error",
    },
  },
  // Unit tests (allow non-null assertions like e2e tests)
  {
    files: ["frontend/**/*.test.{ts,tsx}"],
    rules: {
      "@typescript-eslint/no-non-null-assertion": "off",
      "@typescript-eslint/await-thenable": "error",
      "@typescript-eslint/no-deprecated": "error",
      "@typescript-eslint/no-misused-promises": "error",
      "@typescript-eslint/only-throw-error": "error",
      // node:test owns registration promises. Test mocks deliberately implement
      // promise-returning interfaces without requiring artificial awaits.
      "@typescript-eslint/no-floating-promises": [
        "error",
        { allowForKnownSafeCalls: [{ from: "package", name: ["test", "it", "describe"], package: "@types/node" }] },
      ],
    },
  },
  // E2E tests (Playwright)
  {
    files: ["e2e/**/*.ts", "playwright.config.ts"],
    languageOptions: {
      globals: {
        ...globals.node,
      },
      parserOptions: {
        project: "./tsconfig.e2e.json",
      },
    },
    rules: {
      ...sharedRules,
      "@typescript-eslint/await-thenable": "error",
      "@typescript-eslint/no-floating-promises": "error",
      "@typescript-eslint/no-misused-promises": "error",
      "@typescript-eslint/only-throw-error": "error",
      "@typescript-eslint/no-deprecated": "error",
      "no-console": "off", // Allow console in e2e tests
      "@typescript-eslint/no-non-null-assertion": "off", // Common in tests after expect assertions
    },
  },
  // Node-side build scripts and reporters
  {
    files: ["frontend/scripts/**/*.mjs", "scripts/**/*.mjs"],
    languageOptions: {
      globals: {
        ...globals.node,
      },
    },
    rules: {
      ...sharedRules,
      "no-console": "off", // Command line scripts report progress on stdout
    },
  },
  {
    ignores: [
      "**/*.module.css.d.ts",
      "frontend/src/*.gen.ts",
      "frontend/src/sw.ts", // Built separately by VitePWA injectManifest
      "frontend/dist/**",
      "sdk/mddb/**", // Generated package
      "e2e/*.cjs", // CommonJS scripts
    ],
  },
  prettier,
);

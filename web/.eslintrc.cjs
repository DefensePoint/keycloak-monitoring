module.exports = {
  root: true,
  env: { browser: true, es2020: true },
  extends: [
    'eslint:recommended',
    'plugin:@typescript-eslint/recommended',
    'plugin:react-hooks/recommended',
  ],
  ignorePatterns: ['dist', '.eslintrc.cjs'],
  parser: '@typescript-eslint/parser',
  parserOptions: {
    ecmaVersion: 'latest',
    sourceType: 'module',
  },
  plugins: ['react-refresh', 'import', 'boundaries'],
  settings: {
    'import/resolver': {
      typescript: {
        alwaysTryTypes: true,
        project: './tsconfig.json',
      },
    },
    'boundaries/elements': [
      { type: 'features', pattern: 'src/features/*' },
      { type: 'shared', pattern: 'src/shared/*' },
      { type: 'app', pattern: 'src/app/*' },
      { type: 'theme', pattern: 'src/theme/*' },
    ],
    'boundaries/ignore': ['**/*.test.*', '**/*.spec.*'],
  },
  rules: {
    'react-refresh/only-export-components': 'off',
    // Enforce named exports instead of default exports (only for project files)
    'import/no-default-export': 'error',
    // Enforce feature boundaries
    'boundaries/element-types': [
      'error',
      {
        default: 'disallow',
        rules: [
          // Features can import from shared, theme, and themselves
          { from: 'features', allow: ['shared', 'theme'] },
          // Shared cannot import from features
          { from: 'shared', allow: ['shared', 'theme'] },
          // App can import from everything
          { from: 'app', allow: ['features', 'shared', 'theme', 'app'] },
          // Theme can only import from itself
          { from: 'theme', allow: ['theme'] },
        ],
      },
    ],
  },
  overrides: [
    // Services should only be used in hooks, not in pages/components
    {
      files: ['**/pages/**/*.tsx', '**/components/**/*.tsx'],
      rules: {
        'no-restricted-imports': [
          'error',
          {
            patterns: [
              {
                // Only match internal services, not node_modules
                group: [
                  '@/*/services',
                  '@/*/services/*',
                  '@/shared/services',
                  '@/shared/services/*',
                  '../services',
                  '../services/*',
                  '../../services',
                  '../../services/*',
                  './services',
                  './services/*',
                ],
                message:
                  'Services should only be used in hooks, not in pages/components. Create a custom hook instead.',
              },
            ],
          },
        ],
      },
    },
    // Prevent imports from old restructured paths
    {
      files: ['**/*.ts', '**/*.tsx'],
      rules: {
        'no-restricted-imports': [
          'error',
          {
            patterns: [
              {
                group: ['@/shared/hooks/form', '@/shared/hooks/form/*'],
                message:
                  'Import from @/shared/hooks instead. Form hooks are now at the root level (useFormErrors).',
              },
              {
                group: ['@/shared/lib/zod', '@/shared/lib/zod/*'],
                message:
                  'Import from @/shared/lib instead. Zod files are now at the root level (zodSchemas, zodValidators, zodFormSchemas).',
              },
              {
                group: ['@/shared/components/Form', '@/shared/components/Form/*'],
                message:
                  'Import from @/shared/components instead. Form components are now at the root level (FormTextField, FormSelect, FormCheckbox, FormSwitch).',
              },
            ],
          },
        ],
      },
    },
    // Form components must not import from features
    {
      files: [
        'src/shared/components/FormTextField/**/*.tsx',
        'src/shared/components/FormSelect/**/*.tsx',
        'src/shared/components/FormCheckbox/**/*.tsx',
        'src/shared/components/FormSwitch/**/*.tsx',
        'src/shared/hooks/useFormErrors.ts',
      ],
      rules: {
        'no-restricted-imports': [
          'error',
          {
            patterns: [
              {
                group: ['@/features', '@/features/*'],
                message:
                  'Form components and hooks must remain generic and cannot import from features. Keep them framework-agnostic.',
              },
            ],
          },
        ],
      },
    },
    // Allow default exports in specific config files
    {
      files: ['vite.config.ts', 'vitest.config.ts', 'tailwind.config.js'],
      rules: {
        'import/no-default-export': 'off',
      },
    },
  ],
};

// ESLint flat config — correctness-focused, not a style enforcer (Prettier is
// intentionally not wired in to avoid a whole-repo reformat). Vue *essential*
// (error-prevention) rules + TypeScript-recommended + unused-imports.
import vue from 'eslint-plugin-vue'
import tseslint from 'typescript-eslint'
import unusedImports from 'eslint-plugin-unused-imports'

export default tseslint.config(
  { ignores: ['dist/**', 'node_modules/**', 'coverage/**', '*.config.*', 'e2e/**'] },
  ...tseslint.configs.recommended,
  ...vue.configs['flat/essential'],
  {
    files: ['**/*.vue'],
    languageOptions: { parserOptions: { parser: tseslint.parser } },
  },
  {
    plugins: { 'unused-imports': unusedImports },
    rules: {
      // Let unused-imports own unused detection (it can auto-fix dead imports).
      '@typescript-eslint/no-unused-vars': 'off',
      'unused-imports/no-unused-imports': 'error',
      'unused-imports/no-unused-vars': [
        'warn',
        { argsIgnorePattern: '^_', varsIgnorePattern: '^_', caughtErrors: 'none' },
      ],
      // `any` is used deliberately at dynamic boundaries (WebAuthn options, JSON,
      // error catches); `tsc --strict` is the type-safety gate. Off to keep the
      // lint signal high (no wall of warnings).
      '@typescript-eslint/no-explicit-any': 'off',
      // UI primitives are intentionally single-word (Badge, Toggle, Donut…).
      'vue/multi-word-component-names': 'off',
    },
  },
)

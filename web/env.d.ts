/// <reference types="vite/client" />

// Injected at build time from package.json (see vite.config.ts).

declare module '*.vue' {
  import type { DefineComponent } from 'vue'
  // Standard Vue SFC shim; the empty-object generics are the documented form.
  // eslint-disable-next-line @typescript-eslint/no-empty-object-type
  const component: DefineComponent<{}, {}, any>
  export default component
}

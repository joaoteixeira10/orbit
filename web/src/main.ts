import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { router } from './router'
import { usePrefsStore } from './stores/prefs'
import { useAuthStore } from './stores/auth'
import { useBrandingStore } from './stores/branding'
import { i18n, currentLocale } from './i18n'
import { installStepUp } from './lib/stepup'
import './styles/main.css'

async function bootstrap() {
  const app = createApp(App)
  app.use(createPinia())
  app.use(i18n)
  installStepUp() // wire the step-up re-auth handler into the api client
  document.documentElement.setAttribute('lang', currentLocale())

  // Apply cached theme immediately, then load branding + restore the session
  // (which may hydrate fresh prefs) before the router guard evaluates.
  usePrefsStore().applyTheme()
  await Promise.all([useBrandingStore().load(), useAuthStore().restore()])

  app.use(router)
  app.mount('#app')
}

bootstrap()

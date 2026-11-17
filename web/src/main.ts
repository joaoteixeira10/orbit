import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import { router } from './router'
import { usePrefsStore } from './stores/prefs'
import { useAuthStore } from './stores/auth'
import { useBrandingStore } from './stores/branding'
import { i18n, currentLocale } from './i18n'
import { installStepUp } from './lib/stepup'
import { installTooltips } from './lib/tooltips'
import './styles/main.css'

// iPhone and iPad zoom in on any text field under 16px when it's tapped, and
// stay zoomed. maximum-scale=1 stops that there (iOS still lets the person
// pinch to zoom); elsewhere it would block pinching, so iOS only.
const ios = /iPhone|iPad|iPod/.test(navigator.userAgent) || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1)
if (ios) {
  const vp = document.querySelector('meta[name="viewport"]')
  if (vp && !/maximum-scale/.test(vp.getAttribute('content') ?? '')) vp.setAttribute('content', `${vp.getAttribute('content')}, maximum-scale=1`)
}

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
  installTooltips()
}

bootstrap()

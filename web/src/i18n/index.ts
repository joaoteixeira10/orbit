// vue-i18n setup. Locale is persisted in localStorage (orbit_locale); English is
// the base + fallback. Use the t() function in components via useI18n().
import { createI18n } from 'vue-i18n'
import { messages, SUPPORTED_LOCALES, type LocaleCode } from './messages'

const STORAGE_KEY = 'orbit_locale'

function initialLocale(): LocaleCode {
  const stored = localStorage.getItem(STORAGE_KEY)
  if (stored && SUPPORTED_LOCALES.some((l) => l.code === stored)) return stored as LocaleCode
  // Fall back to the browser's preferred language family (e.g. "pt-BR" → "pt").
  const nav = navigator.language?.slice(0, 2)
  if (nav && SUPPORTED_LOCALES.some((l) => l.code === nav)) return nav as LocaleCode
  return 'en'
}

export const i18n = createI18n({
  legacy: false, // Composition API
  locale: initialLocale(),
  fallbackLocale: 'en',
  messages,
})

// setLocale switches the active locale, persists it, and reflects it on <html lang>.
export function setLocale(code: LocaleCode): void {
  i18n.global.locale.value = code
  localStorage.setItem(STORAGE_KEY, code)
  document.documentElement.setAttribute('lang', code)
}

export function currentLocale(): LocaleCode {
  return i18n.global.locale.value as LocaleCode
}

export { SUPPORTED_LOCALES }
export type { LocaleCode }

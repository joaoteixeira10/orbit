import { usePrefsStore } from '@/stores/prefs'
import { locale } from '@/lib/dates'
import { i18n } from '@/i18n'

// daysUntil returns whole days from now to the given ISO timestamp (may be negative).
export function daysUntil(iso: string): number {
  const then = new Date(iso).getTime()
  return Math.floor((then - Date.now()) / 86_400_000)
}

// fmtDate formats an ISO timestamp in the app's language, the user's time
// zone and their 12- or 24-hour clock.
export function fmtDate(iso?: string | null): string {
  if (!iso) return '—'
  const prefs = usePrefsStore()
  const tz = prefs.timezone
  const opts: Intl.DateTimeFormatOptions = {
    year: 'numeric',
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: prefs.timeFormat === '12h',
  }
  if (tz && tz !== 'local') opts.timeZone = tz
  try {
    return new Intl.DateTimeFormat(locale(), opts).format(new Date(iso))
  } catch {
    return new Date(iso).toLocaleString()
  }
}

// fmtWhen is when something was said: the time if today, else the day too.
export function fmtWhen(iso: string): string {
  const prefs = usePrefsStore()
  const d = new Date(iso)
  const opts: Intl.DateTimeFormatOptions = { hour: '2-digit', minute: '2-digit', hour12: prefs.timeFormat === '12h' }
  if (prefs.timezone && prefs.timezone !== 'local') opts.timeZone = prefs.timezone
  if (d.toDateString() !== new Date().toDateString()) Object.assign(opts, { day: 'numeric', month: 'short' })
  try {
    return new Intl.DateTimeFormat(locale(), opts).format(d)
  } catch {
    return d.toLocaleString()
  }
}

// fmtTook is a length of time to the second: "4.2 s", "42 s", "2 min 37 s".
export function fmtTook(ms: number): string {
  const s = ms / 1000
  if (s < 10) return `${s.toLocaleString(locale(), { maximumFractionDigits: 1 })} s`
  if (s < 60) return `${Math.floor(s)} s`
  return `${Math.floor(s / 60)} min ${Math.floor(s % 60)} s`
}

// expiryTone classifies remaining lifetime for color coding.
export function expiryTone(iso: string): 'ok' | 'warn' | 'crit' {
  const d = daysUntil(iso)
  if (d < 7) return 'crit'
  if (d < 30) return 'warn'
  return 'ok'
}

// parseUA reduces a User-Agent string to a friendly "OS · Browser" pair.
export function parseUA(ua: string): { os: string; browser: string } {
  const os = /Windows/.test(ua)
    ? 'Windows'
    : /iPhone|iPad|iPod/.test(ua)
      ? 'iOS'
      : /Mac OS X|Macintosh/.test(ua)
        ? 'macOS'
        : /Android/.test(ua)
          ? 'Android'
          : /Linux/.test(ua)
            ? 'Linux'
            : i18n.global.t('acct.ua.unknownOS')
  const browser = /Edg\//.test(ua)
    ? 'Edge'
    : /OPR\/|Opera/.test(ua)
      ? 'Opera'
      : /Firefox\//.test(ua)
        ? 'Firefox'
        : /Chrome\//.test(ua)
          ? 'Chrome'
          : /Safari\//.test(ua)
            ? 'Safari'
            : i18n.global.t('acct.ua.browser')
  return { os, browser }
}

// relTime renders a coarse "time ago" for session activity, in the
// interface language.
export function relTime(iso: string): string {
  const m = Math.floor((Date.now() - new Date(iso).getTime()) / 60000)
  if (m < 2) return i18n.global.t('acct.ua.activeNow')
  const rtf = new Intl.RelativeTimeFormat(locale(), { numeric: 'auto' })
  if (m < 60) return rtf.format(-m, 'minute')
  const h = Math.floor(m / 60)
  if (h < 24) return rtf.format(-h, 'hour')
  return rtf.format(-Math.floor(h / 24), 'day')
}

import { usePrefsStore } from '@/stores/prefs'

// daysUntil returns whole days from now to the given ISO timestamp (may be negative).
export function daysUntil(iso: string): number {
  const then = new Date(iso).getTime()
  return Math.floor((then - Date.now()) / 86_400_000)
}

// fmtDate formats an ISO timestamp in the user's preferred timezone.
export function fmtDate(iso?: string | null): string {
  if (!iso) return '—'
  const tz = usePrefsStore().timezone
  const opts: Intl.DateTimeFormatOptions = {
    year: 'numeric',
    month: 'short',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }
  if (tz && tz !== 'local') opts.timeZone = tz
  try {
    return new Intl.DateTimeFormat(undefined, opts).format(new Date(iso))
  } catch {
    return new Date(iso).toLocaleString()
  }
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
            : 'Unknown'
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
            : 'Browser'
  return { os, browser }
}

// relTime renders a coarse "time ago" for session activity.
export function relTime(iso: string): string {
  const ms = Date.now() - new Date(iso).getTime()
  const m = Math.floor(ms / 60000)
  if (m < 2) return 'Active now'
  if (m < 60) return `${m} minutes ago`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h} hour${h > 1 ? 's' : ''} ago`
  const d = Math.floor(h / 24)
  return `${d} day${d > 1 ? 's' : ''} ago`
}

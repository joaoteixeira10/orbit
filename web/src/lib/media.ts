// Images in pages that come from other sites. Such an image tells its site
// who reads the page, and when (the request carries the reader's address and
// the time). So by default they wait behind a placeholder until clicked; a
// person can choose to always show them (Settings → Appearance). Images
// uploaded to Orbit, and data: URLs, show at once.
import { reactive } from 'vue'

// The person's preference, set by the prefs store.
export const mediaPrefs = reactive({
  remoteImages: (localStorage.getItem('orbit_remote_images') === 'always' ? 'always' : 'ask') as 'ask' | 'always',
})

// Shown this session (a click shows that image's site from then on).
const revealedHosts = reactive(new Set<string>())

// remoteHost is the site of an image from elsewhere, or '' when it's Orbit's
// own (a path, the same origin, data:) or not an http(s) address at all.
export function remoteHost(src: string): string {
  try {
    const u = new URL(src, location.origin)
    if (u.protocol !== 'https:' && u.protocol !== 'http:') return ''
    return u.origin === location.origin ? '' : u.host
  } catch {
    return ''
  }
}

// safeImageSrc is src if it may be put in an <img> (http, https, a path,
// data:image), else ''.
export function safeImageSrc(src: string): string {
  if (/^data:image\/(png|jpe?g|gif|webp);/i.test(src)) return src
  try {
    const u = new URL(src, location.origin)
    return u.protocol === 'https:' || u.protocol === 'http:' ? u.href : ''
  } catch {
    return ''
  }
}

// showsAtOnce: an image that needn't wait for a click.
export function showsAtOnce(src: string): boolean {
  const host = remoteHost(src)
  return !host || mediaPrefs.remoteImages === 'always' || revealedHosts.has(host)
}

export function reveal(src: string) {
  const host = remoteHost(src)
  if (host) revealedHosts.add(host)
}

// Fetch wrapper for the /api/v1 surface with transparent access-token refresh:
// the app's single place for auth and errors.

const BASE = '/api/v1'

// CLIENT_ID names this tab. It rides on every request (X-Orbit-Client) and
// comes back on the live updates a request caused, so the tab that made a
// change doesn't re-fetch what it already has.
export const CLIENT_ID = Math.random().toString(36).slice(2) + Date.now().toString(36)

// The access token lives ONLY in memory (never localStorage): an XSS can't read
// a credential that isn't persisted. The refresh token is an HttpOnly cookie the
// browser sends automatically to /auth/* — never visible to JS. A fresh page
// load therefore has no in-memory token, so the app silently refreshes from the
// cookie on boot (see the auth store's restore()).
let accessToken: string | null = null

export function getAccessToken(): string | null {
  return accessToken
}

// setAccessToken sets (or clears, with null) the in-memory access token. Used
// after login, refresh, and step-up.
export function setAccessToken(access: string | null) {
  accessToken = access
}

// readCookie returns a non-HttpOnly cookie value (used for the CSRF token).
function readCookie(name: string): string | null {
  const m = document.cookie.match(
    new RegExp('(?:^|; )' + name.replace(/[.$?*|{}()[\]\\/+^]/g, '\\$&') + '=([^;]*)'),
  )
  return m ? decodeURIComponent(m[1]) : null
}

// csrfHeader carries the double-submit CSRF token (the readable companion to the
// HttpOnly refresh cookie) so /auth/refresh and /auth/logout are accepted.
function csrfHeader(): Record<string, string> {
  const t = readCookie('orbit_csrf')
  return t ? { 'X-CSRF-Token': t } : {}
}

// Step-up bridge: the app registers a handler that re-authenticates the user
// (modal → password/passkey → mint a step-up token). api() invokes it on a
// `step_up_required` 403 and retries the request once on success.
let stepUpFn: (() => Promise<boolean>) | null = null
export function setStepUpHandler(fn: (() => Promise<boolean>) | null) {
  stepUpFn = fn
}

// maybeStepUp: when a response is a `step_up_required` 403, run the re-auth
// handler and, on success, re-issue the request via retry(). Shared by api()
// and the file-download helpers so every gated path prompts for re-auth.
async function maybeStepUp(res: Response, path: string, retry: () => Promise<Response>): Promise<Response> {
  if (res.status === 403 && stepUpFn && !path.startsWith('/auth/step-up')) {
    const body = await res.clone().json().catch(() => null)
    if (body?.error === 'step_up_required' && (await stepUpFn())) {
      return retry()
    }
  }
  return res
}

// --- Impersonation -------------------------------------------------------
// Impersonation swaps the in-memory access token for the target user's
// short-lived token (which has no refresh cookie of its own). The admin's access
// token is stashed in memory and the admin's HttpOnly refresh cookie stays put,
// so stopping — or a reload, or token expiry — returns to the admin. We track an
// explicit flag so a 401 mid-impersonation ends it via the admin cookie rather
// than silently refreshing into an admin session.
let impersonating = false
let adminAccessStash: string | null = null

export function isImpersonating(): boolean {
  return impersonating
}

export function beginImpersonation(impAccess: string) {
  adminAccessStash = accessToken
  impersonating = true
  setAccessToken(impAccess)
}

// endImpersonation restores the stashed admin access token. Returns false when
// not impersonating.
export function endImpersonation(): boolean {
  if (!impersonating) return false
  impersonating = false
  setAccessToken(adminAccessStash)
  adminAccessStash = null
  return true
}

// Fired when an impersonation token expires mid-session and we transparently
// fall back to the admin's stashed tokens.
export const onImpersonationEnded = new Set<() => void>()

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    // The parsed JSON body of the error (e.g. a 409 carries the newer page).
    public body?: any,
  ) {
    super(message)
  }
}

// Notifies the app when the session ends (refresh failed). The auth store
// subscribes to redirect to /login without creating an import cycle.
export const onSessionExpired = new Set<() => void>()

// The refresh token rotates on every use and all tabs share one cookie, so
// concurrent refreshes race: the loser presents an already-rotated token. The
// server tolerates that briefly (a reuse-grace window), but past it a replayed
// token is treated as theft and the whole session is revoked. So refreshes are
// coordinated: one in flight per tab (single-flight), and serialized across tabs
// with the Web Locks API where available (each tab then refreshes with the
// cookie its predecessor just rotated).
let refreshInFlight: Promise<boolean> | null = null

async function doRefresh(): Promise<boolean> {
  try {
    const res = await fetch(`${BASE}/auth/refresh`, {
      method: 'POST',
      credentials: 'include',
      headers: { 'Content-Type': 'application/json', ...csrfHeader() },
      body: '{}',
    })
    if (!res.ok) return false
    const data = await res.json()
    setAccessToken(data.access_token)
    return true
  } catch {
    return false
  }
}

// tryRefresh mints a new in-memory access token using the HttpOnly refresh
// cookie (sent automatically). Exported so the auth store can refresh on boot.
export function tryRefresh(): Promise<boolean> {
  if (!refreshInFlight) {
    const locks = typeof navigator !== 'undefined' ? navigator.locks : undefined
    const run = locks ? locks.request('orbit-auth-refresh', doRefresh) : doRefresh()
    refreshInFlight = run.finally(() => {
      refreshInFlight = null
    })
  }
  return refreshInFlight
}

async function rawFetch(path: string, init: RequestInit): Promise<Response> {
  const headers = new Headers(init.headers)
  if (!(init.body instanceof FormData)) headers.set('Content-Type', 'application/json')
  if (accessToken) headers.set('Authorization', `Bearer ${accessToken}`)
  headers.set('X-Orbit-Client', CLIENT_ID)
  const csrf = readCookie('orbit_csrf')
  if (csrf) headers.set('X-CSRF-Token', csrf) // needed by /auth/logout; harmless elsewhere
  // Include cookies so the HttpOnly refresh cookie rides on the /auth/* calls.
  return fetch(`${BASE}${path}`, { ...init, credentials: 'include', headers })
}

export async function api<T = unknown>(path: string, init: RequestInit = {}): Promise<T> {
  let res = await rawFetch(path, init)

  // On 401, recover transparently and retry once.
  if (res.status === 401 && !path.startsWith('/auth/')) {
    if (isImpersonating()) {
      // The impersonation token (no refresh cookie of its own) lapsed. End
      // impersonation back to the admin instead of refreshing into the admin
      // session unexpectedly, then retry and let the app re-sync.
      if (endImpersonation()) {
        res = await rawFetch(path, init)
        onImpersonationEnded.forEach((fn) => fn())
      } else {
        setAccessToken(null)
        onSessionExpired.forEach((fn) => fn())
      }
    } else if (await tryRefresh()) {
      res = await rawFetch(path, init)
    } else {
      setAccessToken(null)
      onSessionExpired.forEach((fn) => fn())
    }
  }

  // A gated action needs fresh re-auth: run the step-up handler, retry once.
  res = await maybeStepUp(res, path, () => rawFetch(path, init))

  if (!res.ok) {
    let msg = res.statusText
    let body: any
    try {
      body = await res.json()
      if (body?.error) msg = body.error
    } catch {
      /* non-JSON error body */
    }
    throw new ApiError(res.status, msg, body)
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

// downloadAuthed fetches a (bearer-protected) endpoint and saves the body as a
// file. Used for PEM/key/CRL downloads that the browser can't link directly.
export async function downloadAuthed(path: string, filename: string): Promise<void> {
  let res = await rawFetch(path, { method: 'GET' })
  if (res.status === 401 && (await tryRefresh())) {
    res = await rawFetch(path, { method: 'GET' })
  }
  res = await maybeStepUp(res, path, () => rawFetch(path, { method: 'GET' }))
  if (!res.ok) throw new ApiError(res.status, res.statusText)
  const blob = await res.blob()
  saveBlob(blob, filename)
}

// downloadAuthedPost POSTs a JSON body to a bearer-protected endpoint and saves
// the (binary/blob) response as a file. Used for the backup export, which needs
// a request body (the optional passphrase) yet streams a file back.
export async function downloadAuthedPost(
  path: string,
  body: unknown,
  filename: string,
): Promise<void> {
  const init: RequestInit = { method: 'POST', body: JSON.stringify(body ?? {}) }
  let res = await rawFetch(path, init)
  if (res.status === 401 && (await tryRefresh())) {
    res = await rawFetch(path, init)
  }
  res = await maybeStepUp(res, path, () => rawFetch(path, init))
  if (!res.ok) {
    let msg = res.statusText
    try {
      const b = await res.json()
      if (b?.error) msg = b.error
    } catch {
      /* non-JSON error body */
    }
    throw new ApiError(res.status, msg)
  }
  saveBlob(await res.blob(), filename)
}

// saveBlob triggers a browser download for an in-memory blob (internal helper).
function saveBlob(blob: Blob, filename: string): void {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  a.remove()
  URL.revokeObjectURL(url)
}

// saveText saves a string as a downloadable file.
export function saveText(text: string, filename: string, mime = 'application/x-pem-file'): void {
  saveBlob(new Blob([text], { type: mime }), filename)
}

// saveBase64 decodes base64 and saves it as a binary file (e.g. PFX).
export function saveBase64(b64: string, filename: string, mime = 'application/x-pkcs12'): void {
  const bytes = Uint8Array.from(atob(b64), (c) => c.charCodeAt(0))
  saveBlob(new Blob([bytes], { type: mime }), filename)
}

import { test, expect, type Page } from '@playwright/test'
import { createHmac } from 'node:crypto'

// Runs against a fresh instance (see the e2e workflow): the bootstrap admin
// signs in with ADMIN_PASSWORD and must choose a new password first. The tests
// run in order and share that new password.
const ADMIN_USER = process.env.E2E_ADMIN_USER || 'admin'
const FIRST_PASSWORD = process.env.E2E_ADMIN_PASSWORD || 'admin123'
const NEW_PASSWORD = 'e2e-harbor-lamp-42'

test.describe.configure({ mode: 'serial' })

async function signIn(page: Page, password: string) {
  await page.goto('/')
  // Redirected to /login. The username field is the webauthn-autofill input.
  await page.locator('input[autocomplete="username webauthn"]').fill(ADMIN_USER)
  await page.locator('input[autocomplete="current-password"]').fill(password)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
}

test('first sign-in forces a new password, then lands on Proxies', async ({ page }) => {
  await signIn(page, FIRST_PASSWORD)
  await expect(page.getByRole('heading', { name: 'Set a new password' })).toBeVisible()
  await page.getByPlaceholder('Current password').fill(FIRST_PASSWORD)
  await page.getByPlaceholder('New password (min 8)').fill(NEW_PASSWORD)
  await page.getByPlaceholder('Confirm new password').fill(NEW_PASSWORD)
  await page.getByRole('button', { name: 'Update password' }).click()
  // The dashboard renders behind the forced-change dialog, so wait for the
  // dialog to close (the change was accepted) before checking it.
  await expect(page.getByRole('heading', { name: 'Set a new password' })).toBeHidden()

  // Lands on Proxies; a fresh instance has none yet.
  await expect(page.getByText('No proxies yet')).toBeVisible()
  await page.getByRole('button', { name: 'Your access' }).click()
  await expect(page.getByRole('heading', { name: 'Your access' })).toBeVisible()
  // The bootstrap admin is a superuser: every server privilege on every server.
  await expect(page.getByText('every server').first()).toBeVisible()
})

test('an admin creates a role limited by scope rules', async ({ page }) => {
  await signIn(page, NEW_PASSWORD)
  await expect(page.getByText('No proxies yet')).toBeVisible()

  await page.getByRole('button', { name: 'Admin', exact: true }).click()
  await page.getByRole('button', { name: /^Roles/ }).click()
  await page.getByRole('button', { name: 'New role' }).click()

  const name = `e2e-staging-${Date.now()}`
  await page.getByPlaceholder('e.g. Staging operators').fill(name)
  // Switch the grant from "All servers" to scope rules, then pick a privilege.
  await page.getByRole('button', { name: /All servers/ }).click()
  await page.locator('textarea').first().fill('env=gmr   service=face*')
  await page.getByRole('button', { name: /Disable \(maintenance\)/ }).click()
  await page.getByRole('button', { name: 'Save role' }).click()

  // Roles are cards: members on one side, access on the other.
  const card = page.locator('.role-card', { hasText: name })
  await expect(card).toBeVisible()
  await expect(card).toContainText('env=gmr service=face*')
  await expect(card).toContainText('Disable (maintenance)')

  // Add the admin as a member from the card, then take it out again.
  await card.locator('select').selectOption({ label: 'admin' })
  await card.getByRole('button', { name: 'Add', exact: true }).click()
  await expect(card.getByTitle('Remove from this role')).toBeVisible()
  await card.getByTitle('Remove from this role').click()
  await expect(card).toContainText('No members yet.')
})

test('an admin creates a user with a temporary password and required two-factor', async ({ page }) => {
  await signIn(page, NEW_PASSWORD)
  await expect(page.getByText('No proxies yet')).toBeVisible()
  await page.getByRole('button', { name: 'Admin', exact: true }).click()

  const name = `e2e-user-${Date.now()}`
  await page.getByRole('button', { name: 'New user' }).click()
  await page.locator('form input').first().fill(name)
  await page.getByText('Require two-factor').click()
  await page.getByRole('button', { name: 'Create user' }).click()

  // The generated password is shown once.
  await expect(page.getByText('it is shown only once')).toBeVisible()
  await expect(page.locator('code.select-all')).toHaveText(/^[a-z0-9]{4}(-[a-z0-9]{4}){3}$/)
  await page.getByRole('button', { name: 'Done' }).click()

  const row = page.getByRole('row', { name: new RegExp(name) })
  await expect(row).toContainText('temporary password')
  await expect(row).toContainText('two-factor not set up')
  await expect(row).toContainText('never')

  // Disable from the row (signs them out), then enable again.
  await row.getByTitle('Disable', { exact: true }).click()
  await page.getByRole('dialog').getByRole('button', { name: 'Disable', exact: true }).click()
  await expect(row).toContainText('disabled')
  await row.getByTitle('Enable', { exact: true }).click()
  await expect(row).toContainText('active')
})

test('a user creates a limited API token and an admin sees it', async ({ page }) => {
  await signIn(page, NEW_PASSWORD)
  await expect(page.getByText('No proxies yet')).toBeVisible()
  await page.goto('/settings')
  await page.getByRole('button', { name: 'New token' }).click()
  const name = `e2e-token-${Date.now()}`
  await page.getByPlaceholder('CI deploy').fill(name)
  await page.getByText('Limit this token').click()
  await page.getByText('See servers', { exact: true }).click()
  await page.locator('textarea').fill('env=gmr')
  await page.getByRole('button', { name: 'Create token' }).click()
  await expect(page.getByText("Copy this token now")).toBeVisible()
  await page.getByRole('button', { name: 'Done' }).click()
  await expect(page.getByText(name).locator('..').locator('..')).toContainText('env=gmr')

  await page.getByRole('button', { name: 'Admin', exact: true }).click()
  await page.locator('.subtab', { hasText: 'API tokens' }).click()
  const row = page.getByRole('row', { name: new RegExp(name) })
  await expect(row).toContainText('See servers')
  await expect(row).toContainText('env=gmr')
})

// RFC 6238 TOTP (SHA-1, 6 digits, 30 s) from a base32 secret.
function totp(secret: string, at = Date.now()): string {
  const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567'
  let bits = ''
  for (const c of secret.replace(/[=\s]/g, '').toUpperCase()) bits += alphabet.indexOf(c).toString(2).padStart(5, '0')
  const key = Buffer.from(bits.match(/.{8}/g)!.map((b) => parseInt(b, 2)))
  const counter = Buffer.alloc(8)
  counter.writeBigUInt64BE(BigInt(Math.floor(at / 1000 / 30)))
  const h = createHmac('sha1', key).update(counter).digest()
  const o = h[h.length - 1] & 0xf
  return String((h.readUInt32BE(o) & 0x7fffffff) % 1_000_000).padStart(6, '0')
}

test('two-factor: set up with the six boxes, sign in with a code, then with a recovery code', async ({ page }) => {
  // An admin creates the user with a known password.
  await signIn(page, NEW_PASSWORD)
  await page.waitForURL(/\/proxies/)
  await page.getByRole('button', { name: 'Admin', exact: true }).click()
  await page.getByRole('button', { name: 'New user' }).click()
  const user = `e2e-otp-${Date.now()}`
  const form = page.getByRole('dialog')
  await form.locator('input').first().fill(user)
  await form.getByText('Generate a temporary password').click() // untick: type one
  await form.locator('input[type=password]').fill('otp-user-pass-42')
  await form.getByText('Require password change on first login').click() // untick
  await form.getByRole('button', { name: 'Create user' }).click()
  await expect(page.getByRole('row', { name: new RegExp(user) })).toBeVisible()
  await page.locator('button.user-btn').click()
  await page.getByRole('button', { name: 'Sign out' }).click()

  // The user sets up two-factor in Settings, in the same session.
  const signInAs = async () => {
    await page.goto('/login')
    await page.locator('input[autocomplete="username webauthn"]').fill(user)
    await page.locator('input[autocomplete="current-password"]').fill('otp-user-pass-42')
    await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  }
  await signInAs()
  await page.waitForURL(/\/proxies/)
  await page.locator('button.user-btn').click()
  await page.getByRole('button', { name: 'Settings' }).click()
  await page.getByRole('button', { name: 'Set up two-factor' }).click()
  const setup = page.getByRole('dialog')
  const secret = (await setup.locator('code').first().innerText()).trim()
  await expect(setup.getByLabel('Digit 6')).toBeVisible()
  await setup.getByLabel('Digit 1').click()
  await page.keyboard.type(totp(secret)) // sent on the sixth digit
  await expect(setup.locator('.select-all')).toHaveCount(10) // shown once the code is accepted
  const codes = (await setup.locator('.select-all').allInnerTexts()).map((c) => c.trim())
  expect(codes).toHaveLength(10)
  await setup.getByRole('button', { name: 'Done' }).click()
  await page.locator('button.user-btn').click()
  await page.getByRole('button', { name: 'Sign out' }).click()

  // Sign-in asks for the code in six boxes, with Back and "Use a recovery code".
  await signInAs()
  await expect(page.getByRole('heading', { name: 'Two-factor authentication' })).toBeVisible()
  await expect(page.getByText('Enter the 6-digit code from your authenticator app.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Back' })).toBeVisible()
  await page.getByLabel('Digit 1').click()
  await page.keyboard.type(totp(secret, Date.now() + 30_000)) // the next step (±1 is accepted); sent on the sixth digit
  await page.waitForURL(/\/proxies/)
  await page.locator('button.user-btn').click()
  await page.getByRole('button', { name: 'Sign out' }).click()

  // And with a recovery code instead.
  await signInAs()
  await page.getByRole('button', { name: 'Use a recovery code' }).click()
  await page.getByPlaceholder('xxxxx-xxxxx').fill(codes[0])
  await page.getByRole('button', { name: 'Verify' }).click()
  await page.waitForURL(/\/proxies/)
})

// Needs a real lb-agent (the lab's test agent): E2E_AGENT_URL, E2E_AGENT_TOKEN
// and optionally E2E_LB_URL, as Orbit reaches them. Skipped otherwise.
const AGENT_URL = process.env.E2E_AGENT_URL
const AGENT_TOKEN = process.env.E2E_AGENT_TOKEN

test('an admin adds a proxy, disables a service everywhere and enables it again', async ({ page }) => {
  test.skip(!AGENT_URL || !AGENT_TOKEN, 'set E2E_AGENT_URL and E2E_AGENT_TOKEN to run against an lb-agent')
  await signIn(page, NEW_PASSWORD)
  await page.waitForURL(/\/proxies/)

  // Admin → Proxies: add it, testing the connection first.
  await page.getByRole('button', { name: 'Admin', exact: true }).click()
  await page.locator('.subtab', { hasText: 'Proxies' }).click()
  await page.getByRole('button', { name: 'Add proxy' }).first().click()
  const dlg = page.getByRole('dialog')
  await dlg.getByPlaceholder('gmr-nginx').fill('e2e-proxy')
  await dlg.getByPlaceholder('http://proxy-gmr:9000').fill(AGENT_URL!)
  await dlg.locator('input[type=password]').fill(AGENT_TOKEN!)
  if (process.env.E2E_LB_URL) await dlg.getByPlaceholder('http://proxy-gmr:80').fill(process.env.E2E_LB_URL)
  await dlg.getByRole('button', { name: 'Test connection' }).click()
  await expect(dlg).toContainText('Connected:')
  await dlg.getByRole('button', { name: 'Add proxy' }).click()

  // Operations. A target's own Disable can't take a backend's last live
  // server out (analytics has only docker108)…
  await page.getByRole('button', { name: /^Services/ }).click()
  const analytics = page.locator('.gcard', { hasText: 'analytics' })
  await analytics.locator('.target').getByRole('button', { name: 'Disable', exact: true }).click()
  let confirm = page.getByRole('dialog')
  await expect(confirm).toContainText('Not from this button')
  await expect(confirm.getByRole('button', { name: 'Disable', exact: true })).toBeDisabled()
  await confirm.getByRole('button', { name: 'Cancel' }).click()

  // …but a group action can, after the warning and the typed name.
  const card = page.locator('.gcard', { hasText: 'facebook' })
  await expect(card.locator('.state')).toHaveText('ACTIVE')
  await card.getByRole('button', { name: 'Disable all' }).click()
  confirm = page.getByRole('dialog')
  await expect(confirm).toContainText('This leaves no servers in rotation')
  const go = confirm.getByRole('button', { name: 'Disable', exact: true })
  await expect(go).toBeDisabled()
  await confirm.locator('.m-typed input').fill('facebook')
  await go.click()
  await expect(card.locator('.state')).toHaveText('MAINT')

  // With the admin's setting on, the target button may take a service's last server out too.
  await page.getByRole('button', { name: 'Admin', exact: true }).click()
  await page.locator('.subtab', { hasText: 'Proxies' }).click()
  await page.locator('.card', { hasText: "A service's last server" }).getByRole('switch').click()
  await expect(page.locator('.toasts')).toContainText("Target buttons may now take a service's last server out")
  await page.getByRole('button', { name: /^Services/ }).click()
  await analytics.locator('.target').getByRole('button', { name: 'Disable', exact: true }).click()
  await page.getByRole('dialog').getByRole('button', { name: 'Disable', exact: true }).click()
  await expect(analytics.locator('.state')).toHaveText('MAINT')
  // Even one server asks first.
  await analytics.locator('.target').getByRole('button', { name: 'Enable', exact: true }).click()
  await expect(page.getByRole('dialog')).toContainText('gmr_docker108_analytics')
  await page.getByRole('dialog').getByRole('button', { name: 'Enable', exact: true }).click()
  await expect(analytics.locator('.state')).toHaveText('ACTIVE')

  // Timed maintenance: needs a reason; the place shows the time left.
  await analytics.locator('.target').getByRole('button', { name: 'Disable', exact: true }).click()
  confirm = page.getByRole('dialog')
  await confirm.getByLabel('For').selectOption({ label: '15 min' })
  const disable = confirm.getByRole('button', { name: 'Disable', exact: true })
  await expect(disable).toBeDisabled()
  await confirm.getByPlaceholder('e.g. deploy 1.4.2, ticket OPS-123').fill('e2e timed')
  await disable.click()
  await expect(page.locator('.toasts')).toContainText('back in rotation at')
  await expect(analytics.locator('.chip')).toContainText('⏱')
  await analytics.locator('.target').getByRole('button', { name: 'Enable', exact: true }).click()
  await page.getByRole('dialog').getByRole('button', { name: 'Enable', exact: true }).click()
  await expect(analytics.locator('.state')).toHaveText('ACTIVE')
  await expect(analytics.locator('.chip')).not.toContainText('⏱')

  await card.getByRole('button', { name: 'Enable all' }).click()
  await page.getByRole('dialog').locator('.m-typed input').fill('facebook')
  await page.getByRole('dialog').getByRole('button', { name: 'Enable', exact: true }).click()
  await expect(card.locator('.state')).toHaveText('ACTIVE')

  // Both changes are in the audit log, named by what they covered; the chain verifies.
  await page.getByRole('button', { name: /^Audit/ }).click()
  await expect(page.locator('table.list')).toContainText('DISABLE')
  await expect(page.locator('table.list')).toContainText('ENABLE')
  await expect(page.locator('table.list')).toContainText('facebook · All envs')
  await page.getByRole('button', { name: 'Verify integrity' }).click()
  await expect(page.getByText(/Chain intact/)).toBeVisible()
})

test('the owner turns on a developer theme; the Konami code shows who built it', async ({ page }) => {
  await signIn(page, NEW_PASSWORD)
  await page.waitForURL(/\/proxies/)
  await page.getByRole('button', { name: 'Owner', exact: true }).click()
  await page.locator('.subtab', { hasText: 'Branding' }).click()
  await page.locator('.dev-master').getByRole('switch').click()
  await page.locator('.theme-card', { hasText: 'Ink' }).click()
  await expect(page.locator('.lock-note.show').first()).toContainText('controlled by the active developer theme')
  await page.screenshot({ path: 'test-results/branding-themes.png', fullPage: true })
  await page.getByRole('button', { name: 'Save branding' }).click()
  await expect(page.locator('.toasts')).toContainText('Branding saved')

  // The sidebar shows the theme's crest and the "UBonito" wordmark; the tab its favicon.
  await expect(page.locator('.side .brand img')).toHaveAttribute('src', /\/dev-themes\/proxy-ubonito-(dark|bone)\.svg/)
  await expect(page.locator('.side .brand-word')).toHaveText('UBonito')
  await expect(page.locator('link[rel=icon]')).toHaveAttribute('href', /\/dev-themes\//)
  await page.screenshot({ path: 'test-results/sidebar-ink.png' })

  // Off again: back to the default crest and the app name.
  await page.locator('.dev-master').getByRole('switch').click()
  await page.getByRole('button', { name: 'Save branding' }).click()
  await expect(page.locator('.side .brand img')).toHaveAttribute('src', '/favicon.svg')
  await expect(page.locator('.side .brand b')).toHaveText('Orbit')

  for (const k of ['ArrowUp', 'ArrowUp', 'ArrowDown', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'ArrowLeft', 'ArrowRight', 'b', 'a']) await page.keyboard.press(k)
  await expect(page.getByText('Engineered by')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByText('Engineered by')).toBeHidden()
})

test('the owner sends server events to Teams and an email list', async ({ page }) => {
  await signIn(page, NEW_PASSWORD)
  await page.waitForURL(/\/proxies/)
  await page.getByRole('button', { name: 'Owner', exact: true }).click()
  await page.locator('.subtab', { hasText: 'Notifications' }).click()
  const teams = page.locator('.card', { hasText: 'Microsoft Teams' })
  await teams.getByRole('switch').click()
  await teams.getByPlaceholder('https://….logic.azure.com/workflows/…').fill('https://example.logic.azure.com/workflows/abc')
  const events = page.locator('.card', { hasText: 'Server events' })
  await events.getByRole('switch', { name: 'A server goes down, and is back' }).click()
  await events.getByRole('switch', { name: 'Changes' }).click()
  await expect(events.getByText('Teams', { exact: true })).toBeVisible()
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await expect(page.locator('.toasts')).toContainText('Notification channels saved')
  // Saved: the secret URL comes back redacted, the events stay on.
  await page.reload()
  await page.locator('.subtab', { hasText: 'Notifications' }).click()
  await expect(page.locator('.card', { hasText: 'Microsoft Teams' }).locator('input').first()).toHaveValue('********')
  const saved = page.locator('.card', { hasText: 'Server events' })
  await expect(saved.getByRole('switch', { name: 'A server goes down, and is back' })).toHaveAttribute('aria-checked', 'true')
  await expect(saved.getByRole('switch', { name: 'Changes' })).toHaveAttribute('aria-checked', 'true')
  await expect(saved.getByRole('switch', { name: 'Drift and failures' })).toHaveAttribute('aria-checked', 'false')
})

test('a change scheduled for later, and a change freeze with its banner', async ({ page }) => {
  test.skip(!AGENT_URL || !AGENT_TOKEN, 'needs the lab agent (added by the proxy test)')
  await signIn(page, NEW_PASSWORD)
  await page.waitForURL(/\/proxies/)
  await page.getByRole('button', { name: /^Services/ }).click()
  const analytics = page.locator('.gcard', { hasText: 'analytics' })

  // Later: needs a reason; then it's listed, and nothing changed yet.
  const card = page.locator('.gcard', { hasText: 'facebook' })
  await card.getByRole('button', { name: 'Disable all' }).click()
  let dlg = page.getByRole('dialog')
  await dlg.getByLabel('Later, at').check()
  await dlg.locator('.m-typed input').fill('facebook')
  const go = dlg.getByRole('button', { name: 'Schedule', exact: true })
  await expect(go).toBeDisabled()
  await dlg.getByPlaceholder('e.g. deploy 1.4.2, ticket OPS-123').fill('e2e night window')
  await go.click()
  await expect(page.locator('.toasts')).toContainText('scheduled for')
  const sched = page.locator('.sched')
  await expect(sched).toContainText('e2e night window')
  await expect(card.locator('.state')).toHaveText('ACTIVE')
  await sched.getByRole('button', { name: 'Cancel' }).click()
  await page.getByRole('dialog').getByRole('button', { name: 'Cancel it' }).click()
  await expect(page.locator('.toasts')).toContainText('Scheduled change cancelled')
  await expect(sched).toBeHidden()

  // A freeze: everyone sees the banner; an admin is told it goes through.
  await page.getByRole('button', { name: 'Admin', exact: true }).click()
  await page.locator('.subtab', { hasText: 'Proxies' }).click()
  const fz = page.locator('.card', { hasText: 'Change freezes' })
  await fz.getByPlaceholder('Black Friday').fill('e2e freeze')
  await fz.getByRole('button', { name: 'Add freeze' }).click()
  await expect(page.locator('.toasts')).toContainText('Change freeze added')
  await expect(page.locator('.imp-banner.freeze')).toContainText('e2e freeze')
  await page.getByRole('button', { name: /^Services/ }).click()
  await analytics.locator('.target').getByRole('button', { name: 'Disable', exact: true }).click()
  dlg = page.getByRole('dialog')
  await expect(dlg).toContainText('You are an admin, so this goes through')
  await dlg.getByRole('button', { name: 'Cancel' }).click()
  await page.getByRole('button', { name: 'Admin', exact: true }).click()
  await page.locator('.subtab', { hasText: 'Proxies' }).click()
  await fz.getByRole('button', { name: 'End it' }).click()
  await page.getByRole('dialog').getByRole('button', { name: 'End it' }).click()
  await expect(page.locator('.imp-banner.freeze')).toBeHidden()
})

test('weights: the owner turns them on; a canary weight on the real nginx, then back', async ({ page }) => {
  test.skip(!AGENT_URL || !AGENT_TOKEN, 'needs the lab agent (added by the proxy test)')
  await signIn(page, NEW_PASSWORD)
  await page.waitForURL(/\/proxies/)
  await page.getByRole('button', { name: 'Owner', exact: true }).click()
  await page.getByRole('switch', { name: 'Weights (canary releases)' }).click()
  await page.getByRole('button', { name: /^Proxies/ }).first().click()
  const be = page.locator('.be', { hasText: 'facebook' })
  if (!(await be.evaluate((el) => el.classList.contains('open')))) await be.locator('.be-row').click()
  await expect(be.locator('.srv-head')).toContainText('Last hour')
  await expect(be.locator('.srv-head')).toContainText('p95')
  // Run a server's health check from the proxy: what was sent, what came back.
  const srvRow = be.locator('.srv', { hasText: 'gmr_web01_facebook' })
  await srvRow.getByRole('button', { name: /^Actions for/ }).click()
  await srvRow.getByRole('button', { name: 'Run health check' }).click()
  const hc = page.getByRole('dialog', { name: 'Health check, run from the proxy' })
  await expect(hc).toContainText('passes')
  await expect(hc).toContainText('HTTP 200')
  await expect(hc.locator('.cmd code')).toContainText('curl -sS -i http://')
  await hc.getByRole('button', { name: 'Close' }).click()
  await be.getByRole('button', { name: 'Weights…' }).click()
  const dlg = page.getByRole('dialog')
  const input = dlg.getByLabel('New weight for gmr_web01_facebook')
  await input.fill('3')
  await expect(dlg).toContainText('75%') // 3 of 3 + 1
  await dlg.getByPlaceholder('e.g. canary 1.5.0').fill('e2e canary')
  await dlg.getByRole('button', { name: 'Apply' }).click()
  await expect(page.locator('.toasts')).toContainText('Weight changed')
  const row = be.locator('.srv', { hasText: 'gmr_web01_facebook' })
  await expect(row.locator('.srv-grid > div').nth(2)).toHaveText('3')
  // Back to 1.
  await be.getByRole('button', { name: 'Weights…' }).click()
  await page.getByRole('dialog').getByLabel('New weight for gmr_web01_facebook').fill('1')
  await page.getByRole('dialog').getByRole('button', { name: 'Apply' }).click()
  await expect(row.locator('.srv-grid > div').nth(2)).toHaveText('1')
  await page.getByRole('button', { name: /^Audit/ }).click()
  await expect(page.locator('table.list')).toContainText('WEIGHT')
})

test('rolling: one server at a time, Next after each, then done', async ({ page }) => {
  test.skip(!AGENT_URL || !AGENT_TOKEN, 'needs the lab agent (added by the proxy test)')
  test.setTimeout(120_000)
  await signIn(page, NEW_PASSWORD)
  await page.waitForURL(/\/proxies/)
  await page.getByRole('button', { name: 'Owner', exact: true }).click()
  await page.getByRole('switch', { name: 'Rolling actions' }).click()
  await page.getByRole('button', { name: /^Services/ }).click()
  const card = page.locator('.gcard', { hasText: 'facebook' })
  await card.getByRole('button', { name: 'Roll…' }).click()
  const dlg = page.getByRole('dialog')
  await dlg.getByPlaceholder('e.g. deploy 1.4.2, ticket OPS-123').fill('e2e rolling')
  await dlg.getByRole('button', { name: /^Start/ }).click()
  await expect(page.locator('.toasts')).toContainText('Rolling facebook')
  const panel = page.locator('.ro')
  for (let i = 0; i < 2; i++) {
    await expect(panel.getByRole('button', { name: 'Next' })).toBeVisible({ timeout: 30_000 })
    await expect(panel).toContainText('deploy it, then Next')
    await expect(card.locator('.target.MAINT')).toHaveCount(1) // only one out at a time
    await panel.getByRole('button', { name: 'Next' }).click()
  }
  await expect(panel.locator('.st')).toHaveText('done', { timeout: 30_000 })
  await expect(card.locator('.state')).toHaveText('ACTIVE')
})

async function signInAsUser(page: Page, user: string, password: string) {
  await page.goto('/')
  await page.locator('input[autocomplete="username webauthn"]').fill(user)
  await page.locator('input[autocomplete="current-password"]').fill(password)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await page.waitForURL(/\/proxies/)
}
async function signOut(page: Page) {
  await page.locator('button.user-btn').click()
  await page.getByRole('button', { name: 'Sign out' }).click()
  await page.waitForURL(/\/login/)
}

test('a viewer asks for maintenance, an approver approves it, the viewer extends it', async ({ page, request }) => {
  test.skip(!AGENT_URL || !AGENT_TOKEN, 'needs the lab agent (added by the proxy test)')
  test.setTimeout(90_000)
  const login = await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })
  const H = { Authorization: `Bearer ${(await login.json()).access_token}` }
  for (const [name, privs] of [['e2e-viewers', ['servers.read']], ['e2e-approvers', ['servers.read', 'maint.approve']]] as const) {
    const role = await (await request.post('/api/v1/roles', { headers: H, data: { name, grants: [{ rules: ['env=gmr'], privileges: privs }] } })).json()
    const user = name === 'e2e-viewers' ? 'vera' : 'abe'
    const res = await request.post('/api/v1/users', { headers: H, data: { username: user, password: `${user}-pass-1234`, role_ids: [role.id], must_change_password: false } })
    expect(res.ok()).toBeTruthy()
  }

  // vera asks.
  await signInAsUser(page, 'vera', 'vera-pass-1234')
  await page.getByRole('button', { name: /^Services/ }).click()
  const analytics = page.locator('.gcard', { hasText: 'analytics' })
  await analytics.getByRole('button', { name: 'Request…' }).click()
  let dlg = page.getByRole('dialog')
  await dlg.getByPlaceholder('e.g. deploy 1.4.2, ticket OPS-123').fill('e2e disk swap')
  await dlg.getByRole('button', { name: 'Send the request' }).click()
  await expect(page.locator('.toasts')).toContainText('Request sent')
  await expect(page.locator('.rq')).toContainText('waiting for an approver')
  await signOut(page)

  // abe approves, letting vera extend by up to 30 min.
  await signInAsUser(page, 'abe', 'abe-pass-1234')
  await expect(page.locator('.count.alert')).toHaveText('1')
  await page.getByRole('button', { name: /^Services/ }).click()
  await page.locator('.rq').getByRole('button', { name: 'Approve' }).click()
  dlg = page.getByRole('dialog')
  await dlg.locator('select').selectOption('30')
  await dlg.getByRole('button', { name: 'Approve' }).click()
  await expect(page.locator('.toasts')).toContainText('Approved')
  await expect(analytics.locator('.state')).toHaveText('MAINT')
  await signOut(page)

  // vera extends within the pre-approval, directly.
  await signInAsUser(page, 'vera', 'vera-pass-1234')
  await page.getByRole('button', { name: /^Services/ }).click()
  await analytics.getByRole('button', { name: 'Extend…' }).click()
  dlg = page.getByRole('dialog')
  await dlg.getByLabel('+30 min').check()
  await dlg.getByRole('button', { name: 'Extend', exact: true }).click()
  await expect(page.locator('.toasts')).toContainText('Extended by 30 min')

  // Back in rotation (as the admin).
  const st = await (await request.get('/api/v1/state', { headers: H })).json()
  const targets = st.proxies.flatMap((p: any) => p.state.backends.flatMap((b: any) => b.servers.filter((s: any) => (s.label || '').includes('analytics')).map((s: any) => ({ proxy: p.name, backend: b.name, server: s.name }))))
  expect((await request.post('/api/v1/operations/ready', { headers: H, data: { targets } })).ok()).toBeTruthy()
})

test('certificates: the lab certificate is replaced through the page (nginx -t + reload)', async ({ page }) => {
  test.skip(!AGENT_URL || !AGENT_TOKEN, 'needs the lab agent (added by the proxy test)')
  // A new certificate for the lab's TLS server (conf.d/tls.conf), made here.
  const { execSync } = await import('node:child_process')
  const { mkdtempSync, readFileSync } = await import('node:fs')
  const { join } = await import('node:path')
  const { tmpdir } = await import('node:os')
  const dir = mkdtempSync(join(tmpdir(), 'cert-'))
  execSync(
    `openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:P-256 -nodes -days 200 -keyout ${dir}/k.pem -out ${dir}/c.pem ` +
      `-subj /CN=lab.local -addext subjectAltName=DNS:lab.local,DNS:proxy-gmr,DNS:proxy-rda -addext extendedKeyUsage=serverAuth`,
    { stdio: 'ignore' },
  )

  await signIn(page, NEW_PASSWORD)
  await page.getByRole('button', { name: 'Certificates' }).click()
  const row = page.locator('tr', { hasText: '/etc/ssl/lab/lab.crt' })
  await expect(row).toContainText('lab.local')
  await expect(row).toContainText('self-signed')
  await row.getByRole('button', { name: 'Replace…' }).click()

  const dlg = page.getByRole('dialog', { name: 'Replace a certificate' })
  await dlg.locator('textarea').nth(0).fill(readFileSync(`${dir}/c.pem`, 'utf8'))
  await dlg.locator('textarea').nth(1).fill(readFileSync(`${dir}/k.pem`, 'utf8'))
  await dlg.getByRole('button', { name: 'Check' }).click()
  await expect(dlg.getByText('What each agent will do')).toBeVisible()
  await expect(dlg).toContainText('nginx -t')
  // Same names, self-signed like the one it replaces: no warnings to accept.
  await expect(dlg.getByText('Go ahead despite the warnings')).toBeHidden()
  await dlg.getByRole('button', { name: /Replace on 1 file/ }).click()
  await expect(dlg).toBeHidden()
  await expect(row).toContainText(/(199|200) days left/)
})

test('on a phone: the sidebar is a drawer, and the app is installable', async ({ page }) => {
  // Installable: the static manifest is served, and the CSP lets the branded blob: one load.
  const manifest = await page.request.get('/manifest.webmanifest')
  expect(manifest.status()).toBe(200)
  expect(manifest.headers()['content-type']).toContain('application/manifest+json')
  expect((await manifest.json()).icons.map((i: { sizes: string }) => i.sizes)).toEqual(['192x192', '512x512'])
  const csp = (await page.request.get('/')).headers()['content-security-policy'] ?? ''
  expect(csp).toContain("manifest-src 'self' blob:")

  await page.setViewportSize({ width: 390, height: 844 })
  await signIn(page, NEW_PASSWORD)
  const menu = page.getByRole('button', { name: 'Open menu' })
  await expect(menu).toBeVisible()
  const sidebar = page.locator('#sidebar')
  await expect(sidebar.getByRole('button', { name: 'Audit' })).toBeHidden()
  await menu.click()
  await expect(page.getByRole('button', { name: 'Close menu' })).toHaveAttribute('aria-expanded', 'true')
  await page.keyboard.press('Escape')
  await expect(sidebar.getByRole('button', { name: 'Audit' })).toBeHidden()
  // Picking a page closes it.
  await menu.click()
  await sidebar.getByRole('button', { name: 'Audit' }).click()
  await expect(page).toHaveURL(/\/audit$/)
  await expect(sidebar.getByRole('button', { name: 'Audit' })).toBeHidden()

  // Desktop: the sidebar is in place, no menu button.
  await page.setViewportSize({ width: 1280, height: 800 })
  await expect(menu).toBeHidden()
  await expect(sidebar.getByRole('button', { name: 'Audit' })).toBeVisible()
})

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
  await page.waitForURL((u) => !u.pathname.startsWith('/login'))
}

test('first sign-in forces a new password, then lands on an empty Today', async ({ page }) => {
  await signIn(page, FIRST_PASSWORD)
  await expect(page.getByRole('heading', { name: 'Set a new password' })).toBeVisible()
  await page.getByPlaceholder('Current password').fill(FIRST_PASSWORD)
  await page.getByPlaceholder('New password (min 8)').fill(NEW_PASSWORD)
  await page.getByPlaceholder('Confirm new password').fill(NEW_PASSWORD)
  await page.getByRole('button', { name: 'Update password' }).click()
  await expect(page.getByRole('heading', { name: 'Set a new password' })).toBeHidden()
  // A fresh instance has no workspaces yet.
  await expect(page.getByRole('heading', { name: 'Welcome to Orbit' })).toBeVisible()
})

test('a School workspace: quick-add a task, see it on the board and in Today', async ({ page }) => {
  await signIn(page, NEW_PASSWORD)
  await page.getByRole('button', { name: 'New workspace' }).first().click()
  await page.getByRole('button', { name: /^School/ }).click()
  const name = `School ${Date.now()}`
  await page.getByPlaceholder('Workspace name').fill(name)
  await page.getByRole('button', { name: 'Create', exact: true }).click()
  await expect(page.getByRole('heading', { name })).toBeVisible()

  // Tasks: the first list (Assignments) opens; quick add parses the details.
  await page.getByRole('link', { name: /^Tasks/ }).first().click()
  await expect(page.getByRole('heading', { name: 'Assignments' })).toBeVisible()
  await page.getByRole('button', { name: 'New Task' }).first().click()
  await page.getByPlaceholder(/New task/).fill('Essay today 23:59 !! #maths')
  await page.keyboard.press('Enter')
  const row = page.locator('.task-row', { hasText: 'Essay' })
  await expect(row).toContainText('#maths')
  await expect(row).toContainText('Today')

  // The same task on the board, in the first column.
  await page.locator('#sidebar').getByRole('button', { name: 'Board', exact: true }).click()
  await expect(page.locator('main').getByText(/Essay/).first()).toBeVisible()

  // And on Today.
  await page.goto('/')
  await expect(page.locator('.task-row', { hasText: 'Essay' })).toBeVisible()
})

test('a page: write in the editor, and it is saved', async ({ page }) => {
  await signIn(page, NEW_PASSWORD)
  await page.locator('.ws-row').first().click()
  await page.getByRole('link', { name: /^Pages/ }).first().click()
  await page.getByRole('button', { name: 'New page' }).first().click()
  await page.getByPlaceholder('Untitled').fill('Biology notes')
  await page.keyboard.press('Enter')
  await page.keyboard.type('Cells divide by mitosis.')
  await expect(page.getByText('Saved')).toBeVisible({ timeout: 10_000 })
  await page.reload()
  await expect(page.getByText('Cells divide by mitosis.')).toBeVisible()
  await expect(page.locator('.side-item', { hasText: 'Biology notes' })).toBeVisible()
})

test('a calendar event is added and shown', async ({ page }) => {
  await signIn(page, NEW_PASSWORD)
  await page.locator('.ws-row').first().click()
  await page.getByRole('link', { name: /^Calendar/ }).nth(1).click()
  await page.getByRole('button', { name: 'New event' }).click()
  await page.getByPlaceholder('Title').fill('Maths class')
  await page.getByRole('button', { name: 'Add', exact: true }).click()
  await expect(page.getByText('Maths class').first()).toBeVisible()
})

test('an admin creates a role with instance privileges', async ({ page }) => {
  await signIn(page, NEW_PASSWORD)
  await page.goto('/admin')
  await page.getByRole('button', { name: /^Roles/ }).click()
  await page.getByRole('button', { name: 'New role' }).click()
  const name = `e2e-auditors-${Date.now()}`
  await page.getByPlaceholder('e.g. Staging operators').fill(name)
  await page.getByRole('button', { name: /View audit log/ }).click()
  await page.getByRole('button', { name: 'Save role' }).click()
  const card = page.locator('.role-card', { hasText: name })
  await expect(card).toBeVisible()
  await expect(card).toContainText('View audit log')
})

test('an admin creates a user with a temporary password and required two-factor', async ({ page }) => {
  await signIn(page, NEW_PASSWORD)
  await page.goto('/admin')

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

test('a user creates a read-only API token and an admin sees it', async ({ page }) => {
  await signIn(page, NEW_PASSWORD)
  await page.goto('/settings')
  await page.getByRole('button', { name: 'New token' }).click()
  const name = `e2e-token-${Date.now()}`
  await page.getByPlaceholder('CI deploy').fill(name)
  await page.getByText('Read-only:').click()
  await page.getByRole('button', { name: 'Create token' }).click()
  await expect(page.getByText('Copy this token now')).toBeVisible()
  await page.getByRole('button', { name: 'Done' }).click()
  await expect(page.getByText(name).locator('..').locator('..')).toContainText('read-only')

  await page.goto('/admin')
  await page.locator('.subtab', { hasText: 'API tokens' }).click()
  await expect(page.getByRole('row', { name: new RegExp(name) })).toContainText('read-only')
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
  await page.goto('/admin')
  await page.getByRole('button', { name: 'New user' }).click()
  const user = `e2e-otp-${Date.now()}`
  const form = page.getByRole('dialog')
  await form.locator('input').first().fill(user)
  await form.getByText('Generate a temporary password').click() // untick: type one
  await form.locator('input[type=password]').fill('otp-user-pass-42')
  await form.getByText('Require password change on first login').click() // untick
  await form.getByRole('button', { name: 'Create user' }).click()
  await expect(page.getByRole('row', { name: new RegExp(user) })).toBeVisible()
  await page.goto('/')
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
  await page.waitForURL((u) => u.pathname === '/')
  await page.goto('/settings')
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
  await page.goto('/')
  await page.locator('button.user-btn').click()
  await page.getByRole('button', { name: 'Sign out' }).click()

  // Sign-in asks for the code in six boxes, with Back and "Use a recovery code".
  await signInAs()
  await expect(page.getByRole('heading', { name: 'Two-factor authentication' })).toBeVisible()
  await expect(page.getByText('Enter the 6-digit code from your authenticator app.')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Back' })).toBeVisible()
  await page.getByLabel('Digit 1').click()
  await page.keyboard.type(totp(secret, Date.now() + 30_000)) // the next step (±1 is accepted); sent on the sixth digit
  await page.waitForURL((u) => u.pathname === '/')
  await page.locator('button.user-btn').click()
  await page.getByRole('button', { name: 'Sign out' }).click()

  // And with a recovery code instead.
  await signInAs()
  await page.getByRole('button', { name: 'Use a recovery code' }).click()
  await page.getByPlaceholder('xxxxx-xxxxx').fill(codes[0])
  await page.getByRole('button', { name: 'Verify' }).click()
  await page.waitForURL((u) => u.pathname === '/')
})

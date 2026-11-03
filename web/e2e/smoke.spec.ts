import { test, expect, type Page } from '@playwright/test'
import { createHmac } from 'node:crypto'
import { strToU8, zipSync } from 'fflate'

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
  // Into the document (once its editor is there), not the title.
  await expect(page.locator('.ProseMirror')).toBeFocused()
  await page.keyboard.type('Cells divide by mitosis.')
  await expect(page.getByText('Saved')).toBeVisible({ timeout: 10_000 })
  await page.reload()
  await expect(page.getByText('Cells divide by mitosis.')).toBeVisible()
  await expect(page.locator('.side-item', { hasText: 'Biology notes' })).toBeVisible()
})

test('a page in Markdown: write the source, read it rendered, tick a box', async ({ page }) => {
  await signIn(page, NEW_PASSWORD)
  await page.locator('.ws-row').first().click()
  await page.getByRole('link', { name: /^Pages/ }).first().click()
  await page.getByRole('button', { name: 'New page' }).first().click()
  await page.getByPlaceholder('Untitled').fill('Packing list')
  await page.getByRole('tab', { name: 'Markdown' }).click()
  const src = page.getByRole('textbox', { name: 'Page (Markdown)' })
  await src.click()
  await page.keyboard.type('## Clothes\n- [ ] Coat')
  await page.keyboard.press('Enter') // continues the task list
  await page.keyboard.type('Boots\n\n> [!TIP]\n> Pack light.')
  await expect(page.getByText('Saved')).toBeVisible({ timeout: 10_000 })
  await page.getByRole('button', { name: 'Done' }).click()
  await expect(page.getByRole('heading', { name: 'Clothes' })).toBeVisible()
  await expect(page.locator('.md-alert-tip')).toContainText('Pack light.')
  await page.locator('.md-view input[data-task="1"]').click()
  await expect(page.getByText('Saved')).toBeVisible({ timeout: 10_000 })
  await page.reload()
  await expect(page.locator('.md-view input[data-task="1"]')).toBeChecked()
  await expect(page.locator('.md-view input[data-task="0"]')).not.toBeChecked()
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

test('quick notes: a checklist note is kept, then turned into tasks', async ({ page }) => {
  await signIn(page, NEW_PASSWORD)
  await page.locator('.ws-row').first().click()
  await page.getByRole('link', { name: /^Notes/ }).first().click()
  // A new School workspace starts with a pinned tip.
  await expect(page.locator('.note-card').first()).toBeVisible()

  await page.getByRole('button', { name: 'New checklist' }).click()
  await page.keyboard.type('Buy milk')
  await page.keyboard.press('Enter')
  await page.keyboard.type('Buy eggs')
  await page.keyboard.press('Escape')
  const card = page.locator('.note-card', { hasText: 'Buy milk' })
  await expect(card).toContainText('Buy eggs')
  await page.reload()
  await expect(card).toBeVisible()

  await card.hover()
  await card.getByRole('button', { name: 'More options' }).click()
  await page.getByRole('button', { name: 'Turn into tasks' }).click()
  await expect(page.getByText('2 tasks added')).toBeVisible()
})

test('quick notes do sums: an answer after "=", names, Tab writes it in', async ({ page }) => {
  await signIn(page, NEW_PASSWORD)
  await page.goto('/notes')
  await page.getByRole('button', { name: 'Take a note…' }).first().click()
  const body = page.getByTestId('note-body').first()
  await body.click()
  await page.keyboard.type('rent = 650\nbills = 120\nrent + bills =')
  await expect(page.locator('.cm-answer')).toHaveText('770')
  await body.press('End')
  await body.press('Tab')
  await expect(body.locator('.cm-line')).toHaveText(['rent = 650', 'bills = 120', 'rent + bills = 770'])
  await expect(page.locator('.cm-answer')).toHaveCount(0)
})

test('notes edit like VS Code: Mod-D picks the next one, Alt-↓ moves a line, Alt-click adds a cursor', async ({ page }) => {
  await signIn(page, NEW_PASSWORD)
  await page.goto('/notes')
  await page.getByRole('button', { name: 'Take a note…' }).first().click()
  const body = page.getByTestId('note-body').first()
  await body.click()
  await page.keyboard.type('apple pie\napple tart\nbanana')
  const lines = body.locator('.cm-line')
  // The first "apple", then the next one: both become "pear".
  await lines.nth(0).click()
  await page.keyboard.press('Home')
  await page.keyboard.press('Shift+End')
  await page.keyboard.press('Shift+ArrowLeft')
  await page.keyboard.press('Shift+ArrowLeft')
  await page.keyboard.press('Shift+ArrowLeft')
  await page.keyboard.press('Shift+ArrowLeft')
  await page.keyboard.press('ControlOrMeta+d')
  await page.keyboard.type('pear')
  await expect(lines).toHaveText(['pear pie', 'pear tart', 'banana'])
  // Alt-↓ moves the line down.
  await page.keyboard.press('Escape')
  await lines.nth(0).click()
  await page.keyboard.press('Alt+ArrowDown')
  await expect(lines).toHaveText(['pear tart', 'pear pie', 'banana'])
  // Alt-click: a cursor at the end of two lines, one typing for both.
  await lines.nth(0).click()
  await page.keyboard.press('End')
  const box = (await lines.nth(2).boundingBox())!
  await page.keyboard.down('Alt')
  await page.mouse.click(box.x + box.width - 2, box.y + box.height / 2)
  await page.keyboard.up('Alt')
  await page.keyboard.press('End')
  await page.keyboard.type('!')
  await expect(lines).toHaveText(['pear tart!', 'pear pie', 'banana!'])
  await page.screenshot({ path: 'test-results/vscode-note.png' })
})

test('quick notes: jot one on Today, find it in Quick Notes, move it into a workspace', async ({ page }) => {
  await signIn(page, NEW_PASSWORD)
  await page.getByText('Jot something down…').click()
  await page.keyboard.type('Buy a birthday card')
  await page.keyboard.press('Escape')
  await expect(page.locator('.mini-note', { hasText: 'Buy a birthday card' })).toBeVisible()

  // ⌥N / Alt+N from anywhere.
  await page.keyboard.press('Alt+KeyN')
  await page.keyboard.type('Renew the passport')
  await page.getByRole('button', { name: 'Done' }).click()

  await page.locator('#sidebar').getByRole('link', { name: 'Quick Notes' }).click()
  const card = page.locator('.note-card', { hasText: 'Buy a birthday card' })
  await expect(card).toBeVisible()
  await expect(page.locator('.note-card', { hasText: 'Renew the passport' })).toBeVisible()
  await card.hover()
  await card.getByRole('button', { name: 'More options' }).click()
  await page.getByRole('button', { name: 'Move to…' }).click()
  await page.getByRole('dialog').locator('.group-row').nth(0).click()
  await expect(card).toBeHidden()
  await expect(page.getByText(/^Moved to /)).toBeVisible()
})

test('quick notes colour their text as an editor would: Markdown, code, or plain — guessed, or chosen', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  await request.post('/api/v1/me/notes', { headers: H, data: { title: 'Compose file', body: 'services:\n  orbit:\n    image: orbit:0.1.0\n    ports:\n      - "8080:8080"' } })
  await request.post('/api/v1/me/notes', { headers: H, data: { title: 'Vault steps', body: '# Add entity to group\n\nGet the id:\n\n```\nvault read -field=id identity/group/name/linux-users\n```' } })

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1280, height: 900 })
  await page.goto('/notes')
  // On the wall: YAML in its colours, Markdown's heading and its code.
  const yaml = page.locator('.note-card', { hasText: 'Compose file' })
  await expect(yaml.getByTestId('card-coloured')).toHaveClass(/code/)
  await expect(yaml.locator('.hljs-attr').first()).toHaveText('services:')
  const md = page.locator('.note-card', { hasText: 'Vault steps' })
  await expect(md.locator('.md-heading')).toHaveText('# Add entity to group')
  await expect(md.locator('.md-codeblock').first()).toBeVisible()

  // Open it: the same colours while writing, and the corner says Markdown.
  await md.click()
  await expect(page.getByTestId('note-lang')).toHaveText('Markdown')
  await expect(page.getByTestId('note-body').locator('.md-heading')).toHaveText('# Add entity to group')
  // Chosen: plain text — no colours, here and on the wall.
  await page.getByTestId('note-lang').click()
  await page.getByTestId('note-lang-menu').getByRole('menuitemradio', { name: 'Plain text' }).click()
  await expect(page.getByTestId('note-lang')).toHaveText('Plain text')
  await expect(page.getByTestId('note-body').locator('.md-heading')).toHaveCount(0)
  await page.getByRole('button', { name: 'Close' }).click()
  await expect(md.getByTestId('card-coloured')).toHaveCount(0)
  const notes = (await (await request.get('/api/v1/me/notes', { headers: H })).json()) as { title: string; lang: string }[]
  expect(notes.find((n) => n.title === 'Vault steps')?.lang).toBe('text')

  // Typed: a few lines of shell become Bash after a moment.
  await page.getByText('Take a note…').click()
  await page.keyboard.type('docker compose pull\ndocker compose up -d\ndocker logs -f orbit')
  await expect(page.getByTestId('note-lang')).toHaveText('Bash')
})

test('share target: a link shared from another app becomes a Quick Note (after signing in)', async ({ page }) => {
  // Not signed in: the sign-in keeps what was shared.
  await page.goto('/share?title=Trail%20map&text=For%20Saturday&url=https%3A%2F%2Fexample.com%2Ftrail')
  await page.locator('input[autocomplete="username webauthn"]').fill(ADMIN_USER)
  await page.locator('input[autocomplete="current-password"]').fill(NEW_PASSWORD)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await expect(page.getByText('Saved to Quick Notes')).toBeVisible()
  await expect(page).toHaveURL(/\/notes\?note=/)
  await expect(page.locator('.note-card', { hasText: 'Trail map' })).toContainText('https://example.com/trail')
})

test('a reminder on a note, and quiet hours in the notification settings', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const note = await (await request.post('/api/v1/me/notes', { headers: H, data: { title: 'Call the dentist', body: 'Ask about Friday' } })).json()

  await signIn(page, NEW_PASSWORD)
  await page.goto(`/notes?note=${note.id}`)
  await page.getByTestId('note-remind').click()
  await page.getByTestId('remind-picker').getByRole('button', { name: /^Tomorrow/ }).click()
  await expect(page.getByTestId('note-reminder')).toContainText('Reminder:')
  const saved = await (await request.get('/api/v1/me/notes', { headers: H })).json()
  const at = new Date(saved.find((n: { id: string }) => n.id === note.id).remind_at)
  expect(at.getTime()).toBeGreaterThan(Date.now())
  await page.getByRole('button', { name: 'No reminder' }).click()
  await expect(page.getByTestId('note-reminder')).toHaveCount(0)
  await page.keyboard.press('Escape')

  await page.goto('/settings')
  const quiet = page.getByTestId('quiet-hours')
  await quiet.getByRole('switch', { name: 'Quiet hours' }).click()
  await expect(quiet.locator('input[type="time"]').first()).toHaveValue('22:00')
  const prefs = await (await request.get('/api/v1/me/prefs', { headers: H })).json()
  expect([prefs.quiet_from, prefs.quiet_to]).toEqual(['22:00', '07:00'])
  await quiet.getByRole('switch', { name: 'Quiet hours' }).click()
})

test('timeline and charts show the tasks, and the filter narrows them', async ({ page }) => {
  await signIn(page, NEW_PASSWORD)
  await page.locator('.ws-row').first().click()
  await page.waitForURL(/\/w\/[^/]+$/)
  const ws = new URL(page.url()).pathname

  // "Essay" (due today) is a one-day bar on the timeline.
  await page.goto(ws + '/timeline')
  await expect(page.locator('.tl-row', { hasText: 'Essay' })).toBeVisible()
  await expect(page.locator('.bar').first()).toBeVisible()

  await page.goto(ws + '/charts')
  await expect(page.getByRole('heading', { name: 'Status' })).toBeVisible()

  // Tasks → All: "Essay" is medium priority; filtering by High hides it.
  await page.goto(ws + '/tasks/all')
  const essay = page.locator('.task-row', { hasText: 'Essay' })
  await expect(essay).toBeVisible()
  await page.getByRole('button', { name: /^Filter/ }).click()
  await page.locator('.popover .chip', { hasText: 'High' }).click()
  await expect(essay).toBeHidden()
  await page.locator('.popover .chip', { hasText: 'Medium' }).click()
  await expect(essay).toBeVisible()
  await page.getByRole('button', { name: 'Clear filter' }).last().click()
})

// Git. With E2E_GIT_URL and E2E_GIT_TOKEN (a Gitea the server may reach —
// named in its GIT_SERVERS — whose account has a repository named
// E2E_GIT_REPO, default "website", with open issues) the whole round trip;
// without them, that a server Orbit can't reach is refused.
const GIT_URL = process.env.E2E_GIT_URL
const GIT_TOKEN = process.env.E2E_GIT_TOKEN
const GIT_REPO = process.env.E2E_GIT_REPO || 'website'

test('git: connect an account, make a list from a repository, a new task opens an issue', async ({ page }) => {
  await signIn(page, NEW_PASSWORD)
  await page.goto('/settings#git')
  const git = page.locator('#git')
  await expect(git.getByRole('heading', { name: 'Git' })).toBeVisible()
  if (!GIT_URL || !GIT_TOKEN) {
    await git.locator('input[type="url"]').fill('https://gitea.invalid')
    await git.locator('input[type="password"]').fill('not-a-token')
    await git.getByRole('button', { name: 'Connect' }).click()
    await expect(page.getByText(/couldn.t reach https:\/\/gitea\.invalid/)).toBeVisible()
    return
  }
  await git.locator('input[type="url"]').fill(GIT_URL)
  await git.locator('input[type="password"]').fill(GIT_TOKEN)
  await git.getByRole('button', { name: 'Connect' }).click()
  await expect(git.locator('.group-row', { hasText: new URL(GIT_URL).host })).toBeVisible()

  // A list from the repository: its open issues come in, with their numbers.
  await page.goto('/')
  await page.locator('.ws-row').first().click()
  await page.waitForURL(/\/w\/[^/]+$/)
  await page.goto(new URL(page.url()).pathname + '/tasks')
  await page.getByRole('button', { name: 'Add List' }).first().click()
  await page.getByRole('button', { name: 'From a Git repository…' }).click()
  await page.getByPlaceholder('Search repositories').fill(GIT_REPO)
  await page.getByRole('button', { name: new RegExp(`/${GIT_REPO}`) }).first().click()
  await page.getByRole('button', { name: 'Create list' }).click()
  await expect(page.getByRole('heading', { name: GIT_REPO })).toBeVisible()
  await expect(page.locator('.task-row .git-badge').first()).toBeVisible()

  // A new task becomes an issue: it gets a number.
  await page.getByRole('button', { name: 'New Task' }).first().click()
  const title = `From Orbit ${Date.now()}`
  await page.getByPlaceholder(/New task/).fill(title)
  await page.keyboard.press('Enter')
  const badge = page.locator('.task-row', { hasText: title }).locator('.git-badge')
  await expect(badge).toHaveText(/#\d+/)

  // A comment on it goes to the issue.
  const number = (await badge.innerText()).replace(/\D/g, '')
  await page.locator('.task-row', { hasText: title }).click()
  await page.getByTestId('comment-box').fill('Hello from **Orbit**')
  await page.getByRole('button', { name: 'Send', exact: true }).click()
  await expect(page.getByTestId('comments')).toContainText('Hello from Orbit')
  const r = await page.request.get(`${GIT_URL}/api/v1/repos/${process.env.E2E_GIT_OWNER || 'ana'}/${GIT_REPO}/issues/${number}/comments`, { headers: { Authorization: `token ${GIT_TOKEN}` } })
  const onGit = (await r.json()) as { body: string }[]
  expect(onGit.some((c) => c.body === 'Hello from **Orbit**')).toBeTruthy()

  // A milestone picked here becomes the issue's (and shows on the row).
  const owner = process.env.E2E_GIT_OWNER || 'ana'
  const gtok = { Authorization: `token ${GIT_TOKEN}` }
  const msTitle = `Sprint ${Date.now()}`
  const ms = await (await page.request.post(`${GIT_URL}/api/v1/repos/${owner}/${GIT_REPO}/milestones`, { headers: gtok, data: { title: msTitle } })).json()
  await page.keyboard.press('Escape')
  await page.locator('.task-row', { hasText: title }).click()
  await page.getByTestId('milestone').selectOption(String(ms.id))
  await expect
    .poll(async () => (await (await page.request.get(`${GIT_URL}/api/v1/repos/${owner}/${GIT_REPO}/issues/${number}`, { headers: gtok })).json()).milestone?.title)
    .toBe(msTitle)
  await page.keyboard.press('Escape')
  await expect(page.locator('.task-row', { hasText: title }).locator('.milestone')).toHaveText(msTitle)

  // Instant updates: the account owns the repository, so Orbit adds the webhook itself.
  await page.getByRole('button', { name: 'Edit list' }).click()
  await page.getByTestId('git-instant').getByRole('button', { name: 'Turn on' }).click()
  await expect(page.getByText('Instant updates on')).toBeVisible()
  await expect(page.getByTestId('git-instant')).toContainText('waiting for the first change')
  const hooks = (await (await page.request.get(`${GIT_URL}/api/v1/repos/${owner}/${GIT_REPO}/hooks`, { headers: gtok })).json()) as { id: number; config: { url: string } }[]
  const ours = hooks.filter((h) => h.config.url.includes('/api/v1/git/hook/'))
  expect(ours.length).toBe(1)
  await page.getByTestId('git-instant').getByRole('button', { name: 'Turn off' }).click()
  await expect(page.getByTestId('git-instant')).toContainText('Off')
  const after = (await (await page.request.get(`${GIT_URL}/api/v1/repos/${owner}/${GIT_REPO}/hooks`, { headers: gtok })).json()) as { id: number }[]
  expect(after.some((h) => h.id === ours[0].id)).toBeFalsy()
  await page.keyboard.press('Escape')

  // Someone without a git account can't change it (nothing goes as the admin).
  const H = async (u: string, p: string) => ({ Authorization: `Bearer ${(await (await page.request.post('/api/v1/auth/login', { data: { username: u, password: p } })).json()).access_token}` })
  const admin = await H(ADMIN_USER, NEW_PASSWORD)
  await page.request.post('/api/v1/users', { headers: admin, data: { username: 'nogit', password: 'nogit-e2e-password-42' } })
  const wsId = new URL(page.url()).pathname.split('/')[2]
  await page.request.post(`/api/v1/workspaces/${wsId}/members`, { headers: admin, data: { user: 'nogit', role: 'editor' } })
  const nogit = await H('nogit', 'nogit-e2e-password-42')
  const tasks = (await (await page.request.get(`/api/v1/workspaces/${wsId}/tasks`, { headers: nogit })).json()) as { id: string; title: string }[]
  const linked = tasks.find((x) => x.title === title)!
  const refused = await page.request.put(`/api/v1/tasks/${linked.id}`, { headers: nogit, data: { title: 'Renamed by nogit' } })
  expect(refused.status()).toBe(403)
  expect((await refused.json()).git_read_only).toBeTruthy()
})

test('git: sign in with the server (OAuth) instead of a token', async ({ page, request }) => {
  test.skip(!GIT_URL || !GIT_TOKEN, 'needs a test Gitea')
  const G = { Authorization: `token ${GIT_TOKEN}` }
  const user = 'e2e-signin'
  const pass = 'E2e-signin-pass-42!'
  // A person on the server (made once: gitea admin user create … e2e-signin),
  // and Orbit registered there as an application.
  const base = process.env.E2E_BASE_URL || 'http://orbit-e2e:8080'
  const app = await (await request.post(`${GIT_URL}/api/v1/user/applications/oauth2`, { headers: G, data: { name: `Orbit e2e ${Date.now()}`, redirect_uris: [`${base}/api/v1/git/oauth/callback`], confidential_client: true } })).json()
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const r = await request.put('/api/v1/git/oauth-apps', { headers: { Authorization: `Bearer ${login.access_token}` }, data: { apps: [{ base_url: GIT_URL, client_id: app.client_id, client_secret: app.client_secret }] } })
  expect(r.ok()).toBeTruthy()

  await signIn(page, NEW_PASSWORD)
  await page.goto('/settings#git')
  await page.getByTestId('git-sign-in').getByRole('button').first().click()
  // On the server: its own sign-in, then approving Orbit.
  await page.locator('#user_name').fill(user)
  await page.locator('#password').fill(pass)
  await page.getByRole('button', { name: 'Sign In', exact: true }).click()
  await page.getByRole('button', { name: /Authorize Application/i }).click()
  await page.waitForURL(/\/settings/)
  await expect(page.locator('#git')).toContainText(user)
  await expect(page.locator('#git')).toContainText('signed in')
})

test('task notes: written in Markdown, read rendered, a box ticks, the row shows plain text', async ({ page }) => {
  await signIn(page, NEW_PASSWORD)
  await page.locator('.ws-row').first().click()
  await page.waitForURL(/\/w\/[^/]+$/)
  await page.goto(new URL(page.url()).pathname + '/tasks/all')
  const row = page.locator('.task-row', { hasText: 'Essay' })
  await row.hover()
  await row.getByTitle('Details').click()
  await page.getByTestId('task-notes').click()
  await page.keyboard.insertText('**Bring** the [rubric](https://example.org)\n\n- [ ] outline\n- [ ] draft')
  await page.locator('#task-title').click() // leave the notes: they save and render
  const view = page.getByTestId('task-notes-view')
  await expect(view.locator('strong')).toHaveText('Bring')
  await expect(view.getByRole('link', { name: 'rubric' })).toHaveAttribute('href', 'https://example.org')
  await view.locator('input[type=checkbox]').first().check()
  await expect(view.locator('input[type=checkbox]').first()).toBeChecked()
  // Clicking the text edits the source, with the tick in it.
  await view.locator('p').first().click()
  await expect(page.getByTestId('task-notes')).toContainText('- [x] outline')
  await page.keyboard.press('Escape')
  await page.keyboard.press('Escape')
  await expect(row).toContainText('Bring the rubric')
})

test('attachments: an image on a task shows as a thumbnail, and goes when removed', async ({ page }) => {
  await signIn(page, NEW_PASSWORD)
  await page.locator('.ws-row').first().click()
  await page.waitForURL(/\/w\/[^/]+$/)
  await page.goto(new URL(page.url()).pathname + '/tasks/all')
  const row = page.locator('.task-row', { hasText: 'Essay' })
  await row.hover()
  await row.getByTitle('Details').click()
  // A 1×1 PNG, attached through the comment box's clip: it shows in the activity.
  const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==', 'base64')
  await page.getByTestId('comment-files').setInputFiles({ name: 'diagram.png', mimeType: 'image/png', buffer: png })
  const file = page.getByTestId('activity-file').filter({ hasText: 'diagram.png' })
  await expect(file).toContainText('attached a file')
  await expect(file.locator('img.file-thumb')).toHaveAttribute('src', /^blob:/)
  await file.getByTestId('activity-file-remove').click()
  await expect(file).toHaveCount(0)
})

test('courses and grades: a course with a weekly class, a grade, a goal and what is needed', async ({ page }) => {
  await signIn(page, NEW_PASSWORD)
  await page.locator('.ws-row').first().click()
  await page.waitForURL(/\/w\/[^/]+$/)
  const base = new URL(page.url()).pathname
  await page.locator('#sidebar').getByRole('button', { name: 'Courses', exact: true }).click()
  await page.getByRole('button', { name: 'New course' }).first().click()
  await page.getByPlaceholder('Maths', { exact: true }).fill('Maths')
  await page.getByPlaceholder('MAT', { exact: true }).fill('MAT')
  await page.getByRole('button', { name: 'Add a class' }).click()
  await expect(page.getByTestId('class-row')).toHaveCount(1)
  await page.getByTestId('course-save').click()
  // The class is on the timetable; the card says so.
  await expect(page.locator('.tt-block', { hasText: 'MAT' })).toBeVisible()
  await expect(page.locator('.course', { hasText: 'Maths' })).toContainText('1 class a week')

  await page.goto(base + '/grades')
  const maths = page.getByTestId('grades-Maths')
  await maths.getByPlaceholder('Test 1').fill('Test 1')
  await maths.getByPlaceholder('Mark').fill('15')
  await maths.getByRole('button', { name: 'Add a grade' }).click()
  await expect(maths.locator('.avg')).toHaveText('15')
  await expect(page.getByTestId('overall')).toHaveText('15')
  // Aiming at 16 with a next mark as heavy as this one: 17 is needed.
  await maths.getByLabel('Goal').fill('16')
  await maths.getByLabel('Goal').press('Enter')
  await expect(maths.getByTestId('need')).toHaveText('You need 17/20 on it to reach 16.')
  await page.reload()
  await expect(page.getByTestId('grades-Maths').getByLabel('Goal')).toHaveValue('16')
})

test('invite links: an owner makes one; a newcomer opens it, makes an account and is in', async ({ page, browser }) => {
  await signIn(page, NEW_PASSWORD)
  const row = page.locator('.ws-row').first()
  const name = (await row.innerText()).trim().split('\n')[0]
  await row.hover()
  await row.locator('.more').click()
  await page.getByRole('button', { name: 'Members', exact: true }).click()
  await page.getByRole('button', { name: 'Make a link' }).click()
  const url = await page.getByTestId('invite-url').inputValue()
  expect(url).toMatch(/\/join\/[\w-]{30,}$/)

  // Someone with no account, in a browser of their own.
  const other = await browser.newContext()
  const p = await other.newPage()
  await p.goto(url)
  await expect(p.getByRole('heading', { name: `Join ${name}` })).toBeVisible()
  await expect(p.getByText(/invited you to join as editor/)).toBeVisible()
  await p.getByRole('button', { name: 'Create an account' }).click()
  await p.getByLabel('Your name').fill('Rui')
  await p.getByLabel('Username').fill('rui')
  await p.getByLabel('Password').fill('rui-e2e-password-42')
  await p.getByRole('button', { name: 'Create account and join' }).click()
  await p.waitForURL(/\/w\/[^/]+$/)
  await expect(p.getByRole('heading', { name })).toBeVisible()

  // The link was for one person: it's spent now.
  await p.goto(url)
  await expect(p.getByRole('heading', { name: /expired, was revoked or was used up/ })).toBeVisible()
  await other.close()

  // The owner is told who joined.
  await page.keyboard.press('Escape')
  const bell = page.locator('#sidebar').getByRole('button', { name: /^Notifications/ })
  await expect(bell).toContainText('1')
  await bell.click()
  await page.getByRole('button', { name: /Rui joined/ }).click()
  await expect(bell).not.toContainText('1')
})

test('notifications: a task someone gives you rings the bell and opens', async ({ page, request }) => {
  // Another member (made through the API) gives the admin a task.
  const json = async (r: { json: () => Promise<any> }) => r.json()
  const login = async (username: string, password: string) =>
    (await json(await request.post('/api/v1/auth/login', { data: { username, password } }))).access_token as string
  const admin = await login(ADMIN_USER, NEW_PASSWORD)
  const auth = (tok: string) => ({ Authorization: `Bearer ${tok}` })
  await request.post('/api/v1/users', { headers: auth(admin), data: { username: 'ana', display_name: 'Ana', password: 'ana-e2e-password-42' } })
  const me = await json(await request.get('/api/v1/auth/me', { headers: auth(admin) }))
  const ws = (await json(await request.get('/api/v1/workspaces', { headers: auth(admin) })))[0]
  await request.post(`/api/v1/workspaces/${ws.id}/members`, { headers: auth(admin), data: { user: 'ana', role: 'editor' } })
  const ana = await login('ana', 'ana-e2e-password-42')
  const r = await request.post(`/api/v1/workspaces/${ws.id}/tasks`, { headers: auth(ana), data: { title: 'Water the plants', assignee_id: (me.user ?? me).id } })
  expect(r.ok()).toBeTruthy()

  await signIn(page, NEW_PASSWORD)
  const bell = page.locator('#sidebar').getByRole('button', { name: /^Notifications/ })
  await expect(bell).toContainText('1')
  await bell.click()
  await page.getByRole('button', { name: /Ana assigned you a task/ }).click()
  await expect(page.locator('#task-title')).toHaveValue('Water the plants')
  // Several people: add Ana as well.
  const who = page.getByTestId('assignees')
  await expect(who.locator('.person')).toHaveCount(1)
  await who.getByLabel('Assign someone').click()
  await page.getByRole('menuitem', { name: 'Ana' }).click()
  await expect(who.locator('.person')).toHaveCount(2)
  // A comment, mentioning Ana through the suggestions.
  const box = page.getByTestId('comment-box')
  await box.fill('Thanks! ')
  await box.press('End')
  await box.pressSequentially('@an')
  await page.getByRole('option', { name: /Ana/ }).click()
  await box.pressSequentially('will water them **tomorrow**')
  await page.getByRole('button', { name: 'Send', exact: true }).click()
  const comments = page.getByTestId('comments')
  await expect(comments.locator('strong')).toHaveText('tomorrow')
  await expect(comments).toContainText('Thanks! @ana will water them')
  await expect(box).toHaveValue('')
  await page.keyboard.press('Escape')
  await expect(bell).not.toContainText('1')

  // The daily summary is a setting that sticks.
  await page.goto('/settings')
  await page.getByRole('switch', { name: 'Daily summary', exact: true }).click()
  await expect(page.locator('#notifications input[type="time"]')).toHaveValue('08:00')
  await page.reload()
  await expect(page.locator('#notifications input[type="time"]')).toBeVisible()
})

test('the owner turns on a developer theme; the Konami code shows who built it', async ({ page }) => {
  await signIn(page, NEW_PASSWORD)
  await page.goto('/owner')
  await page.locator('.subtab', { hasText: 'Branding' }).click()
  await page.locator('.dev-master').getByRole('switch').click()
  await page.locator('.theme-card', { hasText: 'Ink' }).click()
  await expect(page.locator('.lock-note.show').first()).toContainText('active developer theme')
  await page.getByRole('button', { name: 'Save branding' }).click()
  await expect(page.getByText('Branding saved')).toBeVisible()

  // The sidebar shows the theme's crest and the "UBonito" wordmark; the tab its favicon.
  await expect(page.locator('#sidebar .brand img')).toHaveAttribute('src', /\/dev-themes\/crest-(dark|bone)\.svg/)
  await expect(page.locator('#sidebar .brand-word')).toHaveText('UBonito')
  await expect(page.locator('link[rel=icon]')).toHaveAttribute('href', /\/dev-themes\//)

  // Off again: back to Orbit's icon and name.
  await page.locator('.dev-master').getByRole('switch').click()
  await page.getByRole('button', { name: 'Save branding' }).click()
  await expect(page.locator('#sidebar .brand img')).toHaveAttribute('src', '/favicon.svg')
  await expect(page.locator('#sidebar .brand b')).toHaveText('Orbit')

  for (const k of ['ArrowUp', 'ArrowUp', 'ArrowDown', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'ArrowLeft', 'ArrowRight', 'b', 'a']) await page.keyboard.press(k)
  await expect(page.getByText('Engineered by')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByText('Engineered by')).toBeHidden()
})

test('Forest Green for the instance; a person picks their own theme; sign-in shows the instance\'s', async ({ page }) => {
  await signIn(page, NEW_PASSWORD)
  await page.goto('/owner')
  await page.locator('.subtab', { hasText: 'Branding' }).click()
  await page.locator('.dev-master').getByRole('switch').click()
  await page.locator('.theme-card', { hasText: 'Forest Green' }).click()
  await page.locator('.src-row', { hasText: 'Let each person choose' }).getByRole('switch').click()
  await page.getByRole('button', { name: 'Save branding' }).click()
  await expect(page.getByText('Branding saved')).toBeVisible()
  await expect(page.locator('link[rel=icon]')).toHaveAttribute('href', '/dev-themes/crest-green.svg')
  await expect(page.locator('link[rel=apple-touch-icon]')).toHaveAttribute('href', '/dev-themes/crest-green-touch-180.png')

  // Settings → Appearance: this person picks Gold Leaf for themselves.
  await page.goto('/settings')
  await page.getByRole('radio', { name: 'Gold Leaf' }).click()
  await expect(page.locator('link[rel=icon]')).toHaveAttribute('href', '/dev-themes/crest-gold.svg')
  await page.reload()
  await expect(page.locator('link[rel=icon]')).toHaveAttribute('href', '/dev-themes/crest-gold.svg')

  // Signed out, the sign-in page is the instance's: Forest Green, on its ground.
  await page.locator('button.user-btn').click()
  await page.getByRole('button', { name: 'Sign out' }).click()
  await page.waitForURL(/\/login/)
  await expect(page.locator('.login.themed')).toBeVisible()
  await expect(page.locator('link[rel=icon]')).toHaveAttribute('href', '/dev-themes/crest-green.svg')

  // Back to no theme for the tests after this one.
  await signIn(page, NEW_PASSWORD)
  await page.goto('/settings')
  await page.getByRole('radio', { name: /As the instance/ }).click()
  await page.goto('/owner')
  await page.locator('.subtab', { hasText: 'Branding' }).click()
  await page.locator('.dev-master').getByRole('switch').click()
  await page.getByRole('button', { name: 'Save branding' }).click()
  await expect(page.getByText('Branding saved')).toBeVisible()
})

test('calendar: the small month, calendars to hide, and a task dragged onto a day', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = (await (await request.get('/api/v1/workspaces', { headers: H })).json())[0]
  const r = await request.post(`/api/v1/workspaces/${ws.id}/tasks`, { headers: H, data: { title: 'Plan me' } })
  expect(r.ok()).toBeTruthy()

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1500, height: 900 })
  await page.goto('/calendar')
  // The left pane: a small month and the workspaces, ticked.
  const mine = page.getByRole('checkbox', { name: ws.name, exact: true })
  await expect(mine).toHaveAttribute('aria-checked', 'true')
  await mine.click()
  await expect(mine).toHaveAttribute('aria-checked', 'false')
  await mine.click()

  // The right pane: Tasks; drag "Plan me" onto the 15th.
  await page.getByTestId('toggle-pane').click()
  await page.getByRole('tab', { name: 'Tasks' }).click()
  const task = page.getByTestId('pane-tasks').locator('.task', { hasText: 'Plan me' })
  await expect(task).toBeVisible()
  const now = new Date()
  const day = `${now.getFullYear()}-${String(now.getMonth() + 1).padStart(2, '0')}-15`
  await task.dragTo(page.locator(`.mg-cell[data-day="${day}"]`))
  await expect(page.getByText(/^Planned for/)).toBeVisible()
  await expect(page.locator(`.mg-cell[data-day="${day}"]`)).toContainText('Plan me')
})

test('calendar Week view: an event drags to another day and time, its bottom edge makes it longer', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = (await (await request.get('/api/v1/workspaces', { headers: H })).json())[0]
  const tz = Intl.DateTimeFormat().resolvedOptions().timeZone
  // Wednesday 9 December 2026, 10:00 – 11:00 in the browser's zone.
  const at = (h: number, d = 9) => new Date(2026, 11, d, h).toISOString()
  const ev = await (await request.post(`/api/v1/workspaces/${ws.id}/events`, { headers: H, data: { title: 'Design review', starts_at: at(10), ends_at: at(11), tz } })).json()

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1500, height: 900 })
  await page.goto(`/w/${ws.id}/calendar?date=2026-12-10`)
  await page.getByRole('tab', { name: 'Week' }).click()
  const block = page.locator('.tg-ev', { hasText: 'Design review' })
  // The grid opens scrolled to the working day: measure after that.
  await page.waitForFunction(() => (document.querySelector('.tg-scroll')?.scrollTop ?? 0) > 0)
  await expect(block).toBeVisible()
  const b = (await block.boundingBox())!
  const colW = (await page.locator('.tg-col').first().boundingBox())!.width
  // One day right, two hours (2 × 48 px) down.
  await page.mouse.move(b.x + b.width / 2, b.y + 10)
  await page.mouse.down()
  await page.mouse.move(b.x + b.width / 2 + colW / 2, b.y + 60, { steps: 5 })
  await page.mouse.move(b.x + b.width / 2 + colW, b.y + 106, { steps: 5 })
  await page.mouse.up()
  await expect.poll(async () => (await (await request.get(`/api/v1/events/${ev.id}`, { headers: H })).json()).starts_at).toBe(at(12, 10).replace('.000Z', 'Z'))

  // The bottom edge, one hour down.
  const b2 = (await block.boundingBox())!
  await page.mouse.move(b2.x + b2.width / 2, b2.y + b2.height - 2)
  await page.mouse.down()
  await page.mouse.move(b2.x + b2.width / 2, b2.y + b2.height + 20, { steps: 4 })
  await page.mouse.move(b2.x + b2.width / 2, b2.y + b2.height + 46, { steps: 4 })
  await page.mouse.up()
  await expect.poll(async () => (await (await request.get(`/api/v1/events/${ev.id}`, { headers: H })).json()).ends_at).toBe(at(14, 10).replace('.000Z', 'Z'))
  // A drag doesn't open the event.
  await expect(page.getByTestId('event-tz')).toHaveCount(0)
  await request.delete(`/api/v1/events/${ev.id}`, { headers: H })
})

test('calendar: a repeating event changed from one occurrence on, and one occurrence deleted', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Team', kind: 'work', modules: ['home', 'calendar'] } })).json()
  const tz = Intl.DateTimeFormat().resolvedOptions().timeZone
  // Wednesdays 10:00 from 2 December 2026, six times.
  const at = (d: number, h = 10) => new Date(2026, 11, d, h).toISOString()
  await request.post(`/api/v1/workspaces/${ws.id}/events`, { headers: H, data: { title: 'Weekly sync', starts_at: at(2), ends_at: at(2, 11), tz, rrule: 'FREQ=WEEKLY;BYDAY=WE;COUNT=6' } })
  const titles = async () => {
    const c = await (await request.get(`/api/v1/workspaces/${ws.id}/calendar?from=2026-12-01T00:00:00Z&to=2027-01-31T00:00:00Z`, { headers: H })).json()
    return c.events.map((e: { title: string; start: string }) => `${new Date(e.start).getDate()} ${e.title}`)
  }

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1500, height: 900 })
  await page.goto(`/w/${ws.id}/calendar?date=2026-12-16`)
  await page.getByRole('tab', { name: 'Week' }).click()
  await page.locator('.tg-ev', { hasText: 'Weekly sync' }).click()
  // The sheet shows the occurrence opened, not the series' first.
  await expect(page.locator('input[type="datetime-local"]').first()).toHaveValue('2026-12-16T10:00')
  await page.getByPlaceholder('Title').fill('Planning')
  await page.getByRole('button', { name: 'Save', exact: true }).click()
  await page.getByTestId('scope-following').click()
  await expect.poll(titles).toEqual(['2 Weekly sync', '9 Weekly sync', '16 Planning', '23 Planning', '30 Planning', '6 Planning'])

  // One of the new ones deleted alone.
  await page.goto(`/w/${ws.id}/calendar?date=2026-12-23`)
  await page.getByRole('tab', { name: 'Week' }).click()
  await page.locator('.tg-ev', { hasText: 'Planning' }).click()
  await page.getByRole('button', { name: 'Delete', exact: true }).click()
  await page.getByTestId('delete-this').click()
  await expect.poll(titles).toEqual(['2 Weekly sync', '9 Weekly sync', '16 Planning', '30 Planning', '6 Planning'])
})

test('board: swimlanes by priority and by person, and a column limit', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Sprint', kind: 'work', modules: ['home', 'tasks', 'board'], starter: false } })).json()
  const list = await (await request.post(`/api/v1/workspaces/${ws.id}/lists`, { headers: H, data: { name: 'Backlog' } })).json()
  const mk = async (title: string, priority: number) => (await request.post(`/api/v1/workspaces/${ws.id}/tasks`, { headers: H, data: { title, list_id: list.id, priority } })).json()
  await mk('Fix login', 3)
  const docs = await mk('Write docs', 0)
  await mk('Tidy CSS', 1)

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1400, height: 900 })
  await page.goto(`/w/${ws.id}/board/${list.id}`)
  await page.getByTestId('board-lanes-by').selectOption('priority')
  const lanes = page.getByTestId('lane')
  await expect(lanes).toHaveCount(4)
  await expect(lanes.nth(0)).toContainText('Fix login')
  await expect(lanes.nth(3)).toContainText('Write docs')

  // Write docs dragged into the high lane: it becomes high priority.
  const card = lanes.nth(3).locator('.board-card', { hasText: 'Write docs' })
  const target = lanes.nth(0).locator('.cell-cards').first()
  const a = (await card.boundingBox())!
  const b = (await target.boundingBox())!
  await page.mouse.move(a.x + a.width / 2, a.y + a.height / 2)
  await page.mouse.down()
  await page.mouse.move(a.x + a.width / 2, a.y - 20, { steps: 5 })
  await page.mouse.move(b.x + b.width / 2, b.y + b.height - 6, { steps: 12 })
  // Settle on the target before letting go (as a hand does).
  await page.waitForTimeout(150)
  await page.mouse.move(b.x + b.width / 2 + 2, b.y + b.height - 8, { steps: 2 })
  await page.waitForTimeout(150)
  await page.mouse.up()
  const taskOf = async (id: string) => ((await (await request.get(`/api/v1/workspaces/${ws.id}/tasks`, { headers: H })).json()) as { id: string; priority: number }[]).find((x) => x.id === id)
  await expect.poll(async () => (await taskOf(docs.id))?.priority).toBe(3)
  await expect(lanes.nth(0)).toContainText('Write docs')

  // By person: me, and no one.
  await page.getByTestId('board-lanes-by').selectOption('person')
  await expect(lanes).toHaveCount(2)
  await expect(lanes.nth(1)).toContainText('Fix login')

  // A limit of 2 on To do: 3 cards there is over.
  await page.getByTestId('board-lanes-by').selectOption('')
  const todo = page.locator('section.col').first()
  await todo.getByRole('button', { name: 'Column options' }).click()
  await page.getByTestId('column-limit').fill('2')
  await page.getByTestId('column-limit').press('Enter')
  await expect(todo.getByTestId('column-count')).toHaveText('3/2')
  await expect(todo.getByTestId('column-count')).toHaveClass(/over/)
  const lists = await (await request.get(`/api/v1/workspaces/${ws.id}/lists`, { headers: H })).json()
  expect(lists[0].columns[0].limit).toBe(2)
})

test('time estimates and the week planner: tasks fitted into free working hours as time blocks', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Planner', kind: 'work', modules: ['home', 'tasks', 'calendar'], starter: false } })).json()
  const list = await (await request.post(`/api/v1/workspaces/${ws.id}/lists`, { headers: H, data: { name: 'Work' } })).json()
  const ymd = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
  const monday = new Date()
  monday.setHours(0, 0, 0, 0)
  do monday.setDate(monday.getDate() + 1)
  while (monday.getDay() !== 1)
  const tuesday = new Date(monday.getFullYear(), monday.getMonth(), monday.getDate() + 1)
  // A meeting Monday 9:00–10:00: the planner works around it.
  await request.post(`/api/v1/workspaces/${ws.id}/events`, { headers: H, data: { title: 'Standup', starts_at: new Date(monday.getFullYear(), monday.getMonth(), monday.getDate(), 9).toISOString(), ends_at: new Date(monday.getFullYear(), monday.getMonth(), monday.getDate(), 10).toISOString() } })
  await request.post(`/api/v1/workspaces/${ws.id}/tasks`, { headers: H, data: { title: 'Write the report', list_id: list.id, due: ymd(tuesday), estimate: 90, priority: 2 } })
  const slides = await (await request.post(`/api/v1/workspaces/${ws.id}/tasks`, { headers: H, data: { title: 'Prepare slides', list_id: list.id } })).json()

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1400, height: 900 })
  // Quick add reads an estimate.
  await page.goto(`/w/${ws.id}/tasks/${list.id}`)
  await page.getByRole('button', { name: 'New Task' }).first().click()
  await page.getByPlaceholder(/New task/).fill('Reply to emails ~30m')
  await page.keyboard.press('Enter')
  await expect(page.locator('.task-row', { hasText: 'Reply to emails' })).toContainText('30m')

  await page.goto(`/w/${ws.id}/calendar?date=${ymd(monday)}`)
  await page.getByRole('tab', { name: 'Week' }).click()
  await page.getByTestId('plan-open').click()
  const sheet = page.getByTestId('plan-sheet')
  await expect(sheet.getByTestId('plan-task')).toHaveCount(3)
  // Slides has no estimate yet: given one here.
  await sheet.getByTestId('plan-task').filter({ hasText: 'Prepare slides' }).getByTestId('plan-estimate').selectOption('60')
  await expect(sheet.getByTestId('plan-block')).toHaveCount(3)
  // The report (due first) goes first, after the standup.
  await expect(sheet.getByTestId('plan-block').first()).toContainText('10:00')
  await expect(sheet.getByTestId('plan-block').first()).toContainText('Write the report')
  await page.getByTestId('plan-apply').click()

  const range = `from=${new Date(monday).toISOString()}&to=${new Date(monday.getFullYear(), monday.getMonth(), monday.getDate() + 7).toISOString()}`
  await expect.poll(async () => ((await (await request.get(`/api/v1/me/blocks?${range}`, { headers: H })).json()) as { title: string }[]).map((b) => b.title)).toEqual(['Write the report', 'Prepare slides', 'Reply to emails'])
  const all = (await (await request.get(`/api/v1/workspaces/${ws.id}/tasks`, { headers: H })).json()) as { id: string; estimate: number }[]
  expect(all.find((x) => x.id === slides.id)?.estimate).toBe(60)
  const block = page.locator('.tg-ev.blk', { hasText: 'Write the report' })
  await expect(block).toBeVisible()
  await page.screenshot({ path: 'test-results/planner.png' })
  await block.click()
  await expect(page.getByTestId('task-estimate')).toHaveText('1h 30m')
})

test('Google Calendar: the owner sets up signing in with Google; a workspace owner is sent to Google to sign in', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Family', kind: 'personal', modules: ['home', 'calendar'], starter: false } })).json()

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1400, height: 900 })
  // Not set up yet: the calendars sheet says how it gets set up.
  await page.goto(`/w/${ws.id}/calendar`)
  await page.getByRole('button', { name: 'Calendars', exact: true }).click()
  await expect(page.getByTestId('caldav')).toContainText('Owner → Google Calendar')
  await expect(page.getByTestId('google-start')).toHaveCount(0)

  await page.goto('/owner')
  const card = page.getByTestId('google-admin')
  await expect(card).toContainText('/api/v1/google/oauth/callback')
  await page.getByTestId('google-client-id').fill('123-abc.apps.googleusercontent.com')
  await page.getByTestId('google-client-secret').fill('GOCSPX-secret')
  await card.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByTestId('google-client-secret')).toHaveAttribute('placeholder', /kept/)

  // Signing in goes to Google with this instance's client, offline, with PKCE.
  let asked = ''
  await page.route('https://accounts.google.com/**', (route) => {
    asked = route.request().url()
    return route.fulfill({ contentType: 'text/html', body: '<h1>Google</h1>' })
  })
  await page.goto(`/w/${ws.id}/calendar`)
  await page.getByRole('button', { name: 'Calendars', exact: true }).click()
  await page.getByTestId('google-start').click()
  await expect(page.getByRole('heading', { name: 'Google' })).toBeVisible()
  const q = new URL(asked).searchParams
  expect(q.get('client_id')).toBe('123-abc.apps.googleusercontent.com')
  expect(q.get('redirect_uri')).toMatch(/\/api\/v1\/google\/oauth\/callback$/)
  expect(q.get('access_type')).toBe('offline')
  expect(q.get('code_challenge_method')).toBe('S256')
  // Back without a valid sign-in: said so, on the calendar.
  await page.goto(`/api/v1/google/oauth/callback?state=nope&code=x`)
  await expect(page).toHaveURL(/\/\?google_error=expired|calendar\?google_error=expired/)

  // Removed again (the other tests don't expect it).
  await page.goto('/owner')
  await page.getByTestId('google-admin').getByRole('button', { name: 'Remove' }).click()
  await expect(page.getByTestId('google-client-secret')).not.toHaveAttribute('placeholder', /kept/)
})

test('a workspace exported to a file comes back as a new one with its tasks', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Move me', kind: 'work' } })).json()
  expect((await request.post(`/api/v1/workspaces/${ws.id}/tasks`, { headers: H, data: { title: 'Pack the boxes' } })).ok()).toBeTruthy()

  await signIn(page, NEW_PASSWORD)
  await page.goto(`/w/${ws.id}`)
  await page.getByRole('button', { name: 'Workspace settings' }).click()
  const [download] = await Promise.all([page.waitForEvent('download'), page.getByTestId('export-ws').click()])
  expect(download.suggestedFilename()).toBe('Move-me.orbit.zip')
  const file = await download.path()
  await page.keyboard.press('Escape')

  await page.getByRole('button', { name: 'New workspace' }).first().click()
  await page.getByTestId('import-ws').click()
  await page.getByTestId('import-file').setInputFiles(file)
  await page.getByRole('button', { name: 'Import', exact: true }).click()
  await expect(page.getByTestId('import-result')).toContainText('Move me is here')
  await expect(page.getByTestId('import-result')).toContainText('1 task ·')
  await page.getByRole('button', { name: 'Open', exact: true }).click()
  await expect(page).not.toHaveURL(new RegExp(ws.id))
  await page.getByRole('link', { name: /^Tasks/ }).first().click()
  await expect(page.getByText('Pack the boxes')).toBeVisible()
})

// A CalDAV server (Radicale, any password) the instance can reach; skipped without.
const CALDAV_URL = process.env.E2E_CALDAV_URL

test('CalDAV: a workspace calendar syncs both ways with a calendar on a CalDAV server', async ({ page, request }) => {
  test.skip(!CALDAV_URL, 'set E2E_CALDAV_URL to a CalDAV server')
  const dav = { Authorization: 'Basic ' + Buffer.from('ana:any').toString('base64') }
  const cal = `${CALDAV_URL}/ana/orbit-${Date.now()}/`
  const mk = await request.fetch(cal, {
    method: 'MKCALENDAR',
    headers: { ...dav, 'Content-Type': 'application/xml' },
    data: '<?xml version="1.0"?><c:mkcalendar xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:set><d:prop><d:displayname>Team</d:displayname></d:prop></d:set></c:mkcalendar>',
  })
  expect(mk.status()).toBe(201)
  const ics = (uid: string, title: string, start: string) =>
    `BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//e2e//EN\r\nBEGIN:VEVENT\r\nUID:${uid}\r\nDTSTAMP:20261001T080000Z\r\nDTSTART:${start}\r\nDTEND:${start.slice(0, 9)}235900Z\r\nSUMMARY:${title}\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n`
  expect((await request.put(cal + 'dentist.ics', { headers: { ...dav, 'Content-Type': 'text/calendar' }, data: ics('dentist-1', 'Dentist', '20261105T090000Z') })).status()).toBe(201)

  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Synced', kind: 'work' } })).json()
  const ev = await (await request.post(`/api/v1/workspaces/${ws.id}/events`, { headers: H, data: { title: 'Standup', starts_at: '2026-11-06T10:00:00Z', ends_at: '2026-11-06T10:15:00Z', tz: 'Europe/Lisbon' } })).json()

  await signIn(page, NEW_PASSWORD)
  await page.goto(`/w/${ws.id}/calendar?date=2026-11-05`)
  await page.getByRole('button', { name: 'Calendars', exact: true }).click()
  await page.getByTestId('caldav-start').click()
  await page.getByLabel('Server').fill(CALDAV_URL!)
  await page.getByLabel('Username').fill('ana')
  await page.getByLabel('App password').fill('any')
  await page.getByRole('button', { name: 'Find calendars' }).click()
  await page.getByRole('radio', { name: 'Team' }).click()
  await page.getByRole('button', { name: 'Sync', exact: true }).click()
  await expect(page.getByTestId('caldav')).toContainText('2 events')

  // The server's event is in Orbit; Orbit's is on the server, in its zone.
  const cal1 = await (await request.get(`/api/v1/workspaces/${ws.id}/calendar?from=2026-11-01T00:00:00Z&to=2026-11-30T00:00:00Z`, { headers: H })).json()
  expect(cal1.events.map((e: { title: string }) => e.title).sort()).toEqual(['Dentist', 'Standup'])
  const there = await (await request.get(cal, { headers: dav })).text()
  expect(there).toContain('SUMMARY:Standup')
  expect(there).toContain('TZID=Europe/Lisbon')

  // Renamed in Orbit: the server follows within seconds.
  await request.put(`/api/v1/events/${ev.id}`, { headers: H, data: { title: 'Daily standup' } })
  await expect.poll(async () => (await (await request.get(cal, { headers: dav })).text()).includes('SUMMARY:Daily standup'), { timeout: 15000 }).toBe(true)

  // Moved on the server: Sync now brings it.
  await request.put(cal + 'dentist.ics', { headers: { ...dav, 'Content-Type': 'text/calendar' }, data: ics('dentist-1', 'Dentist (moved)', '20261112T090000Z') })
  await page.getByRole('button', { name: 'Sync now' }).click()
  await expect.poll(async () => {
    const c = await (await request.get(`/api/v1/workspaces/${ws.id}/calendar?from=2026-11-01T00:00:00Z&to=2026-11-30T00:00:00Z`, { headers: H })).json()
    return c.events.map((e: { title: string }) => e.title).sort().join(',')
  }).toBe('Daily standup,Dentist (moved)')
})

test('whiteboards and a note\'s drawing (Excalidraw): drawn, saved, shown', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Studio', kind: 'custom', modules: ['home', 'notes', 'whiteboards'], starter: false } })).json()

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1400, height: 900 })
  await page.goto(`/w/${ws.id}/whiteboards`)
  await page.getByTestId('new-whiteboard').first().click()
  await page.waitForURL(/\/whiteboards\/[^/]+$/)
  const id = page.url().split('/').pop()!
  const canvas = page.locator('.excalidraw canvas.interactive')
  await expect(canvas).toBeVisible({ timeout: 20000 })
  await page.getByTestId('whiteboard-title').fill('Floor plan')
  await page.getByTestId('whiteboard-title').press('Enter')

  // A rectangle: R, then drag.
  const box = (await canvas.boundingBox())!
  await page.mouse.click(box.x + box.width / 2, box.y + box.height - 40)
  await page.keyboard.press('r')
  await page.mouse.move(box.x + 300, box.y + 200)
  await page.mouse.down()
  await page.mouse.move(box.x + 520, box.y + 360, { steps: 8 })
  await page.mouse.up()
  await expect(page.getByTestId('draw-state')).toContainText('Saved', { timeout: 10000 })
  const d = await (await request.get(`/api/v1/drawings/${id}`, { headers: H })).json()
  expect(d.title).toBe('Floor plan')
  expect(d.scene).toContain('"rectangle"')
  expect(d.preview.startsWith('<svg')).toBeTruthy()
  await page.screenshot({ path: 'test-results/whiteboard.png' })

  // Back in the list: its card, with the picture.
  await page.getByTestId('whiteboard-back').click()
  await expect(page.getByTestId('whiteboard-card').filter({ hasText: 'Floor plan' }).locator('img')).toBeVisible()

  // A note's drawing: its picture shows in the note and on the card.
  const note = await (await request.post(`/api/v1/workspaces/${ws.id}/notes`, { headers: H, data: { title: 'Sketch' } })).json()
  await page.goto(`/w/${ws.id}/notes?note=${note.id}`)
  await page.getByTestId('note-draw').click()
  const sheet = page.getByTestId('drawing-sheet')
  const c2 = sheet.locator('.excalidraw canvas.interactive')
  await expect(c2).toBeVisible({ timeout: 20000 })
  const b2 = (await c2.boundingBox())!
  await page.mouse.click(b2.x + b2.width / 2, b2.y + b2.height - 40)
  await page.keyboard.press('o')
  await page.mouse.move(b2.x + 250, b2.y + 150)
  await page.mouse.down()
  await page.mouse.move(b2.x + 450, b2.y + 300, { steps: 8 })
  await page.mouse.up()
  await expect(sheet.getByTestId('draw-state')).toContainText('Saved', { timeout: 10000 })
  await page.screenshot({ path: 'test-results/note-drawing.png' })
  await sheet.getByRole('button', { name: 'Done' }).click()
  await expect(page.getByTestId('note-drawing')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByTestId('drawing-sheet')).toHaveCount(0)
  await expect(page.getByTestId('card-drawing')).toBeVisible()
  await page.screenshot({ path: 'test-results/note-card.png' })
})

test('habits: one made, today ticked, a streak', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Me', kind: 'custom', modules: ['home', 'habits'], starter: false } })).json()
  // One with a history already: done yesterday and the day before.
  const old = await (await request.post(`/api/v1/workspaces/${ws.id}/habits`, { headers: H, data: { title: 'Stretch', icon: '🧘', color: '#34C759' } })).json()
  const day = (n: number) => {
    const d = new Date(Date.now() - n * 86400000)
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
  }
  for (const n of [1, 2]) await request.put(`/api/v1/habits/${old.id}/checks/${day(n)}`, { headers: H, data: { done: true } })

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1200, height: 820 })
  await page.goto(`/w/${ws.id}/habits`)
  await expect(page.getByTestId('habit').filter({ hasText: 'Stretch' })).toContainText('2 days')
  await page.getByTestId('new-habit').click()
  const sheet = page.getByTestId('habit-sheet')
  await sheet.getByRole('textbox', { name: 'Name' }).fill('Read')
  await page.getByRole('button', { name: 'Add', exact: true }).click()
  const read = page.getByTestId('habit').filter({ hasText: 'Read' })
  await expect(read).toBeVisible()
  // Today's circle.
  await read.getByRole('button', { name: /^Mark .* done$/ }).and(page.locator('.today')).click()
  await expect.poll(async () => {
    const list = await (await request.get(`/api/v1/workspaces/${ws.id}/habits`, { headers: H })).json()
    return list.find((h: { title: string }) => h.title === 'Read')?.checks ?? []
  }).toEqual([day(0)])
  await expect(read).toContainText('1 day')
  await page.getByTestId('habit').filter({ hasText: 'Stretch' }).getByRole('button', { name: /Stretch/ }).click()
  await page.screenshot({ path: 'test-results/habits.png' })
})

test('focus timer: a session on a task runs (with a pill in the sidebar) and stops', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Deep work', kind: 'work', modules: ['home', 'tasks', 'focus'] } })).json()
  const task = await (await request.post(`/api/v1/workspaces/${ws.id}/tasks`, { headers: H, data: { title: 'Write the report' } })).json()

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1200, height: 820 })
  await page.goto(`/w/${ws.id}/focus?task=${task.id}`)
  await expect(page.getByTestId('focus-task')).toHaveValue(task.id)
  await page.getByRole('radio', { name: '50 min' }).or(page.getByRole('button', { name: '50 min' })).click()
  await page.getByTestId('focus-start').click()
  await expect(page.getByTestId('focus-clock')).toHaveText(/^(50:00|49:5\d)$/)
  await expect(page.getByTestId('focus-pill')).toContainText('Write the report')
  await page.screenshot({ path: 'test-results/focus.png' })
  const st = await (await request.get('/api/v1/me/focus', { headers: H })).json()
  expect(st.running.task_id).toBe(task.id)
  expect(st.running.minutes).toBe(50)
  await page.getByTestId('focus-stop').click()
  await expect(page.getByTestId('focus-pill')).toHaveCount(0)
  await expect(page.getByTestId('focus-clock')).toHaveText('50:00')
})

test('focus timer: my own length, kept on my account, then removed', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Long focus', kind: 'work', modules: ['home', 'tasks', 'focus'] } })).json()

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1200, height: 820 })
  await page.goto(`/w/${ws.id}/focus`)
  await page.getByTestId('focus-add-length').click()
  const form = page.getByTestId('focus-custom')
  await form.getByRole('spinbutton').fill('40')
  await form.getByRole('button', { name: 'Add a length' }).click()
  await expect(page.getByRole('radio', { name: '40 min' })).toHaveAttribute('aria-checked', 'true')
  await expect(page.getByTestId('focus-clock')).toHaveText('40:00')
  await expect.poll(async () => (await (await request.get('/api/v1/me/prefs', { headers: H })).json()).focus_lengths).toEqual([40])
  await page.getByTestId('focus-start').click()
  await expect(page.getByTestId('focus-stop')).toBeVisible()
  expect((await (await request.get('/api/v1/me/focus', { headers: H })).json()).running.minutes).toBe(40)
  await page.getByTestId('focus-stop').click()

  // Still there after a reload, and picked again; then taken off the list.
  await page.reload()
  await expect(page.getByRole('radio', { name: '40 min' })).toHaveAttribute('aria-checked', 'true')
  await page.getByTestId('focus-forget-length').click()
  await expect(page.getByRole('radio', { name: '40 min' })).toHaveCount(0)
  await expect(page.getByRole('radio', { name: '25 min' })).toHaveAttribute('aria-checked', 'true')
  await expect.poll(async () => (await (await request.get('/api/v1/me/prefs', { headers: H })).json()).focus_lengths).toEqual([])
})

test('goals: one with a date, two tasks towards it, half done', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Launch', kind: 'work', modules: ['home', 'tasks', 'goals'] } })).json()

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1200, height: 820 })
  await page.goto(`/w/${ws.id}/goals`)
  await page.getByTestId('new-goal').first().click()
  const sheet = page.getByTestId('goal-sheet')
  await sheet.getByRole('textbox', { name: 'Name' }).fill('Ship the website')
  const d = new Date(Date.now() + 30 * 86400000)
  await sheet.locator('input[type="date"]').fill(`${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`)
  await page.getByRole('button', { name: 'Add', exact: true }).click()
  const card = page.getByTestId('goal').filter({ hasText: 'Ship the website' })
  await expect(card).toContainText('days left')

  await card.click()
  for (const title of ['Write the copy', 'Pick the photos']) {
    await sheet.getByTestId('goal-new-task').fill(title)
    await sheet.getByTestId('goal-new-task').press('Enter')
    await expect(sheet.getByText(title)).toBeVisible()
  }
  await sheet.getByRole('checkbox', { name: 'Write the copy' }).click()
  await expect(sheet).toContainText('50%')
  await page.keyboard.press('Escape')
  await expect(card).toContainText('1 of 2 tasks')
  await expect(card).toContainText('50%')
  await page.screenshot({ path: 'test-results/goals.png' })
})

test('journal: today written with a mood, kept, and last year on this day', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Diary', kind: 'personal', modules: ['home', 'journal'] } })).json()
  const d = new Date()
  const iso = (y: number) => `${y}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
  await request.put(`/api/v1/workspaces/${ws.id}/journal/${iso(d.getFullYear() - 1)}`, { headers: H, data: { content: 'Snow in the mountains.', mood: 5 } })

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1200, height: 820 })
  await page.goto(`/w/${ws.id}/journal`)
  await expect(page.getByText('On this day')).toBeVisible()
  await expect(page.getByText('Snow in the mountains.')).toBeVisible()
  await page.getByTestId('journal-entry').getByTestId('md-source').click()
  await page.keyboard.insertText('Finished the **report**. Long walk after.')
  await page.getByRole('radio', { name: 'Good' }).click()
  await expect(page.getByTestId('journal-state')).toContainText('Saved')
  await page.reload()
  await expect(page.getByTestId('journal-entry')).toContainText('Finished the report')
  await expect(page.getByRole('radio', { name: 'Good' })).toHaveAttribute('aria-checked', 'true')
  await expect(page.locator(`.day[data-day="${iso(d.getFullYear())}"]`)).toContainText('🙂')
  await page.screenshot({ path: 'test-results/journal.png' })
})

test('bookmarks: a link kept, tagged, read later and read', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Reading', kind: 'personal', modules: ['home', 'bookmarks'] } })).json()

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1200, height: 820 })
  await page.goto(`/w/${ws.id}/bookmarks`)
  // The test network has no internet: the link is kept as typed, without a preview.
  await page.getByTestId('bookmark-url').fill('https://example.invalid/sourdough')
  await page.getByRole('switch', { name: 'Read later' }).first().click()
  await page.getByTestId('bookmark-add').click()
  const card = page.getByTestId('bookmark').filter({ hasText: 'example.invalid' })
  await expect(card).toBeVisible({ timeout: 20000 })
  await page.getByRole('radio', { name: 'Read later' }).or(page.locator('.seg button', { hasText: 'Read later' })).click()
  await expect(card).toBeVisible()
  // Mark it read: it leaves "read later".
  await card.getByTestId('bookmark-later').click()
  await expect(card).toHaveCount(0)
  const list = await (await request.get(`/api/v1/workspaces/${ws.id}/bookmarks`, { headers: H })).json()
  expect(list[0].read_at).toBeTruthy()
  await page.locator('.seg button', { hasText: 'All' }).click()
  await page.screenshot({ path: 'test-results/bookmarks.png' })
})

// A two-page PDF (a red and a blue box), small enough to write by hand.
function tinyPdf(): Buffer {
  const streams = ['1 0 0 rg 20 20 160 60 re f', '0 0 1 rg 20 20 160 60 re f']
  const objs = ['<< /Type /Catalog /Pages 2 0 R >>', '<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>']
  streams.forEach((st, i) => {
    objs.push(`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 100] /Contents ${4 + i * 2} 0 R >>`)
    objs.push(`<< /Length ${st.length} >>\nstream\n${st}\nendstream`)
  })
  let out = '%PDF-1.4\n'
  const offs: number[] = []
  objs.forEach((o, i) => {
    offs.push(out.length)
    out += `${i + 1} 0 obj\n${o}\nendobj\n`
  })
  const x = out.length
  out += `xref\n0 ${objs.length + 1}\n0000000000 65535 f \n${offs.map((n) => `${String(n).padStart(10, '0')} 00000 n \n`).join('')}trailer\n<< /Size ${objs.length + 1} /Root 1 0 R >>\nstartxref\n${x}\n%%EOF\n`
  return Buffer.from(out)
}

test('Quick Draw: its own canvas — pens, hold to snap, a real eraser, colours kept, the lasso, paper, layers with a picture and a PDF page', async ({ page, request }) => {
  test.setTimeout(60_000) // a PDF drawn page by page: slow on a busy machine
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1400, height: 900 })
  await page.getByTestId('quick-draw-new').click()
  await page.waitForURL(/\/draw\/[^/]+$/)
  const id = page.url().split('/').pop()!
  const scene = async () => JSON.parse((await (await request.get(`/api/v1/drawings/${id}`, { headers: H })).json()).scene || '{}')
  const items = async () => ((await scene()).layers ?? []).flatMap((l: { items: unknown[] }) => l.items)
  await expect(page.getByTestId('sketch')).toBeVisible()
  await page.getByTestId('whiteboard-title').fill('Napkin idea')
  await page.getByTestId('whiteboard-title').press('Enter')
  const box = (await page.getByTestId('sketch-board').boundingBox())!
  const cx = box.x + box.width / 2
  const cy = box.y + box.height / 2 - 60

  // A line with the pen.
  await page.mouse.move(cx - 200, cy)
  await page.mouse.down()
  await page.mouse.move(cx + 200, cy + 40, { steps: 20 })
  await page.mouse.up()
  // A wobbly circle, held at the end: it snaps into a clean one.
  await page.mouse.move(cx + 60, cy + 150)
  await page.mouse.down()
  for (let k = 1; k <= 28; k++) {
    const a = (k / 28) * 2 * Math.PI
    await page.mouse.move(cx + Math.cos(a) * (60 + (k % 3)), cy + 150 + Math.sin(a) * (58 + (k % 2) * 3))
  }
  await page.waitForTimeout(800)
  await expect(page.getByText('A circle')).toBeVisible()
  await page.mouse.up()
  await expect.poll(async () => (await items()).length).toBe(2)
  const [line, circle] = await items()
  expect(line.tool).toBe('pen')
  expect(circle.snapped).toBe(true)

  // The eraser takes out only the part rubbed: the line in two.
  await page.getByTestId('sketch-eraser').click()
  await page.mouse.move(cx, cy - 30)
  await page.mouse.down()
  await page.mouse.move(cx, cy + 70, { steps: 10 })
  await page.mouse.up()
  await expect.poll(async () => (await items()).length).toBe(3)
  await page.keyboard.press('Control+z')
  await expect.poll(async () => (await items()).length).toBe(2)
  // Shift held: the whole line goes (as Excalidraw's eraser).
  await page.keyboard.down('Shift')
  await page.mouse.move(cx, cy - 30)
  await page.mouse.down()
  await page.mouse.move(cx, cy + 70, { steps: 10 })
  await page.mouse.up()
  await page.keyboard.up('Shift')
  await expect.poll(async () => (await items()).length).toBe(1)
  await page.keyboard.press('Control+z')
  await expect.poll(async () => (await items()).length).toBe(2)
  // Or "Whole line" picked, and kept for next time.
  await page.getByTestId('sketch-erase-whole').click()
  await expect(page.getByTestId('sketch-erase-whole')).toHaveAttribute('aria-checked', 'true')
  await page.mouse.move(cx, cy - 30)
  await page.mouse.down()
  await page.mouse.move(cx, cy + 70, { steps: 10 })
  await page.mouse.up()
  await expect.poll(async () => (await items()).length).toBe(1)
  await page.keyboard.press('Control+z')
  await expect.poll(async () => (await items()).length).toBe(2)
  await page.getByTestId('sketch-erase-part').click()

  // A colour typed in, kept in my colours (with my settings).
  await page.getByTestId('sketch-color').click()
  await page.getByTestId('color-code').fill('#ff3b30')
  await page.getByTestId('color-code').press('Enter')
  await page.getByTestId('color-keep').click()
  await expect(page.getByTestId('my-colors').getByRole('button', { name: '#ff3b30', exact: true })).toBeVisible()
  await expect.poll(async () => (await (await request.get('/api/v1/me/prefs', { headers: H })).json()).sketch_colors).toContain('#ff3b30')
  await page.keyboard.press('Escape')

  // The lasso round the circle: recoloured.
  await page.getByTestId('sketch-lasso').click()
  await page.mouse.move(cx + 110, cy + 150)
  await page.mouse.down()
  for (let k = 1; k <= 24; k++) {
    const a = (k / 24) * 2 * Math.PI
    await page.mouse.move(cx + Math.cos(a) * 110, cy + 150 + Math.sin(a) * 100)
  }
  await page.mouse.up()
  await expect(page.getByTestId('sketch-picked')).toContainText('One chosen')
  await page.getByTestId('sketch-recolor').click()
  await expect.poll(async () => (await items())[1].color).toBe('#ff3b30')
  expect((await items())[0].color).toBe('#1c1c1e')

  // Lined paper.
  await page.getByTestId('sketch-paper').click()
  await page.getByTestId('paper-lined').click()
  await expect.poll(async () => (await scene()).paper).toBe('lined')
  await page.keyboard.press('Escape')

  // A picture, then a PDF's second page, each on a layer under the ink.
  const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==', 'base64')
  await page.getByTestId('sketch-file').setInputFiles({ name: 'photo.png', mimeType: 'image/png', buffer: png })
  await expect.poll(async () => (await scene()).layers.length).toBe(2)
  await page.getByTestId('sketch-file').setInputFiles({ name: 'plan.pdf', mimeType: 'application/pdf', buffer: tinyPdf() })
  await page.getByTestId('sketch-pdf-page').fill('2')
  await page.getByRole('button', { name: 'Put it in' }).click()
  await expect.poll(async () => (await scene()).layers.length).toBe(3)
  const layers = (await scene()).layers as { name: string; items: { kind: string; src?: string }[] }[]
  // Bottom to top: each went in just under the layer drawn on.
  expect(layers.map((l) => l.name)).toEqual(['photo.png', 'plan · page 2', 'Layer 1'])
  expect(layers[1].items[0].src).toMatch(/^data:image\/jpeg/)
  await page.getByTestId('sketch-layers').click()
  await expect(page.getByTestId('sketch-layer')).toHaveCount(3)
  await page.keyboard.press('Escape')
  await page.screenshot({ path: 'test-results/sketch.png' })

  await page.getByTestId('whiteboard-back').click()
  await expect(page).toHaveURL(/\/draw$/)
  await expect(page.getByTestId('whiteboard-card').filter({ hasText: 'Napkin idea' }).locator('img')).toBeVisible()

  // A sketch made before keeps opening in Excalidraw.
  const old = await (await request.post('/api/v1/me/drawings', { headers: H, data: { title: 'Old sketch' } })).json()
  await page.goto(`/draw/${old.id}`)
  await expect(page.locator('.excalidraw canvas.interactive')).toBeVisible({ timeout: 20000 })
})

test('Quick Draw: the bucket fills what the lines close off, under them', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1300, height: 860 })
  await page.getByTestId('quick-draw-new').click()
  await page.waitForURL(/\/draw\/[^/]+$/)
  const id = page.url().split('/').pop()!
  const items = async () =>
    (JSON.parse((await (await request.get(`/api/v1/drawings/${id}`, { headers: H })).json()).scene || '{}').layers ?? []).flatMap(
      (l: { items: unknown[] }) => l.items,
    ) as { kind: string; fill?: boolean; src?: string; w?: number; h?: number }[]
  const box = (await page.getByTestId('sketch-board').boundingBox())!
  const cx = box.x + box.width / 2
  const cy = box.y + box.height / 2

  // A closed square with the pen.
  await page.mouse.move(cx - 80, cy - 80)
  await page.mouse.down()
  for (const [x, y] of [[80, -80], [80, 80], [-80, 80], [-80, -80]]) await page.mouse.move(cx + x, cy + y, { steps: 12 })
  await page.mouse.up()
  await expect.poll(async () => (await items()).length).toBe(1)

  // The bucket inside it: a picture of the square's inside, under the line.
  await page.getByTestId('sketch-fill').click()
  await page.mouse.click(cx, cy)
  await expect.poll(async () => (await items()).length).toBe(2)
  const [fill, line] = await items()
  expect(fill.kind).toBe('image')
  expect(fill.fill).toBe(true)
  expect(fill.src).toMatch(/^data:image\/png/)
  expect(line.kind).toBe('stroke')
  // As big as the square (not the whole paper).
  expect(fill.w!).toBeGreaterThan(150)
  expect(fill.w!).toBeLessThan(175)
  // Again, with G: a fill isn't a wall, so it fills the same again (on top).
  await page.keyboard.press('g')
  await page.mouse.click(cx + 20, cy + 20)
  await expect.poll(async () => (await items()).map((i) => i.kind)).toEqual(['image', 'image', 'stroke'])
  await page.screenshot({ path: 'test-results/bucket.png' })
  await page.keyboard.press('Control+z')
  await expect.poll(async () => (await items()).length).toBe(2)
})

test('Quick Draw: full screen, with its pickers still there, and back', async ({ page }) => {
  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1300, height: 860 })
  await page.getByTestId('quick-draw-new').click()
  await page.waitForURL(/\/draw\/[^/]+$/)
  const before = (await page.getByTestId('sketch-board').boundingBox())!
  expect(before.height).toBeLessThan(860)

  await page.getByTestId('sketch-full').click()
  await expect(page.getByTestId('sketch-full')).toHaveAttribute('aria-pressed', 'true')
  await expect.poll(async () => (await page.getByTestId('sketch-board').boundingBox())!.height).toBe(860)
  expect((await page.getByTestId('sketch-board').boundingBox())!.width).toBe(1300)
  // The colour picker opens over it.
  await page.getByTestId('sketch-color').click()
  await expect(page.locator('.popover')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.locator('.popover')).toHaveCount(0)
  // And still draws.
  await page.mouse.move(400, 400)
  await page.mouse.down()
  await page.mouse.move(600, 500, { steps: 10 })
  await page.mouse.up()
  await expect(page.getByTestId('sketch-undo')).toBeEnabled()
  await page.screenshot({ path: 'test-results/sketch-full.png' })

  await page.getByTestId('sketch-full').click()
  await expect(page.getByTestId('sketch-full')).toHaveAttribute('aria-pressed', 'false')
  await expect.poll(async () => (await page.getByTestId('sketch-board').boundingBox())!.height).toBeLessThan(860)
  expect(await page.evaluate(() => document.fullscreenElement)).toBeNull()
})

test('Quick Draw: a sketch moved into a workspace as a whiteboard, and back', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Atelier', kind: 'custom', modules: ['home', 'whiteboards'], starter: false } })).json()
  const sketch = await (await request.post('/api/v1/me/drawings', { headers: H, data: { title: 'Logo idea' } })).json()

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1300, height: 860 })
  await page.goto('/draw')
  const card = page.getByTestId('whiteboard-card').filter({ hasText: 'Logo idea' })
  await card.getByRole('button', { name: 'More' }).click()
  await page.getByTestId('drawing-move').click()
  await page.getByRole('dialog').getByRole('button', { name: 'Atelier' }).click()
  await expect(card).toHaveCount(0)

  // In the workspace's whiteboards now.
  await page.goto(`/w/${ws.id}/whiteboards`)
  await page.getByTestId('whiteboard-card').filter({ hasText: 'Logo idea' }).click()
  await expect(page).toHaveURL(new RegExp(`/w/${ws.id}/whiteboards/${sketch.id}$`))
  // And back to Quick Draw, from the drawing itself.
  await page.getByTestId('whiteboard-move').click()
  await page.getByRole('dialog').getByRole('button', { name: /Quick Draw/ }).click()
  await expect(page).toHaveURL(new RegExp(`/draw/${sketch.id}$`))
  const mine = await (await request.get('/api/v1/me/drawings', { headers: H })).json()
  expect(mine.map((d: { id: string }) => d.id)).toContain(sketch.id)
})

test('smart lists: a filter saved with a name, kept on the server; quick add reads a repeat', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Smart', kind: 'personal', modules: ['home', 'tasks'] } })).json()

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1200, height: 820 })
  await page.goto(`/w/${ws.id}/tasks`)
  await page.getByRole('button', { name: 'New Task' }).first().click()
  for (const line of ['Pay rent !!! #money', 'Buy milk #food', 'Bins every other Tuesday']) {
    await page.getByPlaceholder(/New task/).fill(line)
    await page.keyboard.press('Enter')
    await expect(page.locator('.task-row', { hasText: line.split(' ')[0] })).toBeVisible()
  }
  await page.keyboard.press('Escape')
  await expect(page.locator('.task-row', { hasText: 'Bins' })).toContainText('Every other Tuesday')

  // All → filter by #money → saved as "Bills".
  await page.goto(`/w/${ws.id}/tasks/all`)
  await page.getByRole('button', { name: /^Filter/ }).click()
  await page.locator('.popover .chip', { hasText: '#money' }).click()
  await page.getByTestId('filter-save').click()
  await page.getByTestId('smart-sheet').getByRole('textbox', { name: 'Name' }).fill('Bills')
  await page.getByTestId('smart-save').click()
  await expect(page.getByRole('heading', { name: 'Bills' })).toBeVisible()
  await expect(page.getByTestId('smart-lists')).toContainText('Bills')
  await expect(page.locator('.task-row', { hasText: 'Pay rent' })).toBeVisible()
  await expect(page.locator('.task-row', { hasText: 'Buy milk' })).toBeHidden()
  // All is whole again (the filter went into the smart list).
  await page.locator('button.smart', { hasText: /All$/ }).click()
  await expect(page.locator('.task-row', { hasText: 'Buy milk' })).toBeVisible()

  // A task added in the smart list gets its tag, so it shows there.
  await page.getByTestId('smart-lists').getByText('Bills').click()
  await page.getByRole('button', { name: 'New Task' }).first().click()
  await page.getByPlaceholder(/New task/).fill('Water bill')
  await page.keyboard.press('Enter')
  await page.keyboard.press('Escape')
  await expect(page.locator('.task-row', { hasText: 'Water bill' })).toContainText('#money')

  // Its filter changes in place and is kept on the server.
  await page.getByRole('button', { name: /^Filter/ }).click()
  await page.locator('.popover .chip', { hasText: '#food' }).click()
  await expect(page.locator('.task-row', { hasText: 'Buy milk' })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect.poll(async () => (await (await request.get(`/api/v1/workspaces/${ws.id}/smart-lists`, { headers: H })).json())[0].filter.tags.length).toBe(2)
  await page.reload()
  await expect(page.getByRole('heading', { name: 'Bills' })).toBeVisible()
  await expect(page.locator('.task-row', { hasText: 'Buy milk' })).toBeVisible()
  await page.screenshot({ path: 'test-results/smart-lists.png' })
})

test('page templates: a new page starts as meeting notes, or from a page made a template', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Docs', kind: 'work', modules: ['home', 'pages'] } })).json()
  const mk = async (data: object) => (await request.post(`/api/v1/workspaces/${ws.id}/pages`, { headers: H, data })).json()
  const content = async (id: string) => (await (await request.get(`/api/v1/pages/${id}`, { headers: H })).json()) as { title: string; content: string }

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1200, height: 820 })
  const blank = await mk({ title: '' })
  await page.goto(`/w/${ws.id}/p/${blank.id}`)
  await page.getByTestId('page-templates').getByRole('button', { name: 'Meeting notes' }).click()
  await expect(page.getByTestId('page-templates')).toBeHidden()
  await expect(page.getByRole('textbox', { name: 'Name' })).toHaveValue(/^Meeting — \d/)
  await expect.poll(async () => (await content(blank.id)).content).toContain('## Action items')
  await expect.poll(async () => (await content(blank.id)).title).toMatch(/^Meeting — \d/)

  // A page of ours becomes a template (from its menu).
  const tpl = await mk({ title: 'Standup {{weekday}}', content: '## Yesterday\n\n- {{me}}\n' })
  await page.goto(`/w/${ws.id}/p/${tpl.id}`)
  await page.getByRole('button', { name: 'Page options' }).click()
  await page.getByTestId('page-template-toggle').click()
  await expect(page.locator('.tpl-note')).toBeVisible()

  const next = await mk({ title: '' })
  await page.goto(`/w/${ws.id}/p/${next.id}`)
  await page.getByTestId('page-templates').getByRole('button', { name: 'Standup {{weekday}}' }).click()
  await expect.poll(async () => (await content(next.id)).content).toMatch(/## Yesterday\s+- admin/)
  await expect.poll(async () => (await content(next.id)).title).toMatch(/^Standup [A-Z][a-z]+day$/)
  await page.screenshot({ path: 'test-results/page-templates.png' })
})

test('pages: the block handle — turn into, colour, duplicate, move, drag, delete', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Blocks', kind: 'work', modules: ['home', 'pages'] } })).json()
  const pg = await (await request.post(`/api/v1/workspaces/${ws.id}/pages`, { headers: H, data: { title: 'Plan', content: 'First\n\nSecond\n\nThird' } })).json()
  const content = async () => ((await (await request.get(`/api/v1/pages/${pg.id}`, { headers: H })).json()) as { content: string }).content

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1300, height: 860 })
  await page.goto(`/w/${ws.id}/p/${pg.id}`)
  await page.getByRole('button', { name: 'Edit', exact: true }).click()
  // The visual editor (an earlier test left Markdown chosen).
  await page.getByRole('tab', { name: 'Visual' }).click()
  await expect(page.locator('.ProseMirror[contenteditable="true"]')).toBeVisible()
  const block = (text: string) => page.locator('.ProseMirror > *', { hasText: text })
  const menuOf = async (text: string) => {
    await block(text).hover()
    await page.getByTestId('block-grip').click()
    await expect(page.getByTestId('block-menu')).toBeVisible()
  }

  // Second becomes a heading, then a green callout.
  await menuOf('Second')
  await page.getByTestId('turn-h2').click()
  await expect.poll(content).toBe('First\n\n## Second\n\nThird')
  await menuOf('Second')
  await page.getByTestId('colour-TIP').click()
  await expect.poll(content).toBe('First\n\n> [!TIP]\n> ## Second\n\nThird')
  // No colour: out of the callout again.
  await menuOf('Second')
  await page.getByTestId('colour-none').click()
  await expect.poll(content).toBe('First\n\n## Second\n\nThird')

  // Third duplicated and moved up.
  await menuOf('Third')
  await page.getByTestId('block-duplicate').click()
  await expect.poll(content).toBe('First\n\n## Second\n\nThird\n\nThird')
  await menuOf('First')
  await page.getByTestId('block-down').click()
  await expect.poll(content).toBe('## Second\n\nFirst\n\nThird\n\nThird')

  // Dragged by its handle: First to the end.
  await block('First').hover()
  await page.getByTestId('block-grip').dragTo(page.locator('.ProseMirror > *').last(), { targetPosition: { x: 40, y: 20 } })
  await expect.poll(content).toBe('## Second\n\nThird\n\nThird\n\nFirst')

  // Deleted.
  await menuOf('First')
  await page.getByTestId('block-delete').click()
  await expect.poll(content).toBe('## Second\n\nThird\n\nThird')

  // + adds a block below with the / menu open.
  await block('Second').hover()
  await page.getByTestId('block-add').click()
  await page.keyboard.type('quote')
  // The menu filtered down to Quote first.
  await expect(page.locator('.slash-menu [role="option"]').first()).toContainText('Quote')
  await page.keyboard.press('Enter')
  await page.keyboard.type('Wise words')
  await expect.poll(content).toBe('## Second\n\n> Wise words\n\nThird\n\nThird')
  await page.getByRole('tab', { name: 'Markdown' }).click()
})

test('mentions: @ links a page and a date (in both editors); the page shows what links to it', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Wiki', kind: 'work', modules: ['home', 'pages'] } })).json()
  const mk = async (data: object) => (await request.post(`/api/v1/workspaces/${ws.id}/pages`, { headers: H, data })).json()
  const content = async (id: string) => ((await (await request.get(`/api/v1/pages/${id}`, { headers: H })).json()) as { content: string }).content
  const roadmap = await mk({ title: 'Roadmap', content: 'Plans.' })
  const notes = await mk({ title: 'Notes' })

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1200, height: 820 })
  await page.goto(`/w/${ws.id}/p/${notes.id}`)
  await page.getByRole('tab', { name: 'Visual' }).click()
  await page.locator('.ProseMirror').click()
  await page.keyboard.type('See @road')
  await expect(page.locator('.slash-menu')).toContainText('Roadmap')
  await page.keyboard.press('Enter')
  await page.keyboard.type('by @tomo')
  await expect(page.locator('.slash-menu')).toContainText('Tomorrow')
  await page.keyboard.press('Enter')
  await expect(page.locator('.ProseMirror .md-date')).toBeVisible()
  await expect.poll(() => content(notes.id)).toMatch(new RegExp(`^See \\[Roadmap\\]\\(/w/${ws.id}/p/${roadmap.id}\\) by @\\d{4}-\\d{2}-\\d{2}`))

  // The Markdown editor offers the same.
  await page.getByRole('tab', { name: 'Markdown' }).click()
  const src = page.getByTestId('md-source')
  await src.click()
  await src.press('End')
  await page.keyboard.press('Enter')
  await page.keyboard.press('Enter')
  await page.keyboard.type('Also @roa')
  await expect(page.locator('.slash-menu')).toContainText('Roadmap')
  await page.keyboard.press('Enter')
  await expect.poll(() => content(notes.id)).toContain(`Also [Roadmap](/w/${ws.id}/p/${roadmap.id})`)

  // Reading: the date is a chip; the page link opens here; Roadmap shows Notes links to it.
  await page.getByRole('button', { name: 'Done' }).click()
  await expect(page.locator('.md-view a.md-date')).toBeVisible()
  await page.locator('.md-view a.md-page').first().click()
  await page.waitForURL(new RegExp(`/p/${roadmap.id}$`))
  await expect(page.getByTestId('backlinks')).toContainText('Notes')
  await page.screenshot({ path: 'test-results/mentions.png' })
})

test('charts: a chart downloads as a picture and as numbers; the workload shows who has what', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  await request.post('/api/v1/users', { headers: H, data: { username: 'rui', display_name: 'Rui', password: 'rui-e2e-password-42' } })
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Crew', kind: 'work', modules: ['home', 'tasks', 'charts'] } })).json()
  await request.post(`/api/v1/workspaces/${ws.id}/members`, { headers: H, data: { user: 'rui', role: 'editor' } })
  const members = await (await request.get(`/api/v1/workspaces/${ws.id}/members`, { headers: H })).json()
  const rui = members.find((m: { username: string }) => m.username === 'rui').user_id
  const lists = await (await request.get(`/api/v1/workspaces/${ws.id}/lists`, { headers: H })).json()
  const yesterday = new Date(Date.now() - 86400000).toISOString().slice(0, 10)
  for (const title of ['Fix the sink', 'Paint the door']) {
    await request.post(`/api/v1/workspaces/${ws.id}/tasks`, { headers: H, data: { title, list_id: lists[0].id, assignee_ids: [rui], due: yesterday } })
  }

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1200, height: 900 })
  await page.goto(`/w/${ws.id}/charts`)
  // Rui has two late tasks.
  const row = page.getByTestId('workload-row').filter({ hasText: 'Rui' })
  await expect(row.locator('.work-cell').first()).toHaveText('2')

  const card = page.getByTestId('workload')
  await card.getByTestId('chart-export').click()
  const [png] = await Promise.all([page.waitForEvent('download'), page.getByTestId('chart-png').click()])
  expect(png.suggestedFilename()).toMatch(/^Workload-\d{4}-\d{2}-\d{2}\.png$/)
  const bytes = await (await import('node:fs/promises')).readFile(await png.path())
  expect(bytes.subarray(1, 4).toString()).toBe('PNG')
  await card.getByTestId('chart-export').click()
  const [csv] = await Promise.all([page.waitForEvent('download'), page.getByTestId('chart-csv').click()])
  const text = (await (await import('node:fs/promises')).readFile(await csv.path())).toString()
  expect(text).toContain('Rui,2,0,0,0,0,0,2')

  // Every task shown, as CSV.
  const [all] = await Promise.all([page.waitForEvent('download'), page.getByTestId('tasks-csv').click()])
  const rows = (await (await import('node:fs/promises')).readFile(await all.path())).toString()
  expect(rows).toContain('Fix the sink')
  expect(rows).toContain('Rui')

  // A cell opens those tasks.
  await row.locator('.work-cell').first().click()
  await page.waitForURL(/\/tasks\/all/)
  await expect(page.locator('.task-row', { hasText: 'Paint the door' })).toBeVisible()
  await page.getByRole('button', { name: 'Clear filter' }).first().click()
  await page.goto(`/w/${ws.id}/charts`)
  await expect(page.getByTestId('workload')).toBeVisible()
  await page.getByTestId('workload').scrollIntoViewIfNeeded()
  await page.screenshot({ path: 'test-results/charts-workload.png' })
  await (await import('node:fs/promises')).copyFile(await png.path(), 'test-results/workload-export.png')
})

test('timeline dependencies: a bar linked to another by dragging; lengthening the first moves the second', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'House', kind: 'work', modules: ['home', 'tasks', 'timeline'] } })).json()
  const lists = await (await request.get(`/api/v1/workspaces/${ws.id}/lists`, { headers: H })).json()
  const day = (n: number) => {
    const d = new Date()
    d.setDate(d.getDate() + n)
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
  }
  const mk = async (title: string, start: string, due: string) =>
    (await request.post(`/api/v1/workspaces/${ws.id}/tasks`, { headers: H, data: { title, list_id: lists[0].id, start, due } })).json()
  const base = await mk('Pour the base', day(1), day(2))
  const walls = await mk('Build the walls', day(3), day(5))
  const get = async (id: string) => {
    const all = await (await request.get(`/api/v1/workspaces/${ws.id}/tasks`, { headers: H })).json()
    return all.find((x: { id: string }) => x.id === id)
  }

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1300, height: 820 })
  await page.goto(`/w/${ws.id}/timeline`)
  await page.getByRole('tab', { name: 'Weeks' }).click()
  const barA = page.getByRole('button', { name: /^Pour the base: / })
  const barB = page.getByRole('button', { name: /^Build the walls: / })
  await barA.hover()
  const dot = barA.getByTestId('tl-link')
  const a = (await dot.boundingBox())!
  const b = (await barB.boundingBox())!
  await page.mouse.move(a.x + a.width / 2, a.y + a.height / 2)
  await page.mouse.down()
  await page.mouse.move(b.x + b.width / 2, b.y + b.height / 2, { steps: 10 })
  await page.mouse.up()
  await expect(page.getByTestId('tl-arrow')).toHaveCount(1)
  await expect.poll(async () => (await get(walls.id)).waits_on).toEqual([base.id])

  // One more day for the base: the walls move a day too.
  await barA.focus()
  await page.keyboard.press('Shift+ArrowRight')
  await expect.poll(async () => (await get(walls.id)).start).toBe(day(4))
  expect((await get(walls.id)).due).toBe(day(6))
  await expect(page.getByTestId('tl-arrow')).toHaveCount(1)
  await page.screenshot({ path: 'test-results/timeline-deps.png' })
})

test('import from Notion and Apple Notes; a page out as PDF', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Imports', kind: 'work', modules: ['home', 'pages'] } })).json()
  const { zipSync } = await import('fflate')
  const u8 = (s: string) => new Uint8Array(Buffer.from(s))
  const png = new Uint8Array(Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==', 'base64'))
  const A = '0123456789abcdef0123456789abcdef'
  const B = 'fedcba9876543210fedcba9876543210'
  const notion = zipSync({
    [`Garden ${A}.md`]: u8(`# Garden\n\nSee [Roses](Garden%20${A}/Roses%20${B}.md).\n\n![bed](Garden%20${A}/bed.png)`),
    [`Garden ${A}/Roses ${B}.md`]: u8('# Roses\n\n- [ ] prune'),
    [`Garden ${A}/bed.png`]: png,
    [`Books ${B}.csv`]: u8('Name,Status,Pages\nDune,Read,412\nEmma,To read,474\n'),
    [`Books ${B}/Dune ${A}.md`]: u8('# Dune\n\nStatus: Read\nPages: 412\n\nGreat.'),
  })

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1200, height: 820 })
  await page.goto(`/w/${ws.id}/pages`)
  await page.getByTestId('pages-import').click()
  await page.getByTestId('import-input').setInputFiles([
    { name: 'Export.zip', mimeType: 'application/zip', buffer: Buffer.from(notion) },
    { name: 'Shopping.html', mimeType: 'text/html', buffer: Buffer.from('<html><head><title>Shopping</title></head><body><div><b>Saturday</b></div><ul class="checklist"><li class="checked">Milk</li><li>Bread</li></ul></body></html>') },
  ])
  await expect(page.getByTestId('import-result')).toContainText('6 pages and 1 file imported')
  await page.keyboard.press('Escape')

  const tree = (await (await request.get(`/api/v1/workspaces/${ws.id}/pages`, { headers: H })).json()) as { id: string; title: string; parent_id: string | null }[]
  const by = Object.fromEntries(tree.map((p) => [p.title, p]))
  expect(by.Roses.parent_id).toBe(by.Garden.id)
  const garden = await (await request.get(`/api/v1/pages/${by.Garden.id}`, { headers: H })).json()
  expect(garden.content).toContain(`[Roses](/w/${ws.id}/p/${by.Roses.id})`)
  expect(garden.content).toMatch(/!\[bed\]\(\/api\/v1\/uploads\/[^)]+\)/)
  const shopping = await (await request.get(`/api/v1/pages/${by.Shopping.id}`, { headers: H })).json()
  expect(shopping.content).toBe('**Saturday**\n\n- [x] Milk\n- [ ] Bread')
  // Notion's database: a database, its rows with their values (Emma had no page).
  const books = await (await request.get(`/api/v1/pages/${by.Books.id}`, { headers: H })).json()
  expect(books.db.props.map((p: { name: string; type: string }) => `${p.name}:${p.type}`)).toEqual(['Status:select', 'Pages:number'])
  const dune = await (await request.get(`/api/v1/pages/${by.Dune.id}`, { headers: H })).json()
  expect(dune.parent_id).toBe(by.Books.id)
  expect(dune.content).toBe('Great.')
  expect(Object.values(dune.props)).toContain(412)
  expect(by.Emma.parent_id).toBe(by.Books.id)

  // The page out as a PDF file, downloaded (no print dialog), its picture in.
  await page.goto(`/w/${ws.id}/p/${by.Garden.id}`)
  await expect(page.locator('.md-view img')).toBeVisible()
  await page.getByRole('button', { name: 'Page options' }).click()
  await page.getByTestId('page-pdf').click()
  const [pdf] = await Promise.all([page.waitForEvent('download'), page.getByTestId('pdf-drawn').click()])
  expect(pdf.suggestedFilename()).toBe('Garden.pdf')
  const bytes = await (await import('node:fs/promises')).readFile(await pdf.path())
  expect(bytes.subarray(0, 5).toString()).toBe('%PDF-')
  expect(bytes.toString('latin1')).toContain('/Subtype /Image')
})

test('databases: a page of rows with properties, edited in place, filtered, on a board', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Shelf', kind: 'personal', modules: ['home', 'pages'] } })).json()
  const list = await (await request.post(`/api/v1/workspaces/${ws.id}/pages`, { headers: H, data: { title: 'Reading list' } })).json()

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1300, height: 860 })
  await page.goto(`/w/${ws.id}/p/${list.id}`)
  await page.getByTestId('make-database').click()
  const dbv = page.getByTestId('database')
  await expect(dbv.getByTestId('db-col')).toHaveText(['Status'])

  // A number property.
  await dbv.getByTestId('db-add-prop').click()
  const menu = page.getByTestId('prop-menu')
  await menu.getByRole('textbox', { name: 'Property name' }).fill('Pages')
  await menu.getByTestId('prop-type').selectOption('number')
  await menu.getByTestId('prop-done').click()
  await expect(dbv.getByTestId('db-col')).toHaveText(['Status', 'Pages'])

  // Two rows; values typed and picked in place.
  await dbv.getByTestId('db-new-row').click()
  for (const name of ['Dune', 'Emma']) {
    await dbv.getByTestId('db-row-input').fill(name)
    await dbv.getByTestId('db-row-input').press('Enter')
    await expect(dbv.getByTestId('db-row').filter({ hasText: name })).toBeVisible()
  }
  await page.keyboard.press('Escape')
  const dune = dbv.getByTestId('db-row').filter({ hasText: 'Dune' })
  await dune.getByRole('textbox', { name: 'Pages' }).fill('412')
  await dune.getByRole('textbox', { name: 'Pages' }).press('Enter')
  await dune.getByRole('button', { name: 'Status' }).click()
  await page.getByTestId('option-picker').getByRole('button', { name: 'Done' }).click()
  await expect(dune).toContainText('Done')

  // Filter: Status is Done.
  await dbv.getByTestId('db-filter').click()
  await page.getByTestId('db-add-filter').click()
  await page.getByTestId('db-filters').getByRole('combobox', { name: 'Value' }).selectOption({ label: 'Done' })
  await expect(dbv.getByTestId('db-row')).toHaveCount(1)
  await page.keyboard.press('Escape')

  // A formula, as in Excel, and a total under a column.
  await dbv.getByTestId('db-add-prop').click()
  await menu.getByRole('textbox', { name: 'Property name' }).fill('Days')
  await menu.getByTestId('prop-type').selectOption('formula')
  await menu.getByTestId('prop-formula').fill('=ROUND([@Pages]/50, 1)')
  await menu.getByTestId('prop-done').click()
  await expect(dune.getByTestId('db-formula')).toHaveText('8.2')
  await dbv.getByTestId('db-total-Pages').selectOption('sum')
  await expect(dbv.getByTestId('db-totals')).toContainText('412')

  // A formula made by clicking: this row's Pages, divided by the column's sum.
  await dbv.getByTestId('db-add-prop').click()
  await menu.getByRole('textbox', { name: 'Property name' }).fill('Share')
  await menu.getByTestId('prop-type').selectOption('formula')
  await menu.getByTestId('formula-prop-Pages').click()
  await page.getByTestId('formula-pick').getByRole('button', { name: /This row/ }).click()
  await menu.getByTestId('prop-formula').press('End')
  await page.keyboard.type('/')
  await menu.getByTestId('formula-prop-Pages').click()
  await page.getByTestId('formula-pick').getByRole('button', { name: /Sum of the column/ }).click()
  await expect(menu.getByTestId('prop-formula')).toHaveValue('[@Pages]/SUM([Pages])')
  await menu.getByTestId('prop-done').click()
  const share = dune.getByTestId('db-formula').last()
  await expect(share).toHaveText('1')
  await expect(share).toHaveCSS('text-align', 'right')
  await page.screenshot({ path: 'test-results/formula-share.png' })

  // A board by Status.
  await dbv.getByTestId('db-add-view').click()
  await page.getByTestId('db-add-board').click()
  const done = dbv.getByTestId('db-board-col').filter({ hasText: 'Done' })
  await expect(done.getByTestId('db-card')).toHaveText([/Dune/])

  // A row's page shows its values.
  await done.getByTestId('db-card').first().click()
  await expect(page.getByTestId('row-props')).toContainText('Pages')
  await expect(page.getByTestId('row-props').getByRole('textbox', { name: 'Pages' })).toHaveValue('412')
  await page.goBack()
  await page.screenshot({ path: 'test-results/database.png' })
})

test('sheets: values and formulas typed and clicked, a fill, a row inserted, saved, out as .xlsx and back', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Money', kind: 'personal', modules: ['home', 'sheets'] } })).json()

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1300, height: 900 })
  await page.goto(`/w/${ws.id}/sheets`)
  await page.getByTestId('new-sheet').first().click()
  await page.getByTestId('sheet-title').fill('Budget')
  await page.getByTestId('sheet-title').press('Enter')
  const cell = (a: string) => page.locator(`[data-addr="${a}"]`)

  // Values typed in: Enter goes down.
  await cell('A1').click()
  for (const v of ['Rent', 'Food', 'Total']) {
    await page.keyboard.type(v)
    await page.keyboard.press('Enter')
  }
  await cell('B1').click()
  await page.keyboard.type('650')
  await page.keyboard.press('Enter')
  await page.keyboard.type('300')
  await page.keyboard.press('Enter')
  // Function suggestions: "=SU" offers SUM; Tab puts it in; then its arguments show.
  await page.keyboard.type('=SU')
  await expect(page.getByTestId('formula-suggestion').first()).toContainText('SUM')
  await page.keyboard.press('Tab')
  await expect(page.getByTestId('sheet-cell-editor')).toHaveValue('=SUM(')
  await expect(page.getByTestId('formula-hint')).toContainText('number1')
  await page.keyboard.type('B1:B2)')
  await page.keyboard.press('Enter')
  await expect(cell('B3')).toHaveText('950')

  // A formula made by clicking a cell.
  await cell('C1').click()
  await page.keyboard.type('=')
  await cell('B1').click()
  await page.keyboard.type('*2')
  await page.keyboard.press('Enter')
  await expect(cell('C1')).toHaveText('1300')
  await cell('C1').click()
  await expect(page.getByTestId('sheet-bar')).toHaveValue('=B1*2')

  // The fill handle drags it down: references move.
  const handle = page.getByTestId('sheet-fill')
  const hb = (await handle.boundingBox())!
  const c2 = (await cell('C2').boundingBox())!
  await page.mouse.move(hb.x + hb.width / 2, hb.y + hb.height / 2)
  await page.mouse.down()
  await page.mouse.move(c2.x + c2.width / 2, c2.y + c2.height / 2, { steps: 5 })
  await page.mouse.up()
  await expect(cell('C2')).toHaveText('600')

  // A row inserted above row 1: the total's formula follows.
  await page.locator('.sh-rowhead', { hasText: /^1$/ }).first().click({ button: 'right' })
  await page.getByTestId('sheet-head-menu').getByRole('button', { name: 'Insert a row above' }).click()
  await expect(cell('B4')).toHaveText('950')
  await cell('B4').click()
  await expect(page.getByTestId('sheet-bar')).toHaveValue('=SUM(B2:B3)')
  // Undo takes it back.
  await page.keyboard.press('ControlOrMeta+z')
  await expect(cell('B3')).toHaveText('950')

  // Saved: still there after a reload.
  await expect(page.getByTestId('sheet-state')).toHaveText(/Saved/)
  await page.reload()
  await expect(cell('B3')).toHaveText('950')
  await expect(cell('C2')).toHaveText('600')

  // Out as Excel, and back in as a new sheet.
  await page.getByTestId('sheet-menu').click()
  const [xlsx] = await Promise.all([page.waitForEvent('download'), page.getByTestId('sheet-xlsx').click()])
  expect(xlsx.suggestedFilename()).toBe('Budget.xlsx')
  const bytes = await (await import('node:fs/promises')).readFile(await xlsx.path())
  expect(bytes.subarray(0, 2).toString()).toBe('PK')
  await page.screenshot({ path: 'test-results/sheet.png' })
  await page.goto(`/w/${ws.id}/sheets`)
  await page.getByTestId('sheet-import-input').setInputFiles({ name: 'Copy.xlsx', mimeType: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', buffer: bytes })
  await page.waitForURL(/\/s\//)
  await expect(page.getByTestId('sheet-title')).toHaveValue('Copy')
  await expect(cell('B3')).toHaveText('950')
  await expect(cell('C2')).toHaveText('600')

  // Its link alone on a line of a page shows the sheet there.
  const sheetId = new URL(page.url()).pathname.split('/').pop()
  const pg = await (await request.post(`/api/v1/workspaces/${ws.id}/pages`, { headers: H, data: { title: 'Plans', content: `Our money:\n\n[Copy](/w/${ws.id}/s/${sheetId})\n\nThat's all.` } })).json()
  await page.goto(`/w/${ws.id}/p/${pg.id}`)
  const embed = page.getByTestId('sheet-embed')
  await expect(embed).toContainText('Copy')
  await expect(embed.locator('td', { hasText: '950' })).toBeVisible()
  await page.screenshot({ path: 'test-results/sheet-embed.png' })
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

test('tasks: sub-tasks fold under their task, with a bar of how many are done', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Essays', kind: 'personal', modules: ['home', 'tasks'] } })).json()
  const parent = await (await request.post(`/api/v1/workspaces/${ws.id}/tasks`, { headers: H, data: { title: 'History essay' } })).json()
  for (const title of ['Find sources', 'Write the draft']) {
    expect((await request.post(`/api/v1/workspaces/${ws.id}/tasks`, { headers: H, data: { title, parent_id: parent.id } })).ok()).toBeTruthy()
  }

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1300, height: 860 })
  await page.goto(`/w/${ws.id}/tasks/all`)
  const fold = page.getByTestId('subtasks-toggle')
  await expect(fold).toHaveText('0/2')
  await expect(fold).toHaveAttribute('aria-expanded', 'false')
  await expect(page.getByText('Find sources')).toHaveCount(0)

  await fold.click()
  await expect(fold).toHaveAttribute('aria-expanded', 'true')
  await page.getByRole('checkbox', { name: 'Find sources' }).click()
  await expect(fold).toHaveText('1/2')
  await expect(fold.getByRole('progressbar')).toHaveAttribute('aria-valuenow', '1')

  // Open stays open, here.
  await page.reload()
  await expect(page.getByText('Write the draft')).toBeVisible()
  await page.screenshot({ path: 'test-results/subtasks.png' })
  await fold.click()
  await expect(page.getByText('Write the draft')).toHaveCount(0)
})

test('databases: rows out as Excel and CSV, rows in from a CSV', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Library', kind: 'personal', modules: ['home', 'pages'] } })).json()
  const list = await (await request.post(`/api/v1/workspaces/${ws.id}/pages`, { headers: H, data: { title: 'Books' } })).json()

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1300, height: 860 })
  await page.goto(`/w/${ws.id}/p/${list.id}`)
  await page.getByTestId('make-database').click()
  const dbv = page.getByTestId('database')
  await expect(dbv.getByTestId('db-col')).toHaveText(['Status'])

  // Rows in: Status matched by name (an option added), Pages and Read new.
  await dbv.getByTestId('db-sheet-menu').click()
  await page.getByTestId('db-import-input').setInputFiles({
    name: 'books.csv',
    mimeType: 'text/csv',
    buffer: Buffer.from('Name,Status,Pages,Read\nDune,Done,412,2026-03-01\nEmma,Shelved,300,\n'),
  })
  await expect(dbv.getByTestId('db-row')).toHaveCount(2)
  await expect(dbv.getByTestId('db-col')).toHaveText(['Status', 'Pages', 'Read'])
  const dune = dbv.getByTestId('db-row').filter({ hasText: 'Dune' })
  await expect(dune).toContainText('Done')
  await expect(dune.getByRole('textbox', { name: 'Pages' })).toHaveValue('412')
  await expect(dbv.getByTestId('db-row').filter({ hasText: 'Emma' })).toContainText('Shelved')

  // Rows out.
  await dbv.getByTestId('db-sheet-menu').click()
  await page.screenshot({ path: 'test-results/db-sheet-menu.png' })
  const [csv] = await Promise.all([page.waitForEvent('download'), page.getByTestId('db-csv').click()])
  expect(csv.suggestedFilename()).toBe('Books.csv')
  const text = (await (await import('node:fs/promises')).readFile(await csv.path())).toString()
  expect(text).toContain('Name,Status,Pages,Read')
  expect(text).toContain('Dune,Done,412,2026-03-01')
  await dbv.getByTestId('db-sheet-menu').click()
  const [xlsx] = await Promise.all([page.waitForEvent('download'), page.getByTestId('db-xlsx').click()])
  expect(xlsx.suggestedFilename()).toBe('Books.xlsx')
  const bytes = await (await import('node:fs/promises')).readFile(await xlsx.path())
  expect(bytes.subarray(0, 2).toString()).toBe('PK')
})

test('diagrams: draw.io in a frame saves the file, a picture and its words; a file comes in; a page shows one', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Ops', kind: 'personal', modules: ['home', 'diagrams', 'pages'] } })).json()
  // A stand-in for draw.io (no internet here): it speaks the same messages.
  await page.route('https://embed.diagrams.net/**', (route) =>
    route.fulfill({
      contentType: 'text/html',
      body: `<button id="draw">Draw</button><script>
        let xml = ''
        const say = (m) => parent.postMessage(JSON.stringify(m), '*')
        addEventListener('message', (e) => {
          const m = JSON.parse(e.data)
          if (m.action === 'load') { xml = m.xml; say({ event: 'load', xml }) }
          if (m.action === 'export') say({ event: 'export', format: m.format, xml, data: m.format.includes('png')
            ? 'data:image/png;base64,iVBORw0KGgo='
            : 'data:image/svg+xml;base64,' + btoa('<svg xmlns="http://www.w3.org/2000/svg" style="color-scheme: light dark;" width="80" height="20"><text y="15">Order placed</text></svg>') })
        })
        document.getElementById('draw').onclick = () => {
          xml = '<mxfile><diagram name="Checkout"><mxGraphModel><root><mxCell id="2" value="Order &lt;b&gt;placed&lt;/b&gt;" vertex="1"/></root></mxGraphModel></diagram></mxfile>'
          say({ event: 'autosave', xml })
        }
        say({ event: 'init' })
      </script>`,
    }),
  )

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1300, height: 860 })
  await page.goto(`/w/${ws.id}/diagrams`)
  await page.getByTestId('new-diagram').click()
  await page.getByTestId('new-drawio').click()
  await expect(page.locator('.drawio-wait')).toBeHidden()
  await page.getByTestId('diagram-title').fill('Checkout')
  await page.getByTestId('diagram-title').press('Enter')
  await page.frameLocator('[data-testid="drawio-frame"]').getByRole('button', { name: 'Draw' }).click()
  const id = page.url().split('/').pop()!
  const get = async () => (await request.get(`/api/v1/diagrams/${id}`, { headers: H })).json()
  await expect.poll(async () => (await get()).data ?? '').toContain('Order &lt;b&gt;placed')
  await expect(page.getByTestId('diagram-state')).toHaveText(/Saved/)
  const saved = await get()
  expect(saved.title).toBe('Checkout')
  // The picture keeps its light colours.
  expect(saved.preview).toContain('color-scheme: light;')
  const hits = await (await request.get('/api/v1/search?q=placed', { headers: H })).json()
  expect(hits.map((h: { kind: string }) => h.kind)).toContain('diagram')

  // Out as a draw.io file and a PNG.
  await page.getByTestId('diagram-menu').click()
  const [file] = await Promise.all([page.waitForEvent('download'), page.getByTestId('diagram-drawio').click()])
  expect(file.suggestedFilename()).toBe('Checkout.drawio')
  expect((await (await import('node:fs/promises')).readFile(await file.path())).toString()).toContain('<mxfile>')
  await page.getByTestId('diagram-menu').click()
  const [png] = await Promise.all([page.waitForEvent('download'), page.getByTestId('diagram-png').click()])
  expect((await (await import('node:fs/promises')).readFile(await png.path())).subarray(1, 4).toString()).toBe('PNG')

  // A draw.io file comes in as a new diagram.
  await page.goto(`/w/${ws.id}/diagrams`)
  await expect(page.getByTestId('diagram-card').locator('img')).toBeVisible()
  await page.getByTestId('diagram-import-input').setInputFiles({
    name: 'Network.drawio',
    mimeType: 'application/xml',
    buffer: Buffer.from('<mxfile><diagram name="LAN"><mxGraphModel><root><mxCell id="2" value="Router" vertex="1"/></root></mxGraphModel></diagram></mxfile>'),
  })
  await expect(page.getByTestId('diagram-title')).toHaveValue('Network')
  await page.goto(`/w/${ws.id}/diagrams`)
  await expect(page.getByTestId('diagram-card')).toHaveCount(2)

  // A page shows the diagram from its link.
  const pg = await (await request.post(`/api/v1/workspaces/${ws.id}/pages`, { headers: H, data: { title: 'Flows', content: `How orders go:\n\n[Checkout](/w/${ws.id}/d/${id})\n` } })).json()
  await page.goto(`/w/${ws.id}/p/${pg.id}`)
  await expect(page.getByTestId('diagram-embed')).toContainText('Checkout')
  await expect(page.getByTestId('diagram-embed').locator('img')).toBeVisible()
})

test('files: sheets, diagrams and whiteboards in folders inside folders — moved, dragged, renamed, copied, a folder deleted keeps what it held', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Studio', kind: 'personal', starter: false, modules: ['home', 'files', 'sheets', 'diagrams', 'whiteboards'] } })).json()
  const base = `/api/v1/workspaces/${ws.id}`
  await request.post(`${base}/sheets`, { headers: H, data: { title: 'Budget', data: { tabs: [] } } })
  await request.post(`${base}/diagrams`, { headers: H, data: { title: 'Flow', data: '<mxfile><diagram name="A"><mxGraphModel><root/></mxGraphModel></diagram></mxfile>', preview: '<svg xmlns="http://www.w3.org/2000/svg" width="40" height="20"><rect width="40" height="20" fill="#ff9500"/></svg>' } })
  await request.post(`${base}/drawings`, { headers: H, data: { title: 'Board' } })

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1400, height: 900 })
  await page.goto(`/w/${ws.id}/files`)
  await expect(page.getByTestId('sheet-row')).toHaveCount(1)
  await expect(page.getByTestId('diagram-card')).toHaveCount(1)
  await expect(page.getByTestId('whiteboard-card')).toHaveCount(1)

  // A folder, and one inside it.
  const newFolder = async (name: string) => {
    await page.getByTestId('new-folder').click()
    await page.getByTestId('folder-name').fill(name)
    await page.getByTestId('folder-name').press('Enter')
    await expect(page.getByTestId('folder-tile').filter({ hasText: name })).toBeVisible()
  }
  await newFolder('Design')
  await page.getByTestId('folder-tile').filter({ hasText: 'Design' }).click()
  await expect(page).toHaveURL(/folder=/)
  await expect(page.getByTestId('files-empty')).toBeVisible()
  await newFolder('Logos')
  await expect(page.getByTestId('crumb')).toHaveText(['Files', 'Design'])
  await page.getByTestId('crumb').first().click()

  // The diagram moved with "Move to…", the sheet dragged onto the folder.
  await page.getByTestId('diagram-card').getByTestId('file-more').click()
  await page.getByTestId('file-move').click()
  await page.getByTestId('move-target').filter({ hasText: 'Design' }).click()
  await page.getByTestId('move-here').click()
  await expect(page.getByTestId('diagram-card')).toHaveCount(0)
  await page.getByTestId('sheet-row').dragTo(page.getByTestId('folder-tile').filter({ hasText: 'Design' }))
  await expect(page.getByTestId('sheet-row')).toHaveCount(0)
  await expect(page.getByTestId('folder-tile').filter({ hasText: 'Design' })).toContainText('1 folder · 1 sheet · 1 diagram')
  const folders = (await (await request.get(`${base}/folders`, { headers: H })).json()) as { id: string; name: string; parent_id: string }[]
  const design = folders.find((f) => f.name === 'Design')!
  expect(folders.find((f) => f.name === 'Logos')!.parent_id).toBe(design.id)
  const sheets = await (await request.get(`${base}/sheets`, { headers: H })).json()
  expect(sheets[0].folder_id).toBe(design.id)

  // A list, and the details of one.
  await page.getByTestId('view-list').click()
  await expect(page.locator('table.fb-list')).toBeVisible()
  await page.getByTestId('whiteboard-card').getByTestId('file-more').click()
  await page.getByTestId('file-info').click()
  await expect(page.getByTestId('files-details')).toContainText('Whiteboard')
  await expect(page.getByTestId('files-details')).toContainText('Files')
  await page.getByTestId('view-grid').click()

  // Renamed; then deleted — what it held moves up.
  await page.getByTestId('folder-tile').filter({ hasText: 'Design' }).getByTestId('folder-more').click()
  await page.getByTestId('folder-rename').click()
  await page.getByTestId('rename-input').fill('Design 2026')
  await page.getByTestId('rename-input').press('Enter')
  await expect(page.getByTestId('folder-tile').filter({ hasText: 'Design 2026' })).toBeVisible()
  await page.getByTestId('folder-tile').filter({ hasText: 'Design 2026' }).getByTestId('folder-more').click()
  await page.getByTestId('folder-delete').click()
  await page.getByRole('button', { name: 'Delete', exact: true }).click()
  await expect(page.getByTestId('folder-tile')).toHaveText([/Logos/])
  await expect(page.getByTestId('sheet-row')).toHaveCount(1)
  await expect(page.getByTestId('diagram-card')).toHaveCount(1)

  // A copy; a draw.io file brought in, into the folder shown.
  await page.getByTestId('sheet-row').getByTestId('file-more').click()
  await page.getByTestId('file-duplicate').click()
  await expect(page.getByTestId('sheet-row')).toHaveCount(2)
  await expect(page.getByTestId('sheet-row').filter({ hasText: 'Budget (copy)' })).toBeVisible()
  await page.getByTestId('folder-tile').filter({ hasText: 'Logos' }).click()
  await page.getByTestId('files-upload-input').setInputFiles({ name: 'Network.drawio', mimeType: 'application/xml', buffer: Buffer.from('<mxfile><diagram name="LAN"><mxGraphModel><root><mxCell id="2" value="Router" vertex="1"/></root></mxGraphModel></diagram></mxfile>') })
  await expect(page.getByTestId('diagram-card').filter({ hasText: 'Network' })).toBeVisible()
  await page.screenshot({ path: 'test-results/files.png' })

  // The module shows its own kind, in the same folders.
  await page.goto(`/w/${ws.id}/diagrams`)
  await expect(page.getByTestId('folder-tile')).toHaveText([/Logos/])
  await expect(page.getByTestId('diagram-card')).toHaveCount(1)
  await expect(page.getByTestId('sheet-row')).toHaveCount(0)
})

test('mermaid: text and drawing side by side, a node added and two joined; pages show diagrams, sheets and whiteboards, edited over the page', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Embeds', kind: 'work', modules: ['home', 'pages', 'sheets', 'diagrams', 'whiteboards'] } })).json()

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1300, height: 860 })
  await page.goto(`/w/${ws.id}/diagrams`)
  await page.getByTestId('new-diagram').click()
  await page.getByTestId('new-mermaid').click()
  const drawing = page.getByTestId('mermaid-drawing')
  await expect(drawing.locator('svg .node')).toHaveCount(4)
  await page.getByTestId('diagram-title').fill('Steps')
  await page.getByTestId('diagram-title').press('Enter')
  const id = page.url().split('/').pop()!
  const get = async () => (await request.get(`/api/v1/diagrams/${id}`, { headers: H })).json()

  // "+ Node" joined to the box picked; "Connect" from one box to another.
  await drawing.locator('.node[id*="flowchart-C-"]').click()
  await page.getByTestId('mermaid-add-node').click()
  await page.getByTestId('mermaid-node-label').fill('Ship it')
  await page.getByTestId('mermaid-node-label').press('Enter')
  await expect(drawing.locator('svg .node')).toHaveCount(5)
  await page.getByTestId('mermaid-connect').click()
  await drawing.locator('.node[id*="flowchart-A-"]').click()
  await drawing.locator('.node[id*="flowchart-N1-"]').click()
  await expect.poll(async () => (await get()).data ?? '').toContain('A --> N1')
  const saved = await get()
  expect(saved.kind).toBe('mermaid')
  expect(saved.data).toContain('C --> N1[Ship it]')
  expect(saved.preview).toMatch(/^<svg/)
  const hits = await (await request.get('/api/v1/search?q=ship', { headers: H })).json()
  expect(hits.map((h: { id: string }) => h.id)).toContain(id)
  // Out as Mermaid's text.
  await page.getByTestId('diagram-menu').click()
  const [file] = await Promise.all([page.waitForEvent('download'), page.getByTestId('diagram-drawio').click()])
  expect(file.suggestedFilename()).toBe('Steps.mmd')
  // A mistake in the text says what's wrong.
  await page.getByTestId('mermaid-code').click()
  await page.keyboard.press('ControlOrMeta+End')
  await page.keyboard.type('\n  B -->')
  await expect(page.getByTestId('mermaid-error')).toBeVisible()
  await page.keyboard.press('ControlOrMeta+z')
  await expect(page.getByTestId('mermaid-error')).toBeHidden()

  // In a page: "/" makes a new Mermaid diagram and a sheet (each opens over
  // the page), and shows one there is.
  const pg = await (await request.post(`/api/v1/workspaces/${ws.id}/pages`, { headers: H, data: { title: 'Launch plan', content: 'Intro' } })).json()
  const content = async () => ((await (await request.get(`/api/v1/pages/${pg.id}`, { headers: H })).json()) as { content: string }).content
  await page.goto(`/w/${ws.id}/p/${pg.id}`)
  await page.getByRole('button', { name: 'Edit', exact: true }).click()
  await page.getByRole('tab', { name: 'Visual' }).click()
  await page.locator('.ProseMirror p', { hasText: 'Intro' }).click()
  await page.keyboard.press('ControlOrMeta+End')
  await page.keyboard.press('Enter')
  await page.keyboard.type('/mermaid')
  await page.keyboard.press('Enter')
  const over = page.getByTestId('embed-editor')
  await expect(over.getByTestId('mermaid-drawing').locator('svg .node')).toHaveCount(4)
  await expect(over.getByTestId('diagram-title')).toHaveValue('Launch plan')
  await over.getByTestId('embed-done').click()
  await expect(over).toBeHidden()
  await expect(page.locator('.ProseMirror').getByTestId('diagram-embed').locator('img')).toBeVisible()
  // In place of the line "/" was typed on: after Intro, with a line after it.
  const order = () => page.locator('.ProseMirror').evaluate((el) => [...el.children].map((c) => (c.querySelector('[data-embed]') ? 'embed' : c.textContent?.trim() || '·')).filter((x) => x !== '·').join(' '))
  expect(await order()).toBe('Intro embed')
  await page.locator('.ProseMirror > p').last().click()

  await page.keyboard.type('/sheet')
  await page.keyboard.press('Enter')
  await expect(over.getByTestId('sheet-title')).toHaveValue('Launch plan')
  await over.getByTestId('embed-done').click()
  await expect(page.locator('.ProseMirror').getByTestId('sheet-embed')).toBeVisible()
  await page.locator('.ProseMirror > p').last().click()

  await page.keyboard.type('/existing')
  await page.keyboard.press('Enter')
  await page.getByTestId('embed-pick-search').fill('step')
  await page.getByTestId('embed-pick-item').click()
  await expect(page.locator('.ProseMirror').getByTestId('diagram-embed')).toHaveCount(2)
  await expect.poll(content).toContain(`[Steps](/w/${ws.id}/d/${id})`)
  await page.locator('.ProseMirror > p').last().click()

  // A quick sketch, drawn over the page.
  await page.keyboard.type('/sketch')
  await page.keyboard.press('Enter')
  await expect(over.getByTestId('sketch')).toBeVisible()
  await over.getByTestId('embed-done').click()
  await expect(page.locator('.ProseMirror').getByTestId('whiteboard-embed')).toBeVisible()
  await expect.poll(content).toMatch(new RegExp(`\\[Launch plan\\]\\(/w/${ws.id}/whiteboards/[^)]+\\)`))
  const md = await content()
  expect(md).toMatch(new RegExp(`\\[Launch plan\\]\\(/w/${ws.id}/d/[^)]+\\)`))
  expect(md).toMatch(new RegExp(`\\[Launch plan\\]\\(/w/${ws.id}/s/[^)]+\\)`))

  // Reading: a whiteboard shows too; a double click opens it over the page,
  // to look at, or to draw on.
  const board = await (await request.post(`/api/v1/workspaces/${ws.id}/drawings`, { headers: H, data: { title: 'Sketches', preview: '<svg xmlns="http://www.w3.org/2000/svg" width="40" height="20"><rect width="40" height="20" fill="#af52de"/></svg>' } })).json()
  const cur = await (await request.get(`/api/v1/pages/${pg.id}`, { headers: H })).json()
  const costs = await (
    await request.post(`/api/v1/workspaces/${ws.id}/sheets`, {
      headers: H,
      data: { title: 'Costs', data: { tabs: [{ id: 't1', name: 'Sheet 1', cells: { A1: { v: 'Item', b: true }, B1: { v: 'Cost', b: true }, A2: { v: 'Venue' }, B2: { v: '1200' }, A3: { v: 'Food' }, B3: { v: '450.5' }, A5: { v: 'Total', b: true }, B5: { v: '=SUM(B2:B3)' }, D2: { v: 'paid' } } }] } },
    })
  ).json()
  const put = await request.put(`/api/v1/pages/${pg.id}`, { headers: H, data: { content: `${md}\n\n[Sketches](/w/${ws.id}/whiteboards/${board.id})\n\n[Costs](/w/${ws.id}/s/${costs.id})\n`, version: cur.version } })
  expect(put.ok()).toBe(true)
  await page.goto(`/w/${ws.id}/p/${pg.id}`)
  await expect(page.getByTestId('whiteboard-embed').last().locator('img')).toBeVisible()
  await page.getByTestId('whiteboard-embed').last().dblclick()
  await expect(over.getByTestId('whiteboard-title')).toHaveValue('Sketches')
  await expect(over.getByTestId('whiteboard-view-mode')).toHaveAttribute('aria-pressed', 'true')
  await over.getByTestId('whiteboard-edit-mode').click()
  await expect(over.getByTestId('whiteboard-edit-mode')).toHaveAttribute('aria-pressed', 'true')
  await over.getByTestId('embed-done').click()
  await expect(over).toBeHidden()
  // Still reading (the double click didn't start editing the page).
  await expect(page.getByRole('button', { name: 'Edit', exact: true })).toBeVisible()
  await page.getByTestId('sheet-embed').first().dblclick()
  await expect(over.getByTestId('sheet-title')).toHaveValue('Launch plan')
  await over.getByTestId('embed-done').click()

  // As a PDF: the diagrams' and the whiteboard's pictures drawn in.
  await page.getByRole('button', { name: 'Page options' }).click()
  await page.getByTestId('page-pdf').click()
  const [pdf] = await Promise.all([page.waitForEvent('download'), page.getByTestId('pdf-drawn').click()])
  await pdf.saveAs('test-results/page.pdf')
  const fs = await import('node:fs/promises')
  const bytes = (await fs.readFile(await pdf.path())).toString('latin1')
  expect(bytes.startsWith('%PDF-')).toBe(true)
  expect((bytes.match(/\/Subtype \/Image/g) ?? []).length).toBeGreaterThanOrEqual(3)
  // Or with their links instead (the choice remembered).
  await page.getByRole('button', { name: 'Page options' }).click()
  await page.getByTestId('page-pdf').click()
  const [linked] = await Promise.all([page.waitForEvent('download'), page.getByTestId('pdf-links').click()])
  await linked.saveAs('test-results/page-links.pdf')
  const plain = (await fs.readFile(await linked.path())).toString('latin1')
  expect((plain.match(/\/Subtype \/Image/g) ?? []).length).toBe(0)
  expect(plain).toContain(`/w/${ws.id}/s/${costs.id}`)
  await page.getByRole('button', { name: 'Page options' }).click()
  await page.getByTestId('page-pdf').click()
  await expect(page.getByTestId('pdf-links').locator('svg').last()).toHaveClass(/text-accent/)
  await page.keyboard.press('Escape')
  // As a Word file, the same way.
  await page.getByRole('button', { name: 'Page options' }).click()
  await page.getByTestId('page-docx').click()
  const [word] = await Promise.all([page.waitForEvent('download'), page.getByTestId('pdf-drawn').click()])
  expect(word.suggestedFilename()).toBe('Launch plan.docx')
  await word.saveAs('test-results/page.docx')
  const docx = await fs.readFile(await word.path())
  expect(docx.subarray(0, 2).toString()).toBe('PK')
  expect(docx.toString('latin1')).toContain('word/document.xml')
})

test('projects: a folder joined to a task list shows its progress; the list shows its folder; pages are filed in folders', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Studio X', kind: 'work', modules: ['home', 'tasks', 'board', 'pages', 'sheets', 'diagrams', 'files'] } })).json()
  const base = `/api/v1/workspaces/${ws.id}`
  // (Made without its own folder, to join one from the folder's side.)
  const list = await (await request.post(`${base}/lists`, { headers: H, data: { name: 'Launch', color: '#ff9500', folder_id: 'none' } })).json()
  const done = await (await request.post(`${base}/tasks`, { headers: H, data: { title: 'Logo', list_id: list.id } })).json()
  await request.post(`${base}/tasks`, { headers: H, data: { title: 'Website', list_id: list.id } })
  await request.put(`/api/v1/tasks/${done.id}`, { headers: H, data: { completed: true } })
  const folder = await (await request.post(`${base}/folders`, { headers: H, data: { name: 'Launch' } })).json()
  await request.post(`${base}/sheets`, { headers: H, data: { title: 'Budget', folder_id: folder.id, data: { tabs: [] } } })
  await request.post(`${base}/diagrams`, { headers: H, data: { title: 'Flow', folder_id: folder.id } })
  await request.post(`${base}/pages`, { headers: H, data: { title: 'Brief', folder_id: folder.id } })
  const notes = await (await request.post(`${base}/pages`, { headers: H, data: { title: 'Meeting notes', content: 'Agenda' } })).json()

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1300, height: 860 })
  // In Sheets, the folder says what it holds of every kind.
  await page.goto(`/w/${ws.id}/sheets`)
  const tile = page.getByTestId('folder-tile').filter({ hasText: 'Launch' })
  await expect(tile.getByTestId('folder-summary')).toHaveText('1 page · 1 sheet · 1 diagram')
  // Joined to the task list.
  await tile.getByTestId('folder-more').click()
  await page.getByTestId('folder-list').click()
  await page.getByTestId('list-pick').filter({ hasText: 'Launch' }).click()
  await expect(tile.getByTestId('folder-summary')).toHaveText('1 page · 1 sheet · 1 diagram · 1 list')
  // Open in Files: the project's progress, and the page in it.
  await tile.getByTestId('folder-more').click()
  await page.getByTestId('folder-in-files').click()
  await expect(page).toHaveURL(new RegExp(`/w/${ws.id}/files\\?folder=${folder.id}`))
  await expect(page.getByTestId('project-progress')).toHaveText('1 open · 1 done')
  await expect(page.getByTestId('page-card')).toContainText('Brief')
  // The list shows its folder, and leads back to it.
  await page.getByTestId('project-list').click()
  await expect(page.getByTestId('list-folder')).toContainText('Launch')
  await page.getByTestId('list-folder').click()
  await expect(page).toHaveURL(new RegExp(`folder=${folder.id}`))
  await expect(page.getByTestId('project-card')).toBeVisible()

  // A page is filed in the folder from its menu.
  await page.goto(`/w/${ws.id}/p/${notes.id}`)
  await page.getByRole('button', { name: 'Page options' }).click()
  await page.getByTestId('page-file-in').click()
  await page.getByTestId('page-folder').filter({ hasText: 'Launch' }).click()
  await expect.poll(async () => ((await (await request.get(`${base}/pages`, { headers: H })).json()) as { id: string; folder_id?: string }[]).find((p) => p.id === notes.id)?.folder_id).toBe(folder.id)
  await page.goto(`/w/${ws.id}/files?folder=${folder.id}`)
  await expect(page.getByTestId('page-card')).toHaveCount(2)
  // A new page made here is filed here.
  await page.getByTestId('files-new').click()
  await page.getByTestId('files-new-page').click()
  await expect(page).toHaveURL(/\/p\//)
  const made = page.url().split('/').pop()!
  const pages = (await (await request.get(`${base}/pages`, { headers: H })).json()) as { id: string; folder_id?: string }[]
  expect(pages.find((p) => p.id === made)?.folder_id).toBe(folder.id)

  // The other way round, from the list's own sheet: a list comes with its
  // own folder; then none; then that folder again.
  const ads = await (await request.post(`${base}/lists`, { headers: H, data: { name: 'Ads', color: '#34c759' } })).json()
  const foldersNow = async () => (await (await request.get(`${base}/folders`, { headers: H })).json()) as { id: string; name: string; list_id?: string }[]
  const adsFolder = (await foldersNow()).find((f) => f.name === 'Ads')!
  expect(adsFolder.list_id).toBe(ads.id)
  await page.goto(`/w/${ws.id}/tasks/${ads.id}`)
  await expect(page.getByTestId('list-folder')).toContainText('Ads')
  await page.getByTitle('Edit list').click()
  await expect(page.getByTestId('list-folder-pick')).toHaveValue(adsFolder.id)
  await page.getByTestId('list-folder-pick').selectOption('')
  await page.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByTestId('list-folder')).toHaveCount(0)
  expect((await foldersNow()).find((f) => f.name === 'Ads')?.list_id ?? '').toBe('')
  await page.getByTitle('Edit list').click()
  await page.getByTestId('list-folder-pick').selectOption({ label: 'Ads' })
  await page.getByRole('button', { name: 'Save' }).click()
  await expect(page.getByTestId('list-folder')).toContainText('Ads')
  expect((await foldersNow()).find((f) => f.name === 'Ads')?.list_id).toBe(ads.id)
})

test('files: any file uploaded into a folder; a task\'s attachment shows in its list\'s folder; versions; the trash; unfiled', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Archive', kind: 'custom', starter: false, modules: ['home', 'tasks', 'files'] } })).json()
  const base = `/api/v1/workspaces/${ws.id}`
  const list = await (await request.post(`${base}/lists`, { headers: H, data: { name: 'Thesis', color: '#5856d6' } })).json()
  const task = await (await request.post(`${base}/tasks`, { headers: H, data: { title: 'Chapter 1', list_id: list.id } })).json()
  const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==', 'base64')
  await request.post(`/api/v1/uploads?target=task&target_id=${task.id}`, { headers: H, multipart: { file: { name: 'figure.png', mimeType: 'image/png', buffer: png } } })

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1300, height: 860 })
  await page.goto(`/w/${ws.id}/files`)
  // The list's own folder, holding the task's picture.
  const thesis = page.getByTestId('folder-tile').filter({ hasText: 'Thesis' })
  await expect(thesis.getByTestId('folder-summary')).toHaveText('1 file · 1 list')
  await thesis.click()
  const fig = page.getByTestId('file-card').filter({ hasText: 'figure.png' })
  await expect(fig.getByTestId('file-on')).toHaveText('On the task “Chapter 1”')
  await expect(fig.locator('img.photo')).toBeVisible()

  // A file from the computer, uploaded here; the same name again: v2.
  await page.getByTestId('files-add-input').setInputFiles({ name: 'notes.txt', mimeType: 'text/plain', buffer: Buffer.from('first') })
  const notes = page.getByTestId('file-card').filter({ hasText: 'notes.txt' })
  await expect(notes).toBeVisible()
  await page.getByTestId('files-add-input').setInputFiles({ name: 'notes.txt', mimeType: 'text/plain', buffer: Buffer.from('second draft') })
  await expect(notes.locator('.vbadge')).toHaveText('v2')
  await expect(page.getByTestId('file-card').filter({ hasText: 'notes.txt' })).toHaveCount(1)
  // The first version made current again.
  await notes.getByTestId('file-more').click()
  await page.getByTestId('file-versions').click()
  await expect(page.getByTestId('version-row')).toHaveCount(2)
  await page.getByTestId('version-row').nth(1).getByTestId('version-current').click()
  await expect(notes.locator('.vbadge')).toHaveText('v3')
  const all = (await (await request.get(`${base}/files`, { headers: H })).json()) as { name: string; size: number; folder_id: string }[]
  expect(all.find((f) => f.name === 'notes.txt')?.size).toBe(5)

  // Dropped onto the page from the computer: here too.
  const dt = await page.evaluateHandle(() => {
    const d = new DataTransfer()
    d.items.add(new File(['dropped'], 'dropped.txt', { type: 'text/plain' }))
    return d
  })
  await page.locator('.fb-main').dispatchEvent('dragover', { dataTransfer: dt })
  await page.locator('.fb-main').dispatchEvent('drop', { dataTransfer: dt })
  await expect(page.getByTestId('file-card').filter({ hasText: 'dropped.txt' })).toBeVisible()
  await page.screenshot({ path: 'test-results/files.png' })

  // To the trash, and back.
  await notes.getByTestId('file-more').click()
  await page.getByTestId('file-delete').click()
  await page.getByRole('button', { name: 'Delete', exact: true }).click()
  await expect(page.getByTestId('file-card').filter({ hasText: 'notes.txt' })).toHaveCount(0)
  await page.getByTestId('files-trash').click()
  await expect(page.getByTestId('trash-row')).toHaveCount(1)
  await page.getByTestId('trash-restore').click()
  await expect(page.getByTestId('trash-row')).toHaveCount(0)
  await page.getByTestId('files-trash').click()
  await expect(page.getByTestId('file-card').filter({ hasText: 'notes.txt' })).toBeVisible()

  // The task deleted for good: its picture in the trash, saying where it was.
  await request.delete(`/api/v1/tasks/${task.id}`, { headers: H })
  await page.getByTestId('files-trash').click()
  await expect(page.getByTestId('trash-row').filter({ hasText: 'figure.png' })).toContainText('Was on the task “Chapter 1”')
  await page.screenshot({ path: 'test-results/files-trash.png' })
  await page.getByTestId('files-trash').click()

  // Unfiled: what's in no folder (a file uploaded at the top).
  await page.getByTestId('crumb').first().click()
  await request.post(`/api/v1/uploads?target=file&target_id=${ws.id}`, { headers: H, multipart: { file: { name: 'loose.pdf', mimeType: 'application/pdf', buffer: Buffer.from('%PDF-1.4') } } })
  await page.getByTestId('files-unfiled').click()
  await expect(page.getByTestId('folder-tile')).toHaveCount(0)
  await expect(page.getByTestId('file-card')).toHaveText([/loose\.pdf/])
})

test('my files: my own drive — a folder, a file uploaded, a sketch started there, a Quick Note\'s picture at the top', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const note = await (await request.post('/api/v1/me/notes', { headers: H, data: { title: 'Shopping', body: 'milk' } })).json()
  const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==', 'base64')
  await request.post(`/api/v1/uploads?target=note&target_id=${note.id}`, { headers: H, multipart: { file: { name: 'receipt.png', mimeType: 'image/png', buffer: png } } })

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1300, height: 860 })
  await page.getByTestId('my-files').click()
  await expect(page).toHaveURL(/\/files$/)
  // A Quick Note's picture, at the top.
  await expect(page.getByTestId('file-card').filter({ hasText: 'receipt.png' }).getByTestId('file-on')).toHaveText('On the note “Shopping”')
  // A folder of my own; a file into it; a sketch started there.
  await page.getByTestId('new-folder').click()
  await page.getByTestId('folder-name').fill('Taxes')
  await page.getByTestId('folder-name').press('Enter')
  await page.getByTestId('folder-tile').filter({ hasText: 'Taxes' }).click()
  await page.getByTestId('files-add-input').setInputFiles({ name: '2026.pdf', mimeType: 'application/pdf', buffer: Buffer.from('%PDF-1.4 taxes') })
  await expect(page.getByTestId('file-card').filter({ hasText: '2026.pdf' })).toBeVisible()
  await page.getByTestId('my-files-sketch').click()
  await expect(page).toHaveURL(/\/draw\/[^/]+$/)
  const sketch = page.url().split('/').pop()
  const mine = (await (await request.get('/api/v1/me/drawings', { headers: H })).json()) as { id: string; folder_id?: string }[]
  const folders = (await (await request.get('/api/v1/me/folders', { headers: H })).json()) as { id: string; name: string }[]
  expect(mine.find((d) => d.id === sketch)?.folder_id).toBe(folders.find((f) => f.name === 'Taxes')?.id)
  await page.goBack()
  await expect(page.getByTestId('folder-tile').or(page.getByTestId('whiteboard-card')).first()).toBeVisible()
  await expect(page.getByTestId('whiteboard-card')).toHaveCount(1)
})

// textPdf: a PDF of one page per line, each line as real text (Helvetica).
function textPdf(lines: string[]): Buffer {
  const objs = ['<< /Type /Catalog /Pages 2 0 R >>', `<< /Type /Pages /Kids [${lines.map((_, i) => `${4 + i * 2} 0 R`).join(' ')}] /Count ${lines.length} >>`, '<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>']
  lines.forEach((line, i) => {
    const st = `BT /F1 18 Tf 20 50 Td (${line}) Tj ET`
    objs.push(`<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 100] /Resources << /Font << /F1 3 0 R >> >> /Contents ${5 + i * 2} 0 R >>`)
    objs.push(`<< /Length ${st.length} >>\nstream\n${st}\nendstream`)
  })
  let out = '%PDF-1.4\n'
  const offs: number[] = []
  objs.forEach((o, i) => {
    offs.push(out.length)
    out += `${i + 1} 0 obj\n${o}\nendobj\n`
  })
  const x = out.length
  out += `xref\n0 ${objs.length + 1}\n0000000000 65535 f \n${offs.map((n) => `${String(n).padStart(10, '0')} 00000 n \n`).join('')}trailer\n<< /Size ${objs.length + 1} /Root 1 0 R >>\nstartxref\n${x}\n%%EOF\n`
  return Buffer.from(out)
}

const officeZip = (files: Record<string, string>) => Buffer.from(zipSync(Object.fromEntries(Object.entries(files).map(([k, v]) => [k, strToU8(v)]))))
const RELS = (list: [string, string][]) => `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">${list.map(([id, t]) => `<Relationship Id="${id}" Target="${t}" Type="x"/>`).join('')}</Relationships>`

test('files shown in place: a Word document, slides, a spreadsheet, a PDF\'s pages; found by what they say', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Office', kind: 'custom', starter: false, modules: ['home', 'tasks', 'files'] } })).json()
  const W = 'xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"'
  const P = 'xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"'
  const docx = officeZip({ 'word/document.xml': `<w:document ${W}><w:body><w:p><w:pPr><w:pStyle w:val="Heading1"/></w:pPr><w:r><w:t>Lease terms</w:t></w:r></w:p><w:p><w:r><w:t>The tenant pays quarterly.</w:t></w:r></w:p></w:body></w:document>` })
  const sp = (text: string, y: number) => `<p:sp><p:nvSpPr><p:cNvPr id="2" name="t"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr><a:xfrm><a:off x="600000" y="${y}"/><a:ext cx="8000000" cy="900000"/></a:xfrm></p:spPr><p:txBody><a:p><a:r><a:rPr sz="3200"/><a:t>${text}</a:t></a:r></a:p></p:txBody></p:sp>`
  const pptx = officeZip({
    'ppt/presentation.xml': `<p:presentation ${P}><p:sldIdLst><p:sldId id="256" r:id="r1"/><p:sldId id="257" r:id="r2"/></p:sldIdLst><p:sldSz cx="12192000" cy="6858000"/></p:presentation>`,
    'ppt/_rels/presentation.xml.rels': RELS([['r1', 'slides/slide1.xml'], ['r2', 'slides/slide2.xml']]),
    'ppt/slides/slide1.xml': `<p:sld ${P}><p:cSld><p:spTree>${sp('Roadmap 2027', 500000)}</p:spTree></p:cSld></p:sld>`,
    'ppt/slides/slide2.xml': `<p:sld ${P}><p:cSld><p:spTree>${sp('Hiring plan', 500000)}</p:spTree></p:cSld></p:sld>`,
  })
  const up = (name: string, mimeType: string, buffer: Buffer) => request.post(`/api/v1/uploads?target=file&target_id=${ws.id}`, { headers: H, multipart: { file: { name, mimeType, buffer } } })
  await up('lease.docx', 'application/octet-stream', docx)
  await up('plan.pptx', 'application/octet-stream', pptx)
  await up('budget.csv', 'text/csv', Buffer.from('Item,Cost\nRent,1200\nFood,450\n'))

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1300, height: 900 })
  await page.goto(`/w/${ws.id}/files`)
  // A PDF uploaded here: its pages drawn; its text read for search.
  await page.getByTestId('files-add-input').setInputFiles({ name: 'invoice.pdf', mimeType: 'application/pdf', buffer: textPdf(['Invoice 4471', 'Thank you']) })
  await page.getByTestId('file-card').filter({ hasText: 'invoice.pdf' }).click()
  const viewer = page.getByTestId('file-viewer')
  await expect(viewer.getByTestId('viewer-pdf').locator('canvas')).toHaveCount(2)
  await expect(viewer.getByTestId('viewer-pdf').locator('canvas.drawn').first()).toBeVisible()
  await page.waitForTimeout(500)
  await page.screenshot({ path: 'test-results/viewer-pdf.png' })
  await page.keyboard.press('Escape')
  // The Word document, as a page.
  await page.getByTestId('file-card').filter({ hasText: 'lease.docx' }).click()
  await expect(viewer.getByTestId('viewer-doc').locator('h2')).toHaveText('Lease terms')
  await expect(viewer.getByTestId('viewer-doc')).toContainText('The tenant pays quarterly.')
  await page.waitForTimeout(500)
  await page.screenshot({ path: 'test-results/viewer-docx.png' })
  await viewer.getByTestId('viewer-close').click()
  // The slides.
  await page.getByTestId('file-card').filter({ hasText: 'plan.pptx' }).click()
  await expect(viewer.getByTestId('viewer-slide')).toHaveCount(2)
  await expect(viewer.getByTestId('viewer-slide').nth(1)).toContainText('Hiring plan')
  await page.waitForTimeout(500)
  await page.screenshot({ path: 'test-results/viewer-pptx.png' })
  await page.keyboard.press('Escape')
  // The spreadsheet.
  await page.getByTestId('file-card').filter({ hasText: 'budget.csv' }).click()
  await expect(viewer.getByTestId('viewer-sheet').locator('tbody tr')).toHaveCount(3)
  await expect(viewer.getByTestId('viewer-sheet').locator('tbody tr').nth(1)).toContainText('1200')
  await page.keyboard.press('Escape')

  // ⌘K finds them by what they say: the Word file's words, the PDF's.
  await page.getByRole('button', { name: /^Search/ }).first().click()
  await page.getByPlaceholder('Search or jump to…').fill('quarterly')
  await page.locator('.cmd-item', { hasText: 'lease.docx' }).click()
  await expect(viewer.getByTestId('viewer-name')).toHaveText('lease.docx')
  await page.keyboard.press('Escape')
  await expect.poll(async () => ((await (await request.get('/api/v1/search?q=4471', { headers: H })).json()) as { kind: string; title: string }[]).filter((h) => h.kind === 'file').map((h) => h.title)).toEqual(['invoice.pdf'])

  // Opened from a task's sheet: over it; ✕ and Esc close the viewer only.
  const list = await (await request.post(`/api/v1/workspaces/${ws.id}/lists`, { headers: H, data: { name: 'Admin' } })).json()
  const task = await (await request.post(`/api/v1/workspaces/${ws.id}/tasks`, { headers: H, data: { title: 'Pay rent', list_id: list.id } })).json()
  await request.post(`/api/v1/uploads?target=task&target_id=${task.id}`, { headers: H, multipart: { file: { name: 'receipt.pdf', mimeType: 'application/pdf', buffer: textPdf(['Paid']) } } })
  await page.goto(`/w/${ws.id}/tasks/all?task=${task.id}`)
  await page.getByText('receipt.pdf').click()
  await expect(viewer.getByTestId('viewer-pdf').locator('canvas.drawn')).toHaveCount(1)
  await viewer.getByTestId('viewer-close').click()
  await expect(viewer).toBeHidden()
  await expect(page.getByText('receipt.pdf')).toBeVisible() // the task is still open
  await page.getByText('receipt.pdf').click()
  await expect(viewer).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(viewer).toBeHidden()
  await expect(page.getByText('receipt.pdf')).toBeVisible()
})

// signInAs signs someone else in (a second browser), setting a new password
// if asked to.
async function signInAs(page: Page, username: string, password: string) {
  await page.goto('/')
  await page.locator('input[autocomplete="username webauthn"]').fill(username)
  await page.locator('input[autocomplete="current-password"]').fill(password)
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await page.waitForURL((u) => !u.pathname.startsWith('/login'))
  if (await page.getByRole('heading', { name: 'Set a new password' }).isVisible().catch(() => false)) {
    await page.getByPlaceholder('Current password').fill(password)
    await page.getByPlaceholder('New password (min 8)').fill(password + '-2')
    await page.getByPlaceholder('Confirm new password').fill(password + '-2')
    await page.getByRole('button', { name: 'Update password' }).click()
    await expect(page.getByRole('heading', { name: 'Set a new password' })).toBeHidden()
  }
}

test('presence: two people on a page see each other; one leaves, the other sees it', async ({ page, request, browser }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  await request.post('/api/v1/users', { headers: H, data: { username: 'pia', display_name: 'Pia Costa', password: 'pia-e2e-password-42' } })
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Together', kind: 'work', starter: false, modules: ['home', 'pages'] } })).json()
  await request.post(`/api/v1/workspaces/${ws.id}/members`, { headers: H, data: { user: 'pia', role: 'editor' } })
  const pg = await (await request.post(`/api/v1/workspaces/${ws.id}/pages`, { headers: H, data: { title: 'Agenda', content: 'Items.' } })).json()

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1300, height: 860 })
  await page.goto(`/w/${ws.id}/p/${pg.id}`)
  await expect(page.getByTestId('presence')).toHaveCount(0) // alone

  const ctx = await browser.newContext()
  const pia = await ctx.newPage()
  await pia.setViewportSize({ width: 1300, height: 860 })
  await signInAs(pia, 'pia', 'pia-e2e-password-42')
  await pia.goto(`/w/${ws.id}/p/${pg.id}`)
  await expect(page.getByTestId('presence-person')).toHaveText(['PC'])
  await expect(page.getByTestId('presence-person')).toHaveAttribute('title', /Pia Costa is/)
  await expect(pia.getByTestId('presence-person')).toHaveCount(1) // and Pia sees me
  // Pia goes elsewhere: gone from the page.
  await pia.goto(`/w/${ws.id}`)
  await expect(page.getByTestId('presence')).toHaveCount(0)
  await ctx.close()
})

test('editing together: two people type in one page at once, see each other\'s text and cursor; one save for both; saved another way, they start over', async ({ page, request, browser }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  await request.post('/api/v1/users', { headers: H, data: { username: 'rlopes', display_name: 'Rui Lopes', password: 'rlopes-e2e-password-42' } })
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Co-writing', kind: 'work', starter: false, modules: ['home', 'pages', 'whiteboards'] } })).json()
  await request.post(`/api/v1/workspaces/${ws.id}/members`, { headers: H, data: { user: 'rlopes', role: 'editor' } })
  const pg = await (await request.post(`/api/v1/workspaces/${ws.id}/pages`, { headers: H, data: { title: 'Minutes', content: 'Present: all.' } })).json()
  const content = async () => (await (await request.get(`/api/v1/pages/${pg.id}`, { headers: H })).json()).content as string

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1300, height: 860 })
  await page.goto(`/w/${ws.id}/p/${pg.id}`)
  await page.getByRole('button', { name: 'Edit', exact: true }).click()
  await page.getByRole('tab', { name: 'Visual' }).click()
  const mine = page.locator('.ProseMirror')
  await expect(mine).toContainText('Present: all.')

  const ctx = await browser.newContext()
  const rui = await ctx.newPage()
  await rui.setViewportSize({ width: 1300, height: 860 })
  await signInAs(rui, 'rlopes', 'rlopes-e2e-password-42')
  await rui.goto(`/w/${ws.id}/p/${pg.id}`)
  await rui.getByRole('button', { name: 'Edit', exact: true }).click()
  await rui.getByRole('tab', { name: 'Visual' }).click()
  const his = rui.locator('.ProseMirror')
  await expect(his).toContainText('Present: all.')
  await expect(his.locator('p')).toHaveCount(1) // started once, not twice

  // Each types; each sees the other's text, and his cursor with his name.
  await mine.locator('p').first().click({ delay: 80 })
  await page.keyboard.press('End')
  await page.keyboard.type(' Budget approved.')
  await expect(his).toContainText('Present: all. Budget approved.')
  await his.locator('p').first().click({ delay: 80 }) // a person's click (an instant one can come before the editor takes focus)
  await rui.keyboard.press('ControlOrMeta+End')
  await rui.keyboard.press('Enter')
  await rui.keyboard.type('Next meeting on Friday.')
  await expect(mine).toContainText('Next meeting on Friday.')
  await expect(page.locator('.collaboration-carets__label', { hasText: 'Rui Lopes' })).toBeVisible()
  await page.screenshot({ path: 'test-results/together.png' })
  await expect(page.getByTestId('presence-person')).toHaveText(['RL'])
  // Saved once, for both: all of it.
  await expect.poll(content, { timeout: 15000 }).toBe('Present: all. Budget approved.\n\nNext meeting on Friday.')

  // Saved another way (the API): both start over from it.
  const cur = await (await request.get(`/api/v1/pages/${pg.id}`, { headers: H })).json()
  await request.put(`/api/v1/pages/${pg.id}`, { headers: H, data: { content: 'Rewritten by the assistant.', version: cur.version } })
  await expect(mine).toHaveText('Rewritten by the assistant.')
  await expect(his).toHaveText('Rewritten by the assistant.')
  await his.locator('p').first().click({ delay: 80 })
  await rui.keyboard.press('End')
  await rui.keyboard.type(' Agreed.') // and they go on together
  await expect(mine).toContainText('Rewritten by the assistant. Agreed.')
  await ctx.close()
})

test('drawing together: a stroke on one screen shows on the other; saved once', async ({ page, request, browser }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  await request.post('/api/v1/users', { headers: H, data: { username: 'rlopes', display_name: 'Rui Lopes', password: 'rlopes-e2e-password-42' } })
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Studio together', kind: 'work', starter: false, modules: ['home', 'whiteboards'] } })).json()
  await request.post(`/api/v1/workspaces/${ws.id}/members`, { headers: H, data: { user: 'rlopes', role: 'editor' } })
  const scene = JSON.stringify({ type: 'orbit-sketch', version: 1, paper: 'plain', paperColor: '#ffffff', layers: [{ id: 'l1', name: 'Layer 1', visible: true, opacity: 1, items: [] }] })
  const board = await (await request.post(`/api/v1/workspaces/${ws.id}/drawings`, { headers: H, data: { title: 'Plan', scene } })).json()
  const items = async () => ((JSON.parse((await (await request.get(`/api/v1/drawings/${board.id}`, { headers: H })).json()).scene).layers ?? []) as { items: unknown[] }[]).flatMap((l) => l.items).length

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1300, height: 860 })
  await page.goto(`/w/${ws.id}/whiteboards/${board.id}`)
  await expect(page.getByTestId('sketch')).toBeVisible()
  const ctx = await browser.newContext()
  const rui = await ctx.newPage()
  await rui.setViewportSize({ width: 1300, height: 860 })
  await signInAs(rui, 'rlopes', 'rlopes-e2e-password-42')
  await rui.goto(`/w/${ws.id}/whiteboards/${board.id}`)
  await expect(rui.getByTestId('sketch')).toBeVisible()
  await expect(page.getByTestId('presence-person')).toHaveText(['RL'])

  const strokes = (p: Page) => p.locator('[data-testid="sketch-board"] path[fill]:not([fill="none"])')
  const before = await strokes(rui).count()
  const box = (await page.getByTestId('sketch-board').boundingBox())!
  await page.mouse.move(box.x + 300, box.y + 300)
  await page.mouse.down()
  await page.mouse.move(box.x + 500, box.y + 360, { steps: 12 })
  await page.mouse.up()
  await expect.poll(async () => strokes(rui).count()).toBe(before + 1)
  // And his, on mine.
  const rbox = (await rui.getByTestId('sketch-board').boundingBox())!
  await rui.mouse.move(rbox.x + 300, rbox.y + 450)
  await rui.mouse.down()
  await rui.mouse.move(rbox.x + 520, rbox.y + 470, { steps: 12 })
  await rui.mouse.up()
  await expect.poll(async () => strokes(page).count()).toBe(before + 2)
  await expect.poll(items, { timeout: 15000 }).toBe(2)
  await ctx.close()
})

test('tags and properties on any page: added under its title, a tag leads to its pages, a property has its kind', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Travels', kind: 'personal', starter: false, modules: ['home', 'pages'] } })).json()
  const lisbon = await (await request.post(`/api/v1/workspaces/${ws.id}/pages`, { headers: H, data: { title: 'Lisbon', content: 'Trams.' } })).json()
  const porto = await (await request.post(`/api/v1/workspaces/${ws.id}/pages`, { headers: H, data: { title: 'Porto', content: 'Wine.' } })).json()
  await request.put(`/api/v1/pages/${porto.id}`, { headers: H, data: { tags: ['portugal'] } })
  const saved = async () => (await (await request.get(`/api/v1/pages/${lisbon.id}`, { headers: H })).json()) as { tags: string[]; fields?: { key: string; type: string; value: unknown }[] }

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1300, height: 860 })
  await page.goto(`/w/${ws.id}/p/${lisbon.id}`)
  // A tag: typed, or one the workspace has (suggested).
  await page.getByTestId('add-tag').click()
  await page.getByTestId('tag-input').fill('port')
  await page.getByTestId('tag-suggestion').filter({ hasText: '#portugal' }).click()
  await page.getByTestId('tag-input').fill('city break')
  await page.getByTestId('tag-input').press('Enter')
  await expect(page.getByTestId('page-tag')).toHaveText(['#portugal', '#city break'])
  await expect.poll(async () => (await saved()).tags).toEqual(['portugal', 'city break'])
  // A property, a date.
  await page.getByTestId('add-field').click()
  await page.getByTestId('field-name').fill('Leaving')
  await page.getByTestId('field-kind-date').click()
  await page.getByTestId('field-add').click()
  await page.getByTestId('page-field').getByTestId('field-value').fill('2026-12-01')
  await page.getByTestId('page-field').getByTestId('field-value').press('Enter')
  await expect.poll(async () => (await saved()).fields).toEqual([{ key: 'Leaving', type: 'date', value: '2026-12-01' }])
  // The tag leads to its pages.
  await page.getByTestId('page-tag').filter({ hasText: '#portugal' }).locator('.tag-name').click()
  await expect(page).toHaveURL(/\/pages\?tag=portugal/)
  await expect(page.getByTestId('tag-pill').filter({ hasText: '#portugal' })).toHaveAttribute('aria-pressed', 'true')
  await expect(page.locator('.tile-card')).toHaveCount(2)
  await page.getByTestId('tag-pill').filter({ hasText: '#city break' }).click()
  await expect(page.locator('.tile-card')).toHaveText([/Lisbon/])
})

test('graph: a workspace\'s things and their links; a small graph beside a page; kinds hidden; a click opens one', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Mapped', kind: 'work', starter: false, modules: ['home', 'tasks', 'pages', 'sheets', 'files'] } })).json()
  const base = `/api/v1/workspaces/${ws.id}`
  const sheet = await (await request.post(`${base}/sheets`, { headers: H, data: { title: 'Budget', data: { tabs: [] } } })).json()
  const goals = await (await request.post(`${base}/pages`, { headers: H, data: { title: 'Goals' } })).json()
  const plan = await (await request.post(`${base}/pages`, { headers: H, data: { title: 'Plan', content: `Costs:\n\n[Budget](/w/${ws.id}/s/${sheet.id})\n\nFor the [goals](/w/${ws.id}/p/${goals.id}).` } })).json()
  await request.post(`${base}/pages`, { headers: H, data: { title: 'Alone' } })

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1300, height: 860 })
  // Beside the page: what it links to.
  await page.goto(`/w/${ws.id}/p/${plan.id}`)
  await page.getByTestId('outline-toggle').click() // the side panel
  const local = page.getByTestId('local-graph')
  await expect(local.getByTestId('graph-node')).toHaveCount(3)
  await local.locator(`[data-testid="graph-node"][data-id="${goals.id}"]`).click()
  await expect(page).toHaveURL(new RegExp(`/p/${goals.id}`))
  await expect(page.getByTestId('local-graph').getByTestId('graph-node')).toHaveCount(2)
  // The whole graph, from it: near Goals, then everything.
  await page.getByTestId('local-graph-open').click()
  await expect(page).toHaveURL(/\/graph\?focus=/)
  await expect(page.getByTestId('graph-node')).toHaveCount(2)
  await page.getByTestId('graph-all').click()
  await expect(page.getByTestId('graph-node')).toHaveCount(4) // three pages and the sheet
  await page.getByTestId('graph-kind-sheet').click()
  await expect(page.getByTestId('graph-node')).toHaveCount(3)
  await page.getByTestId('graph-kind-sheet').click()
  await page.locator(`[data-testid="graph-node"][data-id="${sheet.id}"]`).click()
  await expect(page).toHaveURL(new RegExp(`/s/${sheet.id}`))
})

test('page history: each person\'s saves are versions; one shows its changes; it is put back; versions and pages compared side by side', async ({ page, request }) => {
  const json = async (r: { json: () => Promise<any> }) => r.json()
  const login = async (username: string, password: string) => (await json(await request.post('/api/v1/auth/login', { data: { username, password } }))).access_token as string
  const auth = (tok: string) => ({ Authorization: `Bearer ${tok}` })
  const admin = auth(await login(ADMIN_USER, NEW_PASSWORD))
  await request.post('/api/v1/users', { headers: admin, data: { username: 'vera', display_name: 'Vera', password: 'vera-e2e-password-42' } })
  const ws = await json(await request.post('/api/v1/workspaces', { headers: admin, data: { name: 'Drafts', kind: 'work', starter: false, modules: ['home', 'pages'] } }))
  await request.post(`/api/v1/workspaces/${ws.id}/members`, { headers: admin, data: { user: 'vera', role: 'editor' } })
  const vera = auth(await login('vera', 'vera-e2e-password-42'))
  const pg = await json(await request.post(`/api/v1/workspaces/${ws.id}/pages`, { headers: admin, data: { title: 'Speech', content: 'Hello all.\n\nThanks for coming.' } }))
  const save = async (h: Record<string, string>, content: string) => {
    const cur = await json(await request.get(`/api/v1/pages/${pg.id}`, { headers: h }))
    expect((await request.put(`/api/v1/pages/${pg.id}`, { headers: h, data: { content, version: cur.version } })).ok()).toBe(true)
  }
  await save(vera, 'Hello all.\n\nThanks for coming.\n\nLet us begin.')
  await save(admin, 'Hello everyone.\n\nLet us begin.')
  const other = await json(await request.post(`/api/v1/workspaces/${ws.id}/pages`, { headers: admin, data: { title: 'Toast', content: 'Cheers.' } }))

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1300, height: 860 })
  await page.goto(`/w/${ws.id}/p/${pg.id}`)
  await page.getByTestId('page-status').click()
  const hist = page.getByTestId('page-history')
  await expect(hist.getByTestId('history-version')).toHaveCount(3)
  // The newest, against Vera's: one block in, two out.
  await expect(hist.getByTestId('history-counts')).toHaveText('+1 −2')
  await expect(hist.locator('[data-op="add"]')).toContainText('Hello everyone.')
  await expect(hist.locator('[data-op="del"]').first()).toContainText('Hello all.')
  // Vera's, put back.
  await hist.getByTestId('history-version').nth(1).click()
  await expect(hist.getByTestId('history-version').nth(1)).toContainText('Vera')
  await hist.getByTestId('history-restore').click()
  await page.getByRole('button', { name: 'Restore this version' }).last().click()
  await expect(hist).toBeHidden()
  await expect(page.locator('.md-view')).toContainText('Thanks for coming.')
  expect((await json(await request.get(`/api/v1/pages/${pg.id}`, { headers: admin }))).content).toContain('Thanks for coming.')
  // Side by side: the first version, the one now, and another page.
  await page.getByRole('button', { name: 'Page options' }).click()
  await page.getByTestId('page-history-open').click()
  await expect(hist.getByTestId('history-version')).toHaveCount(4)
  await hist.getByTestId('history-pick').nth(3).check()
  await hist.getByTestId('history-pick').nth(0).check()
  await hist.getByTestId('history-add-page').click()
  await page.getByTestId('history-page').filter({ hasText: 'Toast' }).click()
  const cols = hist.getByTestId('history-columns')
  await expect(cols.locator('.h-col')).toHaveCount(3)
  await expect(cols.locator('.h-col').nth(1).locator('[data-op="add"]')).toContainText('Let us begin.')
  await expect(cols.locator('.h-col').nth(2)).toContainText('Cheers.')
  await hist.getByTestId('history-close').click()
  await expect(hist).toBeHidden()
  expect(other.id).toBeTruthy()
})

test('assistants: an app signs in with OAuth (allowed read only here), then uses Orbit over MCP', async ({ page, request }) => {
  const { createHash } = await import('node:crypto')
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const mine = await (await request.post('/api/v1/workspaces', { headers: { Authorization: `Bearer ${login.access_token}` }, data: { name: 'Assisted', kind: 'work', starter: false, modules: ['home', 'pages'] } })).json()
  const reg = await (await request.post('/api/v1/oauth/register', { data: { client_name: 'Test Assistant', redirect_uris: ['https://assistant.example/callback'] } })).json()
  const verifier = 'e2e-verifier-' + 'x'.repeat(40)
  const challenge = createHash('sha256').update(verifier).digest('base64url')
  let back = ''
  await page.route('https://assistant.example/**', (route) => {
    back = route.request().url()
    return route.fulfill({ contentType: 'text/html', body: '<p>Thanks</p>' })
  })

  await signIn(page, NEW_PASSWORD)
  // Settings shows the address to give an assistant.
  await page.goto('/settings')
  await expect(page.getByTestId('mcp-url')).toHaveText(/\/mcp$/)

  const q = new URLSearchParams({ response_type: 'code', client_id: reg.client_id, redirect_uri: 'https://assistant.example/callback', state: 'abc', code_challenge: challenge, code_challenge_method: 'S256' })
  await page.goto(`/oauth/authorize?${q}`)
  await expect(page.getByTestId('oauth-consent')).toContainText('Test Assistant wants to use Orbit')
  await expect(page.getByTestId('oauth-consent')).toContainText('assistant.example')
  await page.getByTestId('oauth-read-only').check()
  await page.getByTestId('oauth-allow').click()
  await expect.poll(() => back).toContain('code=')
  const got = new URL(back)
  expect(got.searchParams.get('state')).toBe('abc')

  const tok = await (await request.post('/api/v1/oauth/token', { form: { grant_type: 'authorization_code', code: got.searchParams.get('code')!, redirect_uri: 'https://assistant.example/callback', client_id: reg.client_id, code_verifier: verifier } })).json()
  expect(tok.scope).toBe('orbit:read')
  const rpc = async (method: string, params: unknown) =>
    (await (await request.post('/mcp', { headers: { Authorization: `Bearer ${tok.access_token}` }, data: { jsonrpc: '2.0', id: 1, method, params } })).json()) as any
  const init = await rpc('initialize', { protocolVersion: '2025-06-18', capabilities: {}, clientInfo: { name: 'e2e' } })
  expect(init.result.serverInfo.name).toBe('orbit')
  const list = await rpc('tools/call', { name: 'list_workspaces', arguments: {} })
  expect(list.result.isError).toBe(false)
  expect(list.result.content[0].text).toContain(mine.id)
  // Orbit's own guides, for how Orbit works (built into this version).
  const help = await rpc('tools/call', { name: 'orbit_help', arguments: { query: 'Authentik provider redirect' } })
  expect(help.result.isError).toBe(false)
  expect(help.result.content[0].text).toContain('/docs/sso-authentik#')
  const write = await rpc('tools/call', { name: 'create_page', arguments: { workspace_id: mine.id, title: 'Nope' } })
  expect(write.result.isError).toBe(true)

  // The token is one of the person's, named after the app.
  await page.goto('/settings')
  await expect(page.locator('#apitokens')).toContainText('OAuth: Test Assistant')
})

test('guest links and forms: a page read and edited by its link, no account; a public form adds a row', async ({ page, browser, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Club', kind: 'work', starter: false, modules: ['home', 'pages', 'sheets'] } })).json()
  const base = `/api/v1/workspaces/${ws.id}`
  const sheet = await (await request.post(`${base}/sheets`, { headers: H, data: { title: 'Fees', data: { tabs: [{ id: 't1', name: 'Sheet 1', cells: { A1: { v: 'Yearly' }, B1: { v: '40' } } }] } } })).json()
  const root = await (await request.post(`${base}/pages`, { headers: H, data: { title: 'Handbook', content: `Welcome, members.\n\n[Fees](/w/${ws.id}/s/${sheet.id})` } })).json()
  const child = await (await request.post(`${base}/pages`, { headers: H, data: { title: 'Rules', parent_id: root.id, content: 'Be kind.' } })).json()

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1300, height: 860 })
  await page.goto(`/w/${ws.id}/p/${root.id}`)
  await page.getByRole('button', { name: 'Page options' }).click()
  await page.getByTestId('page-share').click()
  await page.getByTestId('share-new').click()
  await page.getByTestId('share-edit').click()
  const url = await page.getByTestId('share-url').inputValue()
  expect(url).toMatch(/\/shared\//)
  // The same, as clean Markdown for agents (no account): the sheet as a table.
  const mdUrl = await page.getByTestId('share-markdown').inputValue()
  const md = await (await request.get(new URL(mdUrl).pathname + new URL(mdUrl).search)).text()
  expect(md).toContain('# Handbook')
  expect(md).toContain('| 1 | Yearly | 40 |')
  expect(md).toContain('# Rules')

  // A guest: no account.
  const guestCtx = await browser.newContext()
  const guest = await guestCtx.newPage()
  await guest.goto(new URL(url).pathname)
  await expect(guest.getByTestId('guest-title')).toHaveText('Handbook')
  await expect(guest.getByTestId('sheet-embed')).toContainText('Yearly')
  await guest.getByTestId('guest-tree-page').filter({ hasText: 'Rules' }).click()
  await expect(guest.locator('.md-view')).toContainText('Be kind.')
  await guest.getByTestId('guest-edit').click()
  await guest.getByTestId('guest-text').click()
  await guest.keyboard.press('ControlOrMeta+End')
  await guest.keyboard.type(' And on time.')
  await guest.getByTestId('guest-save').click()
  await expect(guest.getByTestId('guest-note')).toHaveText('Saved')
  await expect.poll(async () => (await (await request.get(`/api/v1/pages/${child.id}`, { headers: H })).json()).content).toBe('Be kind. And on time.')
  // Taken back: the link opens nothing.
  await page.getByTestId('share-remove').click()
  await guest.reload()
  await expect(guest.getByTestId('guest-gone')).toBeVisible()

  // A form for a database, made public; someone without an account answers.
  const db = await (await request.post(`${base}/pages`, { headers: H, data: { title: 'Sign-ups', db: { props: [{ id: 'level', name: 'Level', type: 'select', options: [{ id: 'b', name: 'Beginner' }, { id: 'a', name: 'Advanced' }] }], views: [{ id: 'v', name: 'All', type: 'table' }] } } })).json()
  await page.goto(`/w/${ws.id}/p/${db.id}`)
  await page.getByTestId('db-forms').click()
  await page.getByTestId('form-new').click()
  await page.getByTestId('form-include-Level').check()
  await page.getByTestId('form-public').click()
  await page.getByTestId('form-save').click()
  const formUrl = (await page.getByTestId('form-url').textContent())!.trim()
  await guest.goto(new URL(formUrl).pathname)
  await expect(guest.getByTestId('form-heading')).toHaveText('Sign-ups')
  await guest.getByTestId('form-send').click()
  await expect(guest.getByTestId('form-error')).toContainText('needed')
  await guest.getByTestId('form-title').fill('Rita')
  await guest.getByText('Advanced').click()
  await guest.getByTestId('form-send').click()
  await expect(guest.getByTestId('form-done')).toBeVisible()
  const rows = (await (await request.get(`${base}/pages`, { headers: H })).json()) as { title: string; parent_id: string; props?: Record<string, unknown> }[]
  const rita = rows.find((r) => r.parent_id === db.id && r.title === 'Rita')
  expect(rita?.props).toEqual({ level: 'a' })
  await guestCtx.close()
})

test('page restrictions: a page open to only some people is not there for the others', async ({ page, request }) => {
  const json = async (r: { json: () => Promise<any> }) => r.json()
  const login = async (username: string, password: string) => (await json(await request.post('/api/v1/auth/login', { data: { username, password } }))).access_token as string
  const auth = (tok: string) => ({ Authorization: `Bearer ${tok}` })
  const admin = auth(await login(ADMIN_USER, NEW_PASSWORD))
  await request.post('/api/v1/users', { headers: admin, data: { username: 'nina', display_name: 'Nina', password: 'nina-e2e-password-42' } })
  await request.post('/api/v1/users', { headers: admin, data: { username: 'omar', display_name: 'Omar', password: 'omar-e2e-password-42' } })
  const ws = await json(await request.post('/api/v1/workspaces', { headers: admin, data: { name: 'Board room', kind: 'work', starter: false, modules: ['home', 'pages'] } }))
  for (const u of ['nina', 'omar']) await request.post(`/api/v1/workspaces/${ws.id}/members`, { headers: admin, data: { user: u, role: 'editor' } })
  const pg = await json(await request.post(`/api/v1/workspaces/${ws.id}/pages`, { headers: admin, data: { title: 'Budget 2027', content: 'Draft numbers.' } }))
  await request.post(`/api/v1/workspaces/${ws.id}/pages`, { headers: admin, data: { title: 'Notes inside', parent_id: pg.id } })

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1300, height: 860 })
  await page.goto(`/w/${ws.id}/p/${pg.id}`)
  await page.getByRole('button', { name: 'Page options' }).click()
  await page.getByTestId('page-access').click()
  await page.getByTestId('access-only').check()
  await page.getByTestId('access-person-Nina').check()
  await page.getByTestId('access-save').click()
  await expect(page.getByTestId('page-lock')).toBeVisible()

  const titles = async (u: string, p: string) =>
    ((await json(await request.get(`/api/v1/workspaces/${ws.id}/pages`, { headers: auth(await login(u, p)) }))) as { title: string }[]).map((x) => x.title).sort()
  expect(await titles('nina', 'nina-e2e-password-42')).toEqual(['Budget 2027', 'Notes inside'])
  expect(await titles('omar', 'omar-e2e-password-42')).toEqual([])
  const omar = auth(await login('omar', 'omar-e2e-password-42'))
  expect((await request.get(`/api/v1/pages/${pg.id}`, { headers: omar })).status()).toBe(404)
})

test('outline: a page\'s headings to jump to, its callouts and to-dos in one list, ticked from there', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Trips', kind: 'personal', starter: false, modules: ['home', 'pages'] } })).json()
  const filler = Array.from({ length: 40 }, (_, i) => `Line ${i + 1}.`).join('\n\n')
  const pg = await (await request.post(`/api/v1/workspaces/${ws.id}/pages`, { headers: H, data: { title: 'Lisbon', content: `# Plan\n\n> [!TIP]\n> Take the tram.\n\n${filler}\n\n## Packing\n\n- [ ] Sunscreen\n- [x] Tickets\n\n## Days\n\nDay one.` } })).json()

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1400, height: 800 })
  await page.goto(`/w/${ws.id}/p/${pg.id}`)
  // The side panel (as Folio's), opened from the page.
  await page.getByTestId('outline-toggle').click()
  const ol = page.getByTestId('outline')
  await expect(ol.getByTestId('local-graph')).toBeVisible()
  await expect(ol.getByTestId('outline-heading')).toHaveText(['Plan', 'Packing', 'Days'])
  await expect(ol.getByTestId('outline-callout')).toContainText('Take the tram.')
  await ol.getByTestId('outline-heading').filter({ hasText: 'Days' }).click()
  await expect(page.locator('.md-view h2', { hasText: 'Days' })).toBeInViewport()
  await ol.getByTestId('outline-task').first().check()
  await expect.poll(async () => (await (await request.get(`/api/v1/pages/${pg.id}`, { headers: H })).json()).content).toContain('- [x] Sunscreen')
  // Closed, it stays closed.
  await page.getByTestId('outline-toggle').click()
  await page.reload()
  await expect(page.locator('.md-view')).toContainText('Day one.')
  await expect(page.getByTestId('outline')).toBeHidden()
  await page.getByTestId('outline-toggle').click()
  await expect(page.getByTestId('outline')).toBeVisible()
  // Only where something is open: not on the workspace's home.
  await page.goto(`/w/${ws.id}`)
  await expect(page.getByTestId('rail-open')).toHaveCount(0)
  await expect(page.getByTestId('outline')).toHaveCount(0)
})

test('recent changes: moving and trashing pages undone from the list', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Oops', kind: 'work', starter: false, modules: ['home', 'pages'] } })).json()
  const base = `/api/v1/workspaces/${ws.id}`
  const a = await (await request.post(`${base}/pages`, { headers: H, data: { title: 'Projects' } })).json()
  const b = await (await request.post(`${base}/pages`, { headers: H, data: { title: 'Ideas' } })).json()
  await request.post(`/api/v1/pages/${b.id}/move`, { headers: H, data: { parent_id: a.id, after_id: '' } })
  await request.delete(`/api/v1/pages/${a.id}`, { headers: H })

  await signIn(page, NEW_PASSWORD)
  await page.goto(`/w/${ws.id}/pages`)
  await page.getByTestId('open-changes').click()
  const rows = page.getByTestId('change-row')
  await expect(rows.first()).toContainText('Put “Projects” in the trash')
  await expect(rows.nth(1)).toContainText('Moved “Ideas” into Projects')
  await rows.first().getByTestId('change-undo').click()
  await expect(rows.first()).toContainText('undone')
  await rows.nth(1).getByTestId('change-undo').click()
  await expect(rows.nth(1)).toContainText('undone')
  const pages = (await (await request.get(`${base}/pages`, { headers: H })).json()) as { id: string; parent_id: string | null }[]
  expect(pages.find((p) => p.id === a.id)).toBeTruthy()
  expect(pages.find((p) => p.id === b.id)?.parent_id).toBeNull()
})

test('page pieces: a toggle section, columns, the sub-pages list, status badges, glossary terms, and a Google Docs paste', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Ops', kind: 'work', starter: false, modules: ['home', 'pages'] } })).json()
  const base = `/api/v1/workspaces/${ws.id}`
  const gl = await (await request.post(`${base}/pages`, { headers: H, data: { title: 'Glossary', content: '- **SLA**: what we promise to answer within' } })).json()
  await request.put(`/api/v1/pages/${gl.id}`, { headers: H, data: { glossary: true } })
  const content = [
    'Our SLA is short. <span data-status="green">Live</span>',
    '',
    '<details>',
    '<summary>Escalation</summary>',
    '',
    'Call the lead.',
    '',
    '</details>',
    '',
    '<div data-columns="2">',
    '<div data-column>',
    '',
    'Left.',
    '',
    '</div>',
    '<div data-column>',
    '',
    'Right.',
    '',
    '</div>',
    '</div>',
    '',
    '::pagetree',
  ].join('\n')
  const pg = await (await request.post(`${base}/pages`, { headers: H, data: { title: 'Runbook', content } })).json()
  await request.post(`${base}/pages`, { headers: H, data: { title: 'Night shift', parent_id: pg.id } })

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1300, height: 860 })
  await page.goto(`/w/${ws.id}/p/${pg.id}`)
  const view = page.locator('.md-view')
  await expect(view.locator('span[data-status="green"]')).toHaveText('Live')
  await expect(view.locator('details summary')).toHaveText('Escalation')
  await expect(view.locator('details')).not.toHaveAttribute('open', '')
  await expect(view.locator('[data-columns="2"] > [data-column]')).toHaveCount(2)
  await expect(view.getByTestId('pagetree')).toContainText('Night shift')
  const term = view.locator('.md-term', { hasText: 'SLA' })
  await term.hover()
  await expect(page.locator('.md-term-card')).toContainText('what we promise to answer within')

  // The editor keeps them all, and "/" adds more.
  await page.getByRole('button', { name: 'Edit', exact: true }).click()
  await page.getByRole('tab', { name: 'Visual' }).click()
  await expect(page.locator('.ProseMirror .details-block')).toBeVisible()
  await expect(page.locator('.ProseMirror .columns-block > .column')).toHaveCount(2)
  await page.locator('.ProseMirror p', { hasText: 'Our SLA is short' }).click()
  await page.keyboard.press('End')
  await page.keyboard.type(' ')
  await page.keyboard.type('/status')
  await page.keyboard.press('Enter')
  await expect(page.locator('.ProseMirror').getByTestId('status-badge')).toHaveCount(2)
  // A paste from Google Docs: its bold wrapper doesn't make everything bold.
  await page.locator('.ProseMirror p', { hasText: 'Left.' }).click()
  await page.keyboard.press('End')
  await page.locator('.ProseMirror').evaluate((el) => {
    const dt = new DataTransfer()
    dt.setData('text/html', '<meta charset="utf-8"><b style="font-weight:normal;" id="docs-internal-guid-1"><span>plain words</span></b>')
    dt.setData('text/plain', 'plain words')
    el.dispatchEvent(new ClipboardEvent('paste', { clipboardData: dt, bubbles: true, cancelable: true }))
  })
  await expect(page.locator('.ProseMirror')).toContainText('plain words')
  await page.getByRole('button', { name: 'Done' }).click()
  await expect.poll(async () => (await (await request.get(`/api/v1/pages/${pg.id}`, { headers: H })).json()).content).toContain('<span data-status="gray">To do</span>')
  const md = (await (await request.get(`/api/v1/pages/${pg.id}`, { headers: H })).json()).content as string
  expect(md).toContain('<details>')
  expect(md).toContain('<div data-columns="2">')
  expect(md).toContain('::pagetree')
  expect(md).toContain('plain words')
  expect(md).not.toContain('**plain words**')
})

test('impersonating shows the person’s workspaces; the audit log has its start and end', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  await request.put('/api/v1/features', { headers: H, data: { impersonation: true } })
  await request.post('/api/v1/users', { headers: H, data: { username: 'ivo', display_name: 'Ivo Reis', password: 'ivo-e2e-password-42' } })
  const ivo = await (await request.post('/api/v1/auth/login', { data: { username: 'ivo', password: 'ivo-e2e-password-42' } })).json()
  await request.post('/api/v1/workspaces', { headers: { Authorization: `Bearer ${ivo.access_token}` }, data: { name: 'Ivo Garden', kind: 'personal', starter: false, modules: ['home', 'pages'] } })
  const mine = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Admin Desk', kind: 'work', starter: false, modules: ['home', 'pages'] } })).json()

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1300, height: 860 })
  const side = page.locator('#sidebar')
  await expect(side.getByText(mine.name, { exact: true })).toBeVisible()
  await page.goto('/admin')
  await page.locator('tr', { hasText: 'Ivo Reis' }).getByTitle('Impersonate').click()
  await expect(page.getByText('Viewing as')).toBeVisible()
  await expect(side.getByText('Ivo Garden', { exact: true })).toBeVisible()
  await expect(side.getByText(mine.name, { exact: true })).toBeHidden()

  await page.getByRole('button', { name: 'Return to my account' }).click()
  await expect(side.getByText(mine.name, { exact: true })).toBeVisible()
  await expect(side.getByText('Ivo Garden', { exact: true })).toBeHidden()

  await page.goto('/audit')
  await page.getByTestId('audit-imp').click()
  await expect(page.locator('.act-kind', { hasText: 'user.impersonate_end' })).toBeVisible()
  await expect(page.locator('.act-kind', { hasText: /^user\.impersonate$/ })).toBeVisible()
  await expect(page.locator('.act-kind', { hasText: 'auth.login' })).toHaveCount(0)
})

test('a forced password change wants a new password, not the same one', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  await request.post('/api/v1/users', { headers: H, data: { username: 'lia', password: 'lia-e2e-password-42', must_change_password: true } })
  await page.goto('/')
  await page.locator('input[autocomplete="username webauthn"]').fill('lia')
  await page.locator('input[autocomplete="current-password"]').fill('lia-e2e-password-42')
  await page.getByRole('button', { name: 'Sign in', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'Set a new password' })).toBeVisible()
  await page.getByPlaceholder('Current password').fill('lia-e2e-password-42')
  await page.getByPlaceholder('New password (min 8)').fill('lia-e2e-password-42')
  await page.getByPlaceholder('Confirm new password').fill('lia-e2e-password-42')
  await page.getByRole('button', { name: 'Update password' }).click()
  await expect(page.getByText('Choose a new password, not the current one.')).toBeVisible()
  await page.getByPlaceholder('New password (min 8)').fill('lia-e2e-password-43')
  await page.getByPlaceholder('Confirm new password').fill('lia-e2e-password-43')
  await page.getByRole('button', { name: 'Update password' }).click()
  await expect(page.getByRole('heading', { name: 'Set a new password' })).toBeHidden()
})

test('a workspace calendar: each list a calendar in its colour, mine to change; "Events" for its own', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Colours', kind: 'personal', starter: false, modules: ['home', 'tasks', 'calendar'] } })).json()
  const a = await (await request.post(`/api/v1/workspaces/${ws.id}/lists`, { headers: H, data: { name: 'Errands', color: '#FF9500' } })).json()
  const b = await (await request.post(`/api/v1/workspaces/${ws.id}/lists`, { headers: H, data: { name: 'Shows', color: '#5856D6' } })).json()
  await request.post(`/api/v1/workspaces/${ws.id}/tasks`, { headers: H, data: { title: 'Post office', list_id: a.id, due: '2026-12-10' } })
  await request.post(`/api/v1/workspaces/${ws.id}/tasks`, { headers: H, data: { title: 'New episode', list_id: b.id, due: '2026-12-10' } })

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1500, height: 900 })
  await page.goto(`/w/${ws.id}/calendar?date=2026-12-10`)
  for (const name of ['Events', 'Errands', 'Shows']) await expect(page.getByRole('checkbox', { name, exact: true })).toBeVisible()
  const cell = page.locator('.mg-cell[data-day="2026-12-10"]')
  await expect(cell).toContainText('Post office')
  await expect(cell).toContainText('New episode')
  // A list hidden: its tasks go; the other's stay.
  await page.getByRole('checkbox', { name: 'Shows', exact: true }).click()
  await expect(cell).not.toContainText('New episode')
  await expect(cell).toContainText('Post office')
  await page.getByRole('checkbox', { name: 'Shows', exact: true }).click()
  // Errands in green, for me.
  await page.getByRole('checkbox', { name: 'Errands', exact: true }).hover()
  await page.getByTestId(`cal-color-list:${a.id}`).click()
  await page.getByTestId('cal-swatch-#34C759').click()
  await expect(page.getByRole('checkbox', { name: 'Errands', exact: true }).locator('.box')).toHaveCSS('background-color', 'rgb(52, 199, 89)')
  await page.reload()
  await expect(page.getByRole('checkbox', { name: 'Errands', exact: true }).locator('.box')).toHaveCSS('background-color', 'rgb(52, 199, 89)')
})

test('task sheet: how its date stands, a status from its menu, any estimate, a file dropped in the activity, Start focus', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'New sheet', kind: 'work', starter: false, modules: ['home', 'tasks', 'focus'] } })).json()
  const list = await (await request.post(`/api/v1/workspaces/${ws.id}/lists`, { headers: H, data: { name: 'Assignments', color: '#5856D6' } })).json()
  const late = new Date(Date.now() - 4 * 86400000)
  const due = `${late.getFullYear()}-${String(late.getMonth() + 1).padStart(2, '0')}-${String(late.getDate()).padStart(2, '0')}`
  const task = await (await request.post(`/api/v1/workspaces/${ws.id}/tasks`, { headers: H, data: { title: 'Biology lab report', list_id: list.id, due, notes: ['1500 words, two sources', ...Array.from({ length: 60 }, (_, i) => `line ${i}`)].join('\n') } })).json()
  const get = async () => ((await (await request.get(`/api/v1/workspaces/${ws.id}/tasks`, { headers: H })).json()) as { id: string; due: string; estimate: number; column_id: string }[]).find((x) => x.id === task.id)!

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1300, height: 900 })
  await page.goto(`/w/${ws.id}/tasks`)
  await page.getByText('Biology lab report').click()
  const sheet = page.getByTestId('task-sheet')
  await expect(sheet).toContainText('Assignments')
  await expect(page.getByTestId('task-due-bar')).toContainText('Overdue by 4 days')
  await expect(page.getByTestId('activity-line').first()).toContainText('created this task')
  // Notes keep their lines; long ones, even while written in, leave the fields and buttons in reach.
  await expect(page.getByTestId('task-notes-view').locator('br')).toHaveCount(60)
  await page.getByTestId('task-notes-view').locator('p').first().click()
  await expect(page.getByTestId('task-start-focus')).toBeInViewport()
  await expect(page.getByTestId('task-due-bar')).toBeInViewport()
  await page.keyboard.press('Escape')
  await page.screenshot({ path: 'test-results/task-sheet.png' })

  // Tomorrow, from the bar.
  await page.getByTestId('task-due-bar').getByRole('button', { name: 'Tomorrow' }).click()
  await expect(page.getByTestId('task-due-bar')).toContainText('Due tomorrow')
  // A status from its menu; an estimate of any length.
  const doing = list.columns[1]
  await page.getByTestId('task-status').click()
  await page.getByTestId('task-status-menu').getByRole('menuitemradio', { name: doing.name }).click()
  await expect(page.getByTestId('task-status')).toHaveText(doing.name)
  await page.getByTestId('task-estimate').click()
  const est = page.getByTestId('task-estimate-pop')
  await est.getByLabel('Hours').fill('2')
  await est.getByLabel('Minutes').fill('40')
  await est.getByRole('button', { name: 'Set' }).click()
  await expect(page.getByTestId('task-estimate')).toHaveText('2h 40m')
  await expect.poll(async () => [(await get()).column_id, (await get()).estimate]).toEqual([doing.id, 160])
  // The date from its chip; removed there too.
  await page.getByTestId('task-due').click()
  await page.getByTestId('task-date-pop').getByRole('button', { name: 'Remove the date' }).click()
  await expect.poll(async () => (await get()).due ?? '').toBe('')
  await expect(page.getByTestId('task-due-bar')).toContainText('No date')

  // A file dropped on the comment box shows in the activity.
  const dt = await page.evaluateHandle(() => {
    const d = new DataTransfer()
    d.items.add(new File(['hello'], 'notes.txt', { type: 'text/plain' }))
    return d
  })
  await page.getByTestId('comment-box').dispatchEvent('drop', { dataTransfer: dt })
  await expect(page.getByTestId('activity-file').filter({ hasText: 'notes.txt' })).toBeVisible()

  // Start focus: a session on it, on the focus timer.
  await page.getByTestId('task-start-focus').click()
  await page.waitForURL(/\/focus/)
  await expect(page.getByTestId('focus-pill')).toContainText('Biology lab report')
  await page.getByTestId('focus-stop').click()
})

test('lists: a finished project archived — out of the lists, Today and the calendar, its tasks kept — then restored', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Projects', kind: 'work', starter: false, modules: ['home', 'tasks', 'calendar'] } })).json()
  const keep = await (await request.post(`/api/v1/workspaces/${ws.id}/lists`, { headers: H, data: { name: 'Ongoing', color: '#007AFF' } })).json()
  const done = await (await request.post(`/api/v1/workspaces/${ws.id}/lists`, { headers: H, data: { name: 'Website launch', color: '#FF9500' } })).json()
  const d = new Date()
  const today = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
  await request.post(`/api/v1/workspaces/${ws.id}/tasks`, { headers: H, data: { title: 'Weekly report', list_id: keep.id } })
  await request.post(`/api/v1/workspaces/${ws.id}/tasks`, { headers: H, data: { title: 'Final launch check', list_id: done.id, due: today } })

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1280, height: 860 })
  await page.goto(`/w/${ws.id}/tasks/${done.id}`)
  await expect(page.getByText('Final launch check')).toBeVisible()
  // Edit list → Archive.
  await page.getByTitle('Edit list').click()
  await page.getByTestId('list-archive').click()
  await expect(page.getByText('“Website launch” archived')).toBeVisible()
  // Still open, marked archived, its task there; no new tasks in it.
  await expect(page.getByTestId('list-archived')).toBeVisible()
  await expect(page.getByText('Final launch check')).toBeVisible()
  await expect(page.getByRole('button', { name: 'New Task' })).toHaveCount(0)
  // My Lists has only the other one; Archived holds it.
  const pane = page.locator('.lists-pane')
  await expect(pane.locator('.group').filter({ hasText: 'Ongoing' }).first()).not.toContainText('Website launch')
  await expect(page.getByTestId('archived-lists')).toContainText('1')
  // Out of Today.
  await page.locator('.lists-pane .smart', { hasText: 'Today' }).click()
  await expect(page.getByText('Final launch check')).toHaveCount(0)
  await page.screenshot({ path: 'test-results/list-archived.png' })

  // Back from the archive (the section, unfolded by archiving, folds and unfolds — an archived list open or not).
  await page.locator('.group.archived').getByText('Website launch').click()
  await expect(page.locator('.group.archived')).toBeVisible()
  await page.getByTestId('archived-lists').click()
  await expect(page.locator('.group.archived')).toHaveCount(0)
  await page.getByTestId('archived-lists').click()
  await expect(page.locator('.group.archived')).toBeVisible()
  await page.getByTestId('list-restore').click()
  await expect(page.getByText('“Website launch” is back')).toBeVisible()
  await expect(page.getByTestId('list-archived')).toHaveCount(0)
  await expect(page.getByTestId('archived-lists')).toHaveCount(0)
  await page.locator('.lists-pane .smart', { hasText: 'Today' }).click()
  await expect(page.getByText('Final launch check')).toBeVisible()
})

test('task sheet: comments beside the details; a note turns into a page with its list; a big note', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Side by side', kind: 'work', starter: false, modules: ['home', 'tasks', 'notes', 'pages'] } })).json()
  const list = await (await request.post(`/api/v1/workspaces/${ws.id}/lists`, { headers: H, data: { name: 'Work', color: '#5856D6' } })).json()
  await request.post(`/api/v1/workspaces/${ws.id}/tasks`, { headers: H, data: { title: 'Two columns', list_id: list.id } })
  await request.post(`/api/v1/workspaces/${ws.id}/notes`, { headers: H, data: { title: 'Packing', body: 'For the trip\n- passport\n- charger\nsee https://example.org/a_b\n```sh\n  pack --all *.txt\n```\nthen `zip` it' } })

  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1400, height: 900 })
  await page.goto(`/w/${ws.id}/tasks`)
  await page.getByText('Two columns').click()
  const side = page.getByTestId('task-side')
  await expect(side.getByTestId('comment-box')).toBeVisible()
  const title = await page.locator('#task-title').boundingBox()
  const box = await side.boundingBox()
  expect(box!.x).toBeGreaterThan(title!.x + 300) // beside, not under
  await page.keyboard.press('Escape')

  await page.goto(`/w/${ws.id}/notes`)
  await page.locator('.note-card', { hasText: 'Packing' }).click()
  await page.getByTestId('note-size').click()
  await expect(page.locator('.note-sheet.wide').first()).toBeVisible()
  await page.getByTestId('note-size').click()
  await page.getByRole('button', { name: 'More options' }).last().click()
  await page.getByRole('button', { name: 'Turn into page' }).click()
  await page.waitForURL(/\/p\//)
  const doc = page.locator('.md-view').first() // it opens to read
  await expect(doc.locator('li')).toHaveCount(2)
  await expect(doc.locator('li').first()).toHaveText('passport')
  await expect(doc.locator('a[href="https://example.org/a_b"]')).toHaveCount(1)
  await expect(doc.locator('pre code')).toHaveText('  pack --all *.txt')
  await expect(doc.locator('p code')).toHaveText('zip')
})

test('editors keep their own look: Sheets light in a dark Orbit', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Looks', kind: 'work', starter: false, modules: ['home', 'sheets'] } })).json()
  const sh = await (await request.post(`/api/v1/workspaces/${ws.id}/sheets`, { headers: H, data: { title: 'Budget' } })).json()

  await signIn(page, NEW_PASSWORD)
  await page.goto('/settings')
  await page.getByRole('button', { name: 'Dark', exact: true }).first().click()
  await page.getByTestId('editor-sheets-light').click()
  await page.goto(`/w/${ws.id}/s/${sh.id}`)
  const ed = page.getByTestId('sheet-editor')
  await expect(ed).toHaveClass(/theme-light/)
  await expect(page.locator('html')).toHaveClass(/dark/)
  await expect(ed).toHaveCSS('background-color', 'rgb(255, 255, 255)')

  // Back as before, for the tests after this one.
  await page.goto('/settings')
  await page.getByTestId('editor-sheets-follow').click()
  await page.getByRole('button', { name: 'Automatic', exact: true }).first().click()
})

test('two Konami cards: ↑↑↓↓←→←→ B A, and ↑↑↓↓←→←→ T N', async ({ page }) => {
  await signIn(page, NEW_PASSWORD)
  const arrows = ['ArrowUp', 'ArrowUp', 'ArrowDown', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'ArrowLeft', 'ArrowRight']
  for (const k of [...arrows, 't', 'n']) await page.keyboard.press(k)
  await expect(page.getByTestId('egg-tn')).toContainText('Tomás Neto')
  await expect(page.getByTestId('egg-tn').getByRole('link')).toHaveAttribute('href', 'https://github.com/Tomasneto404')
  await page.keyboard.press('Escape')
  await expect(page.getByTestId('egg-tn')).toHaveCount(0)
  for (const k of [...arrows, 'b', 'a']) await page.keyboard.press(k)
  await expect(page.getByText('João Teixeira')).toBeVisible()
  await expect(page.getByTestId('egg-tn')).toHaveCount(0)
})

test('documentation: Orbit’s guides in one place — read, follow a link between guides, search', async ({ page }) => {
  await signIn(page, NEW_PASSWORD)
  await page.setViewportSize({ width: 1300, height: 860 })
  await page.locator('button.user-btn').click()
  await page.getByTestId('menu-docs').click()
  await expect(page).toHaveURL(/\/docs$/)
  const guide = page.getByTestId('docs-guide')
  await expect(guide.locator('h1').first()).toContainText('Orbit')
  await page.getByRole('link', { name: 'SSO with Authentik', exact: true }).click()
  await expect(guide.locator('h1').first()).toContainText('SSO with Authentik')
  // A link to another guide stays on the page.
  await guide.locator('a[href^="/docs/sso-entra"]').first().click()
  await expect(page).toHaveURL(/\/docs\/sso-entra/)
  await expect(guide.locator('h1').first()).toContainText('Entra')
  // Search across them: a hit opens its part.
  await page.getByTestId('docs-search').fill('UPLOAD_STORE')
  await expect(page.getByTestId('docs-hit').first()).toBeVisible()
  await page.getByTestId('docs-hit').first().click()
  await expect(page).toHaveURL(/\/docs\/[a-z-]+#/)
  await expect(guide).toBeVisible()
})

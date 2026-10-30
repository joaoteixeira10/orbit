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
  await page.getByRole('button', { name: 'Comment', exact: true }).click()
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
  // A 1×1 PNG, uploaded through the sheet's file input.
  const png = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==', 'base64')
  await page.locator('.attachments input[type=file]').setInputFiles({ name: 'diagram.png', mimeType: 'image/png', buffer: png })
  const thumb = page.locator('.attachments .thumb-img img')
  await expect(thumb).toHaveAttribute('src', /^blob:/)
  await page.locator('.attachments .thumb-x').click()
  await expect(thumb).toHaveCount(0)
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
  await expect(who.locator('.pill')).toHaveCount(1)
  await who.getByLabel('Assign someone').selectOption({ label: 'Ana' })
  await expect(who.locator('.pill')).toHaveCount(2)
  // A comment, mentioning Ana through the suggestions.
  const box = page.getByTestId('comment-box')
  await box.fill('Thanks! ')
  await box.press('End')
  await box.pressSequentially('@an')
  await page.getByRole('option', { name: /Ana/ }).click()
  await box.pressSequentially('will water them **tomorrow**')
  await page.getByRole('button', { name: 'Comment', exact: true }).click()
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
  await expect(page.getByTestId('task-estimate')).toHaveValue('90')
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

  // The page out as PDF: a clean copy, printed.
  await page.goto(`/w/${ws.id}/p/${by.Garden.id}`)
  await expect(page.locator('.md-view img')).toBeVisible()
  await page.getByRole('button', { name: 'Page options' }).click()
  await page.getByTestId('page-pdf').click()
  const frame = page.frameLocator('iframe[data-print]')
  await expect(frame.locator('h1.title')).toHaveText('Garden')
  await expect(frame.locator('a', { hasText: 'Roses' })).toBeVisible()
  await expect.poll(() => frame.locator('img').evaluate((i: HTMLImageElement) => i.naturalWidth)).toBe(1)
  await page.screenshot({ path: 'test-results/imported.png' })
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
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Studio', kind: 'personal', modules: ['home', 'files', 'sheets', 'diagrams', 'whiteboards'] } })).json()
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
  const md = await content()
  expect(md).toMatch(new RegExp(`\\[Launch plan\\]\\(/w/${ws.id}/d/[^)]+\\)`))
  expect(md).toMatch(new RegExp(`\\[Launch plan\\]\\(/w/${ws.id}/s/[^)]+\\)`))

  // Reading: a whiteboard shows too; a double click opens it over the page,
  // to look at, or to draw on.
  const board = await (await request.post(`/api/v1/workspaces/${ws.id}/drawings`, { headers: H, data: { title: 'Sketches', preview: '<svg xmlns="http://www.w3.org/2000/svg" width="40" height="20"><rect width="40" height="20" fill="#af52de"/></svg>' } })).json()
  const cur = await (await request.get(`/api/v1/pages/${pg.id}`, { headers: H })).json()
  const put = await request.put(`/api/v1/pages/${pg.id}`, { headers: H, data: { content: `${md}\n\n[Sketches](/w/${ws.id}/whiteboards/${board.id})\n`, version: cur.version } })
  expect(put.ok()).toBe(true)
  await page.goto(`/w/${ws.id}/p/${pg.id}`)
  await expect(page.getByTestId('whiteboard-embed').locator('img')).toBeVisible()
  await page.getByTestId('whiteboard-embed').dblclick()
  await expect(over.getByTestId('whiteboard-title')).toHaveValue('Sketches')
  await expect(over.getByTestId('whiteboard-view-mode')).toHaveAttribute('aria-pressed', 'true')
  await over.getByTestId('whiteboard-edit-mode').click()
  await expect(over.getByTestId('whiteboard-edit-mode')).toHaveAttribute('aria-pressed', 'true')
  await over.getByTestId('embed-done').click()
  await expect(over).toBeHidden()
  // Still reading (the double click didn't start editing the page).
  await expect(page.getByRole('button', { name: 'Edit', exact: true })).toBeVisible()
  await page.getByTestId('sheet-embed').dblclick()
  await expect(over.getByTestId('sheet-title')).toHaveValue('Launch plan')
  await over.getByTestId('embed-done').click()
})

test('projects: a folder joined to a task list shows its progress; the list shows its folder; pages are filed in folders', async ({ page, request }) => {
  const login = await (await request.post('/api/v1/auth/login', { data: { username: ADMIN_USER, password: NEW_PASSWORD } })).json()
  const H = { Authorization: `Bearer ${login.access_token}` }
  const ws = await (await request.post('/api/v1/workspaces', { headers: H, data: { name: 'Studio X', kind: 'work', modules: ['home', 'tasks', 'board', 'pages', 'sheets', 'diagrams', 'files'] } })).json()
  const base = `/api/v1/workspaces/${ws.id}`
  const list = await (await request.post(`${base}/lists`, { headers: H, data: { name: 'Launch', color: '#ff9500' } })).json()
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
})

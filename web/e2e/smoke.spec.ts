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
  await body.fill('rent = 650\nbills = 120\nrent + bills =')
  await expect(page.locator('.mg-r')).toHaveText(' 770')
  await body.press('End')
  await body.press('Tab')
  await expect(body).toHaveValue('rent = 650\nbills = 120\nrent + bills = 770')
  await expect(page.locator('.mg-r')).toHaveCount(0)
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
  await page.locator('#task-notes').fill('**Bring** the [rubric](https://example.org)\n\n- [ ] outline\n- [ ] draft')
  await page.locator('#task-title').click() // leave the notes: they save and render
  const view = page.getByTestId('task-notes-view')
  await expect(view.locator('strong')).toHaveText('Bring')
  await expect(view.getByRole('link', { name: 'rubric' })).toHaveAttribute('href', 'https://example.org')
  await view.locator('input[type=checkbox]').first().check()
  await expect(view.locator('input[type=checkbox]').first()).toBeChecked()
  // Clicking the text edits the source, with the tick in it.
  await view.locator('p').first().click()
  await expect(page.locator('#task-notes')).toHaveValue(/- \[x\] outline/)
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
  await page.getByTestId('journal-entry').locator('textarea').fill('Finished the **report**. Long walk after.')
  await page.getByRole('radio', { name: 'Good' }).click()
  await expect(page.getByTestId('journal-state')).toContainText('Saved')
  await page.reload()
  await expect(page.getByTestId('journal-entry')).toContainText('Finished the report')
  await expect(page.getByRole('radio', { name: 'Good' })).toHaveAttribute('aria-checked', 'true')
  await expect(page.locator(`.day[data-day="${iso(d.getFullYear())}"]`)).toContainText('🙂')
  await page.screenshot({ path: 'test-results/journal.png' })
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

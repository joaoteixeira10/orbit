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
  await expect(page.locator('.calc-r')).toHaveText(' 770')
  await body.press('End')
  await body.press('Tab')
  await expect(body).toHaveValue('rent = 650\nbills = 120\nrent + bills = 770')
  await expect(page.locator('.calc-r')).toHaveCount(0)
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

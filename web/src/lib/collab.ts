// Editing together: a document's changes (Yjs updates) go through the
// server's log for it — numbered, in order — and on to whoever else is
// there (an event, or fetched when one was missed: the event stream may drop
// one). Cursors and selections (Yjs awareness) are relayed, not kept.
//
// An empty log starts from what's saved: one tab gets to (the server says
// which), building the document under a fixed client id so that even two
// tabs starting at once would make the very same changes. Now and then a
// tab folds the log into one change (so a newcomer reads one, not hundreds).
import * as Y from 'yjs'
import { Awareness, applyAwarenessUpdate, encodeAwarenessUpdate, removeAwarenessStates } from 'y-protocols/awareness'
import { api, CLIENT_ID } from './api'
import { onLiveEvent } from './live'

export type CollabKind = 'page' | 'board'

interface LogOut {
  seq: number
  fresh: boolean
  version: number
  updates: { seq: number; data: string; client: string }[]
}

const b64 = (u: Uint8Array) => {
  let s = ''
  for (let i = 0; i < u.length; i += 0x8000) s += String.fromCharCode(...u.subarray(i, i + 0x8000))
  return btoa(s)
}
const unb64 = (s: string) => Uint8Array.from(atob(s), (c) => c.charCodeAt(0))

const SEND_MS = 80 // gather a burst of typing into one change
const AWARE_MS = 100 // a cursor's moves, at most this often
const POLL_MS = 4000 // and a look at the log, in case an event was missed
const FOLD_EVERY = 200 // changes after which a tab folds the log

export class CollabLink {
  readonly doc = new Y.Doc()
  readonly awareness = new Awareness(this.doc)
  seq = 0 // the last change applied from the log
  private pending: Uint8Array[] = []
  private sendTimer: ReturnType<typeof setTimeout> | undefined
  private sending: Promise<void> | null = null
  private catching: Promise<void> | null = null
  private offs: (() => void)[] = []
  private stopped = false
  private sinceFold = 0
  /** Something went wrong the tab can't mend (saved another way): start over. */
  onReset: () => void = () => {}

  constructor(
    readonly kind: CollabKind,
    readonly id: string,
  ) {}

  get key() {
    return `${this.kind}:${this.id}`
  }
  private get path() {
    return `/collab/${this.kind}/${this.id}`
  }

  // start reads the log; fresh: it was empty, and this tab may (seed) or
  // must wait for another to start it.
  async start(): Promise<{ fresh: boolean; seed: boolean }> {
    this.offs.push(
      onLiveEvent('collab', (e) => {
        if (e.doc !== this.key || this.stopped) return
        if (e.reset) return this.onReset()
        if (e.client === CLIENT_ID) return
        const seq = Number(e.seq)
        if (typeof e.data === 'string' && seq === this.seq + 1) {
          Y.applyUpdate(this.doc, unb64(e.data), this)
          this.seq = seq
          this.sinceFold++
        } else if (seq > this.seq) void this.catchUp()
      }),
      onLiveEvent('awareness', (e) => {
        if (e.doc !== this.key || e.client === CLIENT_ID || typeof e.data !== 'string') return
        applyAwarenessUpdate(this.awareness, unb64(e.data), this)
      }),
    )
    // Our own changes: to the log (those applied from it come with `this`).
    const onUpdate = (u: Uint8Array, origin: unknown) => {
      if (origin === this || this.stopped) return
      this.pending.push(u)
      clearTimeout(this.sendTimer)
      this.sendTimer = setTimeout(() => void this.flush(), SEND_MS)
    }
    this.doc.on('update', onUpdate)
    this.offs.push(() => this.doc.off('update', onUpdate))
    // Only this tab's own cursor goes out (others' come and go here), at
    // most every AWARE_MS.
    let awareTimer: ReturnType<typeof setTimeout> | undefined
    const onAware = ({ added, updated, removed }: { added: number[]; updated: number[]; removed: number[] }, origin: unknown) => {
      if (origin === this || this.stopped) return
      if (![...added, ...updated, ...removed].includes(this.doc.clientID) || awareTimer) return
      awareTimer = setTimeout(() => {
        awareTimer = undefined
        if (this.stopped) return
        void api(`${this.path}/awareness`, { method: 'POST', body: JSON.stringify({ client: CLIENT_ID, data: b64(encodeAwarenessUpdate(this.awareness, [this.doc.clientID])) }) }).catch(() => undefined)
      }, AWARE_MS)
    }
    this.awareness.on('update', onAware)
    this.offs.push(() => this.awareness.off('update', onAware))

    const log = await api<LogOut>(`${this.path}?since=0`)
    this.apply(log)
    const poll = setInterval(() => void this.catchUp(), POLL_MS)
    this.offs.push(() => clearInterval(poll))
    if (!log.fresh) return { fresh: false, seed: false }
    const { go } = await api<{ go: boolean }>(`${this.path}/seed`, { method: 'POST', body: JSON.stringify({ client: CLIENT_ID }) })
    return { fresh: true, seed: go }
  }

  private apply(log: LogOut) {
    // The log went back (started over elsewhere): this tab starts over too.
    if (log.seq < this.seq) return this.onReset()
    Y.transact(
      this.doc,
      () => {
        for (const u of log.updates) {
          if (u.seq <= this.seq) continue
          Y.applyUpdate(this.doc, unb64(u.data), this)
          this.seq = u.seq
          this.sinceFold++
        }
      },
      this,
    )
    if (log.seq > this.seq) this.seq = log.seq
  }

  // catchUp fetches what's after the last change this tab has.
  catchUp(): Promise<void> {
    if (this.catching) return this.catching
    this.catching = (async () => {
      try {
        this.apply(await api<LogOut>(`${this.path}?since=${this.seq}`))
      } catch {
        /* the next event tries again */
      } finally {
        this.catching = null
      }
    })()
    return this.catching
  }

  // seed builds the document from what's saved, as every tab would (a
  // fixed client id: the same changes, whoever makes them).
  seed(build: (doc: Y.Doc) => void) {
    const tmp = new Y.Doc()
    tmp.clientID = 1
    build(tmp)
    Y.applyUpdate(this.doc, Y.encodeStateAsUpdate(tmp)) // goes to the log as ours
    tmp.destroy()
  }

  // waitSeeded waits for another tab's start (or gives up: then seed here).
  async waitSeeded(ms = 6000): Promise<boolean> {
    const until = Date.now() + ms
    while (Date.now() < until && !this.stopped) {
      await new Promise((r) => setTimeout(r, 400))
      await this.catchUp()
      if (this.seq > 0) return true
    }
    return this.seq > 0
  }

  // flush sends what's waiting, as one change.
  flush(): Promise<void> {
    clearTimeout(this.sendTimer)
    if (this.sending) return this.sending.then(() => this.flush())
    if (!this.pending.length || this.stopped) return Promise.resolve()
    const data = Y.mergeUpdates(this.pending)
    this.pending = []
    this.sending = (async () => {
      for (let attempt = 0; attempt < 5 && !this.stopped; attempt++) {
        try {
          const r = await api<{ seq: number }>(this.path, { method: 'POST', body: JSON.stringify({ client: CLIENT_ID, data: b64(data) }) })
          // Ours is in; catch up if others' came between.
          if (r.seq === this.seq + 1) this.seq = r.seq
          else await this.catchUp()
          this.sinceFold++
          if (this.sinceFold >= FOLD_EVERY) void this.fold()
          return
        } catch {
          await new Promise((r) => setTimeout(r, 400 * (attempt + 1)))
        }
      }
    })().finally(() => (this.sending = null))
    return this.sending
  }

  // fold replaces the log so far with one change: the document as it is.
  private async fold() {
    this.sinceFold = 0
    const upto = this.seq
    if (upto < 2) return
    try {
      await api(`${this.path}/compact`, { method: 'POST', body: JSON.stringify({ client: CLIENT_ID, upto, data: b64(Y.encodeStateAsUpdate(this.doc)) }) })
    } catch {
      /* another time */
    }
  }

  async stop() {
    if (this.stopped) return
    await this.flush().catch(() => undefined)
    this.stopped = true
    removeAwarenessStates(this.awareness, [this.doc.clientID], 'leave')
    void api(`${this.path}/awareness`, { method: 'POST', body: JSON.stringify({ client: CLIENT_ID, data: b64(encodeAwarenessUpdate(this.awareness, [this.doc.clientID])) }), keepalive: true }).catch(() => undefined)
    this.offs.forEach((f) => f())
    this.awareness.destroy()
    this.doc.destroy()
  }
}

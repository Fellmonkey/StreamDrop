/**
 * Minimal typed client for the StreamDrop REST API (contract: front_spec.md §8).
 * Used by integration specs to arrange state and assert server-side results.
 */

export class Api {
  token = ''

  constructor(public base: string) {}

  async init(): Promise<void> {
    const res = await fetch(this.base + '/')
    const html = await res.text()
    const m = html.match(/<meta name="streamdrop-token" content="([^"]+)"/)
    if (!m) throw new Error('meta[name=streamdrop-token] not found in index.html')
    this.token = m[1]
  }

  async req<T>(method: string, path: string, body?: unknown): Promise<T> {
    const res = await fetch(this.base + '/api/v1' + path, {
      method,
      headers: { 'X-StreamDrop-Token': this.token, 'Content-Type': 'application/json' },
      body: body === undefined ? undefined : JSON.stringify(body),
    })
    const text = await res.text()
    let json: any = null
    if (text) {
      try {
        json = JSON.parse(text)
      } catch {
        json = null
      }
    }
    if (!res.ok) {
      const code = json?.error?.code
      throw new Error(`API ${method} ${path}: ${res.status} ${code ?? text.slice(0, 200)}`)
    }
    return json as T
  }

  status() {
    return this.req('GET', '/status')
  }
  stats() {
    return this.req('GET', '/stats')
  }
  settings() {
    return this.req<{ settings: Record<string, string> }>('GET', '/settings').then((r) => r.settings)
  }
  putSettings(patch: Record<string, unknown>) {
    return this.req('PUT', '/settings', patch)
  }
  queue() {
    return this.req('GET', '/queue')
  }
  pauseQueue() {
    return this.req('POST', '/queue/pause')
  }
  resumeQueue() {
    return this.req('POST', '/queue/resume')
  }
  reorderQueue(ids: string[]) {
    return this.req('POST', '/queue/reorder', { ids })
  }
  transfers() {
    return this.req<{ transfers: any[] }>('GET', '/transfers?limit=100').then((r) => r.transfers)
  }
  getTransfer(id: string) {
    return this.req('GET', `/transfers/${id}`)
  }
  deleteTransfer(id: string) {
    return this.req('DELETE', `/transfers/${id}`)
  }
  transferAction(id: string, action: string) {
    return this.req('POST', `/transfers/${id}/${action}`)
  }
  createTransfers(body: Record<string, unknown>) {
    return this.req<{ transfers: any[] }>('POST', '/transfers', body).then((r) => r.transfers)
  }
  remotes() {
    return this.req<{ remotes: any[] }>('GET', '/remotes').then((r) => r.remotes ?? [])
  }
  createRemote(body: Record<string, unknown>) {
    return this.req<{ remote: any }>('POST', '/remotes', body)
  }
  deleteRemote(id: string) {
    return this.req('DELETE', `/remotes/${id}`)
  }
  testRemote(id: string) {
    return this.req('POST', `/remotes/${id}/test`)
  }
  explorer(remoteId: string, path: string) {
    return this.req(
      'GET',
      `/explorer?remote_id=${encodeURIComponent(remoteId)}&path=${encodeURIComponent(path)}&q=`,
    )
  }
  mkdir(remoteId: string, path: string, name: string) {
    return this.req('POST', '/explorer/mkdir', { remote_id: remoteId, path, name })
  }
}

/** ensureRemote returns an existing integration remote or creates one from .env. */
export async function ensureRemote(env: { base: string; user: string; pass: string; url: string }) {
  const api = new Api(env.base)
  await api.init()
  const existing = (await api.remotes()).filter((r: any) => r.name.startsWith('e2e-intg-'))
  if (existing.length) return { id: existing[0].id, name: existing[0].name }
  const ts = Date.now()
  const created = await api.createRemote({
    name: `e2e-intg-${ts}`,
    provider: 'mailru',
    endpoint: env.url,
    options: { url: env.url, user: env.user, pass: env.pass },
  })
  return { id: created.remote.id, name: created.remote.name }
}

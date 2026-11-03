import type {
  ApiErrorPayload,
  AuthProfile,
  DepItem,
  DetectedBrowser,
  InspectMetadata,
  LogLine,
  OAuthProvider,
  OAuthSession,
  QueueState,
  Remote,
  Settings,
  Snapshot,
  Status,
  Stats,
  Template,
  Transfer,
} from '@/types/api'

/** ApiError carries the machine-readable code; the UI localizes by code. */
export class ApiError extends Error {
  code: string
  kind: string
  status: number
  params: Record<string, unknown>
  details?: string

  constructor(payload: ApiErrorPayload) {
    super(payload.code)
    this.code = payload.code
    this.kind = payload.kind
    this.status = payload.http_status
    this.params = payload.params ?? {}
    this.details = payload.details
  }
}

/** readToken reads the per-run token injected into the page shell. */
function readToken(): string {
  const meta = document.querySelector('meta[name="streamdrop-token"]')
  return meta?.getAttribute('content') ?? ''
}

let reloading = false

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  headers.set('X-StreamDrop-Token', readToken())
  if (init.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  const response = await fetch(`/api/v1${path}`, { ...init, headers })
  const text = await response.text()
  let json: unknown = null
  if (text) {
    try {
      json = JSON.parse(text)
    } catch {
      json = null
    }
  }
  if (!response.ok) {
    const payload = (json as { error?: ApiErrorPayload } | null)?.error
    if (payload) {
      // The page token is gone or rotated: reload once to pick up a fresh one.
      if (payload.code === 'local_auth_required' && !reloading) {
        reloading = true
        window.location.reload()
      }
      throw new ApiError(payload)
    }
    throw new ApiError({ code: 'internal_error', kind: 'source', http_status: response.status })
  }
  return json as T
}

export const api = {
  token: readToken,

  inspect(url: string, authProfileId?: string) {
    return request<{ metadata: InspectMetadata }>('/inspect', {
      method: 'POST',
      body: JSON.stringify({ url, auth_profile_id: authProfileId ?? '' }),
    }).then((r) => r.metadata)
  },

  createTransfers(body: Record<string, unknown>) {
    return request<{ transfers: Transfer[] }>('/transfers', {
      method: 'POST',
      body: JSON.stringify(body),
    })
  },

  listTransfers(params: { page?: number; limit?: number; status?: string } = {}) {
    const query = new URLSearchParams()
    if (params.page) query.set('page', String(params.page))
    if (params.limit) query.set('limit', String(params.limit))
    if (params.status) query.set('status', params.status)
    return request<{ transfers: Transfer[]; total: number; page: number }>(`/transfers?${query}`)
  },

  getTransfer(id: string) {
    return request<{ transfer: Transfer; logs: LogLine[] }>(`/transfers/${id}`)
  },

  deleteTransfer(id: string) {
    return request<{ deleted: boolean }>(`/transfers/${id}`, { method: 'DELETE' })
  },

  transferAction(id: string, action: 'pause' | 'resume' | 'abort') {
    return request<Transfer>(`/transfers/${id}/${action}`, { method: 'POST' })
  },

  transferLogs(id: string, tail = 200) {
    return request<{ logs: LogLine[] }>(`/transfers/${id}/logs?tail=${tail}`).then((r) => r.logs ?? [])
  },

  queue() {
    return request<QueueState>('/queue')
  },

  pauseQueue() {
    return request<QueueState>('/queue/pause', { method: 'POST' })
  },

  resumeQueue() {
    return request<QueueState>('/queue/resume', { method: 'POST' })
  },

  reorderQueue(ids: string[]) {
    return request<QueueState>('/queue/reorder', {
      method: 'POST',
      body: JSON.stringify({ ids }),
    })
  },

  remotes() {
    return request<{ remotes: Remote[] }>('/remotes').then((r) => r.remotes ?? [])
  },

  createRemote(body: { name: string; provider: string; endpoint?: string; options?: Record<string, string> }) {
    return request<{ remote: Remote; error?: { error: ApiErrorPayload } }>('/remotes', {
      method: 'POST',
      body: JSON.stringify(body),
    })
  },

  deleteRemote(id: string) {
    return request<{ deleted: boolean }>(`/remotes/${id}`, { method: 'DELETE' })
  },

  testRemoteCredentials(provider: string, options: Record<string, string>) {
    return request<{ ok: boolean; rtt_ms?: number; quota?: { total: number; used: number; free: number }; error?: { error: ApiErrorPayload } }>(
      '/remotes/test',
      { method: 'POST', body: JSON.stringify({ provider, options }) },
    )
  },

  testRemote(id: string) {
    return request<{ ok: boolean; rtt_ms?: number; quota?: { total: number; used: number; free: number }; error?: { error: ApiErrorPayload } }>(
      `/remotes/${id}/test`,
      { method: 'POST' },
    )
  },

  explorer(remoteId: string, path: string, q = '') {
    const query = new URLSearchParams({ remote_id: remoteId, path, q })
    return request<{ path: string; directories: { name: string; path: string; is_dir: boolean }[]; quota?: { total: number; used: number; free: number } }>(
      `/explorer?${query}`,
    )
  },

  mkdir(remoteId: string, path: string, name: string) {
    return request<{ path: string }>('/explorer/mkdir', {
      method: 'POST',
      body: JSON.stringify({ remote_id: remoteId, path, name }),
    })
  },

  oauthProviders() {
    return request<{ providers: OAuthProvider[] }>('/oauth/providers').then((r) => r.providers ?? [])
  },

  oauthAuthorize(provider: string, clientId = '', clientSecret = '') {
    return request<{ session_id: string; auth_url: string; redirect_uri: string; expires_at: string }>(
      `/oauth/${provider}/authorize`,
      { method: 'POST', body: JSON.stringify({ client_id: clientId, client_secret: clientSecret }) },
    )
  },

  oauthSession(id: string) {
    return request<OAuthSession>(`/oauth/session/${id}`)
  },

  oauthRevoke(provider: string) {
    return request<{ revoked: boolean }>(`/oauth/${provider}`, { method: 'DELETE' })
  },

  browsers() {
    return request<{ browsers: DetectedBrowser[] }>('/auth/browsers').then((r) => r.browsers ?? [])
  },

  profiles() {
    return request<{ profiles: AuthProfile[] }>('/auth/profiles').then((r) => r.profiles ?? [])
  },

  createProfile(body: Record<string, unknown>) {
    return request<{ profile?: AuthProfile }>('/auth/profiles', {
      method: 'POST',
      body: JSON.stringify(body),
    })
  },

  deleteProfile(id: string) {
    return request<{ deleted: boolean }>(`/auth/profiles/${id}`, { method: 'DELETE' })
  },

  testProfile(id: string, url: string) {
    return request<{ ok: boolean; metadata: InspectMetadata }>(`/auth/profiles/${id}/test`, {
      method: 'POST',
      body: JSON.stringify({ url }),
    })
  },

  deps() {
    return request<{ components: DepItem[]; installed: number; total: number }>('/deps')
  },

  installDep(id: string) {
    return request<{ state: string }>(`/deps/${id}/install`, { method: 'POST' })
  },

  removeDep(id: string) {
    return request<{ deleted: boolean }>(`/deps/${id}`, { method: 'DELETE' })
  },

  settings() {
    return request<{ settings: Settings }>('/settings').then((r) => r.settings)
  },

  updateSettings(body: Record<string, string | number | boolean>) {
    return request<{ settings: Settings }>('/settings', {
      method: 'PUT',
      body: JSON.stringify(body),
    }).then((r) => r.settings)
  },

  testNotification() {
    return request<{ ok: boolean }>('/notifications/test', { method: 'POST' })
  },

  openUI(mode: 'browser' | 'app') {
    return request<{ ok: boolean }>('/ui/open', { method: 'POST', body: JSON.stringify({ mode }) })
  },

  openURL(url: string, browser = 'default') {
    return request<{ ok: boolean }>('/ui/open-url', {
      method: 'POST',
      body: JSON.stringify({ url, browser }),
    })
  },

  stats() {
    return request<Stats>('/stats')
  },

  status() {
    return request<Status>('/status')
  },

  templates() {
    return request<{ templates: Template[] }>('/templates').then((r) => r.templates ?? [])
  },

  update() {
    return request<{
      available: boolean
      current_version: string
      version?: string
      size_bytes?: number
      sha256?: string
      notes?: string
      error_code?: string
      signature_checked?: boolean
    }>('/update')
  },

  applyUpdate() {
    return request<{ state: string }>('/update/apply', { method: 'POST' })
  },

  snapshot(): Promise<Snapshot> {
    // REST has no snapshot endpoint; the initial state is assembled from the
    // same sources the websocket snapshot uses.
    return Promise.all([this.queue(), this.remotes(), this.deps(), this.listTransfers({ limit: 50 })]).then(
      ([queue, remotes, deps, history]) => ({
        server_time: new Date().toISOString(),
        queue,
        transfers: history.transfers,
        remotes: remotes.map((r) => ({
          id: r.id,
          name: r.name,
          provider: r.provider,
          brand: r.brand,
          status: r.status,
          rtt_ms: r.rtt_ms,
          last_error: r.last_error,
        })),
        deps: deps.components,
        metrics: [],
      }),
    )
  },
}

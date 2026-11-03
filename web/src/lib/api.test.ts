import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError } from '@/lib/api'

/** Contract tests for the REST client (front_spec.md §8). */

const TOKEN = 'test-token-123'
const reload = vi.fn()

function installToken() {
  const meta = {
    getAttribute: (name: string) => (name === 'content' ? TOKEN : null),
  }
  const doc = {
    querySelector: (sel: string) => (sel.includes('streamdrop-token') ? meta : null),
    head: { innerHTML: '' },
  }
  vi.stubGlobal('document', doc)
  vi.stubGlobal('window', { location: { reload } })
}

beforeEach(() => {
  reload.mockClear()
  installToken()
  vi.stubGlobal('fetch', vi.fn())
})

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

function mockResponse(status: number, body: unknown) {
  const text = typeof body === 'string' ? body : JSON.stringify(body)
  vi.mocked(fetch).mockImplementation(() =>
    Promise.resolve(new Response(text, { status, headers: { 'Content-Type': 'application/json' } })),
  )
}

describe('api client', () => {
  it('sends the local token on every request', async () => {
    mockResponse(200, { status: { engine_ready: true }, version: 'dev' })
    await api.status()
    const [url, init] = vi.mocked(fetch).mock.calls[0]
    const headers = new Headers(init?.headers)
    expect(headers.get('X-StreamDrop-Token')).toBe(TOKEN)
    expect(String(url)).toBe('/api/v1/status')
  })

  it('sets Content-Type json when a body is present', async () => {
    mockResponse(200, { transfers: [] })
    await api.createTransfers({ urls: ['https://example.com/a.iso'] })
    const [, init] = vi.mocked(fetch).mock.calls[0]
    const headers = new Headers(init?.headers)
    expect(headers.get('Content-Type')).toBe('application/json')
    expect(JSON.parse(String(init?.body))).toEqual({ urls: ['https://example.com/a.iso'] })
  })

  it('maps machine-readable error payloads to ApiError', async () => {
    mockResponse(400, {
      error: { code: 'inspect_html_source', kind: 'source', http_status: 400, params: { engine: 'curl' } },
    })
    await expect(api.inspect('https://example.com/page')).rejects.toMatchObject({
      code: 'inspect_html_source',
      kind: 'source',
      status: 400,
      params: { engine: 'curl' },
    })
  })

  it('maps non-JSON failure to internal_error', async () => {
    mockResponse(502, 'Bad Gateway')
    await expect(api.queue()).rejects.toMatchObject({ code: 'internal_error', status: 502 })
  })

  it('reloads exactly once on local_auth_required', async () => {
    mockResponse(401, { error: { code: 'local_auth_required', kind: 'auth', http_status: 401 } })
    await expect(api.stats()).rejects.toMatchObject({ code: 'local_auth_required' })
    expect(reload).toHaveBeenCalledTimes(1)
    // second call must not reload again
    await expect(api.stats()).rejects.toMatchObject({ code: 'local_auth_required' })
    expect(reload).toHaveBeenCalledTimes(1)
  })

  it('builds query strings for listTransfers and explorer', async () => {
    mockResponse(200, { transfers: [], total: 0, page: 1 })
    await api.listTransfers({ page: 2, limit: 20, status: 'completed' })
    expect(String(vi.mocked(fetch).mock.calls[0][0])).toBe('/api/v1/transfers?page=2&limit=20&status=completed')

    mockResponse(200, { path: '/', directories: [] })
    await api.explorer('r1', '/Distros', 'ubuntu')
    expect(String(vi.mocked(fetch).mock.calls[1][0])).toBe(
      '/api/v1/explorer?remote_id=r1&path=%2FDistros&q=ubuntu',
    )
  })

  it('throwing ApiError is an instanceof Error subclass with payload fields', () => {
    const e = new ApiError({ code: 'x', kind: 'cloud', http_status: 503, details: 'upstream down' })
    expect(e).toBeInstanceOf(Error)
    expect(e.code).toBe('x')
    expect(e.kind).toBe('cloud')
    expect(e.status).toBe(503)
    expect(e.details).toBe('upstream down')
  })
})

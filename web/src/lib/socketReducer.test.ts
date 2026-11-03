import { describe, expect, it } from 'vitest'
import { INITIAL_SOCKET_STATE, reduceSocketState } from '@/lib/socketReducer'
import type { NotifyEvent, QueueView, Snapshot, TransferMetric } from '@/types/api'

/** WS contract tests per front_spec.md §8 (events handled by the client). */

function metric(overrides: Partial<TransferMetric> = {}): TransferMetric {
  return {
    transfer_id: 't1',
    status: 'running',
    mode: 'ring_buffer',
    bytes_downloaded: 1000,
    bytes_confirmed: 500,
    total_size: 2000,
    download_bps: 42_000_000,
    upload_bps: 40_000_000,
    buffer_used_bytes: 64 * 1024 * 1024,
    buffer_size_bytes: 128 * 1024 * 1024,
    buffer_usage_pct: 50,
    eta_seconds: 10,
    retry_attempt: 0,
    ...overrides,
  }
}

describe('reduceSocketState', () => {
  it('returns the same state for unknown events', () => {
    const next = reduceSocketState(INITIAL_SOCKET_STATE, { event: 'unknown_event', data: {} })
    expect(next).toBe(INITIAL_SOCKET_STATE)
  })

  it('applies snapshot: queue + metrics by transfer id', () => {
    const queue: QueueView = { paused: false, parallel_limit: 1, active: 1, pending: 0, items: [] }
    const snapshot: Snapshot = {
      server_time: '2026-11-03T12:00:00Z',
      queue,
      transfers: [],
      remotes: [],
      deps: [],
      metrics: [metric()],
    }
    const next = reduceSocketState(INITIAL_SOCKET_STATE, { event: 'snapshot', data: snapshot })
    expect(next.queue).toBe(queue)
    expect(next.snapshot).toBe(snapshot)
    expect(next.metrics.t1.bytes_downloaded).toBe(1000)
  })

  it('merges transfer_metrics by id, preserving others', () => {
    const a = reduceSocketState(INITIAL_SOCKET_STATE, { event: 'transfer_metrics', data: metric() })
    const b = reduceSocketState(a, {
      event: 'transfer_metrics',
      data: metric({ transfer_id: 't2', bytes_downloaded: 777 }),
    })
    expect(b.metrics.t1.bytes_downloaded).toBe(1000)
    expect(b.metrics.t2.bytes_downloaded).toBe(777)
  })

  it('appends transfer_log capped at 300 lines per transfer', () => {
    let state = INITIAL_SOCKET_STATE
    for (let i = 0; i < 310; i++) {
      state = reduceSocketState(state, {
        event: 'transfer_log',
        data: { transfer_id: 't1', ts: 'x', level: 'info', source: 'engine', message: `m${i}` },
      })
    }
    expect(state.logs.t1).toHaveLength(300)
    expect(state.logs.t1[0].message).toBe('m10')
  })

  it('queue_update replaces the queue view', () => {
    const queue: QueueView = { paused: true, parallel_limit: 2, active: 0, pending: 3, items: [] }
    const next = reduceSocketState(INITIAL_SOCKET_STATE, { event: 'queue_update', data: { queue } })
    expect(next.queue).toEqual(queue)
  })

  it('notify stores the event; clearNotify-style reset is a plain object merge', () => {
    const notify: NotifyEvent = { kind: 'transfer_completed', level: 'success', files_count: 1 }
    const next = reduceSocketState(INITIAL_SOCKET_STATE, { event: 'notify', data: notify })
    expect(next.notify).toEqual(notify)
  })

  it('dep_progress and update_progress are stored by key', () => {
    const s1 = reduceSocketState(INITIAL_SOCKET_STATE, {
      event: 'dep_progress',
      data: { id: 'rclone', received: 10, total: 100, speed_bps: 1, state: 'downloading' },
    })
    expect(s1.depProgress.rclone.state).toBe('downloading')

    const s2 = reduceSocketState(s1, {
      event: 'update_progress',
      data: { version: 'v1.1.0', received: 5, total: 50, state: 'downloading' },
    })
    expect(s2.updateProgress?.version).toBe('v1.1.0')
  })
})

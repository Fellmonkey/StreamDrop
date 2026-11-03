import { useCallback, useEffect, useRef, useState } from 'react'
import type { LogLine, NotifyEvent, QueueView, Snapshot, TransferMetric } from '@/types/api'

interface SocketState {
  connected: boolean
  snapshot: Snapshot | null
  metrics: Record<string, TransferMetric>
  logs: Record<string, LogLine[]>
  notify: NotifyEvent | null
  depProgress: Record<string, { received: number; total: number; speed_bps: number; state: string; code?: string }>
  updateProgress: { version: string; received: number; total: number; state: string } | null
  queue: QueueView | null
}

const INITIAL: SocketState = {
  connected: false,
  snapshot: null,
  metrics: {},
  logs: {},
  notify: null,
  depProgress: {},
  updateProgress: null,
  queue: null,
}

/**
 * useStreamSocket owns the realtime channel. A dropped connection never affects
 * transfers: it reconnects with exponential backoff (1s -> 15s) and receives a
 * fresh snapshot, after which metrics catch up automatically.
 */
export function useStreamSocket(onQueueChange?: () => void) {
  const [state, setState] = useState<SocketState>(INITIAL)
  const retry = useRef(0)
  const closed = useRef(false)
  const socket = useRef<WebSocket | null>(null)
  const queueCb = useRef(onQueueChange)
  queueCb.current = onQueueChange

  const connect = useCallback(() => {
    if (closed.current) return
    const token = document.querySelector('meta[name="streamdrop-token"]')?.getAttribute('content') ?? ''
    const proto = window.location.protocol === 'https:' ? 'wss' : 'ws'
    const ws = new WebSocket(`${proto}://${window.location.host}/api/v1/ws?token=${encodeURIComponent(token)}`)
    socket.current = ws

    ws.onopen = () => {
      retry.current = 0
      setState((s) => ({ ...s, connected: true }))
    }

    ws.onmessage = (event) => {
      let parsed: { event: string; data: unknown }
      try {
        parsed = JSON.parse(event.data)
      } catch {
        return
      }
      const { event: name, data } = parsed
      switch (name) {
        case 'snapshot': {
          const snapshot = data as Snapshot
          const metrics: Record<string, TransferMetric> = {}
          for (const m of snapshot.metrics ?? []) metrics[m.transfer_id] = m
          setState((s) => ({ ...s, snapshot, queue: snapshot.queue, metrics: { ...metrics, ...s.metrics } }))
          break
        }
        case 'transfer_metrics': {
          const metric = data as TransferMetric
          setState((s) => ({ ...s, metrics: { ...s.metrics, [metric.transfer_id]: metric } }))
          break
        }
        case 'transfer_log': {
          const line = data as LogLine
          setState((s) => {
            const existing = s.logs[line.transfer_id] ?? []
            const next = [...existing, line].slice(-300)
            return { ...s, logs: { ...s.logs, [line.transfer_id]: next } }
          })
          break
        }
        case 'queue_update': {
          const payload = data as { queue: QueueView }
          setState((s) => ({ ...s, queue: payload.queue }))
          queueCb.current?.()
          break
        }
        case 'notify': {
          setState((s) => ({ ...s, notify: data as NotifyEvent }))
          break
        }
        case 'dep_progress': {
          const p = data as { id: string; received: number; total: number; speed_bps: number; state: string; code?: string }
          setState((s) => ({ ...s, depProgress: { ...s.depProgress, [p.id]: p } }))
          break
        }
        case 'update_progress': {
          setState((s) => ({
            ...s,
            updateProgress: data as { version: string; received: number; total: number; state: string },
          }))
          break
        }
        default:
          break
      }
    }

    ws.onclose = () => {
      socket.current = null
      setState((s) => ({ ...s, connected: false }))
      if (closed.current) return
      retry.current = Math.min(retry.current + 1, 5)
      const delay = Math.min(15000, 1000 * 2 ** (retry.current - 1))
      window.setTimeout(connect, delay)
    }

    ws.onerror = () => {
      ws.close()
    }
  }, [])

  useEffect(() => {
    closed.current = false
    connect()
    return () => {
      closed.current = true
      socket.current?.close()
    }
  }, [connect])

  const clearNotify = useCallback(() => setState((s) => ({ ...s, notify: null })), [])

  return { ...state, clearNotify }
}

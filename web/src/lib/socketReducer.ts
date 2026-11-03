import type { LogLine, NotifyEvent, QueueView, Snapshot, TransferMetric } from '@/types/api'

export interface SocketState {
  connected: boolean
  snapshot: Snapshot | null
  metrics: Record<string, TransferMetric>
  logs: Record<string, LogLine[]>
  notify: NotifyEvent | null
  depProgress: Record<string, { received: number; total: number; speed_bps: number; state: string; code?: string }>
  updateProgress: { version: string; received: number; total: number; state: string } | null
  queue: QueueView | null
}

export const INITIAL_SOCKET_STATE: SocketState = {
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
 * reduceSocketState applies one parsed WebSocket message to the client state.
 * Pure function — the contract surface of front_spec.md §8 (events:
 * snapshot, transfer_metrics, transfer_log, queue_update, notify,
 * dep_progress, update_progress). Unknown events are ignored.
 */
export function reduceSocketState(state: SocketState, event: { event: string; data: unknown }): SocketState {
  const { event: name, data } = event
  switch (name) {
    case 'snapshot': {
      const snapshot = data as Snapshot
      const metrics: Record<string, TransferMetric> = {}
      for (const m of snapshot.metrics ?? []) metrics[m.transfer_id] = m
      return { ...state, snapshot, queue: snapshot.queue, metrics: { ...metrics, ...state.metrics } }
    }
    case 'transfer_metrics': {
      const metric = data as TransferMetric
      return { ...state, metrics: { ...state.metrics, [metric.transfer_id]: metric } }
    }
    case 'transfer_log': {
      const line = data as LogLine
      const existing = state.logs[line.transfer_id] ?? []
      const next = [...existing, line].slice(-300)
      return { ...state, logs: { ...state.logs, [line.transfer_id]: next } }
    }
    case 'queue_update': {
      const payload = data as { queue: QueueView }
      return { ...state, queue: payload.queue }
    }
    case 'notify': {
      return { ...state, notify: data as NotifyEvent }
    }
    case 'dep_progress': {
      const p = data as { id: string; received: number; total: number; speed_bps: number; state: string; code?: string }
      return { ...state, depProgress: { ...state.depProgress, [p.id]: p } }
    }
    case 'update_progress': {
      return {
        ...state,
        updateProgress: data as { version: string; received: number; total: number; state: string },
      }
    }
    default:
      return state
  }
}

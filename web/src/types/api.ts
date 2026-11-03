/** DTOs shared with the Go backend (front_spec.md §8). */

export type TransferStatus =
  | 'queued'
  | 'running'
  | 'paused'
  | 'retrying'
  | 'completed'
  | 'failed'
  | 'aborted'

export interface Quota {
  total: number
  used: number
  free: number
  trashed?: number
}

export interface Remote {
  id: string
  name: string
  provider: string
  brand: string
  endpoint?: string
  rclone_name?: string
  status: 'online' | 'error' | 'unavailable' | 'unknown'
  last_error?: string
  rtt_ms: number
  last_check_at?: string
  created_at: string
  quota?: Quota | null
}

export interface Transfer {
  id: string
  source_url: string
  resolved_filename: string
  filename_override?: string
  file_size: number
  remote_id: string | null
  remote_label: string
  remote_path: string
  engine: string
  auth_profile_id: string | null
  mode: 'ring_buffer' | 'pure_stream'
  buffer_size_bytes: number
  status: TransferStatus
  priority: number
  bytes_transferred: number
  bytes_confirmed: number
  sha256?: string
  retry_attempts: number
  bandwidth_down_bps: number
  bandwidth_up_bps: number
  average_speed_bps: number
  error_kind?: string
  error_message?: string
  started_at?: string
  completed_at?: string
  created_at: string
}

export interface TransferMetric {
  transfer_id: string
  status: string
  mode: string
  bytes_downloaded: number
  bytes_confirmed: number
  total_size: number
  download_bps: number
  upload_bps: number
  buffer_used_bytes: number
  buffer_size_bytes: number
  buffer_usage_pct: number
  eta_seconds: number
  retry_attempt: number
}

export interface QueueEntry {
  id: string
  status: string
  priority: number
}

export interface QueueView {
  paused: boolean
  parallel_limit: number
  active: number
  pending: number
  items: QueueEntry[]
}

export interface QueueItem {
  id: string
  status: string
  priority: number
  filename: string
  size: number
  remote_label: string
  remote_path: string
  bytes_transferred: number
}

export interface QueueState {
  paused: boolean
  parallel_limit: number
  active: number
  pending: number
  items: QueueItem[]
  active_tasks: Transfer[]
}

export interface InspectMetadata {
  original_url: string
  resolved_url: string
  filename: string
  size_bytes: number
  content_type: string
  is_html: boolean
  supports_range: boolean
  engine_hint: string
}

export interface DepItem {
  id: string
  name: string
  version: string
  purpose: string
  license: string
  source_url: string
  download_url: string
  size_bytes: number
  required: boolean
  status: 'installed' | 'missing' | 'outdated' | 'corrupted'
  path?: string
  installed_version?: string
  sha256?: string
  verified: boolean
  note?: string
  platforms: string[]
}

export interface AuthProfile {
  id: string
  name: string
  kind: string
  mode: string
  profile_dir?: string
  cookie_file?: string
  supported: boolean
  note?: string
  has_manual_cookie: boolean
  cookie_mask?: string
  user_agent?: string
  created_at: string
}

export interface DetectedBrowser {
  browser: string
  label: string
  dir: string
  supported: boolean
  note?: string
}

export interface OAuthProvider {
  id: string
  label: string
  rclone_type: string
  scopes: string[]
  needs_client_id: boolean
}

export interface OAuthSession {
  id: string
  provider_id: string
  status: 'pending' | 'done' | 'failed'
  remote_id?: string
  error_code?: string
  expires_at: string
}

export interface Settings {
  parallel_limit: string
  default_mode: string
  default_buffer_bytes: string
  locale: string
  retry_max_attempts: string
  checksum_enabled: string
  bandwidth_down_bps: string
  bandwidth_up_bps: string
  onboarded: string
  crash_reporting_enabled: string
  notifications_toast: string
  telegram_enabled: string
  has_telegram_token: string
  [key: string]: string
}

export interface Stats {
  saved_bytes: number
  wear: {
    classes: { class: string; label: string; bytes: number; percent: number; approximate?: boolean }[]
    average: number
  }
}

export interface Status {
  port: number
  version: string
  engine_ready: boolean
  daemon_ready: boolean
  active: number
  onboarded: boolean
  uptime_seconds: number
  components_ok: number
  components_total: number
  ws_clients: number
  server_time: string
}

export interface Snapshot {
  server_time: string
  queue: QueueView
  transfers: Transfer[]
  remotes: { id: string; name: string; provider: string; brand: string; status: string; rtt_ms: number; last_error?: string }[]
  deps: { id: string; status: string; version: string; required: boolean; verified: boolean }[]
  metrics: TransferMetric[]
}

export interface LogLine {
  id?: number
  transfer_id: string
  ts: string
  level: string
  source: string
  message: string
}

export interface NotifyEvent {
  kind: string
  level: string
  transfer_id?: string
  files_count: number
  failed_count?: number
  bytes?: number
  duration_seconds?: number
  code?: string
}

export interface Template {
  id: string
  category: string
  url: string
  example: string
}

export interface ApiErrorPayload {
  code: string
  kind: string
  http_status: number
  params?: Record<string, unknown>
  details?: string
}

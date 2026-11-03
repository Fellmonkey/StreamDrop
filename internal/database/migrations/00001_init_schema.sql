-- +goose Up
CREATE TABLE IF NOT EXISTS remotes (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    provider TEXT NOT NULL,
    brand TEXT,
    endpoint TEXT,
    rclone_name TEXT NOT NULL,
    encrypted_credentials BLOB,
    status TEXT NOT NULL DEFAULT 'unknown',
    last_error TEXT,
    last_rtt_ms INTEGER DEFAULT 0,
    last_check_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS oauth_tokens (
    provider TEXT PRIMARY KEY,
    remote_id TEXT,
    encrypted_client_secret BLOB,
    encrypted_access_token BLOB,
    encrypted_refresh_token BLOB,
    scope TEXT,
    expires_at DATETIME,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS auth_profiles (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    mode TEXT NOT NULL,
    profile_dir TEXT,
    cookie_file_path TEXT,
    encrypted_headers BLOB,
    supported INTEGER DEFAULT 1,
    note TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS transfers (
    id TEXT PRIMARY KEY,
    source_url TEXT NOT NULL,
    resolved_filename TEXT NOT NULL,
    filename_override TEXT,
    file_size INTEGER DEFAULT 0,
    remote_id TEXT REFERENCES remotes(id) ON DELETE SET NULL,
    remote_label TEXT,
    remote_path TEXT NOT NULL,
    engine TEXT NOT NULL,
    auth_profile_id TEXT REFERENCES auth_profiles(id) ON DELETE SET NULL,
    mode TEXT NOT NULL DEFAULT 'ring_buffer',
    buffer_size_bytes INTEGER DEFAULT 0,
    status TEXT NOT NULL,
    priority INTEGER NOT NULL DEFAULT 0,
    bytes_transferred INTEGER DEFAULT 0,
    bytes_confirmed INTEGER DEFAULT 0,
    sha256 TEXT,
    retry_attempts INTEGER DEFAULT 0,
    bandwidth_down_bps INTEGER DEFAULT 0,
    bandwidth_up_bps INTEGER DEFAULT 0,
    average_speed_bps INTEGER DEFAULT 0,
    error_kind TEXT,
    error_message TEXT,
    started_at DATETIME,
    completed_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_transfers_status ON transfers(status);
CREATE INDEX IF NOT EXISTS idx_transfers_created_at ON transfers(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_transfers_priority ON transfers(status, priority);

CREATE TABLE IF NOT EXISTS transfer_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    transfer_id TEXT NOT NULL REFERENCES transfers(id) ON DELETE CASCADE,
    ts DATETIME DEFAULT CURRENT_TIMESTAMP,
    level TEXT NOT NULL,
    source TEXT NOT NULL,
    message TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_transfer_logs_transfer ON transfer_logs(transfer_id, id);

CREATE TABLE IF NOT EXISTS dependencies (
    id TEXT PRIMARY KEY,
    catalog_version TEXT NOT NULL,
    installed_version TEXT,
    install_path TEXT,
    sha256 TEXT,
    status TEXT NOT NULL,
    required INTEGER DEFAULT 0,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT OR IGNORE INTO settings(key, value) VALUES ('parallel_limit', '1');
INSERT OR IGNORE INTO settings(key, value) VALUES ('default_mode', 'ring_buffer');
INSERT OR IGNORE INTO settings(key, value) VALUES ('default_buffer_bytes', '134217728');
INSERT OR IGNORE INTO settings(key, value) VALUES ('locale', 'ru');
INSERT OR IGNORE INTO settings(key, value) VALUES ('retry_max_attempts', '3');
INSERT OR IGNORE INTO settings(key, value) VALUES ('checksum_enabled', '0');
INSERT OR IGNORE INTO settings(key, value) VALUES ('bandwidth_down_bps', '0');
INSERT OR IGNORE INTO settings(key, value) VALUES ('bandwidth_up_bps', '0');
INSERT OR IGNORE INTO settings(key, value) VALUES ('onboarded', '0');
INSERT OR IGNORE INTO settings(key, value) VALUES ('crash_reporting_enabled', '0');
INSERT OR IGNORE INTO settings(key, value) VALUES ('last_update_check', '');
INSERT OR IGNORE INTO settings(key, value) VALUES ('telegram_enabled', '0');
INSERT OR IGNORE INTO settings(key, value) VALUES ('notifications_toast', '1');

CREATE TABLE IF NOT EXISTS system_stats (
    key TEXT PRIMARY KEY,
    value INTEGER NOT NULL
);
INSERT OR IGNORE INTO system_stats (key, value) VALUES ('saved_bytes_total', 0);

-- +goose Down
DROP TABLE IF EXISTS system_stats;
DROP TABLE IF EXISTS settings;
DROP TABLE IF EXISTS dependencies;
DROP TABLE IF EXISTS transfer_logs;
DROP TABLE IF EXISTS transfers;
DROP TABLE IF EXISTS auth_profiles;
DROP TABLE IF EXISTS oauth_tokens;
DROP TABLE IF EXISTS remotes;

// Package store is the persistence layer over SQLite. It owns row scanning,
// encryption boundaries and time formats so handlers never touch SQL directly.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"streamdrop/internal/apierr"
	"streamdrop/internal/security"
)

// Store wraps the database handle.
type Store struct {
	db     *sql.DB
	secret security.SecretStore
}

// New builds a store.
func New(db *sql.DB, secret security.SecretStore) *Store {
	return &Store{db: db, secret: secret}
}

// DB exposes the handle (used by migrations and tests).
func (s *Store) DB() *sql.DB { return s.db }

// timeLayout is the format written to DATETIME columns.
const timeLayout = time.RFC3339

func nowText() string { return time.Now().UTC().Format(timeLayout) }

func parseTime(v sql.NullString) *time.Time {
	if !v.Valid || v.String == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05Z"} {
		if t, err := time.Parse(layout, v.String); err == nil {
			u := t.UTC()
			return &u
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------

// Settings returns every settings row.
func (s *Store) Settings() (map[string]string, error) {
	rows, err := s.db.Query(`SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// GetSetting returns a setting value or the fallback.
func (s *Store) GetSetting(key, fallback string) string {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if err != nil {
		return fallback
	}
	return v
}

// SetSetting writes one setting.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO settings(key, value) VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// SetSettings writes several settings in one transaction.
func (s *Store) SetSettings(values map[string]string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for k, v := range values {
		if _, err := tx.Exec(`INSERT INTO settings(key, value) VALUES(?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value`, k, v); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SettingInt reads an integer setting.
func (s *Store) SettingInt(key string, fallback int) int {
	if n, err := strconv.Atoi(s.GetSetting(key, "")); err == nil {
		return n
	}
	return fallback
}

// SettingInt64 reads a 64-bit integer setting.
func (s *Store) SettingInt64(key string, fallback int64) int64 {
	if n, err := strconv.ParseInt(s.GetSetting(key, ""), 10, 64); err == nil {
		return n
	}
	return fallback
}

// SettingBool reads a boolean setting.
func (s *Store) SettingBool(key string, fallback bool) bool {
	v := s.GetSetting(key, "")
	if v == "" {
		return fallback
	}
	return v == "1" || strings.EqualFold(v, "true")
}

// ---------------------------------------------------------------------------
// Stats
// ---------------------------------------------------------------------------

// SavedBytes returns the lifetime streamed byte counter.
func (s *Store) SavedBytes() (int64, error) {
	var n int64
	err := s.db.QueryRow(`SELECT value FROM system_stats WHERE key = 'saved_bytes_total'`).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return n, err
}

// AddSavedBytes increments the counter.
func (s *Store) AddSavedBytes(n int64) error {
	if n <= 0 {
		return nil
	}
	_, err := s.db.Exec(`UPDATE system_stats SET value = value + ? WHERE key = 'saved_bytes_total'`, n)
	return err
}

// ---------------------------------------------------------------------------
// Remotes
// ---------------------------------------------------------------------------

// Remote is a cloud connection. Secrets never live in this struct.
type Remote struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Provider    string     `json:"provider"`
	Brand       string     `json:"brand"`
	Endpoint    string     `json:"endpoint,omitempty"`
	RcloneName  string     `json:"rclone_name"`
	Status      string     `json:"status"`
	LastError   string     `json:"last_error,omitempty"`
	LastRTTMs   int64      `json:"rtt_ms"`
	LastCheckAt *time.Time `json:"last_check_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`

	// Quota is filled by the API layer from the RC daemon, not persisted.
	Quota *Quota `json:"quota,omitempty"`
}

// Quota is a cloud quota snapshot.
type Quota struct {
	Total   int64 `json:"total"`
	Used    int64 `json:"used"`
	Free    int64 `json:"free"`
	Trashed int64 `json:"trashed,omitempty"`
}

const remoteColumns = `id, name, provider, COALESCE(brand,''), COALESCE(endpoint,''), rclone_name,
	status, COALESCE(last_error,''), COALESCE(last_rtt_ms,0), last_check_at, created_at`

func scanRemote(sc interface{ Scan(...any) error }) (Remote, error) {
	var r Remote
	var lastCheck sql.NullString
	var created sql.NullString
	err := sc.Scan(&r.ID, &r.Name, &r.Provider, &r.Brand, &r.Endpoint, &r.RcloneName,
		&r.Status, &r.LastError, &r.LastRTTMs, &lastCheck, &created)
	if err != nil {
		return r, err
	}
	r.LastCheckAt = parseTime(lastCheck)
	if t := parseTime(created); t != nil {
		r.CreatedAt = *t
	}
	return r, nil
}

// ListRemotes returns all connections ordered by creation.
func (s *Store) ListRemotes() ([]Remote, error) {
	rows, err := s.db.Query(`SELECT ` + remoteColumns + ` FROM remotes ORDER BY created_at, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Remote
	for rows.Next() {
		r, err := scanRemote(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetRemote returns one connection.
func (s *Store) GetRemote(id string) (Remote, error) {
	row := s.db.QueryRow(`SELECT `+remoteColumns+` FROM remotes WHERE id = ?`, id)
	r, err := scanRemote(row)
	if errors.Is(err, sql.ErrNoRows) {
		return r, apierr.New(apierr.CodeRemoteNotFound, map[string]any{"remote_id": id})
	}
	return r, err
}

// CreateRemote stores a connection with encrypted options.
func (s *Store) CreateRemote(r Remote, options map[string]string) error {
	blob, err := s.encryptJSON(options)
	if err != nil {
		return err
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	if r.Status == "" {
		r.Status = "unknown"
	}
	_, err = s.db.Exec(`INSERT INTO remotes
		(id, name, provider, brand, endpoint, rclone_name, encrypted_credentials, status, last_error, last_rtt_ms, last_check_at, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.Name, r.Provider, r.Brand, r.Endpoint, r.RcloneName, blob, r.Status,
		r.LastError, r.LastRTTMs, timeText(r.LastCheckAt), r.CreatedAt.UTC().Format(timeLayout))
	return err
}

// UpdateRemote updates mutable metadata fields.
func (s *Store) UpdateRemote(r Remote) error {
	_, err := s.db.Exec(`UPDATE remotes SET name=?, provider=?, brand=?, endpoint=?, rclone_name=?, status=?, last_error=?, last_rtt_ms=?, last_check_at=?
		WHERE id=?`,
		r.Name, r.Provider, r.Brand, r.Endpoint, r.RcloneName, r.Status, r.LastError,
		r.LastRTTMs, timeText(r.LastCheckAt), r.ID)
	return err
}

// UpdateRemoteOptions re-encrypts the credential blob.
func (s *Store) UpdateRemoteOptions(id string, options map[string]string) error {
	blob, err := s.encryptJSON(options)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE remotes SET encrypted_credentials = ? WHERE id = ?`, blob, id)
	return err
}

// SetRemoteStatus records the result of a connection check.
func (s *Store) SetRemoteStatus(id, status, lastError string, rttMs int64) error {
	_, err := s.db.Exec(`UPDATE remotes SET status=?, last_error=?, last_rtt_ms=?, last_check_at=? WHERE id=?`,
		status, lastError, rttMs, nowText(), id)
	return err
}

// DeleteRemote removes a connection. History survives because transfers
// reference remotes with ON DELETE SET NULL and keep remote_label.
func (s *Store) DeleteRemote(id string) error {
	res, err := s.db.Exec(`DELETE FROM remotes WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return apierr.New(apierr.CodeRemoteNotFound, map[string]any{"remote_id": id})
	}
	return nil
}

// RemoteOptions decrypts the credential map.
func (s *Store) RemoteOptions(id string) (map[string]string, error) {
	var blob []byte
	err := s.db.QueryRow(`SELECT encrypted_credentials FROM remotes WHERE id = ?`, id).Scan(&blob)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apierr.New(apierr.CodeRemoteNotFound, map[string]any{"remote_id": id})
	}
	if err != nil {
		return nil, err
	}
	return s.decryptJSON(blob)
}

// ---------------------------------------------------------------------------
// Auth profiles
// ---------------------------------------------------------------------------

// AuthProfile is a source-authorization profile. Secrets are not serialized.
type AuthProfile struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Kind        string    `json:"kind"`
	Mode        string    `json:"mode"`
	ProfileDir  string    `json:"profile_dir,omitempty"`
	CookieFile  string    `json:"cookie_file,omitempty"`
	Supported   bool      `json:"supported"`
	Note        string    `json:"note,omitempty"`
	HasSecret   bool      `json:"has_manual_cookie"`
	CookieMask  string    `json:"cookie_mask,omitempty"`
	UserAgent   string    `json:"user_agent"`
	CreatedAt   time.Time `json:"created_at"`
	CustomNames []string  `json:"custom_headers,omitempty"`
}

// ProfileHeaders is the decrypted secret payload of a profile.
type ProfileHeaders struct {
	Cookie     string            `json:"cookie"`
	UserAgent  string            `json:"user_agent"`
	Referer    string            `json:"referer"`
	Origin     string            `json:"origin"`
	Extra      map[string]string `json:"extra,omitempty"`
	CookieFile string            `json:"cookie_file,omitempty"`
}

// ListAuthProfiles returns profiles without secrets.
func (s *Store) ListAuthProfiles() ([]AuthProfile, error) {
	rows, err := s.db.Query(`SELECT id, name, kind, mode, COALESCE(profile_dir,''), COALESCE(cookie_file_path,''),
		supported, COALESCE(note,''), encrypted_headers, created_at FROM auth_profiles ORDER BY created_at, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuthProfile
	for rows.Next() {
		p, blob, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		if len(blob) > 0 {
			if h, err := s.decryptHeaders(blob); err == nil {
				p.HasSecret = h.Cookie != ""
				p.CookieMask = maskCookie(h.Cookie)
				p.UserAgent = h.UserAgent
			}
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func scanProfile(sc interface{ Scan(...any) error }) (AuthProfile, []byte, error) {
	var p AuthProfile
	var blob []byte
	var created sql.NullString
	var supported int
	err := sc.Scan(&p.ID, &p.Name, &p.Kind, &p.Mode, &p.ProfileDir, &p.CookieFile,
		&supported, &p.Note, &blob, &created)
	p.Supported = supported != 0
	if t := parseTime(created); t != nil {
		p.CreatedAt = *t
	}
	return p, blob, err
}

// GetAuthProfile returns a profile plus its decrypted headers.
func (s *Store) GetAuthProfile(id string) (AuthProfile, ProfileHeaders, error) {
	row := s.db.QueryRow(`SELECT id, name, kind, mode, COALESCE(profile_dir,''), COALESCE(cookie_file_path,''),
		supported, COALESCE(note,''), encrypted_headers, created_at FROM auth_profiles WHERE id = ?`, id)
	p, blob, err := scanProfile(row)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ProfileHeaders{}, apierr.New(apierr.CodeNotFound, map[string]any{"entity": "auth_profile"})
	}
	if err != nil {
		return p, ProfileHeaders{}, err
	}
	h, err := s.decryptHeaders(blob)
	if err != nil {
		return p, ProfileHeaders{}, err
	}
	p.HasSecret = h.Cookie != ""
	p.CookieMask = maskCookie(h.Cookie)
	p.UserAgent = h.UserAgent
	return p, h, nil
}

// CreateAuthProfile stores a profile, encrypting its headers.
func (s *Store) CreateAuthProfile(p AuthProfile, h ProfileHeaders) error {
	blob, err := s.encryptJSON(h)
	if err != nil {
		return err
	}
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now().UTC()
	}
	_, err = s.db.Exec(`INSERT INTO auth_profiles
		(id, name, kind, mode, profile_dir, cookie_file_path, encrypted_headers, supported, note, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.Name, p.Kind, p.Mode, p.ProfileDir, p.CookieFile, blob, boolInt(p.Supported), p.Note,
		p.CreatedAt.UTC().Format(timeLayout))
	return err
}

// UpdateAuthProfile updates a profile and replaces its headers when h is not nil.
func (s *Store) UpdateAuthProfile(p AuthProfile, h *ProfileHeaders) error {
	if h != nil {
		blob, err := s.encryptJSON(*h)
		if err != nil {
			return err
		}
		_, err = s.db.Exec(`UPDATE auth_profiles SET name=?, kind=?, mode=?, profile_dir=?, cookie_file_path=?,
			encrypted_headers=?, supported=?, note=? WHERE id=?`,
			p.Name, p.Kind, p.Mode, p.ProfileDir, p.CookieFile, blob, boolInt(p.Supported), p.Note, p.ID)
		return err
	}
	_, err := s.db.Exec(`UPDATE auth_profiles SET name=?, kind=?, mode=?, profile_dir=?, cookie_file_path=?, supported=?, note=? WHERE id=?`,
		p.Name, p.Kind, p.Mode, p.ProfileDir, p.CookieFile, boolInt(p.Supported), p.Note, p.ID)
	return err
}

// DeleteAuthProfile removes a profile.
func (s *Store) DeleteAuthProfile(id string) error {
	_, err := s.db.Exec(`DELETE FROM auth_profiles WHERE id = ?`, id)
	return err
}

func (s *Store) decryptHeaders(blob []byte) (ProfileHeaders, error) {
	var h ProfileHeaders
	if len(blob) == 0 {
		return h, nil
	}
	plain, err := s.secret.Decrypt(blob)
	if err != nil {
		return h, err
	}
	if len(plain) == 0 {
		return h, nil
	}
	err = json.Unmarshal(plain, &h)
	return h, err
}

func maskCookie(cookie string) string {
	if cookie == "" {
		return ""
	}
	parts := strings.Split(cookie, ";")
	var names []string
	for _, p := range parts {
		name := strings.TrimSpace(strings.SplitN(p, "=", 2)[0])
		if name != "" {
			names = append(names, name+"=…")
		}
		if len(names) == 3 {
			break
		}
	}
	return strings.Join(names, "; ")
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---------------------------------------------------------------------------
// Transfers
// ---------------------------------------------------------------------------

// Transfer is one task / history record.
type Transfer struct {
	ID               string     `json:"id"`
	SourceURL        string     `json:"source_url"`
	ResolvedName     string     `json:"resolved_filename"`
	FilenameOverride string     `json:"filename_override,omitempty"`
	FileSize         int64      `json:"file_size"`
	RemoteID         *string    `json:"remote_id"`
	RemoteLabel      string     `json:"remote_label"`
	RemotePath       string     `json:"remote_path"`
	Engine           string     `json:"engine"`
	AuthProfileID    *string    `json:"auth_profile_id"`
	Mode             string     `json:"mode"`
	BufferSize       int64      `json:"buffer_size_bytes"`
	Status           string     `json:"status"`
	Priority         int        `json:"priority"`
	BytesTransferred int64      `json:"bytes_transferred"`
	BytesConfirmed   int64      `json:"bytes_confirmed"`
	SHA256           string     `json:"sha256,omitempty"`
	RetryAttempts    int        `json:"retry_attempts"`
	BandwidthDown    int64      `json:"bandwidth_down_bps"`
	BandwidthUp      int64      `json:"bandwidth_up_bps"`
	AverageSpeed     int64      `json:"average_speed_bps"`
	ErrorKind        string     `json:"error_kind,omitempty"`
	ErrorMessage     string     `json:"error_message,omitempty"`
	StartedAt        *time.Time `json:"started_at,omitempty"`
	CompletedAt      *time.Time `json:"completed_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

// FileName returns the effective target file name.
func (t Transfer) FileName() string {
	if t.FilenameOverride != "" {
		return t.FilenameOverride
	}
	return t.ResolvedName
}

// transferColumns is used in SELECT statements: COALESCE expressions keep the
// API contract stable for rows written before a column gained a default.
const transferColumns = `id, source_url, resolved_filename, COALESCE(filename_override,''), COALESCE(file_size,0),
	remote_id, COALESCE(remote_label,''), remote_path, engine, auth_profile_id, mode, COALESCE(buffer_size_bytes,0),
	status, COALESCE(priority,0), COALESCE(bytes_transferred,0), COALESCE(bytes_confirmed,0), COALESCE(sha256,''),
	COALESCE(retry_attempts,0), COALESCE(bandwidth_down_bps,0), COALESCE(bandwidth_up_bps,0), COALESCE(average_speed_bps,0),
	COALESCE(error_kind,''), COALESCE(error_message,''), started_at, completed_at, created_at`

// transferInsertColumns is the plain column list for INSERT — SQL expressions
// are not allowed as INSERT column names.
const transferInsertColumns = `id, source_url, resolved_filename, filename_override, file_size,
	remote_id, remote_label, remote_path, engine, auth_profile_id, mode, buffer_size_bytes,
	status, priority, bytes_transferred, bytes_confirmed, sha256,
	retry_attempts, bandwidth_down_bps, bandwidth_up_bps, average_speed_bps,
	error_kind, error_message, started_at, completed_at, created_at`

func scanTransfer(sc interface{ Scan(...any) error }) (Transfer, error) {
	var t Transfer
	var remoteID, authID sql.NullString
	var started, completed, created sql.NullString
	err := sc.Scan(&t.ID, &t.SourceURL, &t.ResolvedName, &t.FilenameOverride, &t.FileSize,
		&remoteID, &t.RemoteLabel, &t.RemotePath, &t.Engine, &authID, &t.Mode, &t.BufferSize,
		&t.Status, &t.Priority, &t.BytesTransferred, &t.BytesConfirmed, &t.SHA256,
		&t.RetryAttempts, &t.BandwidthDown, &t.BandwidthUp, &t.AverageSpeed,
		&t.ErrorKind, &t.ErrorMessage, &started, &completed, &created)
	if remoteID.Valid {
		v := remoteID.String
		t.RemoteID = &v
	}
	if authID.Valid {
		v := authID.String
		t.AuthProfileID = &v
	}
	t.StartedAt = parseTime(started)
	t.CompletedAt = parseTime(completed)
	if v := parseTime(created); v != nil {
		t.CreatedAt = *v
	}
	return t, err
}

// CreateTransfer inserts a queued task.
func (s *Store) CreateTransfer(t Transfer) error {
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.Exec(`INSERT INTO transfers (`+transferInsertColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.SourceURL, t.ResolvedName, nullString(t.FilenameOverride), t.FileSize,
		t.RemoteID, t.RemoteLabel, t.RemotePath, t.Engine, t.AuthProfileID, t.Mode, t.BufferSize,
		t.Status, t.Priority, t.BytesTransferred, t.BytesConfirmed, nullString(t.SHA256),
		t.RetryAttempts, t.BandwidthDown, t.BandwidthUp, t.AverageSpeed,
		nullString(t.ErrorKind), nullString(t.ErrorMessage), timeText(t.StartedAt), timeText(t.CompletedAt),
		t.CreatedAt.UTC().Format(timeLayout))
	return err
}

// UpdateTransfer persists the mutable runtime fields of a task.
func (s *Store) UpdateTransfer(t Transfer) error {
	_, err := s.db.Exec(`UPDATE transfers SET status=?, bytes_transferred=?, bytes_confirmed=?, sha256=?,
		retry_attempts=?, average_speed_bps=?, error_kind=?, error_message=?, file_size=?, started_at=?, completed_at=?,
		remote_id=?, remote_label=?, remote_path=?, filename_override=?, mode=?, buffer_size_bytes=? WHERE id=?`,
		t.Status, t.BytesTransferred, t.BytesConfirmed, nullString(t.SHA256), t.RetryAttempts,
		t.AverageSpeed, nullString(t.ErrorKind), nullString(t.ErrorMessage), t.FileSize,
		timeText(t.StartedAt), timeText(t.CompletedAt), t.RemoteID, t.RemoteLabel, t.RemotePath,
		nullString(t.FilenameOverride), t.Mode, t.BufferSize, t.ID)
	return err
}

// GetTransfer returns one task.
func (s *Store) GetTransfer(id string) (Transfer, error) {
	row := s.db.QueryRow(`SELECT `+transferColumns+` FROM transfers WHERE id = ?`, id)
	t, err := scanTransfer(row)
	if errors.Is(err, sql.ErrNoRows) {
		return t, apierr.New(apierr.CodeNotFound, map[string]any{"entity": "transfer"})
	}
	return t, err
}

// ListTransfers returns a page of history, newest first.
func (s *Store) ListTransfers(status string, page, limit int) ([]Transfer, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if page <= 0 {
		page = 1
	}
	where, args := "", []any{}
	if status != "" {
		where = ` WHERE status = ?`
		args = append(args, status)
	}
	var total int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM transfers`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	argsPage := append(append([]any{}, args...), limit, (page-1)*limit)
	rows, err := s.db.Query(`SELECT `+transferColumns+` FROM transfers`+where+
		` ORDER BY created_at DESC, rowid DESC LIMIT ? OFFSET ?`, argsPage...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Transfer
	for rows.Next() {
		t, err := scanTransfer(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, t)
	}
	return out, total, rows.Err()
}

// ActiveTransfers returns tasks in a live state.
func (s *Store) ActiveTransfers() ([]Transfer, error) {
	rows, err := s.db.Query(`SELECT ` + transferColumns + ` FROM transfers
		WHERE status IN ('queued','running','paused','retrying') ORDER BY priority, created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Transfer
	for rows.Next() {
		t, err := scanTransfer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteTransfer removes a history row (the cloud object is untouched).
func (s *Store) DeleteTransfer(id string) error {
	res, err := s.db.Exec(`DELETE FROM transfers WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return apierr.New(apierr.CodeNotFound, map[string]any{"entity": "transfer"})
	}
	return nil
}

// AddLog appends a diagnostic line.
func (s *Store) AddLog(transferID, level, source, message string) error {
	_, err := s.db.Exec(`INSERT INTO transfer_logs(transfer_id, ts, level, source, message) VALUES (?,?,?,?,?)`,
		transferID, nowText(), level, source, message)
	return err
}

// TransferLog is one diagnostic line.
type TransferLog struct {
	ID         int64     `json:"id,omitempty"`
	TransferID string    `json:"transfer_id"`
	TS         time.Time `json:"ts"`
	Level      string    `json:"level"`
	Source     string    `json:"source"`
	Message    string    `json:"message"`
}

// ListLogs returns the tail of a task journal.
func (s *Store) ListLogs(transferID string, tail int, level string) ([]TransferLog, error) {
	if tail <= 0 || tail > 1000 {
		tail = 200
	}
	query := `SELECT id, transfer_id, ts, level, source, message FROM transfer_logs WHERE transfer_id = ?`
	args := []any{transferID}
	if level != "" {
		query += ` AND level = ?`
		args = append(args, level)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, tail)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TransferLog
	for rows.Next() {
		var l TransferLog
		var ts sql.NullString
		if err := rows.Scan(&l.ID, &l.TransferID, &ts, &l.Level, &l.Source, &l.Message); err != nil {
			return nil, err
		}
		if t := parseTime(ts); t != nil {
			l.TS = *t
		}
		out = append(out, l)
	}
	// Return chronological order for the UI.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Dependencies registry
// ---------------------------------------------------------------------------

// Dependency is the installed-state row for an external component.
type Dependency struct {
	ID               string    `json:"id"`
	CatalogVersion   string    `json:"catalog_version"`
	InstalledVersion string    `json:"installed_version,omitempty"`
	InstallPath      string    `json:"install_path,omitempty"`
	SHA256           string    `json:"sha256,omitempty"`
	Status           string    `json:"status"`
	Required         bool      `json:"required"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// ListDependencies returns the registry.
func (s *Store) ListDependencies() ([]Dependency, error) {
	rows, err := s.db.Query(`SELECT id, catalog_version, COALESCE(installed_version,''), COALESCE(install_path,''),
		COALESCE(sha256,''), status, required, updated_at FROM dependencies`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Dependency
	for rows.Next() {
		var d Dependency
		var required int
		var updated sql.NullString
		if err := rows.Scan(&d.ID, &d.CatalogVersion, &d.InstalledVersion, &d.InstallPath, &d.SHA256, &d.Status, &required, &updated); err != nil {
			return nil, err
		}
		d.Required = required != 0
		if t := parseTime(updated); t != nil {
			d.UpdatedAt = *t
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// UpsertDependency writes one registry row.
func (s *Store) UpsertDependency(d Dependency) error {
	_, err := s.db.Exec(`INSERT INTO dependencies(id, catalog_version, installed_version, install_path, sha256, status, required, updated_at)
		VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET catalog_version=excluded.catalog_version, installed_version=excluded.installed_version,
			install_path=excluded.install_path, sha256=excluded.sha256, status=excluded.status,
			required=excluded.required, updated_at=excluded.updated_at`,
		d.ID, d.CatalogVersion, d.InstalledVersion, d.InstallPath, d.SHA256, d.Status, boolInt(d.Required), nowText())
	return err
}

// DeleteDependency removes a registry row.
func (s *Store) DeleteDependency(id string) error {
	_, err := s.db.Exec(`DELETE FROM dependencies WHERE id = ?`, id)
	return err
}

// ---------------------------------------------------------------------------
// OAuth tokens
// ---------------------------------------------------------------------------

// OAuthToken is a persisted provider authorization.
type OAuthToken struct {
	Provider     string
	RemoteID     string
	ClientSecret string
	AccessToken  string
	RefreshToken string
	Scope        string
	ExpiresAt    *time.Time
}

// SaveOAuthToken encrypts and upserts tokens.
func (s *Store) SaveOAuthToken(t OAuthToken) error {
	clientSecret, err := s.encryptBytes([]byte(t.ClientSecret))
	if err != nil {
		return err
	}
	access, err := s.encryptBytes([]byte(t.AccessToken))
	if err != nil {
		return err
	}
	refresh, err := s.encryptBytes([]byte(t.RefreshToken))
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO oauth_tokens(provider, remote_id, encrypted_client_secret, encrypted_access_token,
		encrypted_refresh_token, scope, expires_at, updated_at) VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(provider) DO UPDATE SET remote_id=excluded.remote_id,
			encrypted_client_secret=excluded.encrypted_client_secret,
			encrypted_access_token=excluded.encrypted_access_token,
			encrypted_refresh_token=excluded.encrypted_refresh_token,
			scope=excluded.scope, expires_at=excluded.expires_at, updated_at=excluded.updated_at`,
		t.Provider, t.RemoteID, clientSecret, access, refresh, t.Scope, timeText(t.ExpiresAt), nowText())
	return err
}

// GetOAuthToken decrypts tokens for a provider.
func (s *Store) GetOAuthToken(provider string) (OAuthToken, error) {
	var t OAuthToken
	var cs, at, rt []byte
	var expires sql.NullString
	err := s.db.QueryRow(`SELECT provider, COALESCE(remote_id,''), encrypted_client_secret, encrypted_access_token,
		encrypted_refresh_token, COALESCE(scope,''), expires_at FROM oauth_tokens WHERE provider = ?`, provider).
		Scan(&t.Provider, &t.RemoteID, &cs, &at, &rt, &t.Scope, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return t, apierr.New(apierr.CodeAuthOAuthRequired, map[string]any{"provider": provider})
	}
	if err != nil {
		return t, err
	}
	if t.ClientSecret, err = s.decryptString(cs); err != nil {
		return t, err
	}
	if t.AccessToken, err = s.decryptString(at); err != nil {
		return t, err
	}
	if t.RefreshToken, err = s.decryptString(rt); err != nil {
		return t, err
	}
	t.ExpiresAt = parseTime(expires)
	return t, nil
}

// DeleteOAuthToken removes provider tokens.
func (s *Store) DeleteOAuthToken(provider string) error {
	_, err := s.db.Exec(`DELETE FROM oauth_tokens WHERE provider = ?`, provider)
	return err
}

// ---------------------------------------------------------------------------
// Encryption helpers
// ---------------------------------------------------------------------------

func (s *Store) encryptJSON(v any) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return s.encryptBytes(raw)
}

func (s *Store) decryptJSON(blob []byte) (map[string]string, error) {
	out := map[string]string{}
	if len(blob) == 0 {
		return out, nil
	}
	plain, err := s.secret.Decrypt(blob)
	if err != nil {
		return nil, err
	}
	if len(plain) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(plain, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) encryptBytes(b []byte) ([]byte, error) {
	if len(b) == 0 {
		return nil, nil
	}
	return s.secret.Encrypt(b)
}

func (s *Store) decryptString(b []byte) (string, error) {
	if len(b) == 0 {
		return "", nil
	}
	plain, err := s.secret.Decrypt(b)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func nullString(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func timeText(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(timeLayout)
}

// Ensure the package compiles against fmt for wrapped errors.
var _ = fmt.Sprintf

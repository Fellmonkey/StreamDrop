package rclone

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"streamdrop/internal/apierr"
)

// RemoteConfig is what the provisioner materialises in the daemon.
type RemoteConfig struct {
	ID         string
	Name       string
	Provider   string
	RcloneName string
	Options    map[string]string
}

// Provisioner writes cloud credentials into the running daemon. Secrets travel
// over the loopback RC channel and live only in daemon memory: no rclone.conf
// is written and RCLONE_CONFIG_* variables are not used.
type Provisioner struct {
	RC *Client
}

// Provision creates or updates the remote.
func (p *Provisioner) Provision(ctx context.Context, cfg RemoteConfig) error {
	providerType, params, err := BuildParameters(cfg.Provider, cfg.Options)
	if err != nil {
		return err
	}
	if err := p.RC.ConfigCreate(ctx, cfg.RcloneName, providerType, params); err != nil {
		// A remote that already exists is updated instead of failing the task.
		if updateErr := p.RC.ConfigUpdate(ctx, cfg.RcloneName, params); updateErr == nil {
			return nil
		}
		return apierr.Wrap(apierr.CodeCloudUnreachable, err, map[string]any{"remote": cfg.Name})
	}
	return nil
}

// Remove drops the remote from the daemon.
func (p *Provisioner) Remove(ctx context.Context, rcloneName string) error {
	return p.RC.ConfigDelete(ctx, rcloneName)
}

// Quota reads the storage quota.
func (p *Provisioner) Quota(ctx context.Context, fs string) (Quota, error) {
	return p.RC.About(ctx, fs)
}

// MeasureLatency takes three samples and returns the median RTT.
func (p *Provisioner) MeasureLatency(ctx context.Context, fs string) (time.Duration, error) {
	samples := make([]time.Duration, 0, 3)
	for i := 0; i < 3; i++ {
		start := time.Now()
		if _, err := p.RC.About(ctx, fs); err != nil {
			return 0, err
		}
		samples = append(samples, time.Since(start))
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	return samples[1], nil
}

// BuildParameters maps application credentials to an rclone backend type and
// its parameters.
func BuildParameters(provider string, opts map[string]string) (string, map[string]string, error) {
	params := map[string]string{}
	get := func(key, fallback string) string {
		if v := opts[key]; v != "" {
			return v
		}
		return fallback
	}

	switch provider {
	case "mailru", "webdav":
		typ := "webdav"
		params["url"] = get("url", "https://webdav.cloud.mail.ru")
		params["vendor"] = get("vendor", "other")
		params["user"] = get("user", "")
		params["pass"] = get("pass", "")
		return typ, params, nil

	case "yandex":
		params["token"] = get("token", "")
		if params["token"] == "" {
			return "", nil, apierr.New(apierr.CodeAuthOAuthRequired, map[string]any{"provider": "yandex"})
		}
		return "yandex", params, nil

	case "drive":
		params["token"] = get("token", "")
		if params["token"] == "" {
			return "", nil, apierr.New(apierr.CodeAuthOAuthRequired, map[string]any{"provider": "drive"})
		}
		if v := get("client_id", ""); v != "" {
			params["client_id"] = v
		}
		if v := get("client_secret", ""); v != "" {
			params["client_secret"] = v
		}
		params["scope"] = get("scope", "drive")
		return "drive", params, nil

	case "onedrive":
		params["token"] = get("token", "")
		if params["token"] == "" {
			return "", nil, apierr.New(apierr.CodeAuthOAuthRequired, map[string]any{"provider": "onedrive"})
		}
		if v := get("client_id", ""); v != "" {
			params["client_id"] = v
		}
		if v := get("client_secret", ""); v != "" {
			params["client_secret"] = v
		}
		params["drive_type"] = get("drive_type", "personal")
		return "onedrive", params, nil

	case "s3":
		params["provider"] = get("s3_provider", "Other")
		params["access_key_id"] = get("access_key_id", "")
		params["secret_access_key"] = get("secret_access_key", "")
		if v := get("endpoint", ""); v != "" {
			params["endpoint"] = v
		}
		if v := get("region", ""); v != "" {
			params["region"] = v
		}
		return "s3", params, nil

	case "ftp":
		params["host"] = get("host", "")
		params["user"] = get("user", "")
		params["pass"] = get("pass", "")
		if v := get("port", ""); v != "" {
			params["port"] = v
		}
		return "ftp", params, nil

	case "sftp":
		params["host"] = get("host", "")
		params["user"] = get("user", "")
		params["pass"] = get("pass", "")
		if v := get("port", ""); v != "" {
			params["port"] = v
		}
		return "sftp", params, nil
	}
	return "", nil, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "provider"})
}

// TokenJSON builds the token blob rclone expects for OAuth backends.
func TokenJSON(access, refresh, tokenType string, expiry time.Time) string {
	if tokenType == "" {
		tokenType = "Bearer"
	}
	payload := map[string]any{
		"access_token": access,
		"token_type":   tokenType,
	}
	if refresh != "" {
		payload["refresh_token"] = refresh
	}
	if !expiry.IsZero() {
		payload["expiry"] = expiry.UTC().Format(time.RFC3339)
	}
	raw, _ := json.Marshal(payload)
	return string(raw)
}

// remoteFS builds an rclone filesystem string: "name:path" with a normalized
// path.
func remoteFS(rcloneName, path string) string {
	path = strings.Trim(path, "/")
	if path == "" {
		return rcloneName + ":"
	}
	return rcloneName + ":" + path
}

// JoinPath joins remote path segments without producing double slashes.
func JoinPath(base, name string) string {
	base = strings.Trim(base, "/")
	name = strings.Trim(name, "/")
	switch {
	case base == "":
		return name
	case name == "":
		return base
	default:
		return base + "/" + name
	}
}

// EscapeName percent-escapes a file name for safe use inside an rclone fs
// string while keeping it readable.
func EscapeName(name string) string {
	return url.PathEscape(name)
}

var _ = fmt.Sprintf

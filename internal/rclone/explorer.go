package rclone

import (
	"context"
	"sort"
	"strings"

	"streamdrop/internal/apierr"
)

// Explorer is the cloud folder browser.
type Explorer struct {
	RC *Client
}

// List returns the directories of one level. q filters by name substring
// (case-insensitive). rclone has no server-side name search, so the daemon
// still lists only directories of this level with dirsOnly + max-depth=1; the
// filter merely narrows what the browser receives.
func (e *Explorer) List(ctx context.Context, rcloneName, path, q string) ([]DirEntry, error) {
	dirs, err := e.RC.ListDirs(ctx, remoteFS(rcloneName, path))
	if err != nil {
		return nil, apierr.Wrap(apierr.CodeCloudUnreachable, err, map[string]any{"remote": rcloneName})
	}
	needle := strings.ToLower(strings.TrimSpace(q))
	out := make([]DirEntry, 0, len(dirs))
	for _, d := range dirs {
		if needle != "" && !strings.Contains(strings.ToLower(d.Name), needle) {
			continue
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

// Mkdir creates a folder inside path.
func (e *Explorer) Mkdir(ctx context.Context, rcloneName, path, name string) error {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, `/\`) {
		return apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "name"})
	}
	target := remoteFS(rcloneName, JoinPath(path, name))
	if err := e.RC.Mkdir(ctx, target); err != nil {
		return apierr.Wrap(apierr.CodeCloudWriteFailed, err, map[string]any{"remote": rcloneName})
	}
	return nil
}

// Quota reads the storage quota.
func (e *Explorer) Quota(ctx context.Context, rcloneName string) (Quota, error) {
	q, err := e.RC.About(ctx, remoteFS(rcloneName, ""))
	if err != nil {
		return Quota{}, apierr.Wrap(apierr.CodeCloudUnreachable, err, map[string]any{"remote": rcloneName})
	}
	return q, nil
}

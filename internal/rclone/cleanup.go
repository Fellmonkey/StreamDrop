package rclone

import (
	"context"

	"streamdrop/internal/apierr"
)

// Cleaner removes partially written objects so a failed transfer does not
// consume the user's quota (fail-safe cleanup).
type Cleaner struct {
	RC *Client
}

// DeleteObject removes remotePath/name.
func (c *Cleaner) DeleteObject(ctx context.Context, rcloneName, remotePath, filename string) error {
	if filename == "" {
		return nil
	}
	fs := remoteFS(rcloneName, JoinPath(remotePath, filename))
	if err := c.RC.DeleteFile(ctx, fs); err != nil {
		return apierr.Wrap(apierr.CodeCloudCleanupFailed, err, map[string]any{"remote": rcloneName})
	}
	return nil
}

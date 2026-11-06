// Command streamdrop-mcp exposes StreamDrop to MCP clients over stdio.
//
// The server is a thin, typed wrapper over the daemon's REST API: every tool
// maps 1:1 to an existing endpoint and is discovered fresh on each call, so a
// daemon restart (new port/token) never breaks a long-lived session.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"streamdrop/internal/client"
	"streamdrop/internal/config"
	"streamdrop/internal/transfer"
)

type api struct {
	dataDir string
}

// call dials the daemon for every invocation and maps the outcome to an MCP
// tool result. Errors never terminate the session; they become isError
// results with a machine-readable code.
func (a *api) call(ctx context.Context, fn func(*client.Client) (any, error)) (*mcp.CallToolResult, error) {
	c, err := client.Dial(a.dataDir)
	if err != nil {
		return failResult("daemon_unreachable", nil), nil
	}
	out, err := fn(c)
	if err != nil {
		var ae *client.APIError
		if errors.As(err, &ae) {
			return failResult(string(ae.Code), map[string]any{
				"kind":        ae.Kind,
				"http_status": ae.StatusCode,
				"params":      ae.Params,
			}), nil
		}
		return failResult("daemon_unreachable", nil), nil
	}
	return okResult(out), nil
}

func okResult(v any) *mcp.CallToolResult {
	raw, err := json.Marshal(v)
	if err != nil {
		return failResult("internal_error", nil)
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(raw)}},
	}
}

func failResult(code string, params map[string]any) *mcp.CallToolResult {
	payload := map[string]any{"code": code}
	if params != nil {
		for k, v := range params {
			payload[k] = v
		}
	}
	raw, _ := json.Marshal(payload)
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{mcp.TextContent{Type: "text", Text: string(raw)}},
	}
}

// readOnly marks a tool as read-only and explicitly non-destructive (the SDK
// defaults destructiveHint to true for every tool, which would mislead
// clients into confirming read-only calls).
func readOnly() mcp.ToolOption {
	return func(t *mcp.Tool) {
		t.Annotations.ReadOnlyHint = mcp.ToBoolPtr(true)
		t.Annotations.DestructiveHint = mcp.ToBoolPtr(false)
	}
}

// nonDestructive resets the SDK default destructiveHint for mutating tools.
func nonDestructive() mcp.ToolOption {
	return func(t *mcp.Tool) {
		t.Annotations.DestructiveHint = mcp.ToBoolPtr(false)
	}
}

// tool builders ------------------------------------------------------------

func reqStr(name, desc string) mcp.ToolOption {
	return mcp.WithString(name, mcp.Description(desc), mcp.Required())
}

func optStr(name, desc string) mcp.ToolOption {
	return mcp.WithString(name, mcp.Description(desc))
}

func optInt(name, desc string) mcp.ToolOption {
	return mcp.WithInteger(name, mcp.Description(desc))
}

func buildTools(a *api) []server.ServerTool {
	tools := []server.ServerTool{
		{
			Tool: mcp.NewTool("get_status",
				mcp.WithDescription("StreamDrop daemon health: version, engine readiness, active transfers, components."),
				readOnly(),
			),
			Handler: func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return a.call(ctx, func(c *client.Client) (any, error) { return c.Status(ctx) })
			},
		},
		{
			Tool: mcp.NewTool("list_remotes",
				mcp.WithDescription("List connected clouds with status and quota."),
				readOnly(),
			),
			Handler: func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return a.call(ctx, func(c *client.Client) (any, error) { return c.Remotes(ctx) })
			},
		},
		{
			Tool: mcp.NewTool("list_transfers",
				mcp.WithDescription("List transfers, optionally filtered by status (queued|running|paused|retrying|completed|failed|aborted)."),
				readOnly(),
				optStr("status", "status filter"),
				optInt("page", "page number (1-based)"),
				optInt("limit", "page size"),
			),
			Handler: func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				status := r.GetString("status", "")
				page := r.GetInt("page", 0)
				limit := r.GetInt("limit", 0)
				return a.call(ctx, func(c *client.Client) (any, error) {
					return c.ListTransfers(ctx, status, page, limit)
				})
			},
		},
		{
			Tool: mcp.NewTool("get_transfer",
				mcp.WithDescription("Transfer details and its recent logs."),
				readOnly(),
				reqStr("id", "transfer id"),
			),
			Handler: func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				id, err := r.RequireString("id")
				if err != nil {
					return failResult("validation_failed", nil), nil
				}
				return a.call(ctx, func(c *client.Client) (any, error) { return c.Transfer(ctx, id) })
			},
		},
		{
			Tool: mcp.NewTool("create_transfers",
				mcp.WithDescription("Create one or several transfers. The batch is atomic: if any URL fails, nothing is queued. The remote_id is required; source_url or urls must be present."),
				nonDestructive(),
				reqStr("remote_id", "target cloud id (see list_remotes)"),
				optStr("source_url", "single file URL"),
				optStr("urls", "JSON array of URLs"),
				optStr("remote_path", "destination folder in the cloud"),
				optStr("engine", "auto|curl|yt-dlp"),
				optStr("auth_profile_id", "authorization profile id"),
				optStr("mode", "pure_stream|ring_buffer"),
				optInt("buffer_size_bytes", "ring buffer size in bytes (32MiB..1GiB)"),
				optStr("filename_override", "target file name"),
				optInt("bandwidth_down_bps", "download limit, bps"),
				optInt("bandwidth_up_bps", "upload limit, bps"),
			),
			Handler: func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				remoteID, err := r.RequireString("remote_id")
				if err != nil {
					return failResult("validation_failed", nil), nil
				}
				req := transfer.CreateRequest{RemoteID: remoteID}
				req.SourceURL = r.GetString("source_url", "")
				for _, u := range r.GetStringSlice("urls", nil) {
					req.URLs = append(req.URLs, u)
				}
				req.RemotePath = r.GetString("remote_path", "")
				req.Engine = r.GetString("engine", "")
				req.AuthProfileID = r.GetString("auth_profile_id", "")
				req.Mode = r.GetString("mode", "")
				if v := r.GetInt("buffer_size_bytes", 0); v > 0 {
					req.BufferBytes = int64(v)
				}
				req.FilenameOverride = r.GetString("filename_override", "")
				if v := r.GetInt("bandwidth_down_bps", 0); v > 0 {
					b := int64(v)
					req.BandwidthDownBps = &b
				}
				if v := r.GetInt("bandwidth_up_bps", 0); v > 0 {
					b := int64(v)
					req.BandwidthUpBps = &b
				}
				return a.call(ctx, func(c *client.Client) (any, error) {
					return c.CreateTransfers(ctx, req)
				})
			},
		},
		{
			Tool: mcp.NewTool("pause_transfer",
				mcp.WithDescription("Pause a running transfer."),
				nonDestructive(),
				reqStr("id", "transfer id"),
			),
			Handler: func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				id, err := r.RequireString("id")
				if err != nil {
					return failResult("validation_failed", nil), nil
				}
				return a.call(ctx, func(c *client.Client) (any, error) { return c.PauseTransfer(ctx, id) })
			},
		},
		{
			Tool: mcp.NewTool("resume_transfer",
				mcp.WithDescription("Resume a paused transfer."),
				nonDestructive(),
				reqStr("id", "transfer id"),
			),
			Handler: func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				id, err := r.RequireString("id")
				if err != nil {
					return failResult("validation_failed", nil), nil
				}
				return a.call(ctx, func(c *client.Client) (any, error) { return c.ResumeTransfer(ctx, id) })
			},
		},
		{
			Tool: mcp.NewTool("abort_transfer",
				mcp.WithDescription("Abort a transfer."),
				mcp.WithDestructiveHintAnnotation(true),
				reqStr("id", "transfer id"),
			),
			Handler: func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				id, err := r.RequireString("id")
				if err != nil {
					return failResult("validation_failed", nil), nil
				}
				return a.call(ctx, func(c *client.Client) (any, error) { return c.AbortTransfer(ctx, id) })
			},
		},
		{
			Tool: mcp.NewTool("delete_transfer",
				mcp.WithDescription("Delete a finished transfer from history."),
				mcp.WithDestructiveHintAnnotation(true),
				reqStr("id", "transfer id"),
			),
			Handler: func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				id, err := r.RequireString("id")
				if err != nil {
					return failResult("validation_failed", nil), nil
				}
				return a.call(ctx, func(c *client.Client) (any, error) {
					return map[string]any{"deleted": true}, c.DeleteTransfer(ctx, id)
				})
			},
		},
		{
			Tool: mcp.NewTool("get_queue",
				mcp.WithDescription("Queue state: paused flag, parallelism limit, entries."),
				readOnly(),
			),
			Handler: func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return a.call(ctx, func(c *client.Client) (any, error) { return c.QueueState(ctx) })
			},
		},
		{
			Tool: mcp.NewTool("pause_queue",
				mcp.WithDescription("Pause the whole queue."),
				nonDestructive(),
			),
			Handler: func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return a.call(ctx, func(c *client.Client) (any, error) { return c.PauseQueue(ctx) })
			},
		},
		{
			Tool: mcp.NewTool("resume_queue",
				mcp.WithDescription("Resume the queue."),
				nonDestructive(),
			),
			Handler: func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return a.call(ctx, func(c *client.Client) (any, error) { return c.ResumeQueue(ctx) })
			},
		},
		{
			Tool: mcp.NewTool("list_dependencies",
				mcp.WithDescription("Installed components (rclone, yt-dlp, curl) and their status."),
				readOnly(),
			),
			Handler: func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return a.call(ctx, func(c *client.Client) (any, error) { return c.Deps(ctx) })
			},
		},
		{
			Tool: mcp.NewTool("install_dependency",
				mcp.WithDescription("Install a component in the background. Poll list_dependencies until its status becomes installed."),
				nonDestructive(),
				reqStr("id", "component id: rclone|yt-dlp|curl"),
			),
			Handler: func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				id, err := r.RequireString("id")
				if err != nil {
					return failResult("validation_failed", nil), nil
				}
				return a.call(ctx, func(c *client.Client) (any, error) {
					return map[string]any{"state": "downloading", "component": id}, c.InstallDep(ctx, id)
				})
			},
		},
		{
			Tool: mcp.NewTool("check_updates",
				mcp.WithDescription("Check for a newer StreamDrop release."),
				readOnly(),
			),
			Handler: func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return a.call(ctx, func(c *client.Client) (any, error) { return c.CheckUpdates(ctx) })
			},
		},
	}
	return tools
}

func main() {
	dataDir := flag.String("data-dir", "", "StreamDrop data directory (default: platform default)")
	flag.Parse()
	if *dataDir == "" {
		*dataDir = config.Defaults().DataDir
	}

	a := &api{dataDir: *dataDir}
	srv := server.NewMCPServer(
		"streamdrop-mcp",
		config.Version,
		server.WithInstructions("StreamDrop: stream files straight into the cloud. Start with get_status, "+
			"then list_remotes to find the target cloud id before creating transfers."),
		server.WithToolCapabilities(false),
	)
	for _, t := range buildTools(a) {
		srv.AddTool(t.Tool, t.Handler)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	stdio := server.NewStdioServer(srv)
	if err := stdio.Listen(ctx, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "streamdrop-mcp:", err)
		os.Exit(1)
	}
}

// Package mcpserver assembles the StreamDrop MCP server: tools, error
// mapping and the stdio listener. It is used by the main daemon binary via
// the "mcp" subcommand (streamdrop mcp) and by the standalone console binary
// cmd/streamdrop-mcp (development, testing).
package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"streamdrop/internal/client"
	"streamdrop/internal/config"
	"streamdrop/internal/transfer"
)

type api struct {
	dataDir string
}

// Run starts the stdio MCP server and blocks until ctx is cancelled or the
// client closes the transport.
func Run(ctx context.Context, dataDir string) error {
	a := &api{dataDir: dataDir}
	srv := server.NewMCPServer(
		"streamdrop-mcp",
		config.Version,
		server.WithInstructions("StreamDrop: stream files straight into the cloud.\n"+
			"Workflow: get_status to check health, list_remotes to find the target cloud id, "+
			"then create_transfers (it inspects every URL and is atomic). Track progress with get_queue and get_transfer.\n"+
			"Tool calls run concurrently: await each result before issuing another state-changing call "+
			"(for example pause_queue followed by resume_queue). If the daemon is not running, every tool "+
			"returns daemon_unreachable."),
		server.WithToolCapabilities(false),
	)
	for _, t := range buildTools(a) {
		srv.AddTool(t.Tool, t.Handler)
	}
	stdio := server.NewStdioServer(srv)
	return stdio.Listen(ctx, os.Stdin, os.Stdout)
}

// ResolveDataDir picks the data directory:
//  1. --data-dir flag
//  2. STREAMDROP_DATA_DIR environment variable
//  3. <dataDir>/bin layout when this binary was provisioned there by the
//     daemon (marker: <dataDir>/run/daemon.json exists)
//  4. the platform default (same resolution the daemon uses)
func ResolveDataDir(explicit string) string {
	if explicit != "" {
		return explicit
	}
	if v := os.Getenv("STREAMDROP_DATA_DIR"); v != "" {
		return v
	}
	if v := dataDirForExe(os.Args[0]); v != "" {
		return v
	}
	return config.Defaults().DataDir
}

// dataDirForExe returns the data directory when the executable lives in
// <dataDir>/bin/ and the daemon discovery marker is present.
func dataDirForExe(exe string) string {
	abs, err := filepath.Abs(exe)
	if err != nil {
		return ""
	}
	binDir := filepath.Dir(abs)
	if filepath.Base(binDir) != "bin" {
		return ""
	}
	dataDir := filepath.Dir(binDir)
	if _, err := os.Stat(filepath.Join(dataDir, "run", "daemon.json")); err == nil {
		return dataDir
	}
	return ""
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

// noArgTool builds a tool with an empty schema.
func noArgTool(a *api, name, desc string, annotations []mcp.ToolOption, fn func(context.Context, *client.Client) (any, error)) server.ServerTool {
	opts := append([]mcp.ToolOption{mcp.WithDescription(desc)}, annotations...)
	return server.ServerTool{
		Tool: mcp.NewTool(name, opts...),
		Handler: func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return a.call(ctx, func(c *client.Client) (any, error) { return fn(ctx, c) })
		},
	}
}

// idTool builds a tool with a single required "id" argument.
func idTool(a *api, name, desc string, annotations []mcp.ToolOption, fn func(context.Context, *client.Client, string) (any, error)) server.ServerTool {
	opts := append([]mcp.ToolOption{mcp.WithDescription(desc), reqStr("id", "transfer id")}, annotations...)
	return server.ServerTool{
		Tool: mcp.NewTool(name, opts...),
		Handler: func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			id, err := r.RequireString("id")
			if err != nil {
				return validationFailed(), nil
			}
			return a.call(ctx, func(c *client.Client) (any, error) { return fn(ctx, c, id) })
		},
	}
}

func validationFailed() *mcp.CallToolResult {
	return failResult("validation_failed", nil)
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
	ro := []mcp.ToolOption{readOnly()}
	rw := []mcp.ToolOption{nonDestructive()}
	destr := []mcp.ToolOption{mcp.WithDestructiveHintAnnotation(true)}

	tools := []server.ServerTool{
		noArgTool(a, "get_status",
			"StreamDrop daemon health: version, engine readiness, active transfers, components.",
			ro,
			func(ctx context.Context, c *client.Client) (any, error) { return c.Status(ctx) }),

		noArgTool(a, "list_remotes",
			"List connected clouds with status and quota.",
			ro,
			func(ctx context.Context, c *client.Client) (any, error) { return c.Remotes(ctx) }),

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

		idTool(a, "get_transfer",
			"Transfer details and its recent logs.",
			ro,
			func(ctx context.Context, c *client.Client, id string) (any, error) { return c.Transfer(ctx, id) }),

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
					return validationFailed(), nil
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

		idTool(a, "pause_transfer",
			"Pause a running transfer.",
			rw,
			func(ctx context.Context, c *client.Client, id string) (any, error) { return c.PauseTransfer(ctx, id) }),

		idTool(a, "resume_transfer",
			"Resume a paused transfer.",
			rw,
			func(ctx context.Context, c *client.Client, id string) (any, error) { return c.ResumeTransfer(ctx, id) }),

		idTool(a, "abort_transfer",
			"Abort a transfer.",
			destr,
			func(ctx context.Context, c *client.Client, id string) (any, error) { return c.AbortTransfer(ctx, id) }),

		idTool(a, "delete_transfer",
			"Delete a finished transfer from history.",
			destr,
			func(ctx context.Context, c *client.Client, id string) (any, error) {
				if err := c.DeleteTransfer(ctx, id); err != nil {
					return nil, err
				}
				return map[string]any{"deleted": true}, nil
			}),

		noArgTool(a, "get_queue",
			"Queue state: paused flag, parallelism limit, entries.",
			ro,
			func(ctx context.Context, c *client.Client) (any, error) { return c.QueueState(ctx) }),

		noArgTool(a, "pause_queue",
			"Pause the whole queue.",
			rw,
			func(ctx context.Context, c *client.Client) (any, error) { return c.PauseQueue(ctx) }),

		noArgTool(a, "resume_queue",
			"Resume the queue.",
			rw,
			func(ctx context.Context, c *client.Client) (any, error) { return c.ResumeQueue(ctx) }),

		noArgTool(a, "list_dependencies",
			"Installed components (rclone, yt-dlp, curl) and their status.",
			ro,
			func(ctx context.Context, c *client.Client) (any, error) { return c.Deps(ctx) }),

		idTool(a, "install_dependency",
			"Install a component in the background. Poll list_dependencies until its status becomes installed.",
			rw,
			func(ctx context.Context, c *client.Client, id string) (any, error) {
				if err := c.InstallDep(ctx, id); err != nil {
					return nil, err
				}
				return map[string]any{"state": "downloading", "component": id}, nil
			}),

		noArgTool(a, "check_updates",
			"Check for a newer StreamDrop release.",
			ro,
			func(ctx context.Context, c *client.Client) (any, error) { return c.CheckUpdates(ctx) }),
	}
	return tools
}

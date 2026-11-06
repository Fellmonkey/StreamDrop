package mcpserver

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestToolInventory(t *testing.T) {
	tools := buildTools(&api{dataDir: t.TempDir()})
	if len(tools) != 15 {
		t.Fatalf("expected 15 tools, got %d", len(tools))
	}

	byName := map[string]mcp.Tool{}
	for _, st := range tools {
		if st.Tool.Name == "" {
			t.Error("tool with empty name")
		}
		byName[st.Tool.Name] = st.Tool
	}

	for _, name := range []string{
		"get_status", "list_remotes", "list_transfers", "get_transfer",
		"create_transfers", "pause_transfer", "resume_transfer",
		"abort_transfer", "delete_transfer",
		"get_queue", "pause_queue", "resume_queue",
		"list_dependencies", "install_dependency", "check_updates",
	} {
		if _, ok := byName[name]; !ok {
			t.Errorf("missing tool %q", name)
		}
	}

	readOnly := []string{
		"get_status", "list_remotes", "list_transfers", "get_transfer",
		"get_queue", "list_dependencies", "check_updates",
	}
	destructive := []string{"abort_transfer", "delete_transfer"}

	for _, name := range readOnly {
		a := byName[name].Annotations
		if a.ReadOnlyHint == nil || !*a.ReadOnlyHint {
			t.Errorf("%s: expected readOnlyHint=true", name)
		}
		if a.DestructiveHint == nil || *a.DestructiveHint {
			t.Errorf("%s: expected destructiveHint=false", name)
		}
	}
	for _, name := range destructive {
		a := byName[name].Annotations
		if a.DestructiveHint == nil || !*a.DestructiveHint {
			t.Errorf("%s: expected destructiveHint=true", name)
		}
		if a.ReadOnlyHint != nil && *a.ReadOnlyHint {
			t.Errorf("%s: expected readOnlyHint=false", name)
		}
	}
	for _, name := range []string{"create_transfers", "pause_transfer", "resume_transfer", "pause_queue", "resume_queue", "install_dependency"} {
		a := byName[name].Annotations
		if a.DestructiveHint == nil || *a.DestructiveHint {
			t.Errorf("%s: expected destructiveHint=false", name)
		}
	}
}

func TestToolSchemasRequireIDs(t *testing.T) {
	tools := buildTools(&api{dataDir: t.TempDir()})
	for _, st := range tools {
		switch st.Tool.Name {
		case "get_transfer", "pause_transfer", "resume_transfer", "abort_transfer", "delete_transfer", "install_dependency":
			required := st.Tool.InputSchema.Required
			if len(required) != 1 || required[0] != "id" {
				t.Errorf("%s: expected required [id], got %v", st.Tool.Name, required)
			}
		case "create_transfers":
			required := st.Tool.InputSchema.Required
			if len(required) != 1 || required[0] != "remote_id" {
				t.Errorf("create_transfers: expected required [remote_id], got %v", required)
			}
		}
	}
}

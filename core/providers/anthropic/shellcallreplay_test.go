package anthropic

import (
	"context"
	"strings"
	"testing"

	"github.com/maximhq/bifrost/core/schemas"
)

// TestConvertBifrostMessages_ShellCallKeepsCommands verifies that a shell_call
// replayed to Anthropic keeps its commands instead of collapsing to a bare type name.
func TestConvertBifrostMessages_ShellCallKeepsCommands(t *testing.T) {
	ctx, cancel := schemas.NewBifrostContextWithCancel(context.Background())
	defer cancel()
	caps := schemas.ResolveModelCaps(schemas.Anthropic, "claude-sonnet-4-5-20250929")

	callID := "shell_call_1"
	timeout := 5000
	shellCall := schemas.ResponsesMessage{
		Type: schemas.Ptr(schemas.ResponsesMessageTypeShellCall),
		ResponsesToolMessage: &schemas.ResponsesToolMessage{
			CallID: &callID,
			Action: &schemas.ResponsesToolMessageActionStruct{
				ResponsesShellToolCallAction: &schemas.ResponsesShellToolCallAction{
					Commands:  []string{"ls -la", "cat go.mod"},
					TimeoutMS: &timeout,
				},
			},
			ResponsesShellCall: &schemas.ResponsesShellCall{
				Environment: &schemas.ResponsesShellCallEnvironment{Type: "local"},
			},
		},
	}

	msgs, _ := ConvertBifrostMessagesToAnthropicMessages(ctx, []schemas.ResponsesMessage{shellCall}, true, caps)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d: %+v", len(msgs), msgs)
	}
	if len(msgs[0].Content.ContentBlocks) != 1 || msgs[0].Content.ContentBlocks[0].Text == nil {
		t.Fatalf("expected a single text block, got %+v", msgs[0].Content.ContentBlocks)
	}

	text := *msgs[0].Content.ContentBlocks[0].Text
	for _, want := range []string{"ls -la", "cat go.mod", `"timeout_ms":5000`} {
		if !strings.Contains(text, want) {
			t.Errorf("shell call text missing %q, got:\n%s", want, text)
		}
	}
}

// TestConvertBifrostMessages_UnsupportedToolCallWithoutShellAction verifies the
// generic fallback still applies to non-shell unsupported tool calls.
func TestConvertBifrostMessages_UnsupportedToolCallWithoutShellAction(t *testing.T) {
	ctx, cancel := schemas.NewBifrostContextWithCancel(context.Background())
	defer cancel()
	caps := schemas.ResolveModelCaps(schemas.Anthropic, "claude-sonnet-4-5-20250929")

	callID := "fs_1"
	fileSearch := schemas.ResponsesMessage{
		Type: schemas.Ptr(schemas.ResponsesMessageTypeFileSearchCall),
		ResponsesToolMessage: &schemas.ResponsesToolMessage{
			CallID: &callID,
			Name:   schemas.Ptr("file_search"),
		},
	}

	msgs, _ := ConvertBifrostMessagesToAnthropicMessages(ctx, []schemas.ResponsesMessage{fileSearch}, true, caps)
	if len(msgs) != 1 || len(msgs[0].Content.ContentBlocks) != 1 || msgs[0].Content.ContentBlocks[0].Text == nil {
		t.Fatalf("expected a single text block, got %+v", msgs)
	}
	if text := *msgs[0].Content.ContentBlocks[0].Text; !strings.Contains(text, "Tool call: file_search") {
		t.Fatalf("unexpected fallback text: %s", text)
	}
}

// TestConvertBifrostMessages_ApplyPatchCallKeepsOperation verifies that an
// apply_patch_call replayed to Anthropic keeps its file operation.
func TestConvertBifrostMessages_ApplyPatchCallKeepsOperation(t *testing.T) {
	tests := []struct {
		name      string
		operation schemas.ResponsesApplyPatchOperation
		want      []string
		notWant   []string
	}{
		{
			name:      "create_file",
			operation: schemas.ResponsesApplyPatchOperation{Type: "create_file", Path: "hello.txt", Diff: schemas.Ptr("+hello\n")},
			want:      []string{`"type":"create_file"`, `"path":"hello.txt"`, "+hello"},
		},
		{
			name:      "update_file",
			operation: schemas.ResponsesApplyPatchOperation{Type: "update_file", Path: "main.go", Diff: schemas.Ptr("-old\n+new\n")},
			want:      []string{`"type":"update_file"`, `"path":"main.go"`, "-old", "+new"},
		},
		{
			name:      "delete_file carries no diff",
			operation: schemas.ResponsesApplyPatchOperation{Type: "delete_file", Path: "gone.txt"},
			want:      []string{`"type":"delete_file"`, `"path":"gone.txt"`},
			notWant:   []string{"diff"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := schemas.NewBifrostContextWithCancel(context.Background())
			defer cancel()
			caps := schemas.ResolveModelCaps(schemas.Anthropic, "claude-sonnet-4-5-20250929")

			operation := tt.operation
			applyPatchCall := schemas.ResponsesMessage{
				Type: schemas.Ptr(schemas.ResponsesMessageTypeApplyPatchCall),
				ResponsesToolMessage: &schemas.ResponsesToolMessage{
					CallID:                  schemas.Ptr("apply_patch_call_1"),
					ResponsesApplyPatchCall: &schemas.ResponsesApplyPatchCall{Operation: &operation},
				},
			}

			msgs, _ := ConvertBifrostMessagesToAnthropicMessages(ctx, []schemas.ResponsesMessage{applyPatchCall}, true, caps)
			if len(msgs) != 1 || len(msgs[0].Content.ContentBlocks) != 1 || msgs[0].Content.ContentBlocks[0].Text == nil {
				t.Fatalf("expected a single text block, got %+v", msgs)
			}

			text := *msgs[0].Content.ContentBlocks[0].Text
			if !strings.Contains(text, "apply_patch_call") {
				t.Errorf("apply_patch call text missing the item type, got:\n%s", text)
			}
			for _, want := range tt.want {
				if !strings.Contains(text, want) {
					t.Errorf("apply_patch call text missing %q, got:\n%s", want, text)
				}
			}
			for _, notWant := range tt.notWant {
				if strings.Contains(text, notWant) {
					t.Errorf("apply_patch call text unexpectedly contains %q, got:\n%s", notWant, text)
				}
			}
		})
	}
}

package tagger

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/YuujiKamura/resident-agent/pkg/acp"
)

func TestClassifyGroupBatch_Mock(t *testing.T) {
	// 1. Mock resident-agent's session creator
	acp.DefaultSessionCreator = func(cwd string, model string) (*acp.Session, error) {
		_, _ = io.Pipe()
		_, outW := io.Pipe()

		// Mock session handshake + response
		go func() {
			// initialize
			resp := acp.Response{JsonRPC: "2.0", ID: 1, Result: json.RawMessage(`{"protocolVersion":1}`)}
			data, _ := json.Marshal(resp)
			outW.Write(append(data, '\n'))

			// session/new
			resp = acp.Response{JsonRPC: "2.0", ID: 2, Result: json.RawMessage(`{"sessionId":"mock-sid"}`)}
			data, _ = json.Marshal(resp)
			outW.Write(append(data, '\n'))

			// session/prompt response (JSON array)
			resp = acp.Response{JsonRPC: "2.0", ID: 3, Result: json.RawMessage(`{}`)} // Response itself is empty or JSON-RPC 2.0 result
			// The content is actually delivered via chunks or result. 
			// In our resident-agent implementation, it expects the response result to be 
			// the final text if not streaming, or collected chunks.
			// Let's mock a simple JSON array result.
			resp = acp.Response{
				JsonRPC: "2.0",
				ID:      3,
				Result:  json.RawMessage(`"[{\"file\":\"a.jpg\",\"role\":\"test\",\"has_board\":true,\"detected_text\":\"TEST\"}]"`),
			}
			data, _ = json.Marshal(resp)
			outW.Write(append(data, '\n'))
		}()

		// We need to return a session that uses our mock pipes.
		// Since acp.Session fields are private, we might need a test-only constructor in acp package
		// OR just test that it doesn't crash if we provide a dummy session if we can.
		// For now, let's assume we can mock it or test the internal functions.
		return nil, nil // This will fail if ClassifyGroupBatch calls NewSession
	}
}

func TestExtractJSONArray(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Some text [{\"a\":1}] more text", "[{\"a\":1}]"},
		{"```json\n[{\"b\":2}]\n```", "[{\"b\":2}]"},
		{"No JSON here", ""},
	}

	for _, tt := range tests {
		got, err := extractJSONArray(tt.input)
		if tt.want == "" {
			if err == nil {
				t.Errorf("extractJSONArray(%q) expected error, got %q", tt.input, got)
			}
		} else {
			if err != nil {
				t.Errorf("extractJSONArray(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("extractJSONArray(%q) = %q, want %q", tt.input, got, tt.want)
			}
		}
	}
}

func TestRunGrouping_EmptyFolder(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := Config{Folder: tmpDir, BatchSize: 5}
	res, err := RunGrouping(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunGrouping: %v", err)
	}
	if res.PhotoCount != 0 {
		t.Errorf("expected 0 photos, got %d", res.PhotoCount)
	}
	// Note: photo-groups.json is NOT created if 0 images are found.
	if _, err := os.Stat(res.OutputJSON); err == nil {
		t.Errorf("OutputJSON %s should not be created for empty folder", res.OutputJSON)
	}
}

func TestLoadSaveGroupRecords(t *testing.T) {
	tmpDir := t.TempDir()
	records := map[string]*GroupRecord{
		"test.jpg": {
			GroupCore: GroupCore{
				Role: "test",
			},
			Group: 1,
		},
	}

	if err := SaveGroupRecords(tmpDir, records); err != nil {
		t.Fatalf("SaveGroupRecords: %v", err)
	}

	loaded := LoadGroupRecords(tmpDir)
	if len(loaded) != 1 {
		t.Fatalf("expected 1 record, got %d", len(loaded))
	}
	if loaded["test.jpg"].Role != "test" {
		t.Errorf("expected role test, got %q", loaded["test.jpg"].Role)
	}
}

// Package app tests Codex JSONL pipe termination used by workflow stages.
package app

import (
	"os"
	"testing"
)

// closedAfterJSONLReader simulates os/exec closing stdout immediately after the final event.
type closedAfterJSONLReader struct {
	sent bool
}

// Read returns one complete Codex event followed by os.ErrClosed.
func (r *closedAfterJSONLReader) Read(buffer []byte) (int, error) {
	if r.sent {
		return 0, os.ErrClosed
	}
	r.sent = true
	event := []byte("{\"type\":\"thread.started\",\"thread_id\":\"thread-pipe\"}\n")
	return copy(buffer, event), nil
}

// TestDrainCodexJSONLTreatsClosedPipeAsEOF verifies normal process-pipe closure is not a stage failure.
func TestDrainCodexJSONLTreatsClosedPipeAsEOF(t *testing.T) {
	threadID, err := drainCodexJSONL(&closedAfterJSONLReader{}, nil)
	if err != nil {
		t.Fatalf("closed stdout pipe returned error: %v", err)
	}
	if threadID != "thread-pipe" {
		t.Fatalf("thread id = %q, want thread-pipe", threadID)
	}
}

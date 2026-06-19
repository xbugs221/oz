// Package app tests detached oz flow worker command lines.
package app

import (
	"reflect"
	"testing"
)

// TestDetachedWorkerCommandsUseOzFlowPrefix verifies background workers re-enter the merged CLI.
func TestDetachedWorkerCommandsUseOzFlowPrefix(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "resume run worker",
			args: []string{"resume", "--run-id", "run-1", "--json"},
			want: []string{"flow", "resume", "--run-id", "run-1", "--json"},
		},
		{
			name: "batch worker",
			args: []string{"batch", "--batch-id", "batch-1", "--json"},
			want: []string{"flow", "batch", "--batch-id", "batch-1", "--json"},
		},
		{
			name: "loop worker",
			args: []string{"loop", "--worker", "--json"},
			want: []string{"flow", "loop", "--worker", "--json"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := flowWorkerCommandArgs(tt.args...); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("worker args = %#v, want %#v", got, tt.want)
			}
		})
	}
}

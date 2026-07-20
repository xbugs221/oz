// Package app tests watch target selection across loop-created batch replacements.
package app

import "testing"

// TestResolveWatchFrameTargetRefreshesImplicitBatch verifies default watch follows a replacement batch.
func TestResolveWatchFrameTargetRefreshesImplicitBatch(t *testing.T) {
	repo := t.TempDir()
	old := BatchState{BatchID: "20260720T090000.000000000Z", Status: batchStatusArchived}
	newer := BatchState{BatchID: "20260720T090100.000000000Z", Status: batchStatusRunning}
	if err := saveBatchState(repo, old); err != nil {
		t.Fatal(err)
	}
	if err := saveBatchState(repo, newer); err != nil {
		t.Fatal(err)
	}

	kind, ref, err := resolveWatchFrameTarget(repo, false, "batch", StatusRef{Alias: "b2", ID: old.BatchID})
	if err != nil {
		t.Fatal(err)
	}
	if kind != "batch" || ref.ID != newer.BatchID {
		t.Fatalf("implicit watch target = %s/%s, want batch/%s", kind, ref.ID, newer.BatchID)
	}

	kind, ref, err = resolveWatchFrameTarget(repo, true, "batch", StatusRef{Alias: "b2", ID: old.BatchID})
	if err != nil {
		t.Fatal(err)
	}
	if kind != "batch" || ref.ID != old.BatchID {
		t.Fatalf("explicit watch target = %s/%s, want batch/%s", kind, ref.ID, old.BatchID)
	}
}

package main

import "testing"

func TestL4SnapshotResetKindInitial(t *testing.T) {
	if got := l4SnapshotResetKind(1); got != "initial" {
		t.Fatalf("got %q, want initial", got)
	}
}

func TestL4SnapshotResetKindReplacement(t *testing.T) {
	for _, count := range []int{2, 10} {
		if got := l4SnapshotResetKind(count); got != "replacement" {
			t.Fatalf("count %d: got %q, want replacement", count, got)
		}
	}
}

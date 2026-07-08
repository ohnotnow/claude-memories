package main

import (
	"testing"
	"time"
)

func TestLocalTimeConvertsStoreTimestamps(t *testing.T) {
	// TestMain pins time.Local to UTC for the golden tests; use an explicit
	// zone here so the conversion is actually exercised.
	old := time.Local
	time.Local = time.FixedZone("TST", 3600)
	defer func() { time.Local = old }()

	if got := localTime("2026-01-05 10:00:00"); got != "2026-01-05 11:00:00" {
		t.Errorf("localTime = %q, want the +1h zone applied", got)
	}
	// Anything unparseable passes through untouched rather than erroring —
	// display code must never lose the underlying value.
	if got := localTime("not a timestamp"); got != "not a timestamp" {
		t.Errorf("unparseable input should pass through, got %q", got)
	}
}

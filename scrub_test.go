package main

import (
	"strings"
	"testing"
)

func TestScrubber(t *testing.T) {
	s := newScrubber("/Users/test-user")
	cases := []struct{ in, want string }{
		{"/Users/test-user/code/sample-app/loader.go", "~/code/sample-app/loader.go"},
		{"cd /Users/test-user", "cd ~"},
		{"path is /Users/test-user, then more", "path is ~, then more"},
		// A longer username is somebody else — leave it alone.
		{"/Users/test-userx/code", "/Users/test-userx/code"},
		{"/Users/test-user.backup/code", "/Users/test-user.backup/code"},
		// Dash-encoded transcript paths (dash is the separator there).
		{"-Users-test-user-Documents-code-app", "~-Documents-code-app"},
		{"ls ~/.claude/projects/-Users-test-user-Documents-code-app/", "ls ~/.claude/projects/~-Documents-code-app/"},
		// Multiple occurrences in one chunk.
		{"/Users/test-user/a and /Users/test-user/b", "~/a and ~/b"},
	}
	for _, c := range cases {
		if got := s.scrub(c.in); got != c.want {
			t.Errorf("scrub(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestScrubWriterReportsFullLength(t *testing.T) {
	var out strings.Builder
	w := newScrubber("/Users/test-user").wrap(&out)
	in := "saw /Users/test-user/code today"
	n, err := w.Write([]byte(in))
	if err != nil {
		t.Fatal(err)
	}
	if n != len(in) {
		t.Errorf("Write reported %d bytes, want %d (callers treat short counts as errors)", n, len(in))
	}
	if out.String() != "saw ~/code today" {
		t.Errorf("scrubbed output = %q", out.String())
	}
}

func TestEmptyScrubberPassesThrough(t *testing.T) {
	var out strings.Builder
	s := newScrubber("")
	if w := s.wrap(&out); w != &out {
		t.Error("empty scrubber should return the writer unwrapped")
	}
}

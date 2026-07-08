package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEncodeProjectDir(t *testing.T) {
	cases := []struct{ in, want string }{
		// Rules verified against real ~/.claude/projects entries 2026-07-08:
		// "/", "." and "_" all become "-"; existing dashes pass through.
		{"/Users/test-user/code/my_app", "-Users-test-user-code-my-app"},
		{"/Users/test-user/.claude", "-Users-test-user--claude"},
		{"/tmp/a.b_c-d", "-tmp-a-b-c-d"},
		{"/Users/test-user/code/qr-to-teams", "-Users-test-user-code-qr-to-teams"},
	}
	for _, c := range cases {
		if got := encodeProjectDir(c.in); got != c.want {
			t.Errorf("encodeProjectDir(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// The fixtures below are synthetic transcripts. NEVER swap in a real one —
// real transcripts carry real hostnames, names and work context.
const olderSession = `{"type":"user","timestamp":"2026-01-02T10:00:00.000Z","message":{"role":"user","content":"Please fix the flaky widget test in example_test.go"}}
{"type":"assistant","timestamp":"2026-01-02T10:00:05.000Z","message":{"role":"assistant","content":[{"type":"text","text":"Looking at example_test.go now."}]}}
{"type":"user","timestamp":"2026-01-02T10:00:10.000Z","message":{"role":"user","content":[{"type":"tool_result","content":"ok"}]}}
{"type":"user","timestamp":"2026-01-02T10:00:15.000Z","isSidechain":true,"message":{"role":"user","content":"Subagent prompt: search test-project for widgets"}}
{"type":"user","timestamp":"2026-01-02T10:30:00.000Z","message":{"role":"user","content":"y"}}
this line is not json and must be skipped without killing the scan
`

const newerSession = `{"type":"user","timestamp":"2026-01-03T09:00:00.000Z","message":{"role":"user","content":"<command-message>demo</command-message>\n<command-name>/demo</command-name>"}}
{"type":"user","timestamp":"2026-01-03T09:05:00.000Z","message":{"role":"user","content":[{"type":"text","text":"How should test-user configure example.com here?"}]}}
{"type":"system","timestamp":"2026-01-03T09:06:00.000Z"}
`

func writeFixtureProject(t *testing.T, root, projectPath string, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(root, encodeProjectDir(projectPath))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestListSessions(t *testing.T) {
	root := t.TempDir()
	project := "/Users/test-user/code/sample-app"
	writeFixtureProject(t, root, project, map[string]string{
		"aaaa1111-0000-0000-0000-000000000000.jsonl": olderSession,
		"bbbb2222-0000-0000-0000-000000000000.jsonl": newerSession,
		"not-a-transcript.txt":                       "ignore me",
	})

	sessions, _, err := listSessions(root, project)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(sessions))
	}

	// Newest first.
	if got := sessions[0].ID; got != "bbbb2222-0000-0000-0000-000000000000" {
		t.Errorf("expected newer session first, got %s", got)
	}

	newer, older := sessions[0], sessions[1]

	// Slash-command wrapper: the command name is the gist, and it counts as
	// a prompt alongside the array-form text block.
	if newer.Gist != "/demo" {
		t.Errorf("newer gist = %q, want %q", newer.Gist, "/demo")
	}
	if newer.Prompts != 2 {
		t.Errorf("newer prompts = %d, want 2", newer.Prompts)
	}

	// Plain prompt gist; tool_result and sidechain entries don't count, the
	// bare "y" does; the malformed trailing line doesn't kill the scan.
	if !strings.HasPrefix(older.Gist, "Please fix the flaky widget test") {
		t.Errorf("older gist = %q", older.Gist)
	}
	if older.Prompts != 2 {
		t.Errorf("older prompts = %d, want 2", older.Prompts)
	}
	if older.Started.IsZero() || !older.Ended.After(older.Started) {
		t.Errorf("older span looks wrong: %v → %v", older.Started, older.Ended)
	}
}

func TestListSessionsMissingDir(t *testing.T) {
	sessions, dir, err := listSessions(t.TempDir(), "/Users/test-user/code/never-visited")
	if err != nil {
		t.Fatalf("missing dir should not error, got %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("expected no sessions, got %d", len(sessions))
	}
	if !strings.HasSuffix(dir, "-Users-test-user-code-never-visited") {
		t.Errorf("unexpected dir %q", dir)
	}
}

func TestSessionLineSameDaySpan(t *testing.T) {
	root := t.TempDir()
	project := "/Users/test-user/code/sample-app"
	writeFixtureProject(t, root, project, map[string]string{
		"aaaa1111-0000-0000-0000-000000000000.jsonl": olderSession,
	})
	sessions, _, err := listSessions(root, project)
	if err != nil || len(sessions) != 1 {
		t.Fatalf("setup failed: %v, %d sessions", err, len(sessions))
	}
	line := sessionLine(sessions[0])
	for _, want := range []string{"aaaa1111", "2 prompts", "→"} {
		if !strings.Contains(line, want) {
			t.Errorf("session line %q missing %q", line, want)
		}
	}
	// Same-day session: the end of the span is a bare time, not a second date.
	if strings.Count(line, "2026-01-02") != 1 {
		t.Errorf("same-day span should show the date once: %q", line)
	}
}

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Header times render in local time; pin the zone so the golden output is
// the same on every machine.
func TestMain(m *testing.M) {
	time.Local = time.UTC
	os.Exit(m.Run())
}

// Synthetic transcript — the fixture rule applies here too: NEVER paste a
// real transcript in, they carry real hostnames, names and work context.
// This one exercises: prose, a slash command, a successful multi-line tool
// call, a one-line success, an error result, a redacted thinking block, a
// sidechain to omit, harness noise to skip, and an array-content result.
const extractFixture = `{"type":"user","timestamp":"2026-01-05T09:00:00.000Z","cwd":"/Users/test-user/code/sample-app","gitBranch":"main","message":{"role":"user","content":"Please fix the widget loader - it fails on empty input"}}
{"type":"assistant","timestamp":"2026-01-05T09:00:05.000Z","message":{"role":"assistant","content":[{"type":"thinking","thinking":"","signature":"REDACTED-SIG"},{"type":"text","text":"Looking at the loader now."},{"type":"tool_use","id":"call-1","name":"Read","input":{"file_path":"/Users/test-user/code/sample-app/loader.go"}}]}}
{"type":"user","timestamp":"2026-01-05T09:00:10.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-1","content":"package main\n\nfunc Load(s string) {}\n"}]}}
{"type":"assistant","timestamp":"2026-01-05T09:00:20.000Z","message":{"role":"assistant","content":[{"type":"tool_use","id":"call-2","name":"Edit","input":{"file_path":"/Users/test-user/code/sample-app/loader.go","old_string":"x","new_string":"y"}}]}}
{"type":"user","timestamp":"2026-01-05T09:00:25.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-2","content":"<tool_use_error>File has not been read yet. Read it first before writing to it.</tool_use_error>","is_error":true}]}}
{"type":"assistant","timestamp":"2026-01-05T09:00:30.000Z","message":{"role":"assistant","content":[{"type":"text","text":"Ah - the actual issue is the loader never handles empty strings. Fixing properly."},{"type":"tool_use","id":"call-3","name":"Bash","input":{"command":"go   test\n./..."}}]}}
{"type":"user","timestamp":"2026-01-05T09:00:40.000Z","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"call-3","content":[{"type":"text","text":"ok"}]}]}}
{"type":"user","timestamp":"2026-01-05T09:01:00.000Z","isSidechain":true,"message":{"role":"user","content":"subagent: go look at the widget docs"}}
{"type":"assistant","timestamp":"2026-01-05T09:01:10.000Z","isSidechain":true,"message":{"role":"assistant","content":[{"type":"text","text":"subagent findings that should not appear"}]}}
{"type":"user","timestamp":"2026-01-05T09:02:00.000Z","message":{"role":"user","content":"<local-command-stdout>noise that should be skipped</local-command-stdout>"}}
{"type":"user","timestamp":"2026-01-05T09:30:00.000Z","message":{"role":"user","content":"<command-message>demo</command-message>\n<command-name>/demo</command-name>"}}
`

const extractGolden = `# Session fixture-session
Project: /Users/test-user/code/sample-app (branch main)
2026-01-05 09:00 → 09:30 (30m) · 2 prompts · %SIZE% raw · 2 subagent lines omitted

---

**User:**
Please fix the widget loader - it fails on empty input

**Claude:**
Looking at the loader now.
⏺ Read: /Users/test-user/code/sample-app/loader.go
  → ok (3 lines)
⏺ Edit: /Users/test-user/code/sample-app/loader.go
  → ERROR (Edit):
    File has not been read yet. Read it first before writing to it.

**Claude:**
Ah - the actual issue is the loader never handles empty strings. Fixing properly.
⏺ Bash: go test ./...
  → ok: ok

**User:**
/demo
`

func TestRenderTranscriptGolden(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture-session.jsonl")
	if err := os.WriteFile(path, []byte(extractFixture), 0o644); err != nil {
		t.Fatal(err)
	}

	var out strings.Builder
	if err := renderTranscript(path, &out); err != nil {
		t.Fatal(err)
	}

	st, _ := os.Stat(path)
	want := strings.ReplaceAll(extractGolden, "%SIZE%", humanSize(st.Size()))
	if out.String() != want {
		t.Errorf("digest mismatch.\n--- got:\n%s\n--- want:\n%s", out.String(), want)
	}
}

func TestResolveSession(t *testing.T) {
	root := t.TempDir()
	project := "/Users/test-user/code/sample-app"
	dir := writeFixtureProject(t, root, project, map[string]string{
		"aaaa1111-0000-0000-0000-000000000000.jsonl": olderSession,
		"aaaa2222-0000-0000-0000-000000000000.jsonl": newerSession,
	})

	if _, err := resolveSession(root, project, "aaaa1111-0000-0000-0000-000000000000"); err != nil {
		t.Errorf("exact id should resolve: %v", err)
	}
	if _, err := resolveSession(root, project, "aaaa1111-0000-0000-0000-000000000000.jsonl"); err != nil {
		t.Errorf("id with .jsonl suffix should resolve: %v", err)
	}
	got, err := resolveSession(root, project, "aaaa2")
	if err != nil {
		t.Errorf("unique prefix should resolve: %v", err)
	} else if filepath.Dir(got) != dir {
		t.Errorf("resolved outside project dir: %s", got)
	}
	if _, err := resolveSession(root, project, "aaaa"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("ambiguous prefix should error, got %v", err)
	}
	if _, err := resolveSession(root, project, "zzzz"); err == nil || !strings.Contains(err.Error(), "no session matching") {
		t.Errorf("unknown id should error, got %v", err)
	}

	// An id that isn't in the current project resolves across all projects
	// (uuids are unique), including when the "current" directory has no
	// transcripts at all — e.g. a session run from a scratch dir.
	otherDir := writeFixtureProject(t, root, "/Users/test-user/code/other-app", map[string]string{
		"cccc3333-0000-0000-0000-000000000000.jsonl": olderSession,
	})
	got, err = resolveSession(root, project, "cccc3333")
	if err != nil {
		t.Errorf("cross-project id should resolve: %v", err)
	} else if filepath.Dir(got) != otherDir {
		t.Errorf("expected resolution into %s, got %s", otherDir, got)
	}
	if _, err := resolveSession(root, "/Users/test-user/never-a-project", "cccc3333"); err != nil {
		t.Errorf("resolution from a transcript-less cwd should still succeed: %v", err)
	}
}

func TestCapRunes(t *testing.T) {
	if got := capRunes("short", 10); got != "short" {
		t.Errorf("capRunes should pass short strings through, got %q", got)
	}
	capped := capRunes(strings.Repeat("é", 50), 10)
	if !strings.HasSuffix(capped, "… [truncated]") {
		t.Errorf("capped string should be marked, got %q", capped)
	}
	if n := len([]rune(strings.TrimSuffix(capped, "… [truncated]"))); n != 9 {
		t.Errorf("expected 9 content runes, got %d", n)
	}
}

func TestLoadBearingInput(t *testing.T) {
	cases := []struct{ in, want string }{
		{`{"command":"ls  -la\n/tmp"}`, "ls -la /tmp"},
		{`{"file_path":"/tmp/x.go","old_string":"a"}`, "/tmp/x.go"},
		{`{"unknown_key":42}`, `{"unknown_key":42}`},
	}
	for _, c := range cases {
		if got := loadBearingInput([]byte(c.in)); got != c.want {
			t.Errorf("loadBearingInput(%s) = %q, want %q", c.in, got, c.want)
		}
	}
}

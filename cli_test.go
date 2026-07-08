package main

import (
	"bytes"
	"context"
	"strconv"
	"strings"
	"testing"
)

func TestSplitSubcommand(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		cmd  string
		rest []string
	}{
		{"empty", nil, "", []string{}},
		{"only flags", []string{"--db", "foo"}, "", []string{"--db", "foo"}},
		{"plain subcommand", []string{"list"}, "list", []string{}},
		{"flag before subcommand", []string{"--db", "foo", "list"}, "list", []string{"--db", "foo"}},
		{"subcommand then args", []string{"search", "glasgow"}, "search", []string{"glasgow"}},
		{"flags split around subcommand", []string{"--db", "x", "list", "--limit", "5"}, "list", []string{"--db", "x", "--limit", "5"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd, rest := splitSubcommand(tc.in)
			if cmd != tc.cmd {
				t.Errorf("cmd = %q, want %q", cmd, tc.cmd)
			}
			if !equalSlice(rest, tc.rest) {
				t.Errorf("rest = %v, want %v", rest, tc.rest)
			}
		})
	}
}

func equalSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestCLIListEmpty(t *testing.T) {
	store := newTestStore(t)
	var buf bytes.Buffer
	memories, err := store.List(context.Background(), 20)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	printMemories(&buf, memories)
	if !strings.Contains(buf.String(), "No memories found") {
		t.Errorf("empty list output = %q", buf.String())
	}
}

func TestCLIRememberFromArgs(t *testing.T) {
	store := newTestStore(t)
	code := cliRemember(context.Background(), store, []string{"likes", "Earl", "Grey"}, strings.NewReader(""))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	hits, err := store.Search(context.Background(), "Earl Grey", 0)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 || hits[0].Content != "likes Earl Grey" {
		t.Errorf("hits = %+v, want single 'likes Earl Grey'", hits)
	}
}

func TestCLIRememberFromStdin(t *testing.T) {
	store := newTestStore(t)
	code := cliRemember(context.Background(), store, nil, strings.NewReader("piped in\n"))
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	hits, err := store.Search(context.Background(), "piped", 0)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 || hits[0].Content != "piped in" {
		t.Errorf("hits = %+v, want single 'piped in'", hits)
	}
}

func TestCLIRememberRequiresContent(t *testing.T) {
	store := newTestStore(t)
	code := cliRemember(context.Background(), store, nil, strings.NewReader(""))
	if code == 0 {
		t.Error("expected non-zero exit code for empty content")
	}
}

func TestCLIShowByID(t *testing.T) {
	store := newTestStore(t)
	m, err := store.Add(context.Background(), "showable fact")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if code := cliShow(context.Background(), store, []string{itoa(m.ID)}); code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
}

func TestCLIShowMissingIDReturnsNonZero(t *testing.T) {
	store := newTestStore(t)
	if code := cliShow(context.Background(), store, []string{"999"}); code == 0 {
		t.Error("expected non-zero exit for missing id")
	}
	if code := cliShow(context.Background(), store, []string{"not-a-number"}); code == 0 {
		t.Error("expected non-zero exit for non-numeric id")
	}
	if code := cliShow(context.Background(), store, nil); code == 0 {
		t.Error("expected non-zero exit for missing argument")
	}
}

func TestCLIDeleteByID(t *testing.T) {
	store := newTestStore(t)
	m, err := store.Add(context.Background(), "temporary")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	code := cliDelete(context.Background(), store, []string{itoa(m.ID)})
	if code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	got, err := store.Get(context.Background(), m.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != nil {
		t.Errorf("expected deleted memory to be gone, got %+v", got)
	}
}

func TestCLIDeleteMissingIDReturnsNonZero(t *testing.T) {
	store := newTestStore(t)
	if code := cliDelete(context.Background(), store, []string{"999"}); code == 0 {
		t.Error("expected non-zero exit for missing id")
	}
	if code := cliDelete(context.Background(), store, []string{"not-a-number"}); code == 0 {
		t.Error("expected non-zero exit for non-numeric id")
	}
}

func TestCLIIndexShowsGistsAndOverflow(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if _, err := store.Add(ctx, "Gist line here.\nfull detail below the fold"); err != nil {
		t.Fatalf("add: %v", err)
	}
	memories, err := store.List(ctx, indexCap)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	var buf bytes.Buffer
	printIndex(&buf, buildIndex(memories), 1)
	out := buf.String()
	if !strings.Contains(out, "Gist line here.") {
		t.Errorf("index output missing gist: %q", out)
	}
	if strings.Contains(out, "full detail") {
		t.Errorf("index output should stop at the first line: %q", out)
	}

	// With more in the store than shown, the overflow note appears.
	buf.Reset()
	printIndex(&buf, buildIndex(memories), 250)
	if !strings.Contains(buf.String(), "older memories not shown") {
		t.Errorf("expected overflow note, got %q", buf.String())
	}
}

func TestCLIIndexEmpty(t *testing.T) {
	var buf bytes.Buffer
	printIndex(&buf, nil, 0)
	if !strings.Contains(buf.String(), "No memories yet") {
		t.Errorf("empty index output = %q", buf.String())
	}
}

func TestDreamInstructionsNonEmpty(t *testing.T) {
	if !strings.Contains(dreamInstructions, "Dream mode") {
		t.Errorf("dream instructions missing expected heading: %q", dreamInstructions[:min(80, len(dreamInstructions))])
	}
}

func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

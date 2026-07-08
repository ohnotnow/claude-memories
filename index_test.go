package main

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestGistFirstLineOnly(t *testing.T) {
	got := gist("First line is the gist.\nSecond line ignored.\nThird too.")
	if got != "First line is the gist." {
		t.Errorf("gist = %q, want first line only", got)
	}
}

func TestGistWholeBodyWhenNoNewline(t *testing.T) {
	got := gist("no newlines here")
	if got != "no newlines here" {
		t.Errorf("gist = %q, want whole body", got)
	}
}

func TestGistTrimsSurroundingWhitespace(t *testing.T) {
	// Leading padding and a Windows \r before the newline should both go.
	got := gist("  padded line \r\nnext")
	if got != "padded line" {
		t.Errorf("gist = %q, want trimmed first line", got)
	}
}

func TestGistTruncatesToRuneCap(t *testing.T) {
	got := gist(strings.Repeat("a", 300))
	if n := utf8.RuneCountInString(got); n > gistMaxRunes {
		t.Errorf("gist rune count = %d, want <= %d", n, gistMaxRunes)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncated gist should end with an ellipsis, got %q", got)
	}
}

func TestGistNeverSplitsMultibyteRune(t *testing.T) {
	// 200 emoji, each several bytes: a byte-slice truncation would split one
	// mid-character and produce invalid UTF-8. Rune truncation must not.
	got := gist(strings.Repeat("😀", 200))
	if !utf8.ValidString(got) {
		t.Errorf("gist is not valid UTF-8: %q", got)
	}
	if n := utf8.RuneCountInString(got); n > gistMaxRunes {
		t.Errorf("gist rune count = %d, want <= %d", n, gistMaxRunes)
	}
}

func TestBuildIndexGistMatchesListFirstLine(t *testing.T) {
	// Acceptance criterion 2: a gist is the first line of the newest memory's
	// content, checkable against list(limit=1).
	s := newTestStore(t)
	ctx := context.Background()
	if _, err := s.Add(ctx, "Newest memory gist.\nwith detail below"); err != nil {
		t.Fatalf("add: %v", err)
	}
	newest, err := s.List(ctx, 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	entries := buildIndex(newest)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Gist != "Newest memory gist." {
		t.Errorf("gist = %q, want first line of newest memory", entries[0].Gist)
	}
	if entries[0].ID != newest[0].ID {
		t.Errorf("entry id = %d, want %d", entries[0].ID, newest[0].ID)
	}
}

func TestRecceInstructionsMentionIndex(t *testing.T) {
	if !strings.Contains(recceInstructions, "index()") {
		t.Errorf("recce instructions should tell the agent to call index(): %q", recceInstructions)
	}
}

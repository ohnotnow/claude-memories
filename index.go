package main

import "strings"

// indexCap bounds how many memories index() returns in full. Beyond this, the
// newest indexCap are shown and the remainder are reported as a count — they
// stay reachable via search(). See PRD §4.4 (the "300 pages about the cat"
// guard rail).
const indexCap = 200

// gistMaxRunes caps a gist's length. Counted in runes, not bytes, so a slice
// never splits a multi-byte character — the store holds emoji and accented
// text, and a byte slice would turn one into mojibake.
const gistMaxRunes = 120

// IndexEntry is one line of the session-start index: enough to recognise a
// memory and decide whether to search() for its full text, and no more.
type IndexEntry struct {
	ID        int64  `json:"id"`
	CreatedAt string `json:"created_at"`
	Gist      string `json:"gist"`
}

// gist derives a one-line summary of a memory's content: everything up to the
// first newline (or the whole body when there is none), trimmed and hard-
// truncated to gistMaxRunes runes with an ellipsis. Rune-based on purpose (see
// gistMaxRunes); no sentence-boundary detection — first newline is the rule.
func gist(content string) string {
	line := content
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	line = strings.TrimSpace(line)
	runes := []rune(line)
	if len(runes) <= gistMaxRunes {
		return line
	}
	// Reserve one rune for the ellipsis so the result never exceeds the cap.
	return string(runes[:gistMaxRunes-1]) + "…"
}

// buildIndex turns full memories into their one-line index entries.
func buildIndex(memories []Memory) []IndexEntry {
	entries := make([]IndexEntry, 0, len(memories))
	for _, m := range memories {
		entries = append(entries, IndexEntry{ID: m.ID, CreatedAt: m.CreatedAt, Gist: gist(m.Content)})
	}
	return entries
}

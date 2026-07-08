package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// transcriptLine is the subset of a Claude Code session JSONL line that the
// transcript tooling (sessions, extract) cares about. Transcripts live at
// ~/.claude/projects/<encoded-dir>/<session-uuid>.jsonl, one JSON object per
// line. Note that assistant "thinking" blocks carry an empty text field in
// these logs — only tool traffic, prompts and narrated text survive.
type transcriptLine struct {
	Type        string         `json:"type"`
	Timestamp   string         `json:"timestamp"`
	IsSidechain bool           `json:"isSidechain"`
	CWD         string         `json:"cwd"`
	GitBranch   string         `json:"gitBranch"`
	Message     *transcriptMsg `json:"message"`
}

type transcriptMsg struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// contentBlock is one element of an array-form message content: a text
// block, a tool_use call, a tool_result, or a (redacted) thinking block.
type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	Name      string          `json:"name,omitempty"`        // tool_use: tool name
	Input     json.RawMessage `json:"input,omitempty"`       // tool_use: arguments
	ID        string          `json:"id,omitempty"`          // tool_use: call id
	ToolUseID string          `json:"tool_use_id,omitempty"` // tool_result: id of the call it answers
	Content   json.RawMessage `json:"content,omitempty"`     // tool_result: string or []text blocks
	IsError   bool            `json:"is_error,omitempty"`    // tool_result: absent/false = success
}

// contentString returns the message content when it is the plain-string form
// (a typed user prompt or slash-command wrapper).
func (m *transcriptMsg) contentString() (string, bool) {
	if m == nil || len(m.Content) == 0 || m.Content[0] != '"' {
		return "", false
	}
	var s string
	if err := json.Unmarshal(m.Content, &s); err != nil {
		return "", false
	}
	return s, true
}

// contentBlocks returns the message content when it is the array-of-blocks
// form (assistant output, or user entries carrying tool_results).
func (m *transcriptMsg) contentBlocks() ([]contentBlock, bool) {
	if m == nil || len(m.Content) == 0 || m.Content[0] != '[' {
		return nil, false
	}
	var blocks []contentBlock
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return nil, false
	}
	return blocks, true
}

// scanTranscript streams a transcript file line by line, calling fn for each
// parseable line. Individual lines can run to megabytes (tool results embed
// whole files), so this reads with an unbounded buffered reader rather than
// bufio.Scanner's capped token size. Malformed lines are skipped, not fatal —
// a half-written last line mustn't hide an otherwise good session.
func scanTranscript(path string, fn func(line transcriptLine)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	r := bufio.NewReaderSize(f, 64*1024)
	for {
		raw, err := r.ReadBytes('\n')
		if len(raw) > 0 {
			var line transcriptLine
			if jsonErr := json.Unmarshal(raw, &line); jsonErr == nil {
				fn(line)
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
	}
}

// encodeProjectDir maps an absolute project path to Claude Code's transcript
// directory name: every character outside [A-Za-z0-9] becomes '-'. Verified
// empirically against ~/.claude/projects (2026-07-08): "/", "." and "_" all
// map to "-", and existing dashes pass through unchanged.
func encodeProjectDir(path string) string {
	var b strings.Builder
	b.Grow(len(path))
	for _, r := range path {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

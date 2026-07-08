package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Per-block caps. Generous on purpose — the digest must stay readable by a
// distilling session, but a single pasted 50KB log in a prompt mustn't
// reimport the noise extract exists to remove.
const (
	proseCap = 4000 // runes per user/assistant prose block
	errorCap = 2000 // runes per error result — errors are the fossils, keep them meaty
	inputCap = 160  // runes for a tool call's load-bearing input
)

// cliExtract renders one session transcript as compact markdown on stdout.
// Like sessions, it reads transcript files rather than the store.
func cliExtract(args []string, w io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "extract: session id (or transcript path) is required — `claude-memories sessions` lists them")
		return 2
	}
	arg := args[0]

	path := arg
	if !strings.ContainsRune(arg, os.PathSeparator) {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		root, err := projectsRoot()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		path, err = resolveSession(root, cwd, arg)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}

	if err := renderTranscript(path, w); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// resolveSession turns a session id (full uuid or unique prefix, with or
// without the .jsonl suffix) into a transcript path. It tries the current
// project's transcript directory first, then falls back to every project —
// session ids are uuids, so a hit elsewhere is unambiguous. (Field-tested
// 2026-07-08: guddling another project's session by id is a real workflow,
// and cwd-relative resolution alone stranded a session run from a scratch
// directory.)
func resolveSession(root, projectPath, arg string) (string, error) {
	id := strings.TrimSuffix(arg, ".jsonl")
	local := filepath.Join(root, encodeProjectDir(projectPath))

	matches := matchSessions(local, id)
	if len(matches) == 0 {
		dirs, err := os.ReadDir(root)
		if err != nil {
			return "", fmt.Errorf("no transcripts found (looked under %s)", root)
		}
		for _, d := range dirs {
			if !d.IsDir() || d.Name() == filepath.Base(local) {
				continue
			}
			matches = append(matches, matchSessions(filepath.Join(root, d.Name()), id)...)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no session matching %q in any project under %s — `claude-memories sessions` lists them", arg, root)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("session id %q is ambiguous: %s", arg, strings.Join(matches, ", "))
	}
}

// matchSessions returns the transcripts in one directory matching an id —
// an exact filename hit, or every prefix match. A missing directory simply
// yields no matches (transcripts get pruned; scratch dirs never had any).
func matchSessions(dir, id string) []string {
	exact := filepath.Join(dir, id+".jsonl")
	if _, err := os.Stat(exact); err == nil {
		return []string{exact}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var matches []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".jsonl") && strings.HasPrefix(e.Name(), id) {
			matches = append(matches, filepath.Join(dir, e.Name()))
		}
	}
	return matches
}

// renderTranscript writes the digest in two passes: a metadata pass (header
// facts live at both ends of the file), then the body. Reading the file
// twice is deliberate — it keeps memory flat no matter how big the session.
func renderTranscript(path string, w io.Writer) error {
	info, err := summariseSession(path)
	if err != nil {
		return err
	}

	var cwd, branch string
	sidechain := 0
	err = scanTranscript(path, func(line transcriptLine) {
		if cwd == "" && line.CWD != "" {
			cwd = line.CWD
		}
		if branch == "" && line.GitBranch != "" {
			branch = line.GitBranch
		}
		if line.IsSidechain {
			sidechain++
		}
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "# Session %s\n", info.ID)
	if cwd != "" {
		if branch != "" {
			fmt.Fprintf(w, "Project: %s (branch %s)\n", cwd, branch)
		} else {
			fmt.Fprintf(w, "Project: %s\n", cwd)
		}
	}
	span := "unknown time"
	if !info.Started.IsZero() {
		span = fmt.Sprintf("%s → %s (%s)",
			info.Started.Local().Format("2006-01-02 15:04"),
			info.Ended.Local().Format("15:04"),
			formatDuration(info.Ended.Sub(info.Started)))
	}
	fmt.Fprintf(w, "%s · %d prompts · %s raw", span, info.Prompts, humanSize(info.Size))
	if sidechain > 0 {
		// Sidechain lines are subagent conversations: bulky, and their
		// conclusions come back through the main thread anyway. Omitted
		// rather than compressed — the raw transcript is there if a
		// subagent's own workings turn out to matter.
		fmt.Fprintf(w, " · %d subagent lines omitted", sidechain)
	}
	fmt.Fprint(w, "\n\n---\n")

	toolNames := map[string]string{}
	return scanTranscript(path, func(line transcriptLine) {
		if line.IsSidechain {
			return
		}
		switch line.Type {
		case "user":
			renderUserLine(w, line, toolNames)
		case "assistant":
			renderAssistantLine(w, line, toolNames)
		}
	})
}

func renderUserLine(w io.Writer, line transcriptLine, toolNames map[string]string) {
	if s, ok := line.Message.contentString(); ok {
		if text, ok := classifyPrompt(s); ok {
			fmt.Fprintf(w, "\n**User:**\n%s\n", capRunes(text, proseCap))
		}
		return
	}
	blocks, ok := line.Message.contentBlocks()
	if !ok {
		return
	}
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if text, ok := classifyPrompt(b.Text); ok {
				fmt.Fprintf(w, "\n**User:**\n%s\n", capRunes(text, proseCap))
			}
		case "tool_result":
			renderToolResult(w, b, toolNames)
		}
	}
}

func renderAssistantLine(w io.Writer, line transcriptLine, toolNames map[string]string) {
	blocks, ok := line.Message.contentBlocks()
	if !ok {
		return
	}
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) != "" {
				fmt.Fprintf(w, "\n**Claude:**\n%s\n", capRunes(b.Text, proseCap))
			}
		case "tool_use":
			toolNames[b.ID] = b.Name
			fmt.Fprintf(w, "⏺ %s: %s\n", b.Name, loadBearingInput(b.Input))
			// thinking: skipped — the logged field is always empty (only a
			// signature is recorded), so there is nothing to render.
		}
	}
}

// renderToolResult is where the digest's asymmetry lives: successes squash
// to a line, errors stay meaty. Errors are what a distiller digs for.
func renderToolResult(w io.Writer, b contentBlock, toolNames map[string]string) {
	name := toolNames[b.ToolUseID]
	if name == "" {
		name = "tool"
	}
	text := strings.TrimSpace(resultText(b))
	if b.IsError {
		text = strings.TrimSuffix(strings.TrimPrefix(text, "<tool_use_error>"), "</tool_use_error>")
		fmt.Fprintf(w, "  → ERROR (%s):\n%s\n", name, indent(capRunes(text, errorCap), "    "))
		return
	}
	lines := strings.Count(text, "\n") + 1
	switch {
	case text == "":
		fmt.Fprint(w, "  → ok\n")
	case lines == 1 && len([]rune(text)) <= 100:
		// A one-line result is often the whole story ("Stored memory 1.").
		fmt.Fprintf(w, "  → ok: %s\n", text)
	case lines == 1:
		fmt.Fprint(w, "  → ok (1 line)\n")
	default:
		fmt.Fprintf(w, "  → ok (%d lines)\n", lines)
	}
}

// resultText flattens a tool_result's content (string, or an array of text
// blocks) into plain text.
func resultText(b contentBlock) string {
	if len(b.Content) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(b.Content, &s); err == nil {
		return s
	}
	var blocks []contentBlock
	if err := json.Unmarshal(b.Content, &blocks); err == nil {
		parts := make([]string, 0, len(blocks))
		for _, inner := range blocks {
			if inner.Text != "" {
				parts = append(parts, inner.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// loadBearingInput reduces a tool call's arguments to the one field a reader
// needs to know what the call did. Priority order favours the common tools;
// anything unrecognised falls back to compacted JSON.
func loadBearingInput(input json.RawMessage) string {
	var fields map[string]any
	if err := json.Unmarshal(input, &fields); err == nil {
		for _, key := range []string{"command", "file_path", "pattern", "query", "path", "url", "skill", "prompt", "description", "title", "content"} {
			if v, ok := fields[key].(string); ok && strings.TrimSpace(v) != "" {
				return capRunes(collapseWhitespace(v), inputCap)
			}
		}
	}
	return capRunes(collapseWhitespace(string(input)), inputCap)
}

func collapseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func capRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "… [truncated]"
}

func indent(s, prefix string) string {
	return prefix + strings.ReplaceAll(s, "\n", "\n"+prefix)
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Minute)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

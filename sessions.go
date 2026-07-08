package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// sessionInfo summarises one transcript file, enough for a reader (usually a
// Claude session resolving "yesterday's session") to pick the right one.
type sessionInfo struct {
	ID      string
	Path    string
	Started time.Time
	Ended   time.Time
	Prompts int
	Size    int64
	Gist    string
}

// projectsRoot returns the directory Claude Code keeps per-project
// transcript folders in.
func projectsRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "projects"), nil
}

// listSessions returns the sessions for one project, newest first, plus the
// transcript directory it looked in. A missing directory is not an error —
// transcripts get pruned, and a project may simply never have had a session —
// so callers get an empty list and report the path instead.
func listSessions(root, projectPath string) ([]sessionInfo, string, error) {
	dir := filepath.Join(root, encodeProjectDir(projectPath))
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, dir, nil
	}
	if err != nil {
		return nil, dir, err
	}

	sessions := []sessionInfo{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		info, err := summariseSession(path)
		if err != nil {
			// One unreadable file shouldn't hide the rest.
			continue
		}
		sessions = append(sessions, info)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Ended.After(sessions[j].Ended)
	})
	return sessions, dir, nil
}

func summariseSession(path string) (sessionInfo, error) {
	info := sessionInfo{
		ID:   strings.TrimSuffix(filepath.Base(path), ".jsonl"),
		Path: path,
	}
	if st, err := os.Stat(path); err == nil {
		info.Size = st.Size()
	}

	err := scanTranscript(path, func(line transcriptLine) {
		if t, err := time.Parse(time.RFC3339Nano, line.Timestamp); err == nil {
			if info.Started.IsZero() || t.Before(info.Started) {
				info.Started = t
			}
			if t.After(info.Ended) {
				info.Ended = t
			}
		}
		if text, ok := promptText(line); ok {
			info.Prompts++
			if info.Gist == "" {
				info.Gist = gist(text)
			}
		}
	})
	return info, err
}

var commandNameRe = regexp.MustCompile(`<command-name>([^<]+)</command-name>`)

// promptText decides whether a transcript line is a human input and, if so,
// what to show for it. Heuristic, deliberately simple:
//   - only main-thread user entries count (sidechains are subagent traffic);
//   - a plain string is a typed prompt, except harness noise: "Caveat:"
//     preambles and "<local-command-stdout>"-style wrappers. Slash commands
//     arrive wrapped in <command-*> tags — the command name is the useful
//     gist, so extract it;
//   - array content counts only if it carries a non-empty text block that
//     isn't harness noise (tool_result-only entries are plumbing, not input).
func promptText(line transcriptLine) (string, bool) {
	if line.Type != "user" || line.IsSidechain || line.Message == nil {
		return "", false
	}
	if s, ok := line.Message.contentString(); ok {
		return classifyPrompt(s)
	}
	if blocks, ok := line.Message.contentBlocks(); ok {
		for _, b := range blocks {
			if b.Type != "text" || strings.TrimSpace(b.Text) == "" {
				continue
			}
			return classifyPrompt(b.Text)
		}
	}
	return "", false
}

func classifyPrompt(s string) (string, bool) {
	trimmed := strings.TrimSpace(s)
	if m := commandNameRe.FindStringSubmatch(trimmed); m != nil {
		return m[1], true
	}
	if trimmed == "" || strings.HasPrefix(trimmed, "<") || strings.HasPrefix(trimmed, "Caveat:") {
		return "", false
	}
	return trimmed, true
}

// cliSessions lists a project's sessions — the current one by default, or
// another via --project (added after field-testing showed cold-guddling one
// project's sessions from inside another is a real workflow). It reads no
// store, so it is dispatched before the --db handling in runCLI.
func cliSessions(w io.Writer, args []string) int {
	fs := flag.NewFlagSet("sessions", flag.ContinueOnError)
	project := fs.String("project", "", "project path to list sessions for (default: current directory)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	cwd := *project
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	root, err := projectsRoot()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	sessions, dir, err := listSessions(root, cwd)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if len(sessions) == 0 {
		fmt.Fprintf(w, "No sessions found for this project (looked in %s).\n", dir)
		return 0
	}
	for _, s := range sessions {
		fmt.Fprintln(w, sessionLine(s))
	}
	return 0
}

// sessionLine renders one session as a single line:
//
//	<id>  <start> → <end>  <n> prompts  <size>  <gist>
func sessionLine(s sessionInfo) string {
	span := "unknown time"
	if !s.Started.IsZero() {
		start := s.Started.Local()
		end := s.Ended.Local()
		if start.Format("2006-01-02") == end.Format("2006-01-02") {
			span = fmt.Sprintf("%s → %s", start.Format("2006-01-02 15:04"), end.Format("15:04"))
		} else {
			span = fmt.Sprintf("%s → %s", start.Format("2006-01-02 15:04"), end.Format("2006-01-02 15:04"))
		}
	}
	plural := "s"
	if s.Prompts == 1 {
		plural = ""
	}
	return fmt.Sprintf("%s  %s  %d prompt%s  %s  %s", s.ID, span, s.Prompts, plural, humanSize(s.Size), s.Gist)
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.0fKB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

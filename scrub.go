package main

import (
	"io"
	"os"
	"regexp"
)

// scrubber replaces the current user's home directory with "~" in digest
// output, so a real username never reaches a distilling session's context —
// and from there a memory — in the first place. dream()'s PII sweep is the
// backstop, not the plan.
//
// Deliberately literal: it scrubs THIS user's home (plain and dash-encoded
// forms), not a blanket /home/<anything>/ regex, because paths like
// /home/forge/app.example.com are often the load-bearing content of a
// lesson rather than PII. Residual it knowingly does not catch: bare
// usernames inside command output (whoami, env dumps, ls -l owners) and
// colleagues' names in prose.
type scrubber struct {
	patterns []*regexp.Regexp
}

func newScrubber(home string) *scrubber {
	if home == "" {
		return &scrubber{}
	}
	// A trailing letter/digit/._- means a longer, different username —
	// /Users/test-userx must survive /Users/test-user's scrub. In the
	// dash-encoded form ("-Users-test-user-Documents-…") the dash is the
	// path separator, so there it counts as a boundary rather than a name
	// char.
	plain := regexp.MustCompile(regexp.QuoteMeta(home) + `($|[^a-zA-Z0-9_.-])`)
	encoded := regexp.MustCompile(regexp.QuoteMeta(encodeProjectDir(home)) + `($|[^a-zA-Z0-9_.])`)
	return &scrubber{patterns: []*regexp.Regexp{plain, encoded}}
}

// defaultScrubber scrubs the home directory of whoever runs the binary. If
// the home directory can't be determined there is nothing to scrub with, so
// output passes through unchanged.
func defaultScrubber() *scrubber {
	home, err := os.UserHomeDir()
	if err != nil {
		return &scrubber{}
	}
	return newScrubber(home)
}

func (s *scrubber) scrub(text string) string {
	for _, re := range s.patterns {
		text = re.ReplaceAllString(text, "~${1}")
	}
	return text
}

// wrap returns a writer that scrubs everything written through it. Each
// fmt.Fprintf in the renderers arrives as a single Write call, so a path
// never straddles a chunk boundary.
func (s *scrubber) wrap(w io.Writer) io.Writer {
	if len(s.patterns) == 0 {
		return w
	}
	return scrubWriter{w: w, s: s}
}

type scrubWriter struct {
	w io.Writer
	s *scrubber
}

func (sw scrubWriter) Write(p []byte) (int, error) {
	if _, err := io.WriteString(sw.w, sw.s.scrub(string(p))); err != nil {
		return 0, err
	}
	// Report the caller's byte count: scrubbing shortens the output, and a
	// short count would make fmt helpers report a spurious error.
	return len(p), nil
}

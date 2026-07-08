package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var version = "dev"

func main() {
	args := os.Args[1:]

	// Intercept help requests before anything else so `--help` / `-h` always
	// show the full CLI usage, not just the MCP flagset's terse output.
	if wantsHelp(args) {
		printHelp(os.Stdout)
		return
	}

	cmd, rest := splitSubcommand(args)

	switch cmd {
	case "", "mcp":
		// No subcommand (or explicit `mcp`) → run as an MCP stdio server.
		// Keeps backward compatibility for `claude mcp add claude-memories <path>`.
		os.Exit(runMCP(rest))
	case "list", "search", "index", "show", "remember", "delete", "dream", "sessions", "extract":
		os.Exit(runCLI(cmd, rest))
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", cmd)
		printHelp(os.Stderr)
		os.Exit(2)
	}
}

func wantsHelp(args []string) bool {
	for _, a := range args {
		if a == "-h" || a == "--help" || a == "help" {
			return true
		}
	}
	return false
}

// splitSubcommand peels the first non-flag argument off as a subcommand.
// Anything else (flags and their values) stays with the remainder so
// global-style flags like `--db /path` placed either before or after the
// subcommand still work.
func splitSubcommand(args []string) (string, []string) {
	// Flags that consume the next arg as their value. Keeps us from
	// mis-identifying "/some/path" as a subcommand in `--db /some/path`.
	valueFlags := map[string]bool{"db": true, "limit": true, "project": true}

	rest := []string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			rest = append(rest, a)
			name, _, hasValue := splitFlag(a)
			if !hasValue && valueFlags[name] && i+1 < len(args) {
				rest = append(rest, args[i+1])
				i++
			}
			continue
		}
		rest = append(rest, args[i+1:]...)
		return a, rest
	}
	return "", rest
}

func runMCP(args []string) int {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	defaultPath, defaultErr := defaultDBPath()
	dbPath := fs.String("db", defaultPath, "path to SQLite database")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if *dbPath == "" {
		fmt.Fprintf(os.Stderr, "could not determine default db path (%v); pass --db\n", defaultErr)
		return 1
	}

	store, err := NewStore(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open store: %v\n", err)
		return 1
	}
	defer store.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	server := mcp.NewServer(&mcp.Implementation{Name: "claude-memories", Version: version}, &mcp.ServerOptions{Instructions: recceInstructions})
	registerTools(server, store)

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		fmt.Fprintf(os.Stderr, "server: %v\n", err)
		return 1
	}
	return 0
}

func printHelp(w *os.File) {
	fmt.Fprintf(w, `claude-memories — Claude's own note-to-self store, usable as MCP or CLI.

Usage:
  claude-memories [--db PATH]                  run as an MCP stdio server (default)
  claude-memories mcp [--db PATH]              same, explicit
  claude-memories list [--db PATH] [--limit N] newest-first list
  claude-memories search [--limit N] QUERY     all-words search, newest first
  claude-memories index [--db PATH]            one-line gist of every memory
  claude-memories show ID                      print one memory in full
  claude-memories remember CONTENT             store a new memory (or pipe via stdin)
  claude-memories delete ID                    delete by id
  claude-memories dream                        print the dream-mode instructions
  claude-memories sessions [--project PATH]    list a project's Claude Code session transcripts (default: cwd)
  claude-memories extract ID                   render one session as a markdown digest (id resolves across all projects)
  claude-memories help                         show this message

Flags:
  --db PATH    path to the SQLite file (default: OS config dir)
  --limit N    cap results (default 20)
`)
}

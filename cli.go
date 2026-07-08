package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// runCLI dispatches a single CLI subcommand against stdout/stderr and the
// default store. Returns the process exit code.
func runCLI(cmd string, args []string) int {
	if cmd == "dream" {
		fmt.Fprint(os.Stdout, dreamInstructions)
		return 0
	}

	// sessions and extract read transcript files, not the store, so they
	// skip the --db handling below. Both write through the home-dir
	// scrubber: usernames shouldn't reach a distilling session at all.
	if cmd == "sessions" {
		return cliSessions(defaultScrubber().wrap(os.Stdout), args)
	}
	if cmd == "extract" {
		return cliExtract(args, defaultScrubber().wrap(os.Stdout))
	}

	store, dbPath, code := openStoreForCLI(cmd, args, &args)
	if code != 0 {
		return code
	}
	defer store.Close()

	ctx := context.Background()
	switch cmd {
	case "list":
		return cliList(ctx, store, args)
	case "search":
		return cliSearch(ctx, store, args)
	case "index":
		return cliIndex(ctx, store)
	case "show":
		return cliShow(ctx, store, args)
	case "remember":
		return cliRemember(ctx, store, args, os.Stdin)
	case "delete":
		return cliDelete(ctx, store, args)
	}

	fmt.Fprintf(os.Stderr, "unhandled command %q (db=%s)\n", cmd, dbPath)
	return 2
}

// openStoreForCLI parses the `--db` flag out of args in-place so subcommands
// don't each need to redeclare it. The remaining args are written back to
// *remaining. On flag errors it prints usage and returns a non-zero code.
func openStoreForCLI(cmd string, args []string, remaining *[]string) (*Store, string, int) {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	defaultPath, defaultErr := defaultDBPath()
	dbPath := fs.String("db", defaultPath, "path to SQLite database")

	// We want --limit / --query etc. parsed by the subcommand itself, so
	// only strip the --db flag here and leave everything else alone.
	rest, err := parseKnownFlags(fs, args, "db")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return nil, "", 2
	}
	*remaining = rest

	if *dbPath == "" {
		fmt.Fprintf(os.Stderr, "could not determine default db path (%v); pass --db\n", defaultErr)
		return nil, "", 1
	}

	store, err := NewStore(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open store: %v\n", err)
		return nil, "", 1
	}
	return store, *dbPath, 0
}

// parseKnownFlags walks args and hands only the flags registered on fs to
// flag parsing. Anything else (unknown flags, positional args) is returned
// untouched so per-subcommand flagsets can deal with them.
func parseKnownFlags(fs *flag.FlagSet, args []string, known ...string) ([]string, error) {
	knownSet := map[string]bool{}
	for _, k := range known {
		knownSet[k] = true
	}

	var forFS []string
	var rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "-") || a == "-" || a == "--" {
			rest = append(rest, args[i:]...)
			break
		}
		name, value, hasValue := splitFlag(a)
		if !knownSet[name] {
			rest = append(rest, a)
			continue
		}
		if hasValue {
			forFS = append(forFS, a)
			continue
		}
		// No inline value — consume the next arg if present.
		forFS = append(forFS, a)
		if i+1 < len(args) {
			forFS = append(forFS, args[i+1])
			i++
		}
		_ = value
	}
	if err := fs.Parse(forFS); err != nil {
		return nil, err
	}
	return rest, nil
}

func splitFlag(a string) (name, value string, hasValue bool) {
	trimmed := strings.TrimLeft(a, "-")
	if i := strings.Index(trimmed, "="); i >= 0 {
		return trimmed[:i], trimmed[i+1:], true
	}
	return trimmed, "", false
}

func cliList(ctx context.Context, store *Store, args []string) int {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	limit := fs.Int("limit", 20, "maximum number of results")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	memories, err := store.List(ctx, *limit)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	printMemories(os.Stdout, memories)
	return 0
}

func cliSearch(ctx context.Context, store *Store, args []string) int {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	limit := fs.Int("limit", 20, "maximum number of results")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	query := strings.Join(fs.Args(), " ")
	if query == "" {
		fmt.Fprintln(os.Stderr, "search: query is required")
		return 2
	}
	memories, err := store.Search(ctx, query, *limit)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	printMemories(os.Stdout, memories)
	return 0
}

func cliIndex(ctx context.Context, store *Store) int {
	memories, err := store.List(ctx, indexCap)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	total, err := store.Count(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	printIndex(os.Stdout, buildIndex(memories), total)
	return 0
}

func cliShow(ctx context.Context, store *Store, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "show: id is required")
		return 2
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "show: invalid id %q\n", args[0])
		return 2
	}
	m, err := store.Get(ctx, id)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if m == nil {
		fmt.Fprintf(os.Stdout, "No memory with id %d.\n", id)
		return 1
	}
	fmt.Fprintln(os.Stdout, memoryLine(*m))
	return 0
}

func cliRemember(ctx context.Context, store *Store, args []string, stdin io.Reader) int {
	content := strings.TrimSpace(strings.Join(args, " "))
	if content == "" {
		buf, err := io.ReadAll(stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		content = strings.TrimSpace(string(buf))
	}
	if content == "" {
		fmt.Fprintln(os.Stderr, "remember: content is required (as argument or stdin)")
		return 2
	}
	m, err := store.Add(ctx, content)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Fprintf(os.Stdout, "Stored memory %d.\n", m.ID)
	return 0
}

func cliDelete(ctx context.Context, store *Store, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "delete: id is required")
		return 2
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "delete: invalid id %q\n", args[0])
		return 2
	}
	ok, err := store.Delete(ctx, id)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if !ok {
		fmt.Fprintf(os.Stdout, "No memory with id %d.\n", id)
		return 1
	}
	fmt.Fprintf(os.Stdout, "Deleted memory %d.\n", id)
	return 0
}

func printMemories(w io.Writer, memories []Memory) {
	if len(memories) == 0 {
		fmt.Fprintln(w, "No memories found.")
		return
	}
	for _, m := range memories {
		fmt.Fprintln(w, memoryLine(m))
	}
}

func printIndex(w io.Writer, entries []IndexEntry, total int) {
	if len(entries) == 0 {
		fmt.Fprintln(w, "No memories yet.")
		return
	}
	for _, e := range entries {
		fmt.Fprintf(w, "[%d] %s  (%s)\n", e.ID, e.Gist, localTime(e.CreatedAt))
	}
	if total > len(entries) {
		fmt.Fprintf(w, "%d older memories not shown — search reaches them.\n", total-len(entries))
	}
}

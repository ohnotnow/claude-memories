# claude-memories

A small MCP server that gives Claude a global note-to-self store, backed by SQLite. Where its sibling [user-memories](https://github.com/ohnotnow/user-memories) records what Claude learns about *you*, this one records what Claude learns about *itself*.

## What it does

Claude hits the same rakes over and over — the same framework edge-case rediscovered in every project that uses it, the same approach tried and backed out of, the same "ah, *that's* why" moment with no memory of the last three times. Project-scoped memory can't help, because the lesson isn't about the project. This MCP server gives those lessons somewhere to live: one SQLite file (under your OS config directory by default) that any Claude session, in any project, can read from and write to.

The filing test, for what belongs here versus in user-memories:

- Still true if a **different user** turned up tomorrow → claude-memories ("PHP's loose typing hydrates boolean columns as strings — check the cast").
- Still true if a **different model** turned up tomorrow → that's a fact about the user ("prefers Lando for Laravel dev", "writes British English"): the sibling [user-memories](https://github.com/ohnotnow/user-memories) if you run it — otherwise your global `CLAUDE.md` already does that job.
- Specific to one codebase → neither; that's what per-project memory and `CLAUDE.md` are for.

It exposes eight tools: `remember`, `get`, `search`, `list`, `index`, `update`, `delete` and `dream`. The tool descriptions and server instructions steer Claude towards writing lessons a stranger could act on — symptom, cause, where to look next time, with breadcrumbs — because the stranger who has to act on them is a future Claude with none of the context.

The same binary doubles as a regular CLI, so you can list, search, add or delete memories straight from your terminal without going through Claude.

## Prerequisites

- Go 1.25 or newer (for building from source)
- [Claude Code](https://docs.claude.com/en/docs/claude-code), or any other MCP-capable client

## Getting started

### Install

The quickest option is `go install`:

```bash
go install github.com/ohnotnow/claude-memories@latest
```

That drops the binary at `$(go env GOPATH)/bin/claude-memories`, which is usually `~/go/bin/claude-memories`.

### Use a prebuilt binary

If you'd rather not build it yourself, grab one for your platform from the [releases page](https://github.com/ohnotnow/claude-memories/releases). Binaries are named `claude-memories-<os>-<arch>`, so pick the one that matches your machine.

On macOS or Linux, make it executable and stash it somewhere on your PATH:

```bash
chmod +x claude-memories-darwin-arm64
mv claude-memories-darwin-arm64 /usr/local/bin/claude-memories
```

The macOS binary isn't signed, so Gatekeeper will block it the first time you try to run it. Right-click the file in Finder, choose Open, and it'll stop complaining from then on.

On Windows, rename `claude-memories-windows-amd64.exe` to something friendlier like `claude-memories.exe` and drop it somewhere on your PATH.

### Register with Claude Code

```bash
claude mcp add -s user claude-memories ~/go/bin/claude-memories
```

Swap `~/go/bin/claude-memories` for the actual path if you downloaded the binary instead.

`-s user` registers it at user scope so every project gets it. Run `/mcp` inside Claude Code and you should see it listed with its eight tools.

### Database location

The SQLite file lives in your OS's standard config directory (whatever Go's `os.UserConfigDir()` returns):

| OS      | Path                                                         |
| ------- | ------------------------------------------------------------ |
| macOS   | `~/Library/Application Support/claude-memories/memories.db`  |
| Linux   | `~/.config/claude-memories/memories.db`                      |
| Windows | `%AppData%\claude-memories\memories.db`                      |

Pass `--db /path/to/custom.db` if you'd like it somewhere else.

## Tools

| Tool                    | Description                                                                                     |
| ----------------------- | ----------------------------------------------------------------------------------------------- |
| `remember(content)`     | Store a new note-to-self. Start it with a one-sentence gist — the first line is what shows in `index()`. |
| `get(id)`               | Fetch one memory in full by id — the follow-up read when `index()` shows a promising gist.      |
| `search(query, limit?)` | All-words match: every word in the query must appear in a memory, in any order (case-insensitive for ASCII). Newest first, default limit 20. |
| `list(limit?)`          | List memories in full, newest first. Default limit 20.                                          |
| `index()`               | One line (id, date, gist) per memory across the whole store, newest first — the cheap session-start recce. |
| `update(id, content)`   | Rewrite a memory in place, keeping its id and `created_at` (`updated_at` records the rewrite).  |
| `delete(id)`            | Remove a memory by id.                                                                          |
| `dream()`               | Return housekeeping instructions for Claude to tidy up the store (see [Dream mode](#dream-mode)). |

## CLI usage

The same binary that serves MCP over stdio also runs as a regular command-line tool. With no subcommand it stays in MCP mode (so `claude mcp add` keeps working); add a subcommand and it'll act on the store directly:

```bash
claude-memories list                        # newest 20
claude-memories list --limit 100
claude-memories search livewire modal       # all words must appear, any order
claude-memories index                       # one line per memory, whole store
claude-memories show 42                     # one memory in full
claude-memories remember "flux:modal inside flux:card doesn't fire - check ..."
echo "piped content works too" | claude-memories remember
claude-memories delete 42
claude-memories dream                       # prints the dream-mode instructions
claude-memories sessions                    # this project's Claude Code sessions
claude-memories sessions --project ~/code/other-app
claude-memories extract 4a1b4c5f            # one session as a compact markdown digest
claude-memories help
```

All subcommands accept `--db PATH` if you want to point at a non-default store. `list` and `search` accept `--limit N`.

## Session-start recce

The store is pull-based: memories only surface when Claude decides to read them. Left to chance that reliably fails — a fresh session doesn't know what it doesn't know, so it steps on the same rake it stepped on last month. The fix is a cheap "have a nosey" at the start of each session, plus one habit mid-session: when something breaks in a puzzling way, `search()` before debugging from scratch.

`index()` returns one line per memory across the whole store — `id`, date, and a gist (the memory's first line, truncated to ~120 characters, counted in runes so it never mangles emoji or accents). At roughly 2–3k tokens for 100 memories it's cheap enough to call once per session: skim the gists, then `get()` or `search()` for the full text of anything relevant to the day's work. Above ~200 memories `index()` returns the newest 200 and notes how many older ones are only a `search()` away.

The server ships this workflow to clients through the MCP `instructions` field, so harnesses that surface server instructions (Claude Code does) pick it up automatically. For anything that doesn't, the block in [Getting claude to use it](#getting-claude-to-use-it) puts the same guidance in your own instructions file. It also pays to write memories gist-first — a one-sentence opening line — since that first line is all `index()` shows.

## Dream mode

The `dream` tool (MCP) and `claude-memories dream` subcommand (CLI) both return a short set of housekeeping instructions asking Claude to:

1. `index` the whole store for a cheap one-line overview, then pull the full text of anything worth a closer look,
2. look for duplicates, contradictions, version-stale lessons (propose scoping "Livewire 3 only" rather than deleting), fragments that would be stronger as one memory, misfiled entries that belong in user-memories or a project `CLAUDE.md`, lessons too thin to act on, weak `index` gists, and PII or real infrastructure names that slipped in,
3. walk you through the plan one category at a time — quoting each memory's content, since you've likely never read them — and only delete, merge or rewrite once you've signed off on that item.

From inside a Claude Code session you can kick it off with something like:

> Run the claude-memories `dream` tool and then follow the instructions it returns.

Or from the terminal, if you'd rather pipe the prompt in yourself:

```bash
claude-memories dream | pbcopy
```

## Getting claude to use it

The server advertises this workflow itself through the MCP `instructions` field, which Claude Code folds into the session automatically. But the tools are 'deferred' (Claude sees only a tool's name until it looks closer), and not every harness surfaces server instructions — so it's worth putting the workflow in your global `~/.claude/CLAUDE.md` too:

```
## Claude memories (notes to self)

For lessons about your own behaviour and recurring technical rakes — things that would still be true with a different user (framework edge-cases, approaches that had to be backed out, your own failure modes) — use the claude-memories MCP. Facts about the user belong in the sibling user-memories MCP if it's installed — otherwise this global CLAUDE.md already does that job. Single-codebase context belongs in project memory.

It offers:

- `remember(content)` -- Store a note-to-self (gist-first; shape it symptom -> cause -> what to look at, with breadcrumbs)
- `get(id)` -- Fetch one memory in full by id
- `search(query, limit?)` -- Case-insensitive all-words search (every word must appear, any order)
- `list(limit?)` -- List memories in full, newest first
- `index()` -- One line per memory across the whole store, newest first
- `update(id, content)` -- Rewrite a memory in place (keeps its id and created_at)
- `delete(id)` -- Remove a memory
- `dream()` -- Fetch housekeeping instructions for tidying the store

At the start of a session, call `index()` once to skim every memory's gist, then `get()` or `search()` for the full text of anything relevant to the work at hand. When you hit a puzzling error or edge-case mid-session, `search()` for it before debugging from scratch — a previous you has probably already paid for this lesson.

Before calling remember, run a quick search for the topic — avoids writing a duplicate or a contradictory version of something already there. If a lesson is wrong or outdated, prefer `update()` over delete + re-remember: it keeps the memory's id and created_at.
```

## Running tests

```bash
go test ./...
```

Tests run against an in-memory SQLite database, so there's no setup to do.

## Transcript tooling: sessions and extract

The store is half the tool; the other half turns Claude Code's session transcripts (`~/.claude/projects/<encoded-dir>/*.jsonl`) into something a session can actually learn from.

`sessions` lists a project's transcripts — id, time span, prompt count, size, and a gist of the opening prompt. It reads the current project from your working directory; `--project ~/path/to/other-app` lists another project's.

`extract <session-id>` renders one transcript as a compact markdown digest, roughly 25:1 against the raw JSONL. The compression is deliberately lopsided: user prompts and Claude's own narration survive nearly whole, tool calls and successful results get squashed to a line each, but errors keep almost everything — errors are where the lessons live. Subagent traffic is omitted (and counted in the header), and your home directory is scrubbed to `~` in both plain and transcript-encoded forms, so your username never reaches the digest — or anything distilled from it. A session id resolves across every project (they're uuids), and a full path to a `.jsonl` works too.

## The guddle skill

*Guddle* (Scots): to fish by hand, feeling under stones in a burn. [`skill/guddle/SKILL.md`](skill/guddle/SKILL.md) is a Claude Code skill that drives the whole loop. Warm mode distils the session you're in at the end of a day's work; cold mode digs lessons out of past sessions via `sessions` + `extract`. Either way, candidates get the same filing test from the top of this README — a lesson about Claude goes here, a fact about the user goes to user-memories, a single-codebase detail stays in that project's memory — plus a duplicate check against the store. Nothing is stored without your explicit nod.

Install it by copying it into your personal skills folder:

```bash
cp -r skill/guddle ~/.claude/skills/
```

Then `/guddle` in any session — or just ask "anything worth remembering from this session?".

## Releases

Pushing a tag matching `v*.*.*` (for example `v0.1.0`) kicks off the release workflow at `.github/workflows/release.yml`. It builds binaries for Linux, macOS and Windows across amd64/arm64, generates SHA256 checksums, and attaches the lot to a GitHub release with auto-generated notes.

```bash
git tag v0.1.0
git push origin v0.1.0
```

## Contributing

```bash
git clone git@github.com:ohnotnow/claude-memories.git
cd claude-memories
go test ./...
```

Then edit, test, open a PR. The project is deliberately tiny, so small changes are very welcome. Please don't send me a PR that turns it into a platform.

## Licence

MIT. See [LICENSE](LICENSE).

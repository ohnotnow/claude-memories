---
name: guddle
description: Guddle through a Claude Code session for lessons worth keeping — distil notes-to-self into the claude-memories store. Warm mode distils the session you are in; cold mode mines past sessions, from this project or any other. Every candidate lesson needs the user's explicit nod before it is stored.
when_to_use: At the end of a working session ("what did we learn today?", "anything worth remembering from this?", "let's guddle"); when mining past work ("guddle yesterday's session", "rake through the session where we fixed X", a session id, another project's sessions); after a hard-won fix the user would not want re-learned from scratch.
argument-hint: "[session id | \"yesterday's session\" | blank = this session]"
---

# Guddle — distil lessons from sessions

To *guddle* (Scots): to fish by hand, feeling under stones in a burn. You are
fishing a session for lessons. Most stones hide nothing — an empty catch is an
honest result, not a failure. What you do pull out must be alive: a lesson a
future you, with none of this context, could act on.

Two modes, one rule. The rule first, because everything else serves it:

**Nothing is stored without the user's explicit nod on that specific
candidate.** No nod, no `remember()`. If the conversation ends before the nod,
the candidates die with it. That is by design — this store shapes your future
behaviour, which is exactly where a human belongs in the loop.

## Which mode?

- No session named, or "this session" → **warm**: distil the conversation you
  are already in, from your own context.
- A reference to another session ("yesterday's", "the one about the CI fix",
  a session id) → **cold**: dig it out of the transcripts.

## Warm mode

Look back across the session you are in for:

1. **Flails** — an error that took more than one attempt to fix. The first
   failed fix is where the wrong mental model shows; the eventual fix is the
   lesson.
2. **Reversals** — approaches backed out of. What made the first approach
   look right, and what revealed it wasn't?
3. **Surprises** — anything you verified that contradicted what you would
   confidently have asserted from memory. These are the highest-value
   catches: your training data will serve you the same wrong answer again.
4. **Corrections** — places the user redirected you. (Mind the filing test:
   a correction about *their* preferences files under user-memories, not
   here.)

Not every fumble generalises. A typo fixed on the second go teaches nothing;
skip it. Prefer two live lessons over six limp ones.

## Cold mode

1. Run `claude-memories sessions` (Bash; if that is not on the PATH, try
   `~/go/bin/claude-memories`). It lists the current project's transcripts;
   `--project ~/path/to/other-app` lists another project's. Resolve the
   user's reference by date and gist. If more than one session fits, show
   the candidate lines and ask rather than guessing.
2. Run `claude-memories extract <session-id>` and read the digest. A full
   or prefix session id resolves across every project's transcripts, so an
   id is enough wherever you are; a full path to a `.jsonl` also works.
   How to read the digest: successes are squashed to one-liners, errors are kept nearly
   whole — the errors are what you are guddling for. Runs of
   `ERROR → tweak → ERROR → ok` followed by a change of approach, or a
   "**Claude:** ah, the actual issue is…" narration, are lessons with flags
   planted on them. Subagent traffic is omitted; home directories are
   scrubbed to `~`.
3. **Cold scepticism.** A digest tells you *that* something was learned, but
   you must guess *which part generalises* — and you were not there. Scope
   candidates honestly ("Livewire 3", "Go 1.25", "this may have been
   project-specific — unverified"), and when you cannot tell whether the
   stack or the project was at fault, say so in the candidate text rather
   than asserting. If both the tools are missing (command not found), stop
   and say so — do not improvise your own JSONL parsing; the repo is
   `~/Documents/code/claude-memories`.

## The filing test — route every candidate

For each candidate, decide where it lives before you present it:

- Still true if a **different user** turned up tomorrow → **claude-memories**
  (framework rakes, your own failure modes, tooling gotchas).
- Still true if a **different model** turned up tomorrow → **user-memories**
  (their preferences, their infrastructure, their working style) — or the
  user's global CLAUDE.md when that store isn't installed.
- Only true in **one codebase** → that project's memory or `CLAUDE.md` — or
  an `ait`/`ant` entry if it is actionable rather than a lesson.
- True nowhere in particular → let it go.

## Lesson shape

Write every candidate as if a total stranger must act on it without you in
the room — because that stranger is a future you:

- **First line = gist.** One sentence; it is all `index()` shows.
- **Symptom → cause → what to look at next time**, with breadcrumbs: the
  exact error text, `file.go:42` references, library versions.
- **Verified vs guessed, distinguished.** "Confirmed with jq against the
  transcript" is worth ten confident guesses. An honest "I assumed X but
  didn't check" is itself useful.
- **Scoped honestly.** "Livewire 3 hydrates…" not "Livewire hydrates…",
  unless you actually verified it against the current version.

A synthetic example of the shape:

> flux:modal inside flux:card silently never opens (Flux 2.x). Symptom: the
> trigger renders, clicking does nothing, no console error. Cause: the card's
> overflow clips the modal target — verified by moving the modal outside the
> card. Next time: check the modal is a direct child of the page layout, not
> nested in a card; see flux docs on modal placement. (Verified on Flux 2.1,
> 2026-07; unverified on 3.x.)

## Presenting the catch

1. First `search()` the target store for each candidate's topic. A near-match
   means propose an `update()` to enrich the existing memory, not a duplicate
   `remember()`. A contradiction means show both and ask which is current.
2. Present the candidates — typically one to three — each with: the full
   proposed text (quoted, the user has not read your mind), the target store,
   and your recommendation. Lead with the recommendation, not a menu.
3. Take the nods one candidate at a time (a blanket "all fine" from the user
   covers the lot). Store approved candidates via the claude-memories MCP
   `remember()` tool, or `claude-memories remember '…'` if the MCP is not
   available in this session. Confirm the stored ids.
4. If the catch is empty, say so plainly. "Nothing in this session
   generalises" is a perfectly good day's guddling.

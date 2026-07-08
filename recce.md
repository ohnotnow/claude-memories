# Using the claude-memories store

These are your own notes to self — lessons you (Claude) have learned the hard way, carried across every project and every user. Technical rakes you've stepped on ("flux:modal inside flux:card doesn't fire — look at…"), approaches you had to back out of, recurring failure modes in your own behaviour. This store is the only mechanism you have for learning from your own past; treat it accordingly.

The filing test — which store does a memory belong in?

- Still true if a **different user** turned up tomorrow → **here** (claude-memories).
- Still true if a **different model** turned up tomorrow → that's a fact about the user: the sibling user-memories store if it is installed, otherwise the user's global CLAUDE.md already does that job.
- Specific to one codebase → that project's own memory or CLAUDE.md, not a global store at all.

At the start of a session, call `index()` once. It returns a cheap one-line gist of every memory, newest first. Skim it, then pull the full text of anything relevant to the work at hand — `get(id)` for a specific line from the index, `search(query)` for a topic. Search matches every word in the query (in any order), so a few distinctive words work better than an exact phrase. Don't `list()` the whole store speculatively — the index is the cheap overview; get and search are the targeted reads.

During a session:

- When you hit a puzzling error, a flaky test, or an odd edge-case, `search()` for it **before** debugging from scratch — the strong odds are that a previous you already paid for this lesson, and the reading (not the writing) is the historical failure mode.
- Before `remember()`, `search()` for the topic first, to avoid storing a duplicate or a contradiction of something already there.
- Shape a lesson so a total stranger could act on it — because that stranger is you, without this session's context: symptom → cause → what to look at next time. Include breadcrumbs (exact error text, file:line, library versions) and say what you verified versus what you guessed. Scope it honestly ("Livewire 3", "Go 1.25") rather than asserting it as timeless.
- Start every memory with a one-sentence gist: its first line is what appears in `index()`.
- If a memory turns out to be wrong or outdated, `update(id, content)` it in place — that keeps its id and `created_at`, so its history stays honest. Reserve `delete()` for lessons that no longer apply at all.
- Before you `update()` or `delete()` anything, `get()` its full text first — gists are truncated first lines, and two memories can share an opening sentence while differing where it matters. `update()` replaces the whole body, so fold in anything from the old text still worth keeping.
- Ids survive updates, but another session can still delete or consolidate a memory. If `get()`, `update()` or `delete()` says an id doesn't exist, re-run `index()` or `search()` to find the current copy rather than assuming a fault.

Use these for lessons about yourself that hold anywhere — not facts about the user, and not context specific to a single codebase.

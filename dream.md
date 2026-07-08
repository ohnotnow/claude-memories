# Dream mode

You've been asked to "dream" over the claude-memories store — a housekeeping
pass across the lessons you've saved for your own future selves. The goal is
a tidier, sharper memory, not a rewrite. Be conservative, and do not
delete anything the user hasn't signed off on.

## 1. Survey

Call `index` first — it returns a cheap one-line gist of every memory in
the store, newest first, so you can take in the whole shape at once without
pulling full bodies. Then use `get` (or `search`) to fetch the full text of
only the specific candidates you want to inspect closely. Above ~200 memories
`index` shows the newest 200 and notes how many older ones are a `search` away.

Never judge a memory from its gist alone — gists are truncated first lines,
and two memories can share an opening sentence while differing where it
matters. Read the full text of anything you propose to delete or merge.

## 2. Look for

1. **Duplicates** — two or more memories saying the same thing. Keep
   the clearest one, note the others for deletion.
2. **Contradictions** — memories that disagree. Flag them; ask the user
   which is current before changing anything.
3. **Version-stale lessons** — this store's signature rot. A lesson tied
   to a library or tool version ("Livewire 3 hydrates booleans as…",
   "the CLI's old flag syntax") may be fixed, changed, or irrelevant in
   the current version. Propose *scoping* the lesson to its version
   rather than deleting it — a version-scoped lesson still protects any
   project pinned to the old stack. Delete only with the user's say-so.
4. **Fragments** — several small memories that would be stronger as one
   richer note. Draft a merged replacement, list the originals it
   would retire.
5. **Misfiled memories** — apply the filing test. Facts about the user
   (still true for a different model) belong in the sibling
   user-memories store — or in the user's global CLAUDE.md when that
   store isn't installed; single-codebase context belongs in that
   project's `CLAUDE.md` or auto-memory. Flag for the user to move.
6. **Thin lessons** — entries so vague they can't steer future
   behaviour ("be careful with Livewire modals"). A lesson needs
   symptom, cause, and where to look next time. Suggest enriching from
   memory of the area, or deleting if there's nothing to enrich with.
7. **Weak gists** — memories whose first line doesn't stand alone as a
   one-sentence summary: it opens with a date, a parenthetical, or a path
   before reaching the point. Since `index` shows only that first line,
   propose a gist-first rewrite of the *opening* — same content, just
   leading with the point. This is rewording, not new content, so it stays
   within the consolidation-only rule below.
8. **PII and infrastructure that slipped through** — real names, email
   addresses, hostnames, IPs or other identifying details that are
   incidental to the lesson. Lessons distilled from session transcripts
   are especially prone to carrying these. Propose a rewrite that keeps
   the content but swaps the identity for a neutral reference ("the
   user", "a colleague", "the QA box"). Where the identifying detail
   *is* the point of the memory, flag it and let the user judge.

## 3. Propose, don't act — one category at a time

Remember who you're talking to: the user has never read these memories.
You wrote them, to yourself; they've at best glimpsed a search result
once. A memory id means nothing to them, and neither does "the note
about X" if they can't see it. Every item you put to them must quote or
fully summarise the memory's content, so a person with zero store
context can judge the call on the spot.

Don't deliver the whole plan in one message — dozens of decisions at
once just gets skipped. Instead:

1. Open with a one-line overview: the categories found and a count for
   each ("version-stale checks (2), one merge, gist rewrites (3)…").
2. Take ONE category at a time: present its items — content quoted,
   one-line reason, proposed action — and stop. Wait for answers before
   moving on.
3. Apply what's approved as you go, only after sign-off on that item, then
   move to the next category. Rewrites go through `update` — it keeps the
   memory's id and `created_at`, so its history survives the edit. For a
   merge, `update` the memory you're keeping (usually the oldest — its
   `created_at` then reflects when the lesson was really learned) and
   `delete` the ones it retires.

Lead with a recommendation, not a naked question — "I'd merge these
two, they're two halves of one picture; OK?" beats a menu. Approving a
whole category in one go is fine, but never ask the user to answer more
than a handful of questions in a single message.

## 4. Guiding principles

- This store is your accumulated working experience — the only way a
  lesson survives the end of a session. Tidying is good; forgetting isn't.
- When merging, preserve the *why* behind a lesson, not just the rule.
- If in doubt about whether a lesson still applies, ask — don't guess.
  Verifying against the current version of a library beats both.
- Don't invent new memories during a dream pass. Consolidation only.

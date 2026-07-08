package main

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type RememberInput struct {
	Content string `json:"content" jsonschema:"the lesson to store - a note to your future self about your own behaviour or a technical rake you stepped on, worth recalling in any future session regardless of project or user"`
}

type RememberOutput struct {
	Memory Memory `json:"memory"`
}

type SearchInput struct {
	Query string `json:"query" jsonschema:"words to look for - every whitespace-separated word must appear somewhere in a memory, in any order (case-insensitive for ASCII characters)"`
	Limit int    `json:"limit,omitempty" jsonschema:"maximum number of results, newest first (default 20)"`
}

type GetInput struct {
	ID int64 `json:"id" jsonschema:"id of the memory to fetch, as shown by index() or search()"`
}

type GetOutput struct {
	Found  bool    `json:"found"`
	ID     int64   `json:"id"`
	Memory *Memory `json:"memory,omitempty"`
	Note   string  `json:"note,omitempty"`
}

type UpdateInput struct {
	ID      int64  `json:"id" jsonschema:"id of the memory to rewrite"`
	Content string `json:"content" jsonschema:"the full replacement text - it overwrites the old content entirely, so include everything worth keeping; start with a one-sentence gist"`
}

type UpdateOutput struct {
	Updated bool    `json:"updated"`
	ID      int64   `json:"id"`
	Memory  *Memory `json:"memory,omitempty"`
	Note    string  `json:"note,omitempty"`
}

type ListInput struct {
	Limit int `json:"limit,omitempty" jsonschema:"maximum number of results, newest first (default 20)"`
}

type MemoriesOutput struct {
	Memories []Memory `json:"memories"`
	Count    int      `json:"count"`
}

type IndexInput struct{}

type IndexOutput struct {
	Memories []IndexEntry `json:"memories"`
	Count    int          `json:"count"`
	Note     string       `json:"note,omitempty"`
}

type DeleteInput struct {
	ID int64 `json:"id" jsonschema:"id of the memory to delete"`
}

type DeleteOutput struct {
	Deleted bool   `json:"deleted"`
	ID      int64  `json:"id"`
	Note    string `json:"note,omitempty"`
}

type DreamInput struct{}

type DreamOutput struct {
	Instructions string `json:"instructions"`
}

func registerTools(server *mcp.Server, store *Store) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "remember",
		Description: "Store a new note-to-self that should persist across every project. Use for lessons about YOUR OWN behaviour and recurring technical rakes - not facts about the user (that's user-memories) and not project-specific context. Filing test: would this still be true if a different user turned up tomorrow? Shape it symptom -> cause -> what to look at next time, with breadcrumbs (exact error text, file:line, versions), and say what you verified versus what you guessed. Start with a one-sentence gist; the first line is what appears in the session-start index().",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in RememberInput) (*mcp.CallToolResult, RememberOutput, error) {
		if in.Content == "" {
			return nil, RememberOutput{}, fmt.Errorf("content is required")
		}
		m, err := store.Add(ctx, in.Content)
		if err != nil {
			return nil, RememberOutput{}, err
		}
		return textResult(fmt.Sprintf("Stored memory %d.", m.ID)), RememberOutput{Memory: *m}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get",
		Description: "Fetch a single memory in full by id — the follow-up read when index() shows a promising gist.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in GetInput) (*mcp.CallToolResult, GetOutput, error) {
		m, err := store.Get(ctx, in.ID)
		if err != nil {
			return nil, GetOutput{}, err
		}
		if m == nil {
			note := fmt.Sprintf("No memory with id %d — another session may have deleted or consolidated it. Re-run index() or search() to find the current copy.", in.ID)
			return textResult(note), GetOutput{Found: false, ID: in.ID, Note: note}, nil
		}
		return textResult(memoryLine(*m)), GetOutput{Found: true, ID: in.ID, Memory: m}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search",
		Description: "Search your notes-to-self, newest first. Every word in the query must appear in a memory (any order), so prefer a few distinctive words over an exact phrase. When you hit a puzzling error or edge-case mid-session, search here BEFORE debugging from scratch - a previous you has probably already paid for this lesson.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in SearchInput) (*mcp.CallToolResult, MemoriesOutput, error) {
		memories, err := store.Search(ctx, in.Query, in.Limit)
		if err != nil {
			return nil, MemoriesOutput{}, err
		}
		return textResult(summarise(memories)), MemoriesOutput{Memories: memories, Count: len(memories)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "list",
		Description: "List notes-to-self newest first.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ListInput) (*mcp.CallToolResult, MemoriesOutput, error) {
		memories, err := store.List(ctx, in.Limit)
		if err != nil {
			return nil, MemoriesOutput{}, err
		}
		return textResult(summarise(memories)), MemoriesOutput{Memories: memories, Count: len(memories)}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "index",
		Description: "One-line index of ALL notes-to-self, newest first. Cheap enough to call once at the start of every session — do so, then use search() to pull the full text of anything relevant to the work at hand.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ IndexInput) (*mcp.CallToolResult, IndexOutput, error) {
		memories, err := store.List(ctx, indexCap)
		if err != nil {
			return nil, IndexOutput{}, err
		}
		total, err := store.Count(ctx)
		if err != nil {
			return nil, IndexOutput{}, err
		}
		entries := buildIndex(memories)
		note := ""
		if total > len(entries) {
			note = fmt.Sprintf("%d older memories not shown — search() reaches them.", total-len(entries))
		}
		return textResult(summariseIndex(entries, note)), IndexOutput{Memories: entries, Count: total, Note: note}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "update",
		Description: "Rewrite an existing memory in place, keeping its id and created_at (updated_at records the rewrite). Prefer this over delete + remember when correcting or consolidating a memory.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in UpdateInput) (*mcp.CallToolResult, UpdateOutput, error) {
		if in.Content == "" {
			return nil, UpdateOutput{}, fmt.Errorf("content is required")
		}
		m, err := store.Update(ctx, in.ID, in.Content)
		if err != nil {
			return nil, UpdateOutput{}, err
		}
		if m == nil {
			note := fmt.Sprintf("No memory with id %d — nothing was updated. Another session may have deleted or consolidated it. Re-run index() or search() to find the current copy.", in.ID)
			return textResult(note), UpdateOutput{Updated: false, ID: in.ID, Note: note}, nil
		}
		return textResult(fmt.Sprintf("Updated memory %d.", in.ID)), UpdateOutput{Updated: true, ID: in.ID, Memory: m}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "delete",
		Description: "Delete a note-to-self by id.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in DeleteInput) (*mcp.CallToolResult, DeleteOutput, error) {
		ok, err := store.Delete(ctx, in.ID)
		if err != nil {
			return nil, DeleteOutput{}, err
		}
		if !ok {
			note := fmt.Sprintf("No memory with id %d — nothing was deleted. Another session may have deleted or consolidated it. Re-run index() or search() to find the current copy.", in.ID)
			return textResult(note), DeleteOutput{Deleted: false, ID: in.ID, Note: note}, nil
		}
		return textResult(fmt.Sprintf("Deleted memory %d.", in.ID)), DeleteOutput{Deleted: true, ID: in.ID}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "dream",
		Description: "Return the 'dream mode' instructions — a housekeeping pass over the stored memories. Call this and then follow the returned instructions to tidy up duplicates, contradictions, and stale entries.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ DreamInput) (*mcp.CallToolResult, DreamOutput, error) {
		return textResult(dreamInstructions), DreamOutput{Instructions: dreamInstructions}, nil
	})
}

func summarise(memories []Memory) string {
	if len(memories) == 0 {
		return "No memories found."
	}
	out := fmt.Sprintf("Found %d memor%s:\n", len(memories), plural(len(memories)))
	for _, m := range memories {
		out += "  " + memoryLine(m) + "\n"
	}
	return out
}

// memoryLine renders one memory in the shared [id] content (dates) shape used
// by both the MCP text results and the CLI.
func memoryLine(m Memory) string {
	if m.UpdatedAt != "" {
		return fmt.Sprintf("[%d] %s  (%s, updated %s)", m.ID, m.Content, localTime(m.CreatedAt), localTime(m.UpdatedAt))
	}
	return fmt.Sprintf("[%d] %s  (%s)", m.ID, m.Content, localTime(m.CreatedAt))
}

// localTime converts a store timestamp (SQLite datetime('now') — UTC with no
// zone marker) to local time for display, so store output reconciles with
// sessions/extract output, which is local. Structured JSON payloads keep the
// raw stored value; only the human/model-facing text lines convert.
func localTime(s string) string {
	t, err := time.Parse("2006-01-02 15:04:05", s)
	if err != nil {
		return s
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

func summariseIndex(entries []IndexEntry, note string) string {
	if len(entries) == 0 {
		return "No memories yet."
	}
	out := fmt.Sprintf("Index of %d memor%s (newest first):\n", len(entries), plural(len(entries)))
	for _, e := range entries {
		out += fmt.Sprintf("  [%d] %s  (%s)\n", e.ID, e.Gist, localTime(e.CreatedAt))
	}
	if note != "" {
		out += note + "\n"
	}
	return out
}

func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}
}

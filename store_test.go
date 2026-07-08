package main

import (
	"context"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(":memory:")
	if err != nil {
		t.Fatalf("create test store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrationsApplyCleanly(t *testing.T) {
	s := newTestStore(t)
	var version int
	if err := s.db.QueryRowContext(context.Background(), "SELECT version FROM schema_version WHERE id = 1").Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if want := migrations[len(migrations)-1].version; version != want {
		t.Fatalf("schema version = %d, want %d", version, want)
	}
}

func TestAddAndGet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	m, err := s.Add(ctx, "user prefers British spelling")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if m.ID == 0 {
		t.Fatal("expected non-zero id")
	}
	if m.Content != "user prefers British spelling" {
		t.Errorf("content = %q, want British spelling memory", m.Content)
	}
	if m.CreatedAt == "" {
		t.Error("created_at should be populated")
	}

	got, err := s.Get(ctx, m.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got == nil || got.ID != m.ID {
		t.Fatalf("get returned %v, want memory %d", got, m.ID)
	}
}

func TestGetMissing(t *testing.T) {
	s := newTestStore(t)
	m, err := s.Get(context.Background(), 999)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if m != nil {
		t.Fatalf("expected nil for missing id, got %v", m)
	}
}

func TestSearchAndList(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	for _, c := range []string{"likes tea", "based in Glasgow", "prefers uv for python"} {
		if _, err := s.Add(ctx, c); err != nil {
			t.Fatalf("add %q: %v", c, err)
		}
	}

	all, err := s.List(ctx, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("list returned %d, want 3", len(all))
	}
	// Newest first.
	if all[0].Content != "prefers uv for python" {
		t.Errorf("first = %q, want newest", all[0].Content)
	}

	hits, err := s.Search(ctx, "Glasgow", 0)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 || hits[0].Content != "based in Glasgow" {
		t.Errorf("search hits = %+v, want single Glasgow memory", hits)
	}

	none, err := s.Search(ctx, "not there", 0)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("expected no hits, got %d", len(none))
	}
}

func TestSearchMatchesAllTokensInAnyOrder(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	// Tokens spread across different sentences — the query never appears verbatim.
	if _, err := s.Add(ctx, "Pressing Ctrl+O gets swallowed by the prompt. The AskUserQuestion answer never arrives."); err != nil {
		t.Fatalf("add: %v", err)
	}

	hits, err := s.Search(ctx, "AskUserQuestion Ctrl+O", 0)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("multi-word query should match tokens across sentences; got %d hits", len(hits))
	}

	none, err := s.Search(ctx, "AskUserQuestion trombone", 0)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("a query with an absent token should not match; got %d hits", len(none))
	}
}

func TestSearchNoHitsReturnsEmptyNotNil(t *testing.T) {
	s := newTestStore(t)
	hits, err := s.Search(context.Background(), "nothing here", 0)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if hits == nil {
		t.Fatal("zero hits should be an empty slice, not nil (nil marshals to JSON null)")
	}
}

func TestSearchIsCaseInsensitive(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	if _, err := s.Add(ctx, "based in Glasgow"); err != nil {
		t.Fatalf("add: %v", err)
	}
	hits, err := s.Search(ctx, "glasgow", 0)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("case-insensitive LIKE should match; got %d hits", len(hits))
	}
}

func TestSearchLimit(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if _, err := s.Add(ctx, "match me"); err != nil {
			t.Fatalf("add: %v", err)
		}
	}
	hits, err := s.Search(ctx, "match", 2)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) != 2 {
		t.Fatalf("with limit=2 got %d hits", len(hits))
	}
}

func TestCount(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	if n, err := s.Count(ctx); err != nil || n != 0 {
		t.Fatalf("empty count = %d (err %v), want 0", n, err)
	}

	for _, c := range []string{"one", "two", "three"} {
		if _, err := s.Add(ctx, c); err != nil {
			t.Fatalf("add: %v", err)
		}
	}

	n, err := s.Count(ctx)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 3 {
		t.Errorf("count = %d, want 3", n)
	}
}

func TestUpdate(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	m, err := s.Add(ctx, "original content")
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if m.UpdatedAt != "" {
		t.Errorf("fresh memory should have empty updated_at, got %q", m.UpdatedAt)
	}

	got, err := s.Update(ctx, m.ID, "rewritten content")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got == nil {
		t.Fatal("update returned nil for existing id")
	}
	if got.ID != m.ID {
		t.Errorf("id changed on update: %d -> %d", m.ID, got.ID)
	}
	if got.Content != "rewritten content" {
		t.Errorf("content = %q, want rewritten content", got.Content)
	}
	if got.CreatedAt != m.CreatedAt {
		t.Errorf("created_at changed on update: %q -> %q", m.CreatedAt, got.CreatedAt)
	}
	if got.UpdatedAt == "" {
		t.Error("updated_at should be set after update")
	}
}

func TestUpdateMissing(t *testing.T) {
	s := newTestStore(t)
	got, err := s.Update(context.Background(), 999, "whatever")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for missing id, got %+v", got)
	}
}

func TestDelete(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	m, err := s.Add(ctx, "forgettable fact")
	if err != nil {
		t.Fatalf("add: %v", err)
	}

	ok, err := s.Delete(ctx, m.ID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !ok {
		t.Error("delete returned false for existing id")
	}

	ok, err = s.Delete(ctx, m.ID)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if ok {
		t.Error("delete returned true for already-deleted id")
	}

	got, err := s.Get(ctx, m.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != nil {
		t.Errorf("expected deleted memory to be gone, got %+v", got)
	}
}

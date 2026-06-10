package tui

import (
	"testing"
)

// ── SelectItem tests ──────────────────────────────────────────────────────────

func TestSelectItem_Struct(t *testing.T) {
	item := SelectItem{Name: "test", Description: "desc", Recommended: true}
	if item.Name != "test" || item.Description != "desc" || !item.Recommended {
		t.Error("SelectItem field mismatch")
	}
}

// ── Remote-merge tests ────────────────────────────────────────────────────────

func TestSelectorModel_FilteredItemsMergesRemote(t *testing.T) {
	m := selectorModel{
		items: []SelectItem{
			{Name: "qwen2.5-coder"},
			{Name: "llama3.1"},
		},
		filter: "qwen",
		remoteItems: []SelectItem{
			{Name: "hf://unsloth/Qwen3-Coder-GGUF:Q4_K_M", Remote: true},
		},
	}
	got := m.filteredItems()
	if len(got) != 2 {
		t.Fatalf("filteredItems len = %d, want 2 (1 local + 1 remote)", len(got))
	}
	if got[0].Name != "qwen2.5-coder" {
		t.Errorf("local match should be first, got %q", got[0].Name)
	}
	if !got[1].Remote || got[1].Name != "hf://unsloth/Qwen3-Coder-GGUF:Q4_K_M" {
		t.Errorf("remote item not appended correctly: %+v", got[1])
	}
}

func TestSelectorModel_RemoteResultsMsgIgnoresStale(t *testing.T) {
	m := selectorModel{
		filter:   "qwen",
		queryGen: 5,
	}
	stale := remoteResultsMsg{gen: 3, query: "qwen", items: []SelectItem{{Name: "stale"}}}
	updated, _ := m.Update(stale)
	if rm := updated.(selectorModel).remoteItems; len(rm) != 0 {
		t.Errorf("stale msg should be dropped; got %+v", rm)
	}

	fresh := remoteResultsMsg{gen: 5, query: "qwen", items: []SelectItem{{Name: "fresh", Remote: true}}}
	updated, _ = m.Update(fresh)
	if rm := updated.(selectorModel).remoteItems; len(rm) != 1 || rm[0].Name != "fresh" {
		t.Errorf("fresh msg not applied; got %+v", rm)
	}
}

func TestSelectorModel_RemoteResultsMsgIgnoresFilterDrift(t *testing.T) {
	m := selectorModel{filter: "llama", queryGen: 7}
	msg := remoteResultsMsg{gen: 7, query: "qwen", items: []SelectItem{{Name: "x"}}}
	updated, _ := m.Update(msg)
	if rm := updated.(selectorModel).remoteItems; len(rm) != 0 {
		t.Errorf("filter-drift msg should be dropped; got %+v", rm)
	}
}

// ── ReorderItems tests ────────────────────────────────────────────────────────

func TestReorderItems_AllRecommended(t *testing.T) {
	items := []SelectItem{
		{Name: "a", Recommended: true},
		{Name: "b", Recommended: true},
	}
	result := ReorderItems(items)
	if len(result) != 2 || result[0].Name != "a" || result[1].Name != "b" {
		t.Errorf("order changed: %+v", result)
	}
}

func TestReorderItems_Mixed(t *testing.T) {
	items := []SelectItem{
		{Name: "one", Recommended: false},
		{Name: "two", Recommended: true},
		{Name: "three", Recommended: false},
		{Name: "four", Recommended: true},
	}
	result := ReorderItems(items)
	// Recommended first (two, four in order), then others (one, three).
	expected := []string{"two", "four", "one", "three"}
	for i, name := range expected {
		if result[i].Name != name {
			t.Errorf("pos %d: got %q, want %q", i, result[i].Name, name)
		}
	}
}

func TestReorderItems_NoneRecommended(t *testing.T) {
	items := []SelectItem{
		{Name: "x"},
		{Name: "y"},
	}
	result := ReorderItems(items)
	if result[0].Name != "x" || result[1].Name != "y" {
		t.Error("order changed for non-recommended items")
	}
}

func TestReorderItems_Empty(t *testing.T) {
	if result := ReorderItems(nil); len(result) != 0 {
		t.Error("expected empty")
	}
}

// ── cursorForCurrent tests ────────────────────────────────────────────────────

func TestCursorForCurrent_Found(t *testing.T) {
	items := []SelectItem{
		{Name: "alpha"},
		{Name: "beta"},
		{Name: "gamma"},
	}
	idx := cursorForCurrent(items, "beta")
	if idx != 1 {
		t.Errorf("got %d, want 1", idx)
	}
}

func TestCursorForCurrent_NotFound(t *testing.T) {
	items := []SelectItem{
		{Name: "alpha"},
	}
	idx := cursorForCurrent(items, "nope")
	if idx != 0 {
		t.Errorf("expected 0 for not found, got %d", idx)
	}
}

func TestCursorForCurrent_Empty(t *testing.T) {
	idx := cursorForCurrent(nil, "anything")
	if idx != 0 {
		t.Errorf("expected 0, got %d", idx)
	}
}

func TestCursorForCurrent_EmptyCurrent(t *testing.T) {
	items := []SelectItem{{Name: "only"}}
	idx := cursorForCurrent(items, "")
	if idx != 0 {
		t.Errorf("expected 0 for empty current, got %d", idx)
	}
}

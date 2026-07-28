package main

import (
	"slices"
	"testing"
)

// Seed state: todo=[1 "Learn htmx basics", 2 "Build kanban board"],
// doing=[3 "Explore hx-swap"], done=[4 "Setup Go project"].

func newTestStore() *Store {
	s := NewStore()
	s.Seed()
	return s
}

func titles(s *Store, col string) []string {
	var out []string
	for _, c := range s.CardsByColumn(col) {
		out = append(out, c.Title)
	}
	return out
}

func TestMoveCardRight(t *testing.T) {
	s := newTestStore()
	card, oldCol, ok := s.MoveCard(1, "right")
	if !ok || oldCol != "todo" || card.Column != "doing" {
		t.Fatalf("got card=%+v oldCol=%q ok=%v", card, oldCol, ok)
	}
	if !slices.Equal(titles(s, "todo"), []string{"Build kanban board"}) {
		t.Errorf("todo = %v", titles(s, "todo"))
	}
	if !slices.Contains(titles(s, "doing"), "Learn htmx basics") {
		t.Errorf("doing = %v", titles(s, "doing"))
	}
}

func TestMoveCardLeft(t *testing.T) {
	s := newTestStore()
	_, oldCol, ok := s.MoveCard(3, "left")
	if !ok || oldCol != "doing" {
		t.Fatalf("oldCol=%q ok=%v", oldCol, ok)
	}
	if !slices.Contains(titles(s, "todo"), "Explore hx-swap") {
		t.Errorf("todo = %v", titles(s, "todo"))
	}
}

func TestMoveCardBoundaries(t *testing.T) {
	s := newTestStore()
	if _, _, ok := s.MoveCard(1, "left"); ok {
		t.Error("moving left from todo should fail")
	}
	if _, _, ok := s.MoveCard(4, "right"); ok {
		t.Error("moving right from done should fail")
	}
}

func TestMoveCardNotFound(t *testing.T) {
	s := newTestStore()
	if _, _, ok := s.MoveCard(999, "right"); ok {
		t.Error("moving a missing card should fail")
	}
}

func TestDropCardCrossColumn(t *testing.T) {
	s := newTestStore()
	_, oldCol, ok := s.DropCard(1, "doing", 0)
	if !ok || oldCol != "todo" {
		t.Fatalf("oldCol=%q ok=%v", oldCol, ok)
	}
	// inserted at index 0 of doing → before "Explore hx-swap"
	if !slices.Equal(titles(s, "doing"), []string{"Learn htmx basics", "Explore hx-swap"}) {
		t.Errorf("doing = %v", titles(s, "doing"))
	}
}

func TestDropCardReorderSameColumn(t *testing.T) {
	s := newTestStore()
	// move card 1 to the bottom of todo → [2, 1]
	if _, _, ok := s.DropCard(1, "todo", 1); !ok {
		t.Fatal("reorder failed")
	}
	if !slices.Equal(titles(s, "todo"), []string{"Build kanban board", "Learn htmx basics"}) {
		t.Errorf("todo = %v", titles(s, "todo"))
	}
}

func TestDropCardMiddleIndex(t *testing.T) {
	s := newTestStore()
	// doing=[3]; drop 4 at index 1 → after 3 → [3, 4]
	if _, _, ok := s.DropCard(4, "doing", 1); !ok {
		t.Fatal("drop failed")
	}
	if !slices.Equal(titles(s, "doing"), []string{"Explore hx-swap", "Setup Go project"}) {
		t.Errorf("doing = %v", titles(s, "doing"))
	}
}

func TestDropCardOutOfRangeAppends(t *testing.T) {
	s := newTestStore()
	if _, _, ok := s.DropCard(1, "doing", 99); !ok {
		t.Fatal("drop failed")
	}
	if !slices.Equal(titles(s, "doing"), []string{"Explore hx-swap", "Learn htmx basics"}) {
		t.Errorf("doing = %v", titles(s, "doing"))
	}
}

func TestDropCardNotFound(t *testing.T) {
	s := newTestStore()
	if _, _, ok := s.DropCard(999, "todo", 0); ok {
		t.Error("dropping a missing card should fail")
	}
}

func TestSearchCardsCaseInsensitive(t *testing.T) {
	s := newTestStore()
	if got := s.SearchCards("HTMX", "todo"); len(got) != 1 || got[0].ID != 1 {
		t.Errorf("search HTMX in todo = %+v", got)
	}
	if got := s.SearchCards("zzz", "todo"); len(got) != 0 {
		t.Errorf("search zzz = %+v", got)
	}
}

func TestCounts(t *testing.T) {
	s := newTestStore()
	counts := s.Counts()
	if counts["todo"] != 2 || counts["doing"] != 1 || counts["done"] != 1 {
		t.Errorf("counts = %v", counts)
	}
}

func TestAddUpdateDelete(t *testing.T) {
	s := newTestStore()
	c := s.AddCard("New", "todo")
	if c.ID == 0 || c.Column != "todo" {
		t.Fatalf("added = %+v", c)
	}
	if _, ok := s.UpdateCard(c.ID, "Renamed"); !ok {
		t.Fatal("update failed")
	}
	if got, _ := s.GetCard(c.ID); got.Title != "Renamed" {
		t.Errorf("title = %q", got.Title)
	}
	if !s.DeleteCard(c.ID) {
		t.Fatal("delete failed")
	}
	if _, ok := s.GetCard(c.ID); ok {
		t.Error("card still present after delete")
	}
}

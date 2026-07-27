package main

import (
	"bytes"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"sync"

	"github.com/go-chi/chi/v5"
)

type Card struct {
	ID     int
	Title  string
	Column string
}

type Store struct {
	mu     sync.RWMutex
	cards  []Card
	nextID int
}

func NewStore() *Store {
	return &Store{nextID: 1}
}

func (s *Store) Seed() {
	s.cards = []Card{
		{ID: s.nextID, Title: "Learn htmx basics", Column: "todo"},
	}
	s.nextID++
	s.cards = append(s.cards, Card{ID: s.nextID, Title: "Build kanban board", Column: "todo"})
	s.nextID++
	s.cards = append(s.cards, Card{ID: s.nextID, Title: "Explore hx-swap", Column: "doing"})
	s.nextID++
	s.cards = append(s.cards, Card{ID: s.nextID, Title: "Setup Go project", Column: "done"})
	s.nextID++
}

func (s *Store) CardsByColumn(col string) []Card {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []Card
	for _, c := range s.cards {
		if c.Column == col {
			result = append(result, c)
		}
	}
	return result
}

func (s *Store) GetCard(id int) (Card, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.cards {
		if c.ID == id {
			return c, true
		}
	}
	return Card{}, false
}

func (s *Store) AddCard(title, col string) Card {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := Card{ID: s.nextID, Title: title, Column: col}
	s.nextID++
	s.cards = append(s.cards, c)
	return c
}

func (s *Store) DeleteCard(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range s.cards {
		if c.ID == id {
			s.cards = append(s.cards[:i], s.cards[i+1:]...)
			return true
		}
	}
	return false
}

func (s *Store) UpdateCard(id int, title string) (Card, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, c := range s.cards {
		if c.ID == id {
			s.cards[i].Title = title
			return s.cards[i], true
		}
	}
	return Card{}, false
}

func (s *Store) MoveCard(id int, dir string) (Card, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	order := []string{"todo", "doing", "done"}
	for i, c := range s.cards {
		if c.ID == id {
			cur := 0
			for j, col := range order {
				if col == c.Column {
					cur = j
					break
				}
			}
			if dir == "right" && cur < len(order)-1 {
				cur++
			} else if dir == "left" && cur > 0 {
				cur--
			} else {
				return c, c.Column, false
			}
			oldCol := s.cards[i].Column
			s.cards[i].Column = order[cur]
			return s.cards[i], oldCol, true
		}
	}
	return Card{}, "", false
}

func (s *Store) Counts() map[string]int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	counts := map[string]int{"todo": 0, "doing": 0, "done": 0}
	for _, c := range s.cards {
		counts[c.Column]++
	}
	return counts
}

var columnTitles = map[string]string{
	"todo":  "Todo",
	"doing": "Doing",
	"done":  "Done",
}

var (
	store     = NewStore()
	templates *template.Template
)

type ColumnData struct {
	ID    string
	Title string
	Cards []Card
}

type PageData struct {
	Columns []ColumnData
	Stats   StatsData
}

type StatsData struct {
	Todo    int
	Doing   int
	Done    int
	Total   int
	Percent int
}

func getStats() StatsData {
	counts := store.Counts()
	total := counts["todo"] + counts["doing"] + counts["done"]
	pct := 0
	if total > 0 {
		pct = counts["done"] * 100 / total
	}
	return StatsData{
		Todo:    counts["todo"],
		Doing:   counts["doing"],
		Done:    counts["done"],
		Total:   total,
		Percent: pct,
	}
}

func renderPage(w http.ResponseWriter) {
	data := PageData{
		Columns: []ColumnData{
			{ID: "todo", Title: "Todo", Cards: store.CardsByColumn("todo")},
			{ID: "doing", Title: "Doing", Cards: store.CardsByColumn("doing")},
			{ID: "done", Title: "Done", Cards: store.CardsByColumn("done")},
		},
		Stats: getStats(),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "layout.html", data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func renderColumn(w http.ResponseWriter, col string) {
	data := ColumnData{
		ID:    col,
		Title: columnTitles[col],
		Cards: store.CardsByColumn(col),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "column", data); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func renderCard(w http.ResponseWriter, card Card) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "card", card); err != nil {
		http.Error(w, err.Error(), 500)
	}
}

func handleCreateCard(w http.ResponseWriter, r *http.Request) {
	title := r.FormValue("title")
	if title == "" {
		title = "Untitled"
	}
	card := store.AddCard(title, "todo")
	renderCard(w, card)
}

func handleDeleteCard(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	store.DeleteCard(id)
	w.WriteHeader(200)
}

func handleGetColumn(w http.ResponseWriter, r *http.Request) {
	col := chi.URLParam(r, "id")
	if _, ok := columnTitles[col]; !ok {
		http.Error(w, "not found", 404)
		return
	}
	renderColumn(w, col)
}

func handleGetCard(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	card, ok := store.GetCard(id)
	if !ok {
		http.Error(w, "not found", 404)
		return
	}
	renderCard(w, card)
}

func handleEditCard(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	card, ok := store.GetCard(id)
	if !ok {
		http.Error(w, "not found", 404)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	templates.ExecuteTemplate(w, "card_edit", card)
}

func handleUpdateCard(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	title := r.FormValue("title")
	if title == "" {
		card, _ := store.GetCard(id)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(422)
		templates.ExecuteTemplate(w, "card_edit_error", card)
		return
	}
	card, ok := store.UpdateCard(id, title)
	if !ok {
		http.Error(w, "not found", 404)
		return
	}
	renderCard(w, card)
}

func handleMoveCard(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	dir := r.URL.Query().Get("dir")
	card, oldCol, ok := store.MoveCard(id, dir)
	if !ok {
		http.Error(w, "cannot move", 400)
		return
	}

	var buf bytes.Buffer

	srcCol := ColumnData{ID: oldCol, Title: columnTitles[oldCol], Cards: store.CardsByColumn(oldCol)}
	templates.ExecuteTemplate(&buf, "column", srcCol)

	dstCol := ColumnData{ID: card.Column, Title: columnTitles[card.Column], Cards: store.CardsByColumn(card.Column)}
	buf.WriteString(`<div hx-swap-oob="outerHTML" id="col-` + card.Column + `">`)
	templates.ExecuteTemplate(&buf, "column_inner", dstCol)
	buf.WriteString(`</div>`)

	stats := getStats()
	buf.WriteString(`<div hx-swap-oob="outerHTML" id="stats">`)
	templates.ExecuteTemplate(&buf, "stats_inner", stats)
	buf.WriteString(`</div>`)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(buf.Bytes())
}

func main() {
	store.Seed()
	templates = template.Must(template.ParseGlob("templates/*.html"))
	templates = template.Must(templates.ParseGlob("templates/partials/*.html"))

	r := chi.NewRouter()
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		renderPage(w)
	})
	r.Get("/columns/{id}", handleGetColumn)
	r.Post("/cards", handleCreateCard)
	r.Delete("/cards/{id}", handleDeleteCard)
	r.Get("/cards/{id}", handleGetCard)
	r.Get("/cards/{id}/edit", handleEditCard)
	r.Patch("/cards/{id}", handleUpdateCard)
	r.Post("/cards/{id}/move", handleMoveCard)

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

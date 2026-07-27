package main

import (
	"html/template"
	"log"
	"net/http"
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
}

func renderPage(w http.ResponseWriter) {
	data := PageData{
		Columns: []ColumnData{
			{ID: "todo", Title: "Todo", Cards: store.CardsByColumn("todo")},
			{ID: "doing", Title: "Doing", Cards: store.CardsByColumn("doing")},
			{ID: "done", Title: "Done", Cards: store.CardsByColumn("done")},
		},
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, "layout.html", data); err != nil {
		http.Error(w, err.Error(), 500)
	}
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

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

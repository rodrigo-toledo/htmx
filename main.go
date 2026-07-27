package main

import (
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

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

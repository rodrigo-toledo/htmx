package main

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

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

func (s *Store) SearchCards(query, col string) []Card {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []Card
	q := strings.ToLower(query)
	for _, c := range s.cards {
		if c.Column == col && strings.Contains(strings.ToLower(c.Title), q) {
			result = append(result, c)
		}
	}
	return result
}

var columnTitles = map[string]string{
	"todo":  "Todo",
	"doing": "Doing",
	"done":  "Done",
}

type sseMessage struct {
	id   int
	data string
}

// frame renders the message in SSE wire format. The id line is what lets a
// reconnecting client resume: the hx-sse extension sends Last-Event-ID and we
// replay everything it missed.
func (m sseMessage) frame() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "id: %d\n", m.id)
	for _, line := range strings.Split(m.data, "\n") {
		sb.WriteString("data: ")
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
	return sb.String()
}

type SSEHub struct {
	mu      sync.Mutex
	clients map[chan sseMessage]bool
	history []sseMessage
	nextID  int
}

const historyLimit = 100

func NewSSEHub() *SSEHub {
	return &SSEHub{clients: make(map[chan sseMessage]bool)}
}

// Subscribe registers a client and, atomically, returns every buffered
// message after lastID. Doing both under one lock makes catch-up gap-free:
// broadcasts before the lock are in history; broadcasts after go to the
// channel; nothing falls between.
func (h *SSEHub) Subscribe(lastID int) (chan sseMessage, []sseMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan sseMessage, 64)
	h.clients[ch] = true
	var missed []sseMessage
	for _, m := range h.history {
		if m.id > lastID {
			missed = append(missed, m)
		}
	}
	return ch, missed
}

func (h *SSEHub) Unsubscribe(ch chan sseMessage) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, ch)
	close(ch)
}

// Broadcast sends one unnamed SSE message to every connected client and
// appends it to the replay buffer. Unnamed messages are what htmx 4's hx-sse
// extension swaps; the targeting lives in the payload itself (hx-partial /
// hx-swap-oob fragments).
func (h *SSEHub) Broadcast(data string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextID++
	msg := sseMessage{id: h.nextID, data: data}
	h.history = append(h.history, msg)
	if len(h.history) > historyLimit {
		h.history = h.history[len(h.history)-historyLimit:]
	}
	for ch := range h.clients {
		select {
		case ch <- msg:
		default:
			// slow client drops live messages; it catches up via replay
			// the next time its connection resets
		}
	}
}

var hub = NewSSEHub()

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

// broadcastActivity pushes one unnamed SSE message to every connected tab:
// an <hx-partial> that appends a feed item, plus an OOB innerMorph fragment
// that refreshes the stats bar. htmx extracts both and swaps them into their
// targets; nothing else on the page is touched.
func broadcastActivity(action string) {
	var buf bytes.Buffer
	buf.WriteString(`<hx-partial hx-target="#feed-items" hx-swap="beforeend"><div class="feed-item">`)
	buf.WriteString(template.HTMLEscapeString(action))
	buf.WriteString(`</div></hx-partial>`)
	templates.ExecuteTemplate(&buf, "stats_sse", getStats())
	hub.Broadcast(buf.String())
}

func columnData(col string) ColumnData {
	return ColumnData{ID: col, Title: columnTitles[col], Cards: store.CardsByColumn(col)}
}

func handleCreateCard(w http.ResponseWriter, r *http.Request) {
	title := r.FormValue("title")
	if title == "" {
		title = "Untitled"
	}
	card := store.AddCard(title, "todo")
	broadcastActivity(fmt.Sprintf(`Created "%s" in Todo`, card.Title))

	var buf bytes.Buffer
	templates.ExecuteTemplate(&buf, "card", card)
	templates.ExecuteTemplate(&buf, "count_oob", columnData("todo"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(buf.Bytes())
}

func handleDeleteCard(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	card, ok := store.GetCard(id)
	if !ok {
		w.WriteHeader(204)
		return
	}
	col := card.Column
	store.DeleteCard(id)
	broadcastActivity(fmt.Sprintf(`Deleted "%s"`, card.Title))

	// The card itself is removed client-side via hx-swap="delete"; the
	// response carries only the OOB count refresh for its column.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	templates.ExecuteTemplate(w, "count_oob", columnData(col))
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
	broadcastActivity(fmt.Sprintf(`Edited card to "%s"`, card.Title))
	renderCard(w, card)
}

func handleMoveCard(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	dir := r.URL.Query().Get("dir")
	card, oldCol, ok := store.MoveCard(id, dir)
	if !ok {
		// 204 never swaps, so a stray boundary request can't clobber the column
		w.WriteHeader(204)
		return
	}

	broadcastActivity(fmt.Sprintf(`Moved "%s" from %s to %s`, card.Title, columnTitles[oldCol], columnTitles[card.Column]))

	var buf bytes.Buffer
	templates.ExecuteTemplate(&buf, "column", columnData(oldCol))
	templates.ExecuteTemplate(&buf, "column_oob", columnData(card.Column))
	templates.ExecuteTemplate(&buf, "stats_oob", getStats())

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(buf.Bytes())
}

func handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Flush a comment immediately so response headers reach the client now,
	// not at the first broadcast. Without this, Go buffers the response and
	// htmx never sees the stream until some mutation happens.
	fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	// Reconnecting clients (the hx-sse extension does this automatically)
	// tell us the last message they saw; replay what they missed.
	lastID := 0
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		lastID, _ = strconv.Atoi(v)
	}
	ch, missed := hub.Subscribe(lastID)
	defer hub.Unsubscribe(ch)

	for _, m := range missed {
		fmt.Fprint(w, m.frame())
	}
	flusher.Flush()

	// Heartbeat keeps intermediaries from killing the idle connection and
	// surfaces dead clients promptly. Comments are ignored by SSE clients.
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case msg := <-ch:
			fmt.Fprint(w, msg.frame())
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func handleSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	cols := []ColumnData{
		{ID: "todo", Title: "Todo", Cards: store.SearchCards(q, "todo")},
		{ID: "doing", Title: "Doing", Cards: store.SearchCards(q, "doing")},
		{ID: "done", Title: "Done", Cards: store.SearchCards(q, "done")},
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	for _, col := range cols {
		templates.ExecuteTemplate(w, "column", col)
	}
}

func handleMoveCardPartial(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(chi.URLParam(r, "id"))
	dir := r.URL.Query().Get("dir")
	card, oldCol, ok := store.MoveCard(id, dir)
	if !ok {
		w.WriteHeader(204)
		return
	}

	broadcastActivity(fmt.Sprintf(`Moved "%s" from %s to %s`, card.Title, columnTitles[oldCol], columnTitles[card.Column]))

	var buf bytes.Buffer
	templates.ExecuteTemplate(&buf, "column", columnData(oldCol))

	buf.WriteString(`<hx-partial hx-target="#col-` + card.Column + `" hx-swap="outerHTML">`)
	templates.ExecuteTemplate(&buf, "column", columnData(card.Column))
	buf.WriteString(`</hx-partial>`)

	buf.WriteString(`<hx-partial hx-target="#stats" hx-swap="outerHTML">`)
	templates.ExecuteTemplate(&buf, "stats", getStats())
	buf.WriteString(`</hx-partial>`)

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
	r.Post("/cards/{id}/move-partial", handleMoveCardPartial)
	r.Get("/events", handleEvents)
	r.Get("/search", handleSearch)

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}

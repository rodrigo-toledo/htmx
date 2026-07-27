# htmx 4.x Kanban Board — Learning Project

## Objective

Build the smallest possible app that demonstrates htmx 4's core concepts:
list views, partial-state action buttons, inline editing, multi-target
updates from a single response, and real-time push — using htmx as the
primary (and initially only) client-side tool.

## Domain: Personal Kanban Board

Three columns (Todo → Doing → Done) with cards you can create, move,
inline-edit, and delete. An activity feed and stats bar update live.

Why this domain:
- Every action maps cleanly to a server-rendered HTML fragment
- Moving a card naturally requires updating multiple page regions
- Real-time collaboration (two tabs) is organic, not forced
- Small enough to hold in one head, rich enough to hit all five concepts

## Stack

| Layer     | Choice                        | Rationale                              |
|-----------|-------------------------------|----------------------------------------|
| Backend   | Go + chi                      | Single binary, stdlib SSE via Flusher  |
| Templates | html/template (server-side)   | htmx expects HTML responses            |
| Frontend  | htmx 4.0.0-beta6 (CDN)       | Learning target                        |
| Real-time | hx-sse extension              | Idiomatic htmx push, no polling        |
| VCS       | jujutsu (jj) → GitHub         | Trunk-based, commit per stage          |

## Architecture

```
htmx-kanban/
├── go.mod
├── main.go                  # store, routes, SSE hub, HTML rendering
├── templates/
│   ├── layout.html          # base page, loads htmx + hx-sse
│   ├── index.html           # board: columns, stats, activity feed
│   └── partials/
│       ├── card.html        # card view mode
│       ├── card_edit.html   # card edit form
│       ├── column.html      # column + its cards
│       ├── stats.html       # counts + progress bar
│       └── activity.html    # single feed entry
├── static/
│   └── app.css
└── PLAN.md                  # this file
```

## Routes

| Method  | Path                | Returns                          |
|---------|---------------------|----------------------------------|
| GET     | /                   | Full page (layout + index)       |
| GET     | /columns/{id}       | Column partial                   |
| GET     | /cards/{id}         | Card partial (view)              |
| GET     | /cards/{id}/edit    | Card partial (edit form)         |
| POST    | /cards              | New card partial                 |
| PATCH   | /cards/{id}         | Updated card partial             |
| DELETE  | /cards/{id}         | Empty (hx-swap="delete")         |
| POST    | /cards/{id}/move    | Multi-target response (OOB)      |
| GET     | /events             | SSE stream                       |

## htmx Concept Coverage

| Concept                          | Where demonstrated                     |
|----------------------------------|----------------------------------------|
| AJAX from any element            | Move/delete buttons (not just forms)   |
| hx-target / hx-swap              | Card → outerHTML, list → beforeend     |
| Inline editing (click-to-edit)   | Edit button swaps card → form → card   |
| Multi-target update (OOB)        | Move updates 2 columns + stats         |
| `<hx-partial>` (v4 new)         | Alternative multi-target mechanism     |
| hx-trigger modifiers             | Search: `input delay:300ms changed`    |
| hx-confirm                       | Delete confirmation                    |
| Request indicators               | htmx-indicator spinner on buttons      |
| SSE real-time                    | Activity feed + live stats via hx-sse  |
| v4 error swaps (422)            | Validation re-renders form with error  |
| hx-swap="delete" (v4 new)       | Card removal without response body     |
| Explicit inheritance (v4)       | hx-target:inherited on column wrapper  |

## Alpine.js Second Pass (future)

After the pure-htmx version is complete, a second branch will add Alpine
to demonstrate where client-side state earns its keep:
- Optimistic UI during in-flight moves
- Transient dropdown/popover state without round-trips
- x-transition for smoother animations than CSS-only

---

## Implementation Stages

Each stage is self-contained: the app runs and can be manually tested
after each one. Each stage = one jj commit + push.

### Stage 0: Project plan (this file)
- [x] Write PLAN.md
- Test: file exists, jj commit succeeds

### Stage 1: Skeleton server + full-page render
- go.mod, main.go with chi, in-memory store (slice of cards)
- GET / renders layout + index with 3 hardcoded columns
- static/app.css with minimal board styling
- Templates: layout.html, index.html
- Test: `go run .` → open localhost:8080 → see 3 empty columns

### Stage 2: Card CRUD (list view + create + delete)
- Seed store with sample cards
- Partials: card.html, column.html
- GET /columns/{id} renders column with cards
- POST /cards creates card (form at top of Todo column)
- DELETE /cards/{id} removes card (hx-swap="delete")
- hx-confirm on delete button
- Test: create a card, see it appear; delete it, confirm dialog, gone

### Stage 3: Inline editing
- Partials: card_edit.html
- GET /cards/{id} returns card view partial
- GET /cards/{id}/edit returns edit form
- PATCH /cards/{id} saves and returns view partial
- Escape key cancels (hx-trigger="keyup[key=='Escape']")
- Validation: empty title → 422 + form re-rendered with error
- Test: click edit → form appears → save → view returns; empty → error

### Stage 4: Move + multi-target updates (OOB)
- POST /cards/{id}/move?dir=left|right
- Response: moved card's new column (main swap) + source column
  (hx-swap-oob) + stats bar (hx-swap-oob)
- Stats partial: templates/partials/stats.html
- ◀ ▶ buttons on cards
- Test: move card → both columns re-render, stats update, one request

### Stage 5: SSE real-time (activity feed + live stats)
- GET /events SSE endpoint with broadcast hub
- Every mutation broadcasts an "activity" event
- Activity feed element: hx-sse:swap="activity" appends entries
- Stats element also listens and re-renders on "stats" event
- Open two tabs: act in one, other updates live
- Test: two tabs, move card in tab A → tab B feed + stats update

### Stage 6: Polish + v4 feature showcase
- hx-partial alternative for move (commented toggle)
- hx-target:inherited on column wrappers
- Active search box (hx-trigger="input delay:300ms changed")
- Request indicators (spinner on move buttons)
- hx-swap="innerMorph" on stats for smooth count transitions
- Test: all features exercisable, README with run instructions

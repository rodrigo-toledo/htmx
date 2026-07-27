# htmx 4.x Kanban Board

A minimal kanban board built to learn htmx 4 core concepts.

## Run

```bash
go run .
# open http://localhost:8080
```

## What it demonstrates

| htmx concept | Where |
|---|---|
| AJAX from any element | Move/delete buttons (`hx-post`, `hx-delete`) |
| `hx-target` / `hx-swap` | Card swaps (`outerHTML`), list append (`beforeend`), removal (`delete`) |
| Inline editing | Edit button → form → save/cancel/Escape |
| Multi-target (OOB) | Move updates source column + dest column + stats in one response |
| `<hx-partial>` (v4) | Alternative move endpoint: `POST /cards/{id}/move-partial` |
| `hx-trigger` modifiers | Search: `input delay:300ms changed` |
| `hx-confirm` | Delete confirmation dialog |
| Request indicators | `htmx-indicator` spinner on move + search |
| SSE real-time | Activity feed + live stats via `hx-sse:connect` |
| v4 error swaps | Empty title → 422 + form re-rendered with error |
| `hx-swap="innerMorph"` | Stats bar morphs smoothly on SSE update |
| Explicit inheritance | `hx-target:inherited` available on column wrappers |

## Comparing OOB vs `<hx-partial>`

Both endpoints do the same thing (move a card, update 3 regions):

- `POST /cards/{id}/move` — classic `hx-swap-oob` (OOB fragments in response)
- `POST /cards/{id}/move-partial` — v4 `<hx-partial>` (explicit target+swap per fragment)

To switch the UI to `<hx-partial>`, change the move buttons in
`templates/partials/card.html` from `/move` to `/move-partial`.

## Architecture

- **Go + chi** — single `main.go`, in-memory store, SSE via `http.Flusher`
- **html/template** — server-rendered HTML partials
- **htmx 4.0.0-beta6** — loaded via CDN with `hx-sse` extension
- **No build step** — no npm, no bundler, no JS framework

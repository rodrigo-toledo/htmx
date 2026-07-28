# htmx 4.x Kanban Board

A minimal kanban board built to learn htmx 4 core concepts.
See [GUIDE.md](GUIDE.md) for a feature-by-feature walkthrough of everything
demonstrated here, and [PLAN.md](PLAN.md) for the staged build plan.

## Run

```bash
make run        # or: go run .
# open http://localhost:8080 in two tabs to see the real-time sync
```

## Commands

A thin `Makefile` wraps the common tasks (run `make` to list them all):

```bash
make run     # start the server
make test    # Go unit + handler tests
make cover   # tests with a coverage report
make e2e     # browser end-to-end suites (auto-installs JS deps once)
make check   # full gate: vet + test + e2e
```

## What it demonstrates

| htmx concept | Where |
|---|---|
| AJAX from any element | Move/delete buttons (`hx-post`, `hx-delete`) |
| `hx-target` / `hx-swap` | Card swaps (`outerHTML`), column swaps by id, removal via column re-render |
| Inline editing | Edit button → form → save/cancel/Escape |
| Multi-target (OOB) | Move updates source column + dest column + stats in one response |
| `<hx-partial>` (v4) | Alternative move endpoint + SSE payloads |
| `hx-trigger` modifiers | Search: `input delay:300ms changed` |
| `hx-confirm` | Delete confirmation dialog |
| Request indicators | `htmx-indicator` spinner on move + search |
| SSE real-time (v4 model) | Unnamed messages carrying `hx-partial`/OOB fragments sync columns, counts, stats and feed across all tabs |
| Drag & drop | SortableJS owns the drag, htmx persists the drop (`POST /cards/{id}/drop`); re-init glue on `htmx:after:settle` |
| Alpine.js (transient UI) | Delete-confirmation modal: `x-data`/`x-show`/`x-transition` own the dialog, `htmx.ajax()` does the delete; `hx-alpine-compat` binds swapped-in nodes |
| v4 error swaps | Empty title → 422 + form re-rendered with error |
| `hx-swap="innerMorph"` | Stats bar morphs smoothly, preserving the progress-bar transition |
| `htmx-config` meta | `sse.pauseOnBackground:false` keeps background tabs live |

## Comparing OOB vs `<hx-partial>`

Both endpoints do the same thing (move a card, update 3 regions):

- `POST /cards/{id}/move` — classic `hx-swap-oob` (OOB fragments in response)
- `POST /cards/{id}/move-partial` — v4 `<hx-partial>` (explicit target+swap per fragment)

To switch the UI to `<hx-partial>`, change the move buttons in
`templates/partials/card.html` from `/move` to `/move-partial`.

## Tests

Two layers, a fast inner loop and a slow real-browser one (both wrapped by
`make`, or run directly):

```bash
make test            # == go test ./...        (~0.5s, no browser)
make e2e             # == ./e2e/run.sh         (drives real Chrome)
./e2e/run.sh replay  # run a single e2e suite
```

**Go (`go test ./...`)**
- `store_test.go` — pure unit tests for the store: move boundaries, drop
  index math (cross-column, reorder, out-of-range), search, counts, CRUD
- `handlers_test.go` — `httptest` tests of the hypermedia contract: each
  endpoint returns the right HTML fragments, ids, `hx-swap-oob` attributes
  and status codes; plus the SSE wire format (immediate header flush,
  `id:`/`data:` frames on broadcast)

**End-to-end (`./e2e/run.sh`)** — puppeteer drives two real Chrome tabs;
each suite runs against a fresh server instance:
- `test.mjs` — move, edit, 422 validation, create/delete counts, search,
  cross-tab SSE, console hygiene (32 checks)
- `replay.mjs` — kills one tab's stream, mutates from the other, and asserts
  the dropped tab catches up via `Last-Event-ID` replay — exactly once
- `dnd.mjs` — real simulated mouse drags: cross-column drop, same-column
  reorder, cross-tab sync, and persistence across a fresh page load
- `modal.mjs` — the Alpine delete modal: open, Escape / click-outside /
  confirm, cross-tab delete sync, and Alpine binding on a swapped-in card

## Architecture

- **Go + chi** — single `main.go`, in-memory store, SSE via `http.Flusher`
- **html/template** — server-rendered HTML partials
- **htmx 4.0.0-beta6** — loaded via CDN with the `hx-sse` and `hx-alpine-compat` extensions
- **SortableJS** — drag-and-drop mechanics; **Alpine.js** — transient UI (the delete modal)
- **No build step** — everything loads from CDN; no npm/bundler for the app (the `e2e/` dir is dev tooling only)

# htmx Feature Guide — applied to the Kanban board

A field guide to the htmx features this project uses, plus everything new
in v4. Every example is lifted straight from the codebase, so you can open the
referenced file and see it in context.

---

## 0. The mental model (read this first)

HTML has exactly two elements that talk to a server: `<a>` (GET, replaces the
window) and `<form>` (POST, replaces the window). htmx's entire pitch is:

> **What if *any* element could issue *any* HTTP request, on *any* event, and
> put the HTML response *anywhere* in the DOM?**

That's it. Four constraints removed. Everything else — `hx-target`, `hx-swap`,
`hx-trigger`, OOB, SSE — is just spelling out those four answers.

The server stays in charge. It returns **HTML, not JSON**. The client never
templates anything; it just moves server-rendered fragments around. This is
[HATEOAS](https://en.wikipedia.org/wiki/HATEOAS): the hypermedia itself drives
the application. Keep this in mind and every attribute below becomes obvious.

The four questions every `hx-*` attribute answers:

| Question | Attribute | Example in our app |
|---|---|---|
| **When?** | `hx-trigger` | `input delay:300ms changed` (search) |
| **What request?** | `hx-get/post/put/patch/delete` | `hx-post="/cards/1/move?dir=right"` |
| **Where does the response go?** | `hx-target` | `closest .column` |
| **How is it placed?** | `hx-swap` | `outerHTML` |

---

## 1. Core features

### 1.1 AJAX from any element

A `<button>` is not a hyperlink and not a form, yet here it issues a POST:

```html
<!-- templates/partials/card.html -->
<button hx-post="/cards/{{.ID}}/move?dir=right" ...>▶</button>
```

No `addEventListener`, no `fetch`, no handler registration. The attribute *is*
the behavior. Because the button lives inside the card, htmx's default target is
the element itself and the default swap is `innerHTML` — we override both below.

### 1.2 `hx-target` — where the response lands

```html
hx-target="closest .column"     <!-- move button: nearest ancestor column -->
hx-target="#card-{{.ID}}"       <!-- edit button: the card by id -->
hx-target="#col-todo .cards"    <!-- create form: the Todo card list -->
```

`closest`, `find`, `next`, `previous`, and plain CSS selectors all work.
`closest .column` is the elegant one: the button doesn't need to know which
column it's in — it just says "the column I live in."

### 1.3 `hx-swap` — how the response is placed

| Value | Meaning | Used for |
|---|---|---|
| `innerHTML` (default) | replace children | — |
| `outerHTML` | replace the element itself | card ↔ edit form, columns |
| `beforeend` | append as last child | new card into list |
| `delete` *(v4)* | remove the element, ignore response | deleting a card |
| `innerMorph` *(v4)* | morph children, preserving state | stats bar |

The click-to-edit pattern is just two `outerHTML` swaps in opposite directions:

```html
<!-- view → form -->
<button hx-get="/cards/1/edit" hx-target="#card-1" hx-swap="outerHTML">Edit</button>

<!-- form → view (inside card_edit.html) -->
<form hx-patch="/cards/1" hx-target="#card-1" hx-swap="outerHTML">
```

The server decides which of the two shapes to return. The client has no idea
it's "editing" — it's just swapping HTML.

### 1.4 `hx-trigger` — when, and with what temperament

The search box is the best demo. One attribute expresses debounce, dedupe, and
event type:

```html
<!-- templates/index.html -->
<input type="search" name="q"
       hx-get="/search"
       hx-trigger="input delay:300ms changed"
       hx-target="#board" hx-swap="innerHTML">
```

Read it as English: *"on input, wait 300ms (resetting if they keep typing), and
only if the value actually changed."* That's a debounce function, a diff check,
and an event binding — zero JavaScript.

Other modifiers worth knowing: `once`, `throttle:1s`, `from:body`, and event
filters like `keyup[key=='Escape']`, which we use to cancel an edit:

```html
<!-- templates/partials/card_edit.html -->
<input name="title" hx-get="/cards/1"
       hx-trigger="keyup[key=='Escape']"
       hx-target="#card-1" hx-swap="outerHTML">
```

Special triggers: `load` (fire on insertion) and `every 2s` (polling). Polling
is how you'd build the activity feed if you didn't want SSE.

### 1.5 `hx-confirm` — free confirmation dialog

```html
<button hx-delete="/cards/1" hx-confirm="Delete this card?">×</button>
```

A native `confirm()` before the request fires. Ugly but correct, and it cost
one attribute. (A custom modal is the classic "now add Alpine" moment.)

### 1.6 Request indicators — feedback without JS

htmx adds an `htmx-request` class to an element while its request is in flight.
We pair that with an element carrying `htmx-indicator`:

```html
<!-- templates/partials/card.html -->
<button hx-post="..." hx-indicator="closest .card .move-spinner">▶</button>
<span class="move-spinner htmx-indicator">...</span>
```

```css
/* static/app.css */
.htmx-indicator { opacity: 0; transition: opacity 0.2s ease; }
.htmx-request .htmx-indicator { opacity: 1; }
```

The spinner fades in for free. The CSS transition *is* the loading state.

### 1.7 Out-of-band swaps — one response, many targets

The move handler is the centerpiece. The button only targets its own column,
but a move affects **three** regions. The server returns the source column as
the "main" content, and tags the other two as out-of-band:

```go
// main.go — handleMoveCard
templates.ExecuteTemplate(&buf, "column", srcCol)   // main swap → source column

buf.WriteString(`<div hx-swap-oob="outerHTML" id="col-doing">`)
templates.ExecuteTemplate(&buf, "column_inner", dstCol)  // OOB → destination
buf.WriteString(`</div>`)

buf.WriteString(`<div hx-swap-oob="outerHTML" id="stats">`)
templates.ExecuteTemplate(&buf, "stats_inner", stats)    // OOB → stats bar
buf.WriteString(`</div>`)
```

htmx pulls any element marked `hx-swap-oob` out of the response and swaps it
into the matching element on the page. One HTTP round-trip, three coordinated
updates, and the button's markup never had to know about the stats bar.

---

## 2. What's new in v4 (applied)

v4 (`4.0.0-beta6`) is a modernization pass: `fetch()` under the hood, explicit
over implicit, and a handful of genuinely new primitives. Here's what matters,
mapped to the project.

### 2.1 `<hx-partial>` — explicit multi-target updates

The OOB approach above works, but targeting is implicit (match by `id`) and
every fragment is forced into the same response shape. v4 adds a first-class
element where each fragment declares its **own** target and swap:

```go
// main.go — handleMoveCardPartial  (POST /cards/{id}/move-partial)
templates.ExecuteTemplate(&buf, "column", srcCol)   // main swap, as before

buf.WriteString(`<hx-partial hx-target="#col-doing" hx-swap="outerHTML">`)
templates.ExecuteTemplate(&buf, "column", dstCol)
buf.WriteString(`</hx-partial>`)

buf.WriteString(`<hx-partial hx-target="#stats" hx-swap="innerHTML">`)
templates.ExecuteTemplate(&buf, "stats_inner", stats)
buf.WriteString(`</hx-partial>`)
```

Notice the stats fragment uses `innerHTML` here (replace contents) where the OOB
version used `outerHTML` (replace the node) — each fragment chooses
independently. Both endpoints produce identical UX; flip the button URLs in
`card.html` from `/move` to `/move-partial` to compare.

**Rule of thumb:** OOB for "also update these," `<hx-partial>` when fragments
need different swap strategies or you want the targeting spelled out.

### 2.2 Error responses now swap by default

In v2, a `422` or `500` was silently dropped unless you opted in. In v4 **every**
response swaps (except `204`/`304`). Our validation leans on this:

```go
// main.go — handleUpdateCard
if title == "" {
    w.WriteHeader(422)
    templates.ExecuteTemplate(w, "card_edit_error", card)  // form + error message
    return
}
```

Submit an empty title and the server returns the edit form *with* the error, and
it swaps straight in. No client config, no error branch. Design your error
responses as real swap content and v4 does the rest.

(If you want the old behavior: `htmx.config.noSwap = [204, 304, '4xx', '5xx']`.)

### 2.3 New swap styles: `delete`, `innerMorph`, `outerMorph`, `textContent`

Two of these are load-bearing in the app:

**`delete`** — removing a card needs no response body at all:

```html
<button hx-delete="/cards/1" hx-target="#card-1" hx-swap="delete">×</button>
```

The server returns `200` with an empty body; htmx removes the target. Cleaner
than returning an empty string and swapping `innerHTML`.

**`innerMorph`** — the stats bar updates via SSE. A naive `innerHTML` swap would
tear down and rebuild the node on every event, killing the progress bar's CSS
width transition. `innerMorph` uses the [idiomorph] algorithm to diff and patch
in place, so the bar *animates* to its new width instead of snapping:

```html
<!-- templates/partials/stats.html -->
<div id="stats" hx-sse:swap="stats" hx-swap="innerMorph">
```

This is the quiet win of morphing: it preserves DOM state (focus, transitions,
scroll) across updates.

### 2.4 Explicit inheritance (`:inherited`)

v2 inherited `hx-*` attributes down the DOM implicitly, which caused surprises.
v4 makes it opt-in. If you want a whole column to share a target, you now say so:

```html
<div hx-target:inherited="#board">
    <button hx-post="...">...</button>   <!-- inherits the target -->
</div>
```

We kept targeting explicit per-element in this project (clearer for learning),
but `:inherited` — and `:append` to extend rather than replace — is the tool when
a whole subtree shares behavior. Revert to v2 semantics with
`htmx.config.implicitInheritance = true`.

### 2.5 `hx-status` — per-status-code swap behavior

New in v4: control what happens for specific response codes, right on the element:

```html
<form hx-post="/cards"
      hx-status:422="swap:outerHTML target:#card-errors"
      hx-status:5xx="swap:none">
```

Wildcards supported (`50x`, `5xx`). We don't use it yet because the default
"swap everything" already covers our validation, but it's the escape hatch when
different status codes need different targets — e.g. render validation errors
into a sidebar while a `500` shows nothing.

### 2.6 Extension loading changed

No more `hx-ext="sse"` attribute. Extensions are just scripts you include:

```html
<!-- templates/layout.html -->
<script src=".../htmx.min.js"></script>
<script src=".../dist/ext/sse.js"></script>
```

Then `hx-sse:connect` works anywhere. The `htmax.js` bundle ships htmx + the
popular extensions (sse, ws, preload, optimistic, live, upsert) in one file if
you want them all.

### 2.7 SSE extension — real-time as an attribute

With the extension loaded, the whole page subscribes:

```html
<!-- templates/layout.html -->
<body hx-sse:connect="/events">
```

And any element can react to named events by swapping their payload:

```html
<!-- append each activity event as a new child -->
<div class="feed-items" hx-sse:swap="activity" hx-swap="beforeend"></div>

<!-- replace stats contents on each stats event -->
<div id="stats" hx-sse:swap="stats" hx-swap="innerMorph">
```

The server side is just the standard SSE wire format — `event:`, `data:`, blank
line — emitted from a Go channel hub:

```go
// main.go
msg := fmt.Sprintf("event: %s\ndata: %s\n\n", event, data)
fmt.Fprint(w, msg)
flusher.Flush()
```

The crucial architecture note: **SSE is a side-channel, not the source of
truth.** The tab that performed the move updates via the normal htmx response
(OOB). *Other* tabs update via SSE. Same server state, two delivery paths, no
client-side model to reconcile. Open two tabs and move a card to see both.

### 2.8 Other v4 changes worth knowing

| Change | Impact |
|---|---|
| `fetch()` replaces XHR | No `htmx:xhr:*` events; use `htmx:before:request` etc. |
| Event names renamed | `htmx:afterSwap` → `htmx:after:swap` (pattern: `htmx:phase:action`) |
| `hx-delete` excludes form data | Add `hx-include="closest form"` if you relied on it |
| OOB swap order | Main content swaps **first**, then OOB — keep swaps independent |
| `queue` trigger modifier removed | Use `hx-sync="this:queue all"` instead |
| 60s default timeout | Was unlimited; `htmx.config.defaultTimeout = 0` to revert |
| `hx-disable` → `hx-ignore` | Rename before upgrading; `hx-disable` now means "disable element" |
| `hx-action` / `hx-method` | Separate URL from method; enables native `action`/`method` fallback |

Run `npx htmx.org@4 upgrade-check -- ./templates` to scan a v2 codebase for all
of these automatically.

---

## 3. Where htmx ends (and what you'd add)

htmx 4 is unusually complete, but it has a hard edge: **everything is a
round-trip.** Anything you want to happen *without* asking the server is out of
scope. Concretely, in this app:

- **Optimistic UI** — the card waits for the server before moving. (v4's
  `hx-optimistic` extension softens this.)
- **Transient client state** — a custom delete-confirm modal, a dropdown, a
  drag handle. All pure client, all awkward in htmx.
- **Rich client validation** — we rely on the server's 422; instant per-keystroke
  rules are client territory.
- **Drag-and-drop reordering** — needs pointer tracking, not HTTP.

That's the natural seam for **Alpine.js**: `x-data` for local state, `x-show` /
`x-transition` for the transient bits, while htmx keeps owning everything that
touches the server. htmx even ships an `hx-alpine-compat` extension so the two
initialize swapped fragments together. The next pass on this project is to add
Alpine at exactly these seams and measure what it buys.

---

## Quick attribute index (as used in this project)

| Attribute | File | Purpose |
|---|---|---|
| `hx-post` | card.html | move a card |
| `hx-delete` + `hx-swap="delete"` | card.html | remove a card |
| `hx-get` + `hx-swap="outerHTML"` | card.html | enter edit mode |
| `hx-patch` | card_edit.html | save a title |
| `hx-trigger="keyup[key=='Escape']"` | card_edit.html | cancel edit |
| `hx-trigger="input delay:300ms changed"` | index.html | debounced search |
| `hx-target="closest .column"` | card.html | target own column |
| `hx-swap="beforeend"` | index.html | append new card |
| `hx-confirm` | card.html | delete confirmation |
| `hx-indicator` | card.html, index.html | loading feedback |
| `hx-swap-oob` | main.go (OOB handler) | multi-target update |
| `<hx-partial>` | main.go (partial handler) | explicit multi-target |
| `hx-sse:connect` | layout.html | subscribe to `/events` |
| `hx-sse:swap` | index.html, stats.html | react to named events |
| `hx-swap="innerMorph"` | stats.html | state-preserving SSE updates |

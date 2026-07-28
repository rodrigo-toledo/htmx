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
| `delete` *(v4)* | remove the element, ignore response | (not in the live UI — see note) |
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

**Note on `delete`:** it's a great fit when the actor alone removes an
element. This app doesn't use it for card deletion, because a delete must
also reach *other* tabs — so the delete button swaps the whole column by id
(`hx-target="#col-{{.Column}}" hx-swap="outerHTML"`), the same confluent
fragment SSE delivers to everyone else (§2.7).

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
templates.ExecuteTemplate(&buf, "column", columnData(oldCol))      // main swap → source column
templates.ExecuteTemplate(&buf, "column_oob", columnData(card.Column)) // OOB → destination
templates.ExecuteTemplate(&buf, "stats_oob", getStats())               // OOB → stats bar
```

```html
<!-- templates/partials/column.html -->
{{define "column_oob"}}
<div class="column" id="col-{{.ID}}" hx-swap-oob="outerHTML">
    {{template "column_inner" .}}
</div>
{{end}}
```

htmx pulls any element marked `hx-swap-oob` out of the response and swaps it
into the matching element on the page. One HTTP round-trip, three coordinated
updates, and the button's markup never had to know about the stats bar.

**Gotcha (learned the hard way):** an `outerHTML` OOB swap *replaces the whole
node*, so the fragment in the response must carry everything the original had —
classes, ids, behavior attributes. Our first version hand-built the wrapper as
`<div hx-swap-oob="outerHTML" id="col-doing">…</div>` and silently lost the
`column` class: the destination column came back unstyled. That's why the OOB
fragments here are full templates (`column_oob`, `stats_oob`) that mirror the
originals exactly, with `hx-swap-oob` as the only addition.

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
templates.ExecuteTemplate(&buf, "column", columnData(oldCol))   // main swap, as before

buf.WriteString(`<hx-partial hx-target="#col-doing" hx-swap="outerHTML">`)
templates.ExecuteTemplate(&buf, "column", columnData(card.Column))
buf.WriteString(`</hx-partial>`)

buf.WriteString(`<hx-partial hx-target="#stats" hx-swap="outerHTML">`)
templates.ExecuteTemplate(&buf, "stats", getStats())   // full stats div, classes intact
buf.WriteString(`</hx-partial>`)
```

Notice each fragment declares its own `hx-target` and `hx-swap` — you could
swap the stats with `innerHTML` and the column with `outerHTML` in the same
response if that suited you. Both endpoints produce identical UX; flip the
button URLs in `card.html` from `/move` to `/move-partial` to compare.

One asymmetry to know: for *non-outer* swap styles, htmx strips the
`<hx-partial>` wrapper and swaps only its children; for outer styles the
wrapper's content replaces the target. Either way, ship complete fragments
(same rule as OOB in §1.7).

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

No more `hx-ext="sse"` attribute. Extensions are just scripts you include —
and note the v4 naming (`hx-sse.js`, not `sse.js`; the old path 404s):

```html
<!-- templates/layout.html -->
<script src=".../dist/htmx.min.js"></script>
<script src=".../dist/ext/hx-sse.min.js"></script>
```

Then `hx-sse:connect` works anywhere. The `htmax.js` bundle ships htmx + the
popular extensions (sse, ws, preload, optimistic, live, upsert) in one file if
you want them all.

### 2.7 SSE extension — real-time as an attribute

v4 rewrote the SSE extension around `fetch()` streams, and **the mental model
changed from htmx 2**. Two rules:

1. **Unnamed messages** (just `data:`, no `event:` field) are swapped
   automatically, using the connected element's `hx-target`/`hx-swap`.
2. **Named messages** (`event: foo`) are *not* swapped — they're dispatched as
   DOM events, which you handle with `hx-on:foo` or
   `hx-trigger="foo from:body"`.

Our board needs to update *several* regions (feed, stats, and the affected
columns) from one stream, so we use the docs' "update elements" pattern: the
server streams **unnamed** messages whose payload carries its own targeting
via `<hx-partial>` and `hx-swap-oob`:

```go
// main.go — broadcast (called by every mutation handler)
buf.WriteString(`<hx-partial hx-target="#feed-items" hx-swap="beforeend"><div class="feed-item">`)
buf.WriteString(template.HTMLEscapeString(action))
buf.WriteString(`</div></hx-partial>`)
board(&buf)                                          // the affected columns/cards, as OOB fragments
templates.ExecuteTemplate(&buf, "stats_sse", getStats())  // <div id="stats" hx-swap-oob="innerMorph">…
hub.Broadcast(buf.String())
```

Client side is two attributes on the page shell — no per-element SSE markup:

```html
<!-- templates/layout.html -->
<body hx-sse:connect="/events">
```

When a message arrives, htmx extracts the `hx-partial` and OOB fragments, swaps
each into its target, and — because nothing is left over and the extension
defaults to `swapEmpty:false` — the connected `<body>` itself is untouched.

The stats fragment uses `hx-swap-oob="innerMorph"`: idiommorph patches the
existing nodes in place, so the `.progress-fill` element survives and its CSS
`width` transition *animates* to the new percentage instead of snapping.

Two server-side gotchas we hit, both invisible in `curl` but fatal in-browser:

- **Flush headers immediately.** Go buffers the response until the first
  write, so without an initial `fmt.Fprint(w, ": connected\n\n")` + `Flush()`,
  the client sees *nothing* — not even response headers — until the first
  broadcast. htmx's connection hook never fires and early events are lost.
- **`sse.pauseOnBackground` defaults to `true`**, pausing the stream in
  background tabs. We disable it in the layout:
  `<meta name="htmx-config" content="sse.pauseOnBackground:false">`.

And one design decision that turns "flaky" into "self-healing":

- **Event ids + replay.** Streams drop — background pauses, wifi blips,
  laptop sleep. Without replay, every event during the gap is lost forever
  and the tab stays stale until a manual refresh. So every broadcast carries
  an `id:`, the hub keeps a ring buffer of recent messages, and on reconnect
  the server replays whatever the client missed:

  ```go
  // main.go — the client's last-seen id arrives as a standard SSE header
  lastID := 0
  if v := r.Header.Get("Last-Event-ID"); v != "" {
      lastID, _ = strconv.Atoi(v)
  }
  ch, missed := hub.Subscribe(lastID)   // subscribe + snapshot under one lock
  for _, m := range missed {
      fmt.Fprint(w, m.frame())          // replay, then stream live
  }
  ```

  The hx-sse extension sends `Last-Event-ID` on reconnect automatically — no
  client markup needed. `Subscribe` registers the client and snapshots the
  buffer under the *same* lock that `Broadcast` uses, so catch-up is
  gap-free and duplicate-free. A 15-second comment heartbeat (`: ping`)
  keeps idle connections from being killed by intermediaries. The e2e suite
  verifies this by killing a tab's stream mid-test and asserting it catches
  up — exactly once — after reconnecting.

With replay in place, `pauseOnBackground:true` (the default) would also be a
legitimate choice: background tabs disconnect to save resources and simply
catch up when refocused. We keep it `false` so background tabs stay live too.

The crucial architecture note: **the acting tab gets each mutation twice** —
once from its own HTTP response (immediate, reliable, works even if the
stream is down) and once from the SSE broadcast (the same fragments every
other tab receives). That only stays safe because the updates are
*confluent*: every fragment — response or SSE — targets the same element
**by id**, swaps `outerHTML`, and carries identical content. Apply them in
any order, twice, and you land on the same state. This is why the move
buttons target `#col-{{.Column}}` instead of `closest .column`: if the SSE
swap wins the race and replaces the button's column first, the response swap
still resolves against a live node by id, not a detached one.

The payoff is that *every* tab is a full participant: move a card in one and
the columns, counts, stats, and feed update in all the others — no refresh,
no client-side model to reconcile. The response remains the actor's source
of truth for its own action; SSE is how everyone (including the actor,
idempotently) converges.

If you *do* want named events, the v4 shape is:

```html
<!-- server sends: event: progress / data: 50 -->
<div hx-on:progress="htmx.find('#p').value = event.detail.data">…</div>
<!-- or use the event as a trigger for a fresh request -->
<div hx-get="/status" hx-trigger="progress from:body"></div>
```

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

## 3. Where htmx ends — the drag-and-drop case study

htmx 4 is unusually complete, but it has a hard edge: **everything is a
round-trip.** Anything that must happen *continuously, without asking the
server* is out of scope. Drag-and-drop is the clearest example, and this app
implements it — so it's worth seeing exactly how the responsibility splits,
because the answer is **not** "add Alpine."

**Neither htmx nor Alpine does the dragging.** A drag is pointer-tracking at
60fps — a ghost element, hover targets, live reflow — all client-side. htmx
deliberately doesn't do that, and Alpine has no drag primitive either (it's
for declarative state: `x-data`, `x-show`, `x-model`). The canonical answer,
shown in htmx's own examples, is a dedicated library: **SortableJS**.

The division of labor in `static/dnd.js`:

```js
new Sortable(el, {
    group: 'kanban',            // SortableJS owns the drag…
    animation: 150,
    onEnd: function (evt) {     // …htmx owns the persistence
        const cardId = evt.item.id.replace('card-', '');
        const col = evt.to.closest('.column').id.replace('col-', '');
        htmx.ajax('POST', `/cards/${cardId}/drop`, {
            target: evt.from.closest('.column'),
            swap: 'outerHTML',
            values: { col, index: evt.newIndex },
        });
    },
});
```

SortableJS rearranges the DOM *locally* while you drag. On drop, one htmx
request persists the new column + index; the confluent response/SSE
re-render (§2.7) then reconciles **every** tab to the server's authoritative
order — the same machinery the move buttons use.

The one genuinely awkward part, and the real lesson: **htmx swaps destroy the
DOM SortableJS is bound to.** Every column re-render replaces the `.cards`
container, orphaning its Sortable instance. So the glue re-initializes after
each settle, guarded so it's idempotent:

```js
function initSortables() {
    document.querySelectorAll('.cards').forEach((el) => {
        if (!el._sortable) el._sortable = makeSortable(el);
    });
}
document.addEventListener('htmx:after:init', initSortables);
document.addEventListener('htmx:after:settle', initSortables);
```

This "re-init JS widgets after swaps" pattern is the standard cost of mixing
stateful client libraries with htmx's replace-the-DOM model. It's small here,
but it's exactly the friction that grows as you add more widgets.

The remaining seams — where Alpine *would* earn its keep — are the other
client-only concerns:

- **Optimistic UI** — the card waits for the server before moving on button
  clicks. (v4's `hx-optimistic` extension softens this; drag-and-drop is
  naturally optimistic since SortableJS moves it first.)
- **Transient client state** — a custom delete-confirm modal, a dropdown, a
  popover. All pure client, all awkward in htmx.
- **Rich client validation** — we rely on the server's 422; instant
  per-keystroke rules are client territory.

That's the natural seam for **Alpine.js**: `x-data` for local state,
`x-show` / `x-transition` for the transient bits, while htmx keeps owning
everything that touches the server. htmx even ships an `hx-alpine-compat`
extension so the two initialize swapped fragments together. The next pass on
this project is to add Alpine at exactly these seams and measure what it buys.

---

## Quick attribute index (as used in this project)

| Attribute | File | Purpose |
|---|---|---|
| `hx-post` | card.html | move a card (◀ ▶) |
| `hx-delete` + `hx-target="#col-{col}"` | card.html | remove a card (column re-render) |
| `hx-get` + `hx-swap="outerHTML"` | card.html | enter edit mode |
| `hx-patch` | card_edit.html | save a title |
| `hx-trigger="keyup[key=='Escape']"` | card_edit.html | cancel edit |
| `hx-trigger="input delay:300ms changed"` | index.html | debounced search |
| `hx-target="#col-{col}"` + `outerHTML` | card.html, column.html | confluent by-id targeting |
| `hx-confirm` | card.html | delete confirmation |
| `hx-indicator` | card.html, index.html | loading feedback |
| `hx-swap-oob` | main.go (OOB handler), SSE payloads | multi-target update |
| `<hx-partial>` | main.go (partial handler + SSE payloads) | explicit multi-target |
| `hx-sse:connect` | layout.html | subscribe to `/events` |
| `hx-swap="innerMorph"` | stats templates | state-preserving updates (OOB + SSE) |
| `htmx.ajax()` + SortableJS `onEnd` | dnd.js | persist drag-and-drop |
| `htmx:after:settle` | dnd.js | re-init Sortable after swaps |
| `htmx-config` meta | layout.html | `sse.pauseOnBackground:false` |

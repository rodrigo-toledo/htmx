// Drag & drop glue: SortableJS owns the drag, htmx owns the persistence.
//
// SortableJS rearranges the DOM locally while you drag. On drop we POST the
// new position to the server; the response (and the SSE broadcast) re-render
// the affected columns with the server's authoritative order. Because those
// swaps replace the column DOM, any fresh .cards container needs a new
// Sortable instance — initSortables() is idempotent and runs after every
// htmx settle.

function makeSortable(el) {
    return new Sortable(el, {
        group: 'kanban',
        animation: 150,
        ghostClass: 'card-ghost',
        chosenClass: 'card-chosen',
        onEnd: function (evt) {
            const cardId = evt.item.id.replace('card-', '');
            const col = evt.to.closest('.column').id.replace('col-', '');
            htmx.ajax('POST', `/cards/${cardId}/drop`, {
                target: evt.from.closest('.column'),
                swap: 'outerHTML',
                values: { col: col, index: evt.newIndex },
            });
        },
    });
}

function initSortables() {
    document.querySelectorAll('.cards').forEach((el) => {
        if (!el._sortable) el._sortable = makeSortable(el);
    });
}

document.addEventListener('htmx:after:init', initSortables);
document.addEventListener('htmx:after:settle', initSortables);

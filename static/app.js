// The delete-confirmation modal — the app's first Alpine feature.
//
// This is transient client state: whether the modal is open, and which card
// is pending deletion, never touch the server. Alpine owns it (x-data,
// x-show, x-transition, escape / click-outside). The actual deletion is
// still htmx's job — we hand it off with htmx.ajax(), the same pattern
// dnd.js uses for drops.

document.addEventListener('alpine:init', () => {
    Alpine.data('deleteModal', () => ({
        deleting: null,

        // The delete button carries the card's identity in data-* attributes
        // so we never interpolate the title into a JS string (titles can
        // contain quotes). html/template HTML-escapes; dataset decodes.
        open(el) {
            this.deleting = {
                id: el.dataset.id,
                col: el.dataset.col,
                title: el.dataset.title,
            };
        },

        confirm() {
            const card = this.deleting;
            this.deleting = null;
            htmx.ajax('DELETE', `/cards/${card.id}`, {
                target: `#col-${card.col}`,
                swap: 'outerHTML',
            });
        },
    }));
});

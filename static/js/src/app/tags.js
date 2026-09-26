// tags.js holds the shared front-end helpers for the Tags feature: a
// select2-based tag editor for the object modals, tag chips for the list
// tables, and a client-side tag filter above each list.
//
// Like the other app scripts, this is loaded via a plain <script> tag (no
// module) and relies on defining real globals (tagCache, initTagInput, ...)
// that the per-page scripts call. It must be loaded before those scripts.

// tagCache holds the team's known tag names, used to power autocomplete
// suggestions in the tag inputs and the options in the tag filters.
var tagCache = []

// loadTagSuggestions refreshes tagCache from the team's existing tags. The
// callback (optional) runs once the cache is populated, whether or not the
// request succeeded, so callers can build their inputs either way.
function loadTagSuggestions(cb) {
    api.tags.get()
        .done(function (tags) {
            tagCache = (tags || []).map(function (t) {
                return t.name
            })
        })
        .always(function () {
            if (cb) {
                cb(tagCache)
            }
        })
}

// tagCacheData returns tagCache formatted as select2 {id, text} options.
function tagCacheData() {
    return tagCache.map(function (n) {
        return {
            id: n,
            text: n
        }
    })
}

// initTagInput turns a <select multiple> into a select2 tag editor allowing
// free-text entry plus suggestions from the team's existing tags. Pass the
// modal element as dropdownParent so the dropdown stacks above the modal.
function initTagInput(selector, dropdownParent) {
    var $el = $(selector)
    // Destroy any previous instance so re-opening a modal starts clean.
    if ($el.hasClass("select2-hidden-accessible")) {
        $el.select2("destroy")
    }
    $el.empty()
    var opts = {
        theme: "bootstrap-5",
        width: "100%",
        tags: true,
        tokenSeparators: [","],
        placeholder: "Add tags...",
        data: tagCacheData()
    }
    if (dropdownParent) {
        opts.dropdownParent = $(dropdownParent)
    }
    $el.select2(opts)
}

// setTagInput selects the given tag names on a select2 tag input, adding any
// that aren't already known options (e.g. tags unique to this object).
function setTagInput(selector, tags) {
    var $el = $(selector)
    tags = tags || []
    tags.forEach(function (name) {
        if ($el.find("option[value='" + $.escapeSelector(name) + "']").length === 0) {
            $el.append(new Option(name, name, true, true))
        }
    })
    $el.val(tags).trigger("change")
}

// getTagInput returns the array of tag strings currently entered.
function getTagInput(selector) {
    return $(selector).val() || []
}

// clearTagInput empties a select2 tag input.
function clearTagInput(selector) {
    var $el = $(selector)
    if ($el.hasClass("select2-hidden-accessible")) {
        $el.val(null).trigger("change")
    }
}

// TAG_TOKEN is an (invisible) delimiter wrapped around each tag name in a
// hidden span inside the chips cell, so the tag filter can match a column's
// search text on whole tags (\x01de\x01) without partial hits (e.g. "de"
// inside "developer").
var TAG_TOKEN = "\u0001"

// renderTagChips renders an array of tag names as small badge chips for a
// table cell, returning an escaped HTML string (a muted dash when empty).
// A hidden, delimited token string is appended so the column stays exactly
// filterable via searchTagColumn() below.
function renderTagChips(tags) {
    if (!tags || tags.length === 0) {
        return "<span class='text-muted'>&mdash;</span>"
    }
    var chips = tags.map(function (t) {
        return "<span class='badge tag-chip'>" + escapeHtml(t) + "</span>"
    }).join(" ")
    var tokens = TAG_TOKEN + tags.map(escapeHtml).join(TAG_TOKEN) + TAG_TOKEN
    return chips + "<span class='tag-tokens'>" + tokens + "</span>"
}

// escapeRegex escapes a string for safe use inside a RegExp.
function escapeRegex(s) {
    return String(s).replace(/[.*+?^${}()|[\]\\]/g, "\\$&")
}

// collectTags returns the sorted, de-duplicated set of tag names present on
// the given items (each item expected to carry a .tags array). Used to
// populate a table's tag-filter options from exactly what's on screen.
function collectTags(items) {
    var seen = {}
    var out = []
    ;(items || []).forEach(function (item) {
        ;(item.tags || []).forEach(function (t) {
            if (!seen[t]) {
                seen[t] = true
                out.push(t)
            }
        })
    })
    out.sort(function (a, b) {
        return a.toLowerCase() < b.toLowerCase() ? -1 : a.toLowerCase() > b.toLowerCase() ? 1 : 0
    })
    return out
}

// buildTagFilter creates a tag-filter control (a labelled multi-select) to
// drop into a DataTable's toolbar via the `layout` option. It returns the
// jQuery container; call wireTagFilter() once the table exists to activate
// the select2 widget and the column search. options is the list of tag
// names to offer (typically collectTags(items)).
function buildTagFilter(options) {
    var $container = $(
        "<div class='dt-tag-filter d-flex align-items-center gap-2'>" +
        "<span class='text-body-secondary small text-nowrap'><i class='fa fa-tag'></i></span>" +
        "<select multiple></select>" +
        "</div>"
    )
    $container.data("tagOptions", options || [])
    return $container
}

// wireTagFilter seats the control built by buildTagFilter into the table's
// top-left toolbar cell (next to the length menu), activates the select2
// widget, and filters the given table's tag column (marked with the
// `.tags-col` header class) on change. Matching is exact per tag and OR
// across selected tags; clearing the selection restores every row.
function wireTagFilter($container, table) {
    // Place the control in the top layout row's start cell, alongside the
    // length menu, rather than relying on passing a node through the
    // DataTables `layout` option (whose node handling varies by version).
    $(table.table().container()).find(".dt-layout-start").first().append($container)
    var $sel = $container.find("select")
    var options = $container.data("tagOptions") || []
    $sel.select2({
        theme: "bootstrap-5",
        width: "240px",
        multiple: true,
        allowClear: true,
        placeholder: "Filter by tag",
        data: options.map(function (n) {
            return { id: n, text: n }
        })
    })
    $sel.on("change", function () {
        var selected = $sel.val() || []
        var regex = ""
        if (selected.length) {
            regex = selected.map(function (s) {
                return TAG_TOKEN + escapeRegex(s) + TAG_TOKEN
            }).join("|")
        }
        // true = treat as regex, false = disable smart search, so our
        // delimited tokens match whole tags only.
        table.column(".tags-col").search(regex, true, false).draw()
    })
}

// tableLayout returns a DataTables `layout` that keeps every control tidy
// and aligned: length menu on the top-left (the tag filter is seated beside
// it afterwards by wireTagFilter), search on the top-right, row info
// bottom-left, pagination bottom-right. The argument is accepted for call
// sites that pass their filter control but isn't used for placement.
function tableLayout(_tagFilter) {
    return {
        topStart: "pageLength",
        topEnd: "search",
        bottomStart: "info",
        bottomEnd: "paging"
    }
}

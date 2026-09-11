// ckeditor-setup.js
//
// CKEditor 5 editor factory + a small custom "{{.Var}}" autocomplete plugin,
// shared by templates.js (html_editor) and landing_pages.js (html_editor,
// redirect_html_editor).

var TEMPLATE_TAGS = [{
        id: 1,
        name: 'RId',
        description: 'The unique ID for the recipient.'
    },
    {
        id: 2,
        name: 'FirstName',
        description: 'The recipient\'s first name.'
    },
    {
        id: 3,
        name: 'LastName',
        description: 'The recipient\'s last name.'
    },
    {
        id: 4,
        name: 'Position',
        description: 'The recipient\'s position.'
    },
    {
        id: 5,
        name: 'Email',
        description: 'The recipient\'s email address.'
    },
    {
        id: 6,
        name: 'From',
        description: 'The address emails are sent from.'
    },
    {
        id: 7,
        name: 'TrackingURL',
        description: 'The URL to track emails being opened.'
    },
    {
        id: 8,
        name: 'Tracker',
        description: 'An HTML tag that adds a hidden tracking image (recommended instead of TrackingURL).'
    },
    {
        id: 9,
        name: 'URL',
        description: 'The URL to your Gophish listener.'
    },
    {
        id: 10,
        name: 'BaseURL',
        description: 'The base URL with the path and rid parameter stripped. Useful for making links to static files.'
    },
    {
        id: 11,
        name: 'Domain',
        description: 'The portion of the recipient\'s email after the "@", e.g. "example.com" for "foo@example.com".'
    }
];

var TEMPLATE_VAR_PATTERN = /\{\{\.?([A-Za-z]|\})*$/;

function templateVarMatches(query) {
    var q = query.toLowerCase();
    return TEMPLATE_TAGS.filter(function (item) {
        var itemName = '{{.' + item.name.toLowerCase() + '}}';
        return itemName.indexOf(q) === 0;
    });
}

// A CKEditor 5 plugin (function-style, matching CKEDITOR.Plugin's expected
// shape: a class/constructor with an init() method) that watches for
// "{{" near the caret and offers matching TEMPLATE_TAGS in a small dropdown,
// inserting the selected variable as plain text.
function TemplateVariableAutocomplete(editor) {
    this.editor = editor;
}
TemplateVariableAutocomplete.pluginName = 'TemplateVariableAutocomplete';

TemplateVariableAutocomplete.prototype.init = function () {
    var editor = this.editor;
    var model = editor.model;

    var lastMatchedText = '';
    var panelEl = null;
    var activeItems = [];
    var activeIndex = -1;

    function hidePanel() {
        if (panelEl) {
            panelEl.remove();
            panelEl = null;
        }
        activeItems = [];
        activeIndex = -1;
    }

    function positionPanel() {
        if (!panelEl) {
            return;
        }
        try {
            var viewRange = editor.editing.mapper.toViewRange(model.document.selection.getFirstRange());
            var domRange = editor.editing.view.domConverter.viewRangeToDom(viewRange);
            // getBoundingClientRect() on a *collapsed* DOM range (just a
            // caret, no selected text - which is what this always is here)
            // returns an empty (0,0) rect in some browsers. getClientRects()
            // is the reliable way to read a collapsed range's actual visual
            // position; fall back to getBoundingClientRect() only if that
            // comes back empty too.
            var rects = domRange.getClientRects();
            var rect = rects.length > 0 ? rects[0] : domRange.getBoundingClientRect();
            panelEl.style.left = (window.scrollX + rect.left) + 'px';
            panelEl.style.top = (window.scrollY + rect.bottom + 4) + 'px';
            panelEl.style.visibility = 'visible';
        } catch (e) {
            // Best-effort positioning - if the range/rect lookup fails for
            // any reason, leave the panel wherever it last was rather than
            // throwing inside an editor event handler.
        }
    }

    function renderPanel(items) {
        if (!panelEl) {
            panelEl = document.createElement('div');
            panelEl.className = 'ck-template-var-panel';
            // Hidden until positionPanel() places it - otherwise the very
            // first paint happens at (0,0) (no left/top set yet) and is
            // visible for a frame before the deferred position lands.
            panelEl.style.visibility = 'hidden';
            document.body.appendChild(panelEl);
        }
        panelEl.innerHTML = '';
        items.forEach(function (item, idx) {
            var row = document.createElement('div');
            row.className = 'ck-template-var-panel-item' + (idx === activeIndex ? ' is-active' : '');
            row.innerHTML = '<strong>{{.' + item.name + '}}</strong><span>' + item.description + '</span>';
            row.addEventListener('mousedown', function (evt) {
                // mousedown (not click) so this fires before the editor's
                // own blur/selection-change handling steals focus first.
                evt.preventDefault();
                insertItem(item);
            });
            panelEl.appendChild(row);
        });
        // Deferred to a fresh task: on the very first open, the view hasn't
        // finished rendering the just-typed "{{" yet when 'matched:data'
        // fires (positioning against it too early landed the panel at
        // (0,0), the browser default for an as-yet-unrendered range - a
        // Backspace afterwards "fixed" it only because that second match
        // positioned against the now-rendered state from the first). A
        // setTimeout(0) runs after the current render cycle instead of
        // racing it.
        setTimeout(positionPanel, 0);
    }

    function insertItem(item) {
        var text = '{{.' + item.name + '}}';
        var length = lastMatchedText.length;
        model.change(function (writer) {
            var position = model.document.selection.getFirstPosition();
            var start = position.getShiftedBy(-length);
            writer.remove(writer.createRange(start, position));
            writer.insertText(text, start);
        });
        editor.editing.view.focus();
        hidePanel();
    }

    var watcher = new CKEDITOR.TextWatcher(model, function (text) {
        var match = text.match(TEMPLATE_VAR_PATTERN);
        if (!match) {
            return false;
        }
        lastMatchedText = match[0];
        return true;
    });

    watcher.on('matched:data', function () {
        var query = lastMatchedText;
        var items = templateVarMatches(query);
        if (items.length === 0) {
            hidePanel();
            return;
        }
        activeItems = items;
        activeIndex = 0;
        renderPanel(items);
    });

    watcher.on('unmatched', hidePanel);

    // Keyboard navigation - up/down to move the highlight, enter/tab to
    // commit, escape to dismiss. Registered on the editing view's document
    // so it only fires while this editor instance has focus.
    editor.editing.view.document.on('keydown', function (evt, data) {
        if (!panelEl || activeItems.length === 0) {
            return;
        }
        var key = data.keyCode;
        if (key === 40) { // down
            activeIndex = (activeIndex + 1) % activeItems.length;
            renderPanel(activeItems);
            data.preventDefault();
            evt.stop();
        } else if (key === 38) { // up
            activeIndex = (activeIndex - 1 + activeItems.length) % activeItems.length;
            renderPanel(activeItems);
            data.preventDefault();
            evt.stop();
        } else if (key === 13 || key === 9) { // enter / tab
            insertItem(activeItems[activeIndex]);
            data.preventDefault();
            evt.stop();
        } else if (key === 27) { // escape
            hidePanel();
            data.preventDefault();
            evt.stop();
        }
    }, {
        priority: 'highest'
    });

    editor.on('destroy', hidePanel);
};

// CKEditor 5's FullPage feature only recognizes "full page" mode from
// content that already starts with an <html> tag - setData("") (used to
// reset these editors, e.g. on modal dismiss) leaves it with no document
// structure to recognize, so Source view shows just the body fragment
// instead of a full document until the next save/reload round-trip (saving
// calls getData(), which does wrap the content - it's specifically the
// *editor's own* full-page recognition that needs seeding). Reset to this
// skeleton instead of "" so a brand new template/page starts in full-page
// mode from the first keystroke.
var EMPTY_FULL_PAGE_HTML = '<html>\n<head>\n</head>\n<body>\n</body>\n</html>';

// editors[id] holds the created CKEditor 5 instances, keyed by the
// <textarea>'s id - there's no CKEDITOR.instances registry in v5 like there
// was in v4.
var editors = {};

/**
 * Creates (or reuses, if already created) a CKEditor 5 instance for the
 * <textarea> with the given id, then calls onReady(editor). Reusing on
 * repeat calls matters because the modals that host these editors stay in
 * the DOM across opens - CKEditor 5 replaces the textarea with its own
 * editable element on create(), so calling create() again on an
 * already-replaced element would not work.
 */
function createEditor(id, onReady) {
    if (editors[id]) {
        onReady(editors[id]);
        return;
    }
    CKEDITOR.ClassicEditor.create(document.getElementById(id), {
        licenseKey: 'GPL',
        plugins: [
            CKEDITOR.Essentials,
            CKEDITOR.Paragraph,
            CKEDITOR.Heading,
            CKEDITOR.Bold,
            CKEDITOR.Italic,
            CKEDITOR.Link,
            CKEDITOR.List,
            CKEDITOR.Indent,
            CKEDITOR.BlockQuote,
            CKEDITOR.Table,
            CKEDITOR.TableToolbar,
            CKEDITOR.SpecialCharacters,
            CKEDITOR.SpecialCharactersEssentials,
            CKEDITOR.PasteFromOffice,
            CKEDITOR.SourceEditing,
            CKEDITOR.GeneralHtmlSupport,
            CKEDITOR.FullPage,
            CKEDITOR.AccessibilityHelp,
            TemplateVariableAutocomplete
        ],
        toolbar: [
            'undo', 'redo', '|',
            'heading', '|',
            'bold', 'italic', '|',
            'link', 'bulletedList', 'numberedList', 'outdent', 'indent', '|',
            'blockQuote', 'insertTable', 'specialCharacters', '|',
            'sourceEditing'
        ],
        heading: {
            options: [{
                    model: 'paragraph',
                    title: 'Paragraph'
                },
                {
                    model: 'heading1',
                    view: 'h1',
                    title: 'Heading 1'
                },
                {
                    model: 'heading2',
                    view: 'h2',
                    title: 'Heading 2'
                },
                {
                    model: 'heading3',
                    view: 'h3',
                    title: 'Heading 3'
                }
            ]
        },
        // Phishing templates need arbitrary/raw HTML (scripts, tracking
        // pixels, form actions, inline event handlers) - this editor is
        // only reachable by an authenticated Gophish admin composing that
        // content on purpose, same reasoning as the old config.allowedContent
        // = true. GHS's default schema is otherwise fairly restrictive.
        htmlSupport: {
            allow: [{
                name: /.*/,
                attributes: true,
                classes: true,
                styles: true
            }],
            // FullPage needs this nested config key to actually preserve
            // the surrounding <html>/<head>/<body> structure on
            // setData()/getData() - the FullPage plugin alone isn't enough.
            //
            // allowRenderStylesFromHead is deliberately left at its default
            // (false/unset) - unlike htmlSupport.allow above (which only
            // affects what gets *stored* in the saved template, ultimately
            // sent to phishing targets, not the admin), this one live-injects
            // a template's own <style> block as real CSS into the admin's
            // *current page* while editing. Templates can come from "Import
            // Email" (a captured real phishing email - attacker-authored
            // content), so this is a live CSS-injection vector into the
            // Gophish UI itself, not just the sandboxed editor content area -
            // confirmed in testing (a template's CSS visibly bled into the
            // surrounding page chrome). CKEditor's own docs flag this exact
            // risk and only recommend enabling it together with a
            // sanitizeCss callback, which we don't have.
            fullPage: {}
        }
    }).then(function (editor) {
        editors[id] = editor;
        onReady(editor);
    }).catch(function (err) {
        console.error('Failed to create CKEditor instance for #' + id, err);
    });
}

/**
 * Toggles a CKEditor 5 instance out of source-editing mode back into
 * WYSIWYG, if it's currently in source mode. Replaces v4's
 * CKEDITOR.instances[id].setMode('wysiwyg').
 */
function ensureWysiwyg(editor) {
    // Looked up by class reference (not the 'SourceEditing' string) so this
    // doesn't depend on matching the plugin's registered name exactly.
    // isSourceEditingMode is a settable observable - set it directly rather
    // than editor.execute('sourceEditing'), which toggles and so would only
    // be correct if it's guaranteed to always flip true->false here (this
    // way there's no dependence on that assumption either).
    var sourceEditing = editor.plugins.get(CKEDITOR.SourceEditing);
    if (sourceEditing) {
        sourceEditing.isSourceEditingMode = false;
    }
}

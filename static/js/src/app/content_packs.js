/*
	content_packs.js
	Lets a user bundle Email Templates and Landing Pages into a single,
	shareable Content Pack file, and import one back in.
*/
var templates = []
var pages = []

function updateExportButtonState() {
    var anySelected = $(".template-select:checked").length > 0 || $(".page-select:checked").length > 0
    $("#exportContentPackButton").prop("disabled", !anySelected)
}

function exportContentPack() {
    var templateIds = $(".template-select:checked").map(function () {
        return parseInt($(this).data("id"))
    }).get()
    var pageIds = $(".page-select:checked").map(function () {
        return parseInt($(this).data("id"))
    }).get()
    if (templateIds.length === 0 && pageIds.length === 0) {
        return
    }
    api.content_pack_export({
            template_ids: templateIds,
            page_ids: pageIds
        })
        .done(function (pack) {
            downloadJSON(pack, "content-pack.json")
        })
        .fail(function () {
            errorFlash("Error exporting content pack")
        })
}

function importContentPack() {
    var files = $("#contentPackFile")[0].files
    if (files.length === 0) {
        contentPackModalError("No file selected")
        return
    }
    var reader = new FileReader()
    reader.onload = function (e) {
        var pack
        try {
            pack = JSON.parse(e.target.result)
        } catch (err) {
            contentPackModalError("Invalid JSON file")
            return
        }
        submitContentPack(pack)
    }
    reader.readAsText(files[0])
}


function submitContentPack(pack) {
    api.content_pack_import(pack)
        .done(function (result) {
            hideModal("#importContentPackModal")
            var parts = []
            if (result.templates_imported.length > 0) {
                parts.push(result.templates_imported.length + " template(s)")
            }
            if (result.pages_imported.length > 0) {
                parts.push(result.pages_imported.length + " page(s)")
            }
            var message = parts.length > 0 ? "Imported " + parts.join(" and ") + "." : "Nothing to import."
            if (result.renamed.length > 0) {
                message += " " + result.renamed.length + " renamed due to a name conflict."
            }
            if (result.errors.length > 0) {
                message += " " + result.errors.length + " item(s) failed."
            }
            successFlash(message)
            loadTemplates()
            loadPages()
        })
        .fail(function (data) {
            contentPackModalError(data.responseJSON ? data.responseJSON.message : "Error importing content pack")
        })
}

function contentPackModalError(message) {
    $("#importContentPack\\.flashes").empty().append("<div style=\"text-align:center\" class=\"alert alert-danger\">\
        <i class=\"fa fa-exclamation-circle\"></i> " + message + "</div>")
}

function loadTemplates() {
    $("#templateTable").hide()
    $("#templatesEmptyMessage").hide()
    $("#templatesLoading").show()
    api.templates.get()
        .done(function (ts) {
            templates = ts
            $("#templatesLoading").hide()
            if (templates.length > 0) {
                $("#templateTable").show()
                var table = $("#templateTable").DataTable({
                    destroy: true,
                    columnDefs: [{
                        orderable: false,
                        targets: "no-sort"
                    }]
                });
                table.clear()
                var rows = []
                $.each(templates, function (i, template) {
                    rows.push([
                        "<input type='checkbox' class='template-select' data-id='" + template.id + "'>",
                        escapeHtml(template.name),
                        moment(template.modified_date).format('MMM D, YYYY h:mm a')
                    ])
                })
                table.rows.add(rows).draw()
                $("#templateTable").off("change", ".template-select").on("change", ".template-select", updateExportButtonState)
                updateExportButtonState()
            } else {
                $("#templatesEmptyMessage").show()
            }
        })
        .fail(function () {
            $("#templatesLoading").hide()
            errorFlash("Error fetching templates")
        })
}

function loadPages() {
    $("#pagesTable").hide()
    $("#pagesEmptyMessage").hide()
    $("#pagesLoading").show()
    api.pages.get()
        .done(function (ps) {
            pages = ps
            $("#pagesLoading").hide()
            if (pages.length > 0) {
                $("#pagesTable").show()
                var table = $("#pagesTable").DataTable({
                    destroy: true,
                    columnDefs: [{
                        orderable: false,
                        targets: "no-sort"
                    }]
                });
                table.clear()
                var rows = []
                $.each(pages, function (i, page) {
                    rows.push([
                        "<input type='checkbox' class='page-select' data-id='" + page.id + "'>",
                        escapeHtml(page.name),
                        moment(page.modified_date).format('MMM D, YYYY h:mm a')
                    ])
                })
                table.rows.add(rows).draw()
                $("#pagesTable").off("change", ".page-select").on("change", ".page-select", updateExportButtonState)
                updateExportButtonState()
            } else {
                $("#pagesEmptyMessage").show()
            }
        })
        .fail(function () {
            $("#pagesLoading").hide()
            errorFlash("Error fetching pages")
        })
}

$(document).ready(function () {
    loadTemplates()
    loadPages()
})

/*
	landing_pages.js
	Handles the creation, editing, and deletion of landing pages
	Author: Jordan Wright <github.com/jordan-wright>
*/
var pages = []


// Save attempts to POST to /templates/
function save(idx) {
    var page = {}
    page.name = $("#name").val()
    page.html = editors["html_editor"].getData()
    page.capture_credentials = $("#capture_credentials_checkbox").prop("checked")
    page.capture_passwords = $("#capture_passwords_checkbox").prop("checked")
    page.redirect_url = $("#redirect_url_input").val()
    page.redirect_mode = $("input[name=redirect_choice]:checked").val()
    page.redirect_html = editors["redirect_html_editor"].getData()
    if (idx != -1) {
        page.id = pages[idx].id
        api.pageId.put(page)
            .done(function (data) {
                successFlash("Page edited successfully!")
                load()
                dismiss()
            })
    } else {
        // Submit the page
        api.pages.post(page)
            .done(function (data) {
                successFlash("Page added successfully!")
                load()
                dismiss()
            })
            .fail(function (data) {
                modalError(data.responseJSON.message)
            })
    }
}

function dismiss() {
    $("#modal\\.flashes").empty()
    $("#name").val("")
    if (editors["html_editor"]) {
        // Force back to WYSIWYG first
        ensureWysiwyg(editors["html_editor"])
        editors["html_editor"].setData(EMPTY_FULL_PAGE_HTML)
    }
    $("#url").val("")
    $("#redirect_url_input").val("")
    if (editors["redirect_html_editor"]) {
        ensureWysiwyg(editors["redirect_html_editor"])
        editors["redirect_html_editor"].setData(EMPTY_FULL_PAGE_HTML)
    }
    $("#modal").find("input[type='checkbox']").prop("checked", false)
    $("#redirect_url_radio").prop("checked", true)
    $("#capture_passwords").hide()
    $("#after-submit").hide()
    $("#redirect_url").show()
    $("#redirect_html").hide()
    hideModal()
}

var deletePage = function (idx) {
    Swal.fire({
        title: "Are you sure?",
        text: "This will delete the landing page. This can't be undone!",
        icon: "warning",
        animation: false,
        showCancelButton: true,
        confirmButtonText: "Delete " + escapeHtml(pages[idx].name),
        confirmButtonColor: "#428bca",
        reverseButtons: true,
        allowOutsideClick: false,
        preConfirm: function () {
            return new Promise(function (resolve, reject) {
                api.pageId.delete(pages[idx].id)
                    .done(function (msg) {
                        resolve()
                    })
                    .fail(function (data) {
                        reject(data.responseJSON.message)
                    })
            })
        }
    }).then(function (result) {
        if (result.value){
            Swal.fire(
                'Landing Page Deleted!',
                'This landing page has been deleted!',
                'success'
            );
        }
        $('button:contains("OK")').on('click', function () {
            location.reload()
        })
    })
}

function importSite() {
    url = $("#url").val()
    if (!url) {
        modalError("No URL Specified!")
    } else {
        api.clone_site({
                url: url,
                include_resources: false
            })
            .done(function (data) {
                editors["html_editor"].setData(data.html)
                ensureWysiwyg(editors["html_editor"])
                hideModal("#importSiteModal")
            })
            .fail(function (data) {
                modalError(data.responseJSON.message)
            })
    }
}

function edit(idx) {
    $("#modalSubmit").unbind('click').click(function () {
        save(idx)
    })
    createEditor("html_editor", function (htmlEditor) {
        createEditor("redirect_html_editor", function (redirectEditor) {
            var page = {}
            if (idx != -1) {
                $("#modalLabel").text("Edit Landing Page")
                page = pages[idx]
                $("#name").val(page.name)
                htmlEditor.setData(page.html)
                redirectEditor.setData(page.redirect_html)
                $("#capture_credentials_checkbox").prop("checked", page.capture_credentials)
                $("#capture_passwords_checkbox").prop("checked", page.capture_passwords)
                $("#redirect_url_input").val(page.redirect_url)
                if (page.capture_credentials) {
                    $("#capture_passwords").show()
                    $("#after-submit").show()
                    if (page.redirect_mode == "html") {
                        $("#redirect_html_radio").prop("checked", true)
                        $("#redirect_url").hide()
                        $("#redirect_html").show()
                    } else {
                        $("#redirect_url_radio").prop("checked", true)
                        $("#redirect_url").show()
                        $("#redirect_html").hide()
                    }
                }
            } else {
                $("#modalLabel").text("New Landing Page")

                htmlEditor.setData(EMPTY_FULL_PAGE_HTML)
                redirectEditor.setData(EMPTY_FULL_PAGE_HTML)
            }
        })
    })
}

function copy(idx) {
    $("#modalSubmit").unbind('click').click(function () {
        save(-1)
    })
    createEditor("html_editor", function (htmlEditor) {
        createEditor("redirect_html_editor", function (redirectEditor) {
            var page = pages[idx]
            $("#name").val("Copy of " + page.name)
            htmlEditor.setData(page.html)
            redirectEditor.setData(page.redirect_html)
            $("#capture_credentials_checkbox").prop("checked", page.capture_credentials)
            $("#capture_passwords_checkbox").prop("checked", page.capture_passwords)
            $("#redirect_url_input").val(page.redirect_url)
            if (page.capture_credentials) {
                $("#capture_passwords").show()
                $("#after-submit").show()
                if (page.redirect_mode == "html") {
                    $("#redirect_html_radio").prop("checked", true)
                    $("#redirect_url").hide()
                    $("#redirect_html").show()
                } else {
                    $("#redirect_url_radio").prop("checked", true)
                    $("#redirect_url").show()
                    $("#redirect_html").hide()
                }
            }
        })
    })
}

function load() {
    /*
        load() - Loads the current pages using the API
    */
    $("#pagesTable").hide()
    $("#emptyMessage").hide()
    $("#loading").show()
    api.pages.get()
        .done(function (ps) {
            pages = ps
            $("#loading").hide()
            if (pages.length > 0) {
                $("#pagesTable").show()
                pagesTable = $("#pagesTable").DataTable({
                    destroy: true,
                    columnDefs: [{
                        orderable: false,
                        targets: "no-sort"
                    }]
                });
                pagesTable.clear()
                pageRows = []
                $.each(pages, function (i, page) {
                    pageRows.push([
                        escapeHtml(page.name),
                        moment(page.modified_date).format('MMM D, YYYY h:mm a'),
                        (canModifyObjects() ? "<div class='pull-right'><span data-bs-toggle='modal' data-bs-backdrop='static' data-bs-target='#modal'><button class='btn btn-sm btn-primary' data-bs-toggle='tooltip' data-bs-placement='left' title='Edit Page' onclick='edit(" + i + ")'>\
                    <i class='fa fa-pencil'></i>\
                    </button></span>\
		    <span data-bs-toggle='modal' data-bs-target='#modal'><button class='btn btn-sm btn-primary' data-bs-toggle='tooltip' data-bs-placement='left' title='Copy Page' onclick='copy(" + i + ")'>\
                    <i class='fa fa-copy'></i>\
                    </button></span>\
                    <button class='btn btn-sm btn-danger' data-bs-toggle='tooltip' data-bs-placement='left' title='Delete Page' onclick='deletePage(" + i + ")'>\
                    <i class='fa fa-trash-o'></i>\
                    </button></div>" : "")
                    ])
                })
                pagesTable.rows.add(pageRows).draw()
                initTooltips()
            } else {
                $("#emptyMessage").show()
            }
        })
        .fail(function () {
            $("#loading").hide()
            errorFlash("Error fetching pages")
        })
}

$(document).ready(function () {
    // Setup multiple modals
    // Code based on http://miles-by-motorcycle.com/static/bootstrap-modal/index.html
    $('.modal').on('hidden.bs.modal', function (event) {
        $(this).removeClass('fv-modal-stack');
        $('body').data('fv_open_modals', $('body').data('fv_open_modals') - 1);
    });
    $('.modal').on('shown.bs.modal', function (event) {
        // Keep track of the number of open modals
        if (typeof ($('body').data('fv_open_modals')) == 'undefined') {
            $('body').data('fv_open_modals', 0);
        }
        // if the z-index of this modal has been set, ignore.
        if ($(this).hasClass('fv-modal-stack')) {
            return;
        }
        $(this).addClass('fv-modal-stack');
        // Increment the number of open modals
        $('body').data('fv_open_modals', $('body').data('fv_open_modals') + 1);
        // Setup the appropriate z-index
        $(this).css('z-index', 1040 + (10 * $('body').data('fv_open_modals')));
        $('.modal-backdrop').not('.fv-modal-stack').css('z-index', 1039 + (10 * $('body').data('fv_open_modals')));
        $('.modal-backdrop').not('fv-modal-stack').addClass('fv-modal-stack');
    });
    $.fn.modal.Constructor.prototype.enforceFocus = function () {
        $(document)
            .off('focusin.bs.modal') // guard against infinite focus loop
            .on('focusin.bs.modal', $.proxy(function (e) {
                if (
                    this.$element[0] !== e.target && !this.$element.has(e.target).length
                    // CKEditor compatibility fix start.
                    &&
                    !$(e.target).closest('.cke_dialog, .cke').length
                    // CKEditor compatibility fix end.
                ) {
                    this.$element.trigger('focus');
                }
            }, this));
    };
    // Scrollbar fix - https://stackoverflow.com/questions/19305821/multiple-modals-overlay
    $(document).on('hidden.bs.modal', '.modal', function () {
        $('.modal:visible').length && $(document.body).addClass('modal-open');
    });
    $('#modal').on('hidden.bs.modal', function (event) {
        dismiss()
    });
    $("#capture_credentials_checkbox").change(function () {
        $("#capture_passwords").toggle()
        $("#after-submit").toggle()
    })
    $("input[name=redirect_choice]").change(function () {
        $("#redirect_url").toggle()
        $("#redirect_html").toggle()
    })
    load()
})

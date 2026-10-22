var groups = []

// ldapState holds the current modal's LDAP / Active Directory context.
var ldapState = {
    source: "manual",      // "manual" | "ldap"
    managed: false,        // existing LDAP-managed group?
    syncId: 0,             // LDAPGroupSync id when managed
    groupId: 0,            // Gophish group id when editing
    connectionId: 0,       // stored connection id when managed
    group: null,           // selected AD group: {directory_id, name, dn} (new groups)
    lastAutoName: "",      // the last Name value auto-filled from a selected group
    searchSeq: 0,          // monotonically increasing group-search request id
    searchTimer: null      // debounce timer for the group search box
}

// Group search UX constants.
var LDAP_GROUP_SEARCH_DEBOUNCE_MS = 300
var LDAP_GROUP_SEARCH_MIN_CHARS = 2
var LDAP_GROUP_SEARCH_LIMIT = 50 // mirrors the backend result limit

// currentSource returns the selected members source from the radios.
function currentSource() {
    return $("input[name=membersource]:checked").val() || "manual"
}

// buildLdapSync assembles the ldap_sync payload from the LDAP form. The
// selected Active Directory group is identified ONLY by its stable
// directory id (the server resolves and stores the authoritative name/DN).
// Blank attribute values are omitted so backend defaults apply;
// exclude_disabled and enabled are always sent explicitly as booleans.
function buildLdapSync() {
    var s = {
        connection_id: currentConnectionId(),
        exclude_disabled: $("#ldapExcludeDisabled").is(":checked"),
        enabled: $("#ldapEnabled").is(":checked"),
        sync_freq: syncFreqSeconds()
    }
    if (ldapState.group && ldapState.group.directory_id) {
        s.group_directory_id = ldapState.group.directory_id
    }
    var mappings = {
        attr_email: $("#ldapAttrEmail").val().trim(),
        attr_first_name: $("#ldapAttrFirst").val().trim(),
        attr_last_name: $("#ldapAttrLast").val().trim(),
        attr_position: $("#ldapAttrPosition").val().trim()
    }
    $.each(mappings, function (k, v) {
        if (v !== "") {
            s[k] = v
        }
    })
    return s
}

// Save attempts to POST or PUT to /groups/
function save(id) {
    // LDAP-managed save path (new group only; existing managed groups edit
    // their config via the LDAP sync endpoint in saveLdapConfig()).
    if (currentSource() === "ldap" && id == -1) {
        if (!currentConnectionId()) {
            modalError("Select an LDAP connection.")
            return
        }
        if (!ldapState.group || !ldapState.group.directory_id) {
            modalError("Search for and select an Active Directory group.")
            return
        }
        var name = $("#name").val().trim()
        if (name === "") {
            modalError("Enter a group name.")
            return
        }
        var group = {
            name: name,
            tags: getTagInput("#tags"),
            targets: [],
            ldap_sync: buildLdapSync()
        }
        api.groups.post(group)
            .done(function () {
                successFlash("Group created. Members will be synchronized from Active Directory.")
                load()
                dismiss()
                hideModal()
            })
            .fail(function (data) {
                modalError((data.responseJSON && data.responseJSON.message) ? data.responseJSON.message : "Unable to create the group.")
            })
        return
    }
    // Existing LDAP-managed group: configuration is saved via the LDAP sync
    // endpoint, never the blocked Group PUT.
    if (currentSource() === "ldap" && id != -1 && ldapState.managed) {
        saveLdapConfig()
        return
    }

    // --- Manual path (unchanged) ---
    var targets = []
    $.each($("#targetsTable").DataTable().rows().data(), function (i, target) {
        targets.push({
            first_name: unescapeHtml(target[0]),
            last_name: unescapeHtml(target[1]),
            email: unescapeHtml(target[2]),
            position: unescapeHtml(target[3])
        })
    })
    var group = {
        name: $("#name").val(),
        targets: targets,
        tags: getTagInput("#tags")
    }
    // Submit the group
    if (id != -1) {
        // If we're just editing an existing group,
        // we need to PUT /groups/:id
        group.id = id
        api.groupId.put(group)
            .done(function (data) {
                successFlash("Group updated successfully!")
                load()
                dismiss()
                hideModal()
            })
            .fail(function (data) {
                modalError(data.responseJSON.message)
            })
    } else {
        // Else, if this is a new group, POST it
        // to /groups
        api.groups.post(group)
            .done(function (data) {
                successFlash("Group added successfully!")
                load()
                dismiss()
                hideModal()
            })
            .fail(function (data) {
                modalError(data.responseJSON.message)
            })
    }
}

// saveLdapConfig updates the editable synchronization settings of an existing
// managed group via PUT /api/ldap/syncs/{id} (never the blocked Group PUT).
// The connection and the selected AD group are fixed and are not sent as
// changes; the server also rejects any attempt to alter them.
function saveLdapConfig() {
    var payload = {
        id: ldapState.syncId,
        group_id: ldapState.groupId,
        connection_id: ldapState.connectionId,
        exclude_disabled: $("#ldapExcludeDisabled").is(":checked"),
        enabled: $("#ldapEnabled").is(":checked"),
        sync_freq: syncFreqSeconds(),
        attr_email: $("#ldapAttrEmail").val().trim(),
        attr_first_name: $("#ldapAttrFirst").val().trim(),
        attr_last_name: $("#ldapAttrLast").val().trim(),
        attr_position: $("#ldapAttrPosition").val().trim()
    }
    api.ldapSyncId.put(payload)
        .done(function () {
            successFlash("Synchronization settings updated!")
            load()
            dismiss()
            hideModal()
        })
        .fail(function (data) {
            modalError((data.responseJSON && data.responseJSON.message) ? data.responseJSON.message : "Unable to update the synchronization settings.")
        })
}

function dismiss() {
    $("#targetsTable").dataTable().DataTable().clear().draw()
    $("#name").val("")
    $("#name").prop("readonly", false)
    clearTagInput("#tags")
    $("#modal\\.flashes").empty()

    // Reset members source to Manual (default) and show the manual editor.
    $("#source_manual").prop("checked", true)
    $("#source_ldap").prop("checked", false)
    $("#sourceSelector input").prop("disabled", false)
    showSourceSelector(true)
    applySourceVisibility("manual")

    // Reset LDAP form + state.
    if (ldapState.searchTimer) { clearTimeout(ldapState.searchTimer) }
    ldapState = { source: "manual", managed: false, syncId: 0, groupId: 0, connectionId: 0, group: null, lastAutoName: "", searchSeq: 0, searchTimer: null }
    $("#ldapConnection").empty().prop("disabled", false).show()
    $("#ldapConnectionFixed").hide().text("")
    $("#ldapGroupPicker").show()
    $("#ldapGroupFixed").hide().text("").removeAttr("title")
    $("#ldapGroupSearch").val("").prop("disabled", false)
    $("#ldapGroupResults").hide().empty()
    $("#ldapGroupStatus").hide().empty()
    $("#ldapGroupSelectedName").text("(none)").removeAttr("title")
    $("#ldapGroupClear").hide()
    $("#ldapAttrEmail").val("")
    $("#ldapAttrFirst").val("")
    $("#ldapAttrLast").val("")
    $("#ldapAttrPosition").val("")
    $("#ldapExcludeDisabled").prop("checked", true)   // explicit default
    $("#ldapEnabled").prop("checked", true)            // explicit default
    setSyncInterval(3600) // default display: 1 Hour
    setAutoSyncEnabledState()
    $("#ldapManagedNotice").hide()
    $("#ldapNoConnections").hide()
    $("#ldapConfig").show()
    $("#ldapSyncStatusArea").hide()
    $("#ldapSyncStatus").empty()
    $("#ldapMembersArea").hide()
    $("#ldapMembersTable tbody").empty()
    $("#ldapMembersEmpty").hide()
    ldapCollapseAdvanced()
}

// applySourceVisibility shows/hides the manual vs LDAP sections.
function applySourceVisibility(source) {
    if (source === "ldap") {
        $("#manualSection").hide()
        $("#ldapSection").show()
    } else {
        $("#ldapSection").hide()
        $("#manualSection").show()
    }
}

// showSourceSelector toggles between the editable radio selector (New Group)
// and a static fixed-source label (Edit, where the source cannot change).
function showSourceSelector(editable, label) {
    if (editable) {
        $("#sourceSelector").show()
        $("#sourceFixed").hide()
    } else {
        $("#sourceSelector").hide()
        $("#sourceFixedLabel").text(label)
        $("#sourceFixed").show()
    }
}

// syncFreqSeconds converts the UI value + unit into seconds for sync_freq,
// enforcing the backend's 30-second minimum so no UI combination can request
// less than 30s. (The backend also clamps, but we avoid sending an invalid
// value in the first place.)
function syncFreqSeconds() {
    var value = parseInt($("#ldapSyncValue").val(), 10)
    if (!value || value < 1) { value = 1 }
    var unit = parseInt($("#ldapSyncUnit").val(), 10) || 3600
    var secs = value * unit
    if (secs < 30) { secs = 30 }
    return secs
}

// setSyncInterval renders stored seconds as a value + unit, choosing the
// largest unit that divides evenly (deterministic): days, then hours, then
// minutes, else falls back to minutes (rounded up).
function setSyncInterval(seconds) {
    var s = parseInt(seconds, 10)
    if (!s || s < 60) { s = 3600 } // guard; backend floor is 30 but UI unit min is minutes
    var unit, value
    if (s % 86400 === 0) {
        unit = 86400; value = s / 86400
    } else if (s % 3600 === 0) {
        unit = 3600; value = s / 3600
    } else {
        unit = 60; value = Math.max(1, Math.round(s / 60))
    }
    $("#ldapSyncUnit").val(String(unit))
    $("#ldapSyncValue").val(value)
}

// setAutoSyncEnabledState visually enables/disables the interval inputs with
// the auto-sync checkbox (the value is still sent so backend stores it).
function setAutoSyncEnabledState() {
    var on = $("#ldapEnabled").is(":checked")
    $("#ldapSyncValue, #ldapSyncUnit").prop("disabled", !on)
}

function edit(id) {
    // Clear any leftover validation/error banner from a previous modal use so
    // a freshly opened New/Edit Group never shows a stale error (e.g. a prior
    // failed save). The shared #modal is reused across operations.
    $("#modal\\.flashes").empty()
    targets = $("#targetsTable").dataTable({
        destroy: true, // Destroy any other instantiated table - http://datatables.net/manual/tech-notes/3#destroy
        columnDefs: [{
            orderable: false,
            targets: "no-sort"
        }]
    })
    $("#modalSubmit").unbind('click').click(function () {
        save(id)
    })
    initTagInput("#tags", "#modal")
    if (id == -1) {
        $("#groupModalLabel").text("New Group");
        // New group is never an existing LDAP-managed group. Establish that
        // state affirmatively here instead of relying on a prior dismiss():
        // the managed notice is driven ONLY by an existing managed mapping
        // (enterLdapManagedEdit), never by source == LDAP.
        ldapState.managed = false
        ldapState.syncId = 0
        ldapState.group = null
        ldapState.lastAutoName = ""
        $("#ldapManagedNotice").hide()
        $("#ldapMembersArea").hide()
        $("#ldapSyncStatusArea").hide()
        $("#name").prop("readonly", false)
        showSourceSelector(true)
        // Source is selectable; connections load lazily when LDAP is selected
        // (handled by the radio change handler).
    } else {
        $("#groupModalLabel").text("Edit Group");
        // Load the group first, then determine manual vs LDAP from the
        // authoritative mapping lookup.
        api.groupId.get(id)
            .done(function (group) {
                ldapState.groupId = id
                api.ldapSyncs.get(id)
                    .done(function (syncs) {
                        if (syncs && syncs.length > 0) {
                            enterLdapManagedEdit(group, syncs[0])
                        } else {
                            fillManualGroup(group)
                        }
                    })
                    .fail(function () {
                        // If the lookup fails, fall back to manual view (safe:
                        // the backend still blocks manual edits on managed groups).
                        fillManualGroup(group)
                    })
            })
            .fail(function () {
                errorFlash("Error fetching group")
            })
    }
    // Handle file uploads
    $("#csvupload").on('change', function (e) {
        $("#modal\\.flashes").empty()
        var acceptFileTypes = /(csv|txt)$/i
        var files = e.target.files
        $.each(files, function (i, file) {
            if (!acceptFileTypes.test(file.name.split(".").pop())) {
                modalError("Unsupported file extension (use .csv or .txt)")
                return
            }
            var formData = new FormData()
            formData.append('file', file)
            fetch("/api/import/group", {
                method: "POST",
                headers: {
                    'Authorization': 'Bearer ' + user.api_key
                },
                body: formData
            })
                .then(function (response) {
                    return response.json().then(function (data) {
                        if (!response.ok) {
                            throw new Error(data.message || "Error importing group")
                        }
                        return data
                    })
                })
                .then(function (result) {
                    $.each(result, function (i, record) {
                        addTarget(
                            record.first_name,
                            record.last_name,
                            record.email,
                            record.position);
                    });
                    targets.DataTable().draw();
                })
                .catch(function (err) {
                    modalError(err.message)
                })
        })
        // Allow re-selecting the same file(s) without a page reload
        e.target.value = ''
    })
}

// fillManualGroup renders a manual group in the (unchanged) manual editor.
function fillManualGroup(group) {
    $("#source_manual").prop("checked", true)
    showSourceSelector(false, "Manual")
    applySourceVisibility("manual")
    $("#name").val(group.name)
    setTagInput("#tags", group.tags)
    var targetRows = []
    $.each(group.targets, function (i, record) {
        targetRows.push([
            escapeHtml(record.first_name),
            escapeHtml(record.last_name),
            escapeHtml(record.email),
            escapeHtml(record.position),
            '<span style="cursor:pointer;"><i class="fa fa-trash-o"></i></span>'
        ])
    })
    targets.DataTable().rows.add(targetRows).draw()
}

// enterLdapManagedEdit renders an existing LDAP-managed group: name, source,
// connection, selected AD group and membership are read-only; the sync
// settings are editable; Sync Now and the last-sync status are shown.
function enterLdapManagedEdit(group, sync) {
    ldapState.source = "ldap"
    ldapState.managed = true
    ldapState.syncId = sync.id
    ldapState.groupId = group.id
    ldapState.connectionId = sync.connection_id
    ldapState.group = { directory_id: sync.group_directory_id, name: sync.group_name, dn: sync.group_dn }

    $("#source_ldap").prop("checked", true)
    showSourceSelector(false, "LDAP / Active Directory")
    applySourceVisibility("ldap")
    $("#ldapManagedNotice").show()

    // Name read-only (never renamed automatically after creation).
    $("#name").val(group.name).prop("readonly", true)
    setTagInput("#tags", group.tags)
    // Tags cannot be changed without Group PUT (blocked), so lock the input.
    try { $("#tags").prop("disabled", true).trigger("change") } catch (e) {}

    // Connection: fixed. Show its name from the non-sensitive summary list.
    $("#ldapConnection").hide()
    $("#ldapConnectionFixed").text("Loading…").show()
    api.ldapConnectionsSummary.get()
        .done(function (conns) {
            var selected = (conns || []).find(function (c) { return c.id === sync.connection_id })
            $("#ldapConnectionFixed").text(selected ? selected.name : "(connection #" + sync.connection_id + ")")
        })
        .fail(function () {
            $("#ldapConnectionFixed").text("(connection #" + sync.connection_id + ")")
        })

    // Selected AD group: fixed; last-known server-observed name (DN as tooltip).
    $("#ldapGroupPicker").hide()
    $("#ldapGroupFixed").text(sync.group_name || "(unknown group)").attr("title", sync.group_dn || "").show()

    // Editable sync settings from stored values.
    $("#ldapAttrEmail").val(sync.attr_email || "")
    $("#ldapAttrFirst").val(sync.attr_first_name || "")
    $("#ldapAttrLast").val(sync.attr_last_name || "")
    $("#ldapAttrPosition").val(sync.attr_position || "")
    $("#ldapExcludeDisabled").prop("checked", !!sync.exclude_disabled)
    $("#ldapEnabled").prop("checked", !!sync.enabled)
    setSyncInterval(sync.sync_freq || 3600)
    setAutoSyncEnabledState()

    // Last-sync status + Sync Now.
    renderLastSync(sync)
    $("#ldapSyncStatusArea").show()
    bindSyncNow(sync.id)

    // Read-only synchronized members from the already-loaded group data.
    renderSyncedMembers(group.targets || [])
}

// renderSyncedMembers fills the read-only member table from the group's
// current targets (Gophish membership after the last successful
// reconciliation - not a live directory preview).
function renderSyncedMembers(targets) {
    var rows = ""
    $.each(targets, function (i, t) {
        rows += "<tr><td>" + escapeHtml(t.first_name) + "</td><td>" + escapeHtml(t.last_name) +
            "</td><td>" + escapeHtml(t.email) + "</td><td>" + escapeHtml(t.position) + "</td></tr>"
    })
    $("#ldapMembersTable tbody").html(rows)
    if (rows) {
        $("#ldapMembersTable").show()
        $("#ldapMembersEmpty").hide()
    } else {
        $("#ldapMembersTable").hide()
        $("#ldapMembersEmpty").show()
    }
    $("#ldapMembersArea").show()
}

// --- LDAP helpers ----------------------------------------------------------

// populateConnections fills the connection dropdown from summary data.
function populateConnections(conns) {
    var $sel = $("#ldapConnection").empty()
    if (!conns || conns.length === 0) {
        $("#ldapConfig").hide()
        $("#ldapNoConnections").show()
        return
    }
    $("#ldapNoConnections").hide()
    $("#ldapConfig").show()
    $.each(conns, function (i, c) {
        $sel.append($("<option></option>").attr("value", c.id).text(c.name))
    })
}

// loadConnectionsForNew loads the team's connections for a new LDAP group.
function loadConnectionsForNew() {
    api.ldapConnectionsSummary.get()
        .done(function (conns) {
            populateConnections(conns)
            clearGroupSelection()
        })
        .fail(function () {
            modalError("Unable to load LDAP connections.")
        })
}

function currentConnectionId() {
    return parseInt($("#ldapConnection").val(), 10) || 0
}

// --- Active Directory group selector ---------------------------------------

function showGroupStatus(html, cls) {
    $("#ldapGroupStatus").removeClass("text-muted text-danger").addClass(cls || "text-muted").html(html).show()
}

// onGroupSearchInput debounces the search box so a request is not issued on
// every keystroke.
function onGroupSearchInput() {
    if (ldapState.searchTimer) { clearTimeout(ldapState.searchTimer) }
    ldapState.searchTimer = setTimeout(searchGroups, LDAP_GROUP_SEARCH_DEBOUNCE_MS)
}

// searchGroups queries GET /api/ldap/connections/:id/groups?q= for the current
// connection and renders the (bounded) result list. Stale responses from an
// earlier, slower request are ignored.
function searchGroups() {
    var q = $("#ldapGroupSearch").val().trim()
    var connId = currentConnectionId()
    $("#ldapGroupResults").hide().empty()
    if (!connId) {
        showGroupStatus("Select an LDAP connection first.")
        return
    }
    if (q.length < LDAP_GROUP_SEARCH_MIN_CHARS) {
        if (q.length === 0) {
            $("#ldapGroupStatus").hide().empty()
        } else {
            showGroupStatus("Type at least " + LDAP_GROUP_SEARCH_MIN_CHARS + " characters.")
        }
        return
    }
    var seq = ++ldapState.searchSeq
    showGroupStatus('<i class="fa fa-spinner fa-spin"></i> Searching…')
    api.ldapConnectionId.groups(connId, q)
        .done(function (items) {
            if (seq !== ldapState.searchSeq) { return }
            renderGroupResults(items || [])
        })
        .fail(function (data) {
            if (seq !== ldapState.searchSeq) { return }
            var msg = (data.responseJSON && data.responseJSON.message) ? data.responseJSON.message : "Unable to search Active Directory groups."
            showGroupStatus(escapeHtml(msg), "text-danger")
        })
}

// renderGroupResults shows the group names only (DN as tooltip); the stable
// id is kept on the element and never displayed.
function renderGroupResults(items) {
    var $list = $("#ldapGroupResults").empty()
    if (items.length === 0) {
        showGroupStatus("No groups found.")
        return
    }
    $.each(items, function (i, g) {
        var $btn = $('<button type="button" class="list-group-item list-group-item-action py-1 ldap-group-item"></button>')
            .text(g.name)
            .attr("data-id", g.directory_id)
            .attr("data-name", g.name)
            .attr("data-dn", g.dn || "")
        if (g.dn) { $btn.attr("title", g.dn) }
        $list.append($btn)
    })
    $list.show()
    if (items.length >= LDAP_GROUP_SEARCH_LIMIT) {
        showGroupStatus("Showing the first " + LDAP_GROUP_SEARCH_LIMIT + " matches. Refine your search to find other groups.")
    } else {
        $("#ldapGroupStatus").hide().empty()
    }
}

// selectGroup records the chosen AD group (id/name/dn as returned by the
// server) and auto-fills the group Name while it is still auto-managed.
function selectGroup(directoryId, name, dn) {
    ldapState.group = { directory_id: directoryId, name: name, dn: dn || "" }
    $("#ldapGroupSelectedName").text(name).attr("title", dn || "")
    $("#ldapGroupClear").show()
    $("#ldapGroupResults").hide().empty()
    $("#ldapGroupStatus").hide().empty()
    $("#ldapGroupSearch").val(name)
    applyAutoName(name)
}

// clearGroupSelection forgets the selected AD group (e.g. when the connection
// changes) and clears a Name that was still auto-filled from it.
function clearGroupSelection() {
    if (ldapState.searchTimer) { clearTimeout(ldapState.searchTimer) }
    ldapState.searchSeq++
    ldapState.group = null
    $("#ldapGroupSearch").val("")
    $("#ldapGroupResults").hide().empty()
    $("#ldapGroupStatus").hide().empty()
    $("#ldapGroupSelectedName").text("(none)").removeAttr("title")
    $("#ldapGroupClear").hide()
    if (!ldapState.managed && $("#name").val() === ldapState.lastAutoName) {
        $("#name").val("")
    }
    ldapState.lastAutoName = ""
}

// applyAutoName fills the group Name from the selected AD group, but only
// while the Name is still auto-managed: empty, or equal to the last
// auto-filled value. Once the admin types something else, selections stop
// overwriting it. Existing managed groups are never renamed.
function applyAutoName(name) {
    if (!name || ldapState.managed) { return }
    var current = $("#name").val()
    if (current !== "" && current !== ldapState.lastAutoName) { return }
    ldapState.lastAutoName = name
    $("#name").val(name)
}

function ldapCollapseAdvanced() {
    $("#ldapGroupAdvancedArea").hide()
    $("#ldapGroupAdvancedCaret").removeClass("fa-caret-down").addClass("fa-caret-right")
}
function ldapExpandAdvanced() {
    $("#ldapGroupAdvancedArea").show()
    $("#ldapGroupAdvancedCaret").removeClass("fa-caret-right").addClass("fa-caret-down")
}

// renderLastSync shows the mapping's last synchronization status and counts.
// A non-zero skipped count is explained (without any member detail).
function renderLastSync(sync) {
    var html = ""
    var when = sync.last_sync ? moment(sync.last_sync).format('MMM D, YYYY h:mm a') : ""
    if (!sync.last_sync_status || !sync.last_sync) {
        html = '<span class="badge text-bg-secondary">Never synchronized</span>'
    } else if (sync.last_sync_status === "ok") {
        html = '<span class="badge text-bg-success">Success</span> <span class="text-muted">' + when + '</span>' +
            '<div class="text-muted">Added: ' + (sync.last_sync_added || 0) +
            ' · Updated: ' + (sync.last_sync_updated || 0) +
            ' · Removed: ' + (sync.last_sync_removed || 0) +
            ' · Skipped: ' + (sync.last_sync_skipped || 0) + '</div>'
    } else {
        html = '<span class="badge text-bg-danger">Failed</span> <span class="text-muted">' + when + '</span>'
        if (sync.last_sync_error) {
            html += '<div class="text-danger">' + escapeHtml(sync.last_sync_error) + '</div>'
        }
        html += '<div class="text-muted">Membership was left unchanged.</div>'
    }
    if (sync.last_sync_skipped > 0) {
        html += '<div class="text-warning"><i class="fa fa-exclamation-triangle"></i> ' + sync.last_sync_skipped +
            ' direct member' + (sync.last_sync_skipped === 1 ? ' was' : 's were') +
            ' not synchronized because the Email attribute (' + escapeHtml(sync.attr_email || "mail") +
            ') is empty or not a valid address. See Advanced to change the attribute.</div>'
    }
    $("#ldapSyncStatus").html(html)
}

function bindSyncNow(syncId) {
    $("#ldapSyncNow").off("click").on("click", function () {
        var btn = this
        var orig = btn.innerHTML
        btn.disabled = true
        btn.innerHTML = '<i class="fa fa-spinner fa-spin"></i> Syncing…'
        api.ldapSyncId.sync(syncId)
            .done(function (res) {
                btn.disabled = false; btn.innerHTML = orig
                if (res && res.success === false) {
                    modalError(res.message || "Synchronization failed. Membership was left unchanged.")
                } else {
                    var msg = "Synchronized: " + (res.discovered || 0) + " direct members found, +" + (res.added || 0) +
                        " added, ~" + (res.updated || 0) + " updated, -" + (res.removed || 0) + " removed"
                    if (res.skipped > 0) { msg += ", " + res.skipped + " skipped (no valid email)" }
                    successFlash(msg)
                }
                refreshManagedState(ldapState.groupId)
            })
            .fail(function (data) {
                btn.disabled = false; btn.innerHTML = orig
                var msg = (data.responseJSON && data.responseJSON.message) ? data.responseJSON.message : "Synchronization failed."
                modalError(msg)
            })
    })
}

// refreshManagedState reloads the group + mapping and re-renders last-sync and
// the read-only member list after a Sync Now.
function refreshManagedState(groupId) {
    api.groupId.get(groupId).done(function (group) {
        api.ldapSyncs.get(groupId).done(function (syncs) {
            if (syncs && syncs.length > 0) {
                renderLastSync(syncs[0])
            }
            renderSyncedMembers(group.targets || [])
        })
    })
}

var downloadCSVTemplate = function () {
    var csvScope = [{
        'First Name': 'Example',
        'Last Name': 'User',
        'Email': 'foobar@example.com',
        'Position': 'Systems Administrator'
    }]
    var filename = 'group_template.csv'
    var csvString = Papa.unparse(csvScope, {})
    // Prepend a UTF-8 BOM so Excel correctly detects the
    // encoding instead of garbling non-ASCII names
    var csvData = new Blob(['\uFEFF' + csvString], {
        type: 'text/csv;charset=utf-8;'
    });
    if (navigator.msSaveBlob) {
        navigator.msSaveBlob(csvData, filename);
    } else {
        var csvURL = window.URL.createObjectURL(csvData);
        var dlLink = document.createElement('a');
        dlLink.href = csvURL;
        dlLink.setAttribute('download', filename)
        document.body.appendChild(dlLink)
        dlLink.click();
        document.body.removeChild(dlLink)
    }
}


var deleteGroup = function (id, el) {
    var group = groups.find(function (x) {
        return x.id === id
    })
    if (!group) {
        return
    }
    Swal.fire({
        title: "Are you sure?",
        text: "This will delete the group. This can't be undone!",
        icon: "warning",
        animation: false,
        showCancelButton: true,
        confirmButtonText: "Delete " + escapeHtml(group.name),
        confirmButtonColor: "#428bca",
        reverseButtons: true,
        allowOutsideClick: false,
        preConfirm: function () {
            return new Promise(function (resolve, reject) {
                api.groupId.delete(id)
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
            $(el).closest('table').DataTable().row($(el).closest('tr')).remove().draw(false)
            Swal.fire(
                'Group Deleted!',
                'This group has been deleted!',
                'success'
            );
        }
    })
}

function addTarget(firstNameInput, lastNameInput, emailInput, positionInput) {
    // Create new data row.
    var email = escapeHtml(emailInput).toLowerCase();
    var newRow = [
        escapeHtml(firstNameInput),
        escapeHtml(lastNameInput),
        email,
        escapeHtml(positionInput),
        '<span style="cursor:pointer;"><i class="fa fa-trash-o"></i></span>'
    ];

    // Check table to see if email already exists.
    var targetsTable = targets.DataTable();
    var existingRowIndex = targetsTable
        .column(2, {
            order: "index"
        }) // Email column has index of 2
        .data()
        .indexOf(email);
    // Update or add new row as necessary.
    if (existingRowIndex >= 0) {
        targetsTable
            .row(existingRowIndex, {
                order: "index"
            })
            .data(newRow);
    } else {
        targetsTable.row.add(newRow);
    }
}

function load() {
    $("#groupTable").hide()
    $("#emptyMessage").hide()
    $("#loading").show()
    // One list call to learn which groups are LDAP-managed (for the badge).
    // Never per-row (?group_id=) - a single /ldap/syncs/ call, no N+1.
    managedGroupIds = {}
    api.ldapSyncs.get()
        .done(function (syncs) {
            $.each(syncs || [], function (i, s) { managedGroupIds[s.group_id] = true })
        })
        .always(function () {
            loadGroups()
        })
}

var managedGroupIds = {}

function loadGroups() {
    api.groups.summary()
        .done(function (response) {
            $("#loading").hide()
            if (response.total > 0) {
                groups = response.groups
                $("#emptyMessage").hide()
                $("#groupTable").show()
                var $tagFilter = buildTagFilter(collectTags(groups))
                var groupTable = $("#groupTable").DataTable({
                    destroy: true,
                    columnDefs: [{
                        orderable: false,
                        targets: "no-sort"
                    }],
                    layout: tableLayout($tagFilter)
                });
                groupTable.clear();
                groupRows = []
                $.each(groups, function (i, group) {
                    groupRows.push([
                        escapeHtml(group.name) + (managedGroupIds[group.id] ? ' <span class="badge text-bg-info">LDAP</span>' : ''),
                        renderTagChips(group.tags),
                        escapeHtml(group.num_targets),
                        moment(group.modified_date).format('MMM D, YYYY h:mm a'),
                        (canModifyObjects() ? "<div class='pull-right'><button class='btn btn-sm btn-primary' data-bs-toggle='modal' data-bs-backdrop='static' data-bs-target='#modal' onclick='edit(" + group.id + ")'>\
                    <i class='fa fa-pencil'></i>\
                    </button>\
                    <button class='btn btn-sm btn-danger' onclick='deleteGroup(" + group.id + ", this)'>\
                    <i class='fa fa-trash-o'></i>\
                    </button></div>" : "")
                    ])
                })
                groupTable.rows.add(groupRows).draw()
                wireTagFilter($tagFilter, groupTable)
                loadTagSuggestions()
            } else {
                $("#emptyMessage").show()
            }
        })
        .fail(function () {
            errorFlash("Error fetching groups")
        })
}

$(document).ready(function () {
    load()
    // Setup the event listeners
    // Handle manual additions
    $("#targetForm").submit(function () {
        // Validate the form data
        var targetForm = document.getElementById("targetForm")
        if (!targetForm.checkValidity()) {
            targetForm.reportValidity()
            return
        }
        addTarget(
            $("#firstName").val(),
            $("#lastName").val(),
            $("#email").val(),
            $("#position").val());
        targets.DataTable().draw();

        // Reset user input.
        $("#targetForm>div>input").val('');
        $("#firstName").focus();
        return false;
    });
    // Handle Deletion
    $("#targetsTable").on("click", "span>i.fa-trash-o", function () {
        targets.DataTable()
            .row($(this).parents('tr'))
            .remove()
            .draw();
    });
    $("#modal").on("hide.bs.modal", function () {
        dismiss();
    });
    $("#csv-template").click(downloadCSVTemplate)

    // --- LDAP source wiring ---
    // Members source toggle. The radios are only interactive for a NEW group
    // (in Edit the selector is replaced by a static label), so toggling source
    // here is always a non-managed context: the managed notice must stay
    // hidden regardless of the chosen source.
    $("input[name=membersource]").on("change", function () {
        var src = currentSource()
        applySourceVisibility(src)
        // The managed/read-only banner belongs ONLY to an existing managed
        // group (enterLdapManagedEdit). It is never driven by source == LDAP.
        $("#ldapManagedNotice").hide()
        $("#ldapMembersArea").hide()
        $("#ldapSyncStatusArea").hide()
        $("#name").prop("readonly", false)
        if (src === "ldap") {
            // Lazily load connections the first time LDAP is selected.
            if ($("#ldapConnection option").length === 0) {
                loadConnectionsForNew()
            }
        }
    })
    // Changing the connection (new groups) forgets the selected AD group and
    // any Name that was still auto-filled from it.
    $("#ldapConnection").on("change", function () {
        if (!ldapState.managed) {
            clearGroupSelection()
        }
    })
    // Advanced attribute mapping toggle.
    $("#ldapGroupAdvancedToggle").on("click", function () {
        if ($("#ldapGroupAdvancedArea").is(":visible")) { ldapCollapseAdvanced() } else { ldapExpandAdvanced() }
    })
    // Enable/disable the interval inputs with the auto-sync checkbox.
    $("#ldapEnabled").on("change", setAutoSyncEnabledState)
    // Active Directory group selector: debounced search, click to select,
    // "change" to forget the selection.
    $("#ldapGroupSearch").on("input", onGroupSearchInput)
    $("#ldapGroupSearch").on("keydown", function (e) {
        if (e.key === "Enter") {
            e.preventDefault()
            var $first = $("#ldapGroupResults .ldap-group-item").first()
            if ($first.length) { $first.trigger("click") }
        }
    })
    $("#ldapGroupResults").on("click", ".ldap-group-item", function () {
        selectGroup($(this).attr("data-id"), $(this).attr("data-name"), $(this).attr("data-dn"))
    })
    $("#ldapGroupClear").on("click", function () {
        clearGroupSelection()
        $("#ldapGroupSearch").trigger("focus")
    })
});

$(document).ready(function () {
    initTooltips();
    $("#apiResetForm").submit(function (e) {
        api.reset()
            .done(function (response) {
                user.api_key = response.data
                successFlash(response.message)
                $("#api_key").val(user.api_key)
            })
            .fail(function (data) {
                errorFlash(data.message)
            })
        return false
    })
    $("#settingsForm").submit(function (e) {
        $.post("/settings", $(this).serialize())
            .done(function (data) {
                successFlash(data.message)
            })
            .fail(function (data) {
                errorFlash(data.responseJSON.message)
            })
        return false
    })
    //$("#imapForm").submit(function (e) {
    // Builds and POSTs the IMAP settings from the form. Returns the jqXHR
    // for the save request, or null if client-side validation failed (in
    // which case an error flash was already shown and nothing was sent).
    // Shared by the "Save" button and "Connect Account", which must save
    // the just-entered OAuth2 config before starting the redirect out to
    // the provider - otherwise the callback finds no saved config to use.
    function saveIMAPSettings() {
        var imapSettings = {}
        imapSettings.host = $("#imaphost").val()
        imapSettings.port = $("#imapport").val()
        imapSettings.username = $("#imapusername").val()
        imapSettings.enabled = $('#use_imap').prop('checked')
        imapSettings.tls = $('#use_tls').prop('checked')
        imapSettings.auth_type = $('input[name=authtype]:checked').val()

        if (imapSettings.auth_type == "oauth2") {
            imapSettings.oauth_provider = $("#oauthprovider").val()
            imapSettings.oauth_tenant_id = $("#oauthtenantid").val()
            imapSettings.oauth_client_id = $("#oauthclientid").val()
            imapSettings.oauth_client_secret = $("#oauthclientsecret").val()
            imapSettings.oauth_auth_url = $("#oauthauthurl").val()
            imapSettings.oauth_token_url = $("#oauthtokenurl").val()
            imapSettings.oauth_scopes = $("#oauthscopes").val()
        } else {
            imapSettings.password = $("#imappassword").val()
        }

        //Advanced settings
        imapSettings.folder = $("#folder").val()
        imapSettings.imap_freq = $("#imapfreq").val()
        imapSettings.restrict_domain = $("#restrictdomain").val()
        imapSettings.ignore_cert_errors = $('#ignorecerterrors').prop('checked')
        imapSettings.delete_reported_campaign_email = $('#deletecampaign').prop('checked')

        //To avoid unmarshalling error in controllers/api/imap.go. It would fail gracefully, but with a generic error.
        if (imapSettings.host == ""){
            errorFlash("No IMAP Host specified")
            document.body.scrollTop = 0;
            document.documentElement.scrollTop = 0;
            return null
        }
        if (imapSettings.port == ""){
            errorFlash("No IMAP Port specified")
            document.body.scrollTop = 0;
            document.documentElement.scrollTop = 0;
            return null
        }
        if (isNaN(imapSettings.port) || imapSettings.port <1 || imapSettings.port > 65535  ){
            errorFlash("Invalid IMAP Port")
            document.body.scrollTop = 0;
            document.documentElement.scrollTop = 0;
            return null
        }
        if (imapSettings.imap_freq == ""){
            imapSettings.imap_freq = "60"
        }

        return api.IMAP.post(imapSettings).done(function (data) {
                if (data.success == true) {
                    successFlashFade("Successfully updated IMAP settings.", 2)
                } else {
                    errorFlash("Unable to update IMAP settings.")
                }
            })
            .done(function (data){
                loadIMAPSettings()
            })
            .fail(function (data) {
                errorFlash(data.responseJSON.message)
            })
            .always(function (data){
                document.body.scrollTop = 0;
                document.documentElement.scrollTop = 0;
            })
    }

    $("#savesettings").click(function() {
        saveIMAPSettings()
        return false
    })

    $("#oauthconnect").click(function(e) {
        e.preventDefault()
        var authorizeUrl = $(this).attr("href")
        var req = saveIMAPSettings()
        if (!req) {
            return false
        }
        req.done(function(data) {
            if (data.success == true) {
                window.location.href = authorizeUrl
            }
        })
        return false
    })

    $("#validateimap").click(function() {

        // Query validate imap server endpoint
        var server = {}
        server.host = $("#imaphost").val()
        server.port = $("#imapport").val()
        server.username = $("#imapusername").val()
        server.tls = $('#use_tls').prop('checked')
        server.ignore_cert_errors = $('#ignorecerterrors').prop('checked')
        server.auth_type = $('input[name=authtype]:checked').val()
        if (server.auth_type != "oauth2") {
            server.password = $("#imappassword").val()
        }

        //To avoid unmarshalling error in controllers/api/imap.go. It would fail gracefully, but with a generic error. 
        if (server.host == ""){
            errorFlash("No IMAP Host specified")
            document.body.scrollTop = 0;
            document.documentElement.scrollTop = 0;
            return false
        }
        if (server.port == ""){
            errorFlash("No IMAP Port specified")
            document.body.scrollTop = 0;
            document.documentElement.scrollTop = 0;
            return false
        }
        if (isNaN(server.port) || server.port <1 || server.port > 65535  ){
            errorFlash("Invalid IMAP Port")
            document.body.scrollTop = 0;
            document.documentElement.scrollTop = 0;
            return false
        }

        var oldHTML = $("#validateimap").html();
        // Disable inputs and change button text
        $("#imaphost").attr("disabled", true);
        $("#imapport").attr("disabled", true);
        $("#imapusername").attr("disabled", true);
        $("#imappassword").attr("disabled", true);
        $("#use_imap").attr("disabled", true);
        $("#use_tls").attr("disabled", true);
        $('#ignorecerterrors').attr("disabled", true);
        $("#folder").attr("disabled", true);
        $("#restrictdomain").attr("disabled", true);
        $('#deletecampaign').attr("disabled", true);
        $('#lastlogin').attr("disabled", true);
        $('#imapfreq').attr("disabled", true);
        $("#validateimap").attr("disabled", true);  
        $("#validateimap").html("<i class='fa fa-circle-o-notch fa-spin'></i> Testing...");
        
        api.IMAP.validate(server).done(function(data) {
            if (data.success == true) {
                Swal.fire({
                    title: "Success",
                    html: "Logged into <b>" + escapeHtml($("#imaphost").val()) + "</b>",
                    icon: "success",
                })
            } else {
                Swal.fire({
                    title: "Failed!",
                    html: "Unable to login to <b>" + escapeHtml($("#imaphost").val()) + "</b>.",
                    icon: "error",
                    showCancelButton: true,
                    cancelButtonText: "Close",
                    confirmButtonText: "More Info",
                    confirmButtonColor: "#428bca",
                    allowOutsideClick: false,
                }).then(function(result) {
                    if (result.value) {
                        Swal.fire({
                            title: "Error:",
                            text: data.message,
                        })
                    }
                  })
            }
            
          })
          .fail(function() {
            Swal.fire({
                title: "Failed!",
                text: "An unecpected error occured.",
                icon: "error",
            })
          })
          .always(function() {
            //Re-enable inputs and change button text
            $("#imaphost").attr("disabled", false);
            $("#imapport").attr("disabled", false);
            $("#imapusername").attr("disabled", false);
            $("#imappassword").attr("disabled", false);
            $("#use_imap").attr("disabled", false);
            $("#use_tls").attr("disabled", false);
            $('#ignorecerterrors').attr("disabled", false);
            $("#folder").attr("disabled", false);
            $("#restrictdomain").attr("disabled", false);
            $('#deletecampaign').attr("disabled", false);
            $('#lastlogin').attr("disabled", false);
            $('#imapfreq').attr("disabled", false);
            $("#validateimap").attr("disabled", false);
            $("#validateimap").html(oldHTML);

          });

      }); //end testclick

    $("#reporttab").click(function() {
        loadIMAPSettings()
    })

    // The OAuth2 connect flow is a real browser redirect out to the
    // provider and back (see controllers/oauth_imap.go), which reloads
    // this page from scratch. Without this, that reload always lands back
    // on the default "Account Settings" tab, discarding the fact that the
    // user was on Reporting Settings. The backend appends this hash to its
    // redirect back to /settings so the right tab re-opens.
    if (location.hash === "#reportingSettings") {
        showTab('.nav-tabs a[href="#reportingSettings"]')
    }

    $("#advanced").click(function() {
        $("#advancedarea").toggle();
    })

    function updateAuthTypeFields(){
        var authType = $('input[name=authtype]:checked').val()
        if (authType == "oauth2") {
            $("#basicauthfields").hide()
            $("#oauth2fields").show()
            $("#oauth2hostnote").show()
        } else {
            $("#basicauthfields").show()
            $("#oauth2fields").hide()
            $("#oauth2hostnote").hide()
        }
    }
    $('input[name=authtype]').on('change', updateAuthTypeFields)

    function updateOAuthProviderFields(){
        var provider = $("#oauthprovider").val()
        $("#oauthtenantiddiv").toggle(provider == "microsoft")
        $("#oauthcustomfields").toggle(provider == "custom")
    }
    $("#oauthprovider").on('change', updateOAuthProviderFields)

    function loadIMAPSettings(){
        api.IMAP.get()
        .done(function (imap) {
            if (imap.length == 0){
                $('#lastlogindiv').hide()
            } else {
                imap = imap[0]
                if (imap.enabled == false){
                    $('#lastlogindiv').hide()
                } else {
                    $('#lastlogindiv').show()
                }
                $("#imapusername").val(imap.username)
                $("#imaphost").val(imap.host)
                $("#imapport").val(imap.port)
                $('#use_tls').prop('checked', imap.tls)
                $('#ignorecerterrors').prop('checked', imap.ignore_cert_errors)
                $('#use_imap').prop('checked', imap.enabled)
                $("#folder").val(imap.folder)
                $("#restrictdomain").val(imap.restrict_domain)
                $('#deletecampaign').prop('checked', imap.delete_reported_campaign_email)
                $('#lastloginraw').val(imap.last_login)
                $('#lastlogin').val(moment.utc(imap.last_login).fromNow())
                $('#imapfreq').val(imap.imap_freq)

                if (imap.last_login_error) {
                    $('#lastloginerror').val(imap.last_login_error)
                    $('#lastloginerrordiv').show()
                } else {
                    $('#lastloginerrordiv').hide()
                }

                if (imap.non_campaign_emails_count > 0) {
                    $('#noncampaigncount').val(imap.non_campaign_emails_count)
                    $('#noncampaigncountdiv').show()
                } else {
                    $('#noncampaigncountdiv').hide()
                }

                var authType = imap.auth_type || "basic"
                $("#authtype_basic").prop('checked', authType == "basic")
                $("#authtype_oauth2").prop('checked', authType == "oauth2")
                $("#oauthprovider").val(imap.oauth_provider || "google")
                $("#oauthtenantid").val(imap.oauth_tenant_id)
                $("#oauthclientid").val(imap.oauth_client_id)
                $("#oauthauthurl").val(imap.oauth_auth_url)
                $("#oauthtokenurl").val(imap.oauth_token_url)
                $("#oauthscopes").val(imap.oauth_scopes)
                if (imap.oauth_connected) {
                    $("#oauthstatus").text("Connected").removeClass("text-bg-secondary text-bg-danger").addClass("text-bg-success")
                    $("#oauthconnect").text("Reconnect Account")
                } else {
                    $("#oauthstatus").text("Not Connected").removeClass("text-bg-success").addClass("text-bg-secondary")
                    $("#oauthconnect").text("Connect Account")
                }
                updateAuthTypeFields()
                updateOAuthProviderFields()
            }

        })
        .fail(function () {
            errorFlash("Error fetching IMAP settings")
        })
    }

    var use_map = localStorage.getItem('gophish.use_map')
    $("#use_map").prop('checked', JSON.parse(use_map))
    $("#use_map").on('change', function () {
        localStorage.setItem('gophish.use_map', JSON.stringify(this.checked))
    })

    loadIMAPSettings()

    // --- LDAP / Active Directory connections (Issue #78) ---------------------
    // Only wired when the LDAP tab is present, which the template renders
    // exclusively for users with the modify_system permission. The backend
    // API enforces that same permission regardless of UI visibility.
    if (document.getElementById("ldapSettings")) {
        initLDAPSettings()
    }
})

function initLDAPSettings() {
    var ldapConnections = []

    // ldapFlash shows a page-level LDAP alert in the LDAP tab's own flash area
    // (namespaced so it never collides with the shared #flashes element).
    function ldapFlash(type, message) {
        var icon = type === "success" ? "fa-check-circle" : "fa-exclamation-circle"
        var cls = type === "success" ? "alert-success" : "alert-danger"
        $("#ldapFlashes").empty().append(
            '<div style="text-align:center" class="alert ' + cls + '">' +
            '<i class="fa ' + icon + '"></i> ' + message + '</div>')
    }
    function ldapModalFlash(message) {
        $("#ldapModalFlashes").empty().append(
            '<div style="text-align:center" class="alert alert-danger">' +
            '<i class="fa fa-exclamation-circle"></i> ' + message + '</div>')
    }

    function collapseAdvanced() {
        $("#ldapAdvancedArea").hide()
        $("#ldapAdvancedCaret").removeClass("fa-caret-down").addClass("fa-caret-right")
    }
    function expandAdvanced() {
        $("#ldapAdvancedArea").show()
        $("#ldapAdvancedCaret").removeClass("fa-caret-right").addClass("fa-caret-down")
    }

    function dismiss() {
        $("#ldap_name").val("")
        $("#ldap_server_url").val("")
        $("#ldap_base_dn").val("")
        $("#ldap_bind_dn").val("")
        $("#ldap_bind_password").val("")
        $("#ldap_start_tls").prop("checked", false)
        updateTlsWarning()
        // Certificate verification stays ENABLED by default: ignore-cert-errors
        // is an opt-in, insecure option, so it starts unchecked.
        $("#ldap_ignore_cert_errors").prop("checked", false)
        // Fresh Add defaults to Active Directory (the primary target); this
        // only takes effect when the admin explicitly saves. Existing
        // connections derive their type from the stored value on load and are
        // never silently rewritten (see setDirectoryTypeFromAttr).
        $("#ldap_directory_type").val("ad")
        $("#ldap_user_identity").val("entryUUID")
        syncUserIdentityVisibility()
        $("#ldap_password_help").text("Service-account password for the bind DN.")
        collapseAdvanced()
        $("#ldapModalFlashes").empty()
    }

    function renderTestStatus(status, error) {
        if (status === "ok") {
            return '<span class="badge text-bg-success">Success</span>'
        }
        if (status === "error") {
            var title = error ? ' title="' + escapeHtml(error) + '"' : ''
            return '<span class="badge text-bg-danger" data-bs-toggle="tooltip"' + title + '>Failed</span>'
        }
        return '<span class="badge text-bg-secondary">Not tested</span>'
    }

    function load() {
        $("#ldapEmptyState").hide()
        $("#ldapTableCard").hide()
        $("#ldapLoading").show()
        api.ldapConnections.get()
            .done(function (cs) {
                ldapConnections = cs || []
                $("#ldapLoading").hide()
                if (ldapConnections.length === 0) {
                    $("#ldapEmptyState").show()
                    return
                }
                $("#ldapTableCard").show()
                var table = $("#ldapTable").DataTable({
                    destroy: true,
                    columnDefs: [{ orderable: false, targets: "no-sort" }]
                })
                table.clear()
                $.each(ldapConnections, function (i, c) {
                    table.row.add([
                        escapeHtml(c.name),
                        escapeHtml(c.server_url),
                        escapeHtml(c.base_dn),
                        renderTestStatus(c.last_test_status, c.last_test_error),
                        '<div class="pull-right">' +
                        '<button class="btn btn-sm btn-secondary ldap_test_button" data-ldap-id="' + c.id + '" title="Test Connection"><i class="fa fa-wrench"></i></button> ' +
                        '<button class="btn btn-sm btn-primary ldap_edit_button" data-bs-toggle="modal" data-bs-backdrop="static" data-bs-target="#ldapModal" data-ldap-id="' + c.id + '" title="Edit"><i class="fa fa-pencil"></i></button> ' +
                        '<button class="btn btn-sm btn-danger ldap_delete_button" data-ldap-id="' + c.id + '" title="Delete"><i class="fa fa-trash-o"></i></button>' +
                        '</div>'
                    ]).draw(false)
                })
                initTooltips()
            })
            .fail(function () {
                $("#ldapLoading").hide()
                ldapFlash("error", "Error fetching LDAP connections")
            })
    }

    // directoryIdAttrFromUI maps the Directory type (+ User identity for
    // Generic LDAP) selection to the persisted directory_id_attr:
    //   Active Directory            -> "objectGUID"
    //   Generic LDAP / entryUUID    -> "entryUUID"
    //   Generic LDAP / Email        -> ""
    function directoryIdAttrFromUI() {
        if ($("#ldap_directory_type").val() === "ad") {
            return "objectGUID"
        }
        return $("#ldap_user_identity").val() // "entryUUID" or ""
    }

    // setDirectoryTypeFromAttr derives the Directory type / User identity UI
    // from a stored directory_id_attr WITHOUT ever silently upgrading an empty
    // value to objectGUID:
    //   "objectGUID" -> Active Directory
    //   "entryUUID"  -> Generic LDAP / entryUUID
    //   "" / null    -> Generic LDAP / Email fallback
    function setDirectoryTypeFromAttr(attr) {
        attr = attr || ""
        if (attr === "objectGUID") {
            $("#ldap_directory_type").val("ad")
            $("#ldap_user_identity").val("entryUUID")
        } else {
            $("#ldap_directory_type").val("generic")
            $("#ldap_user_identity").val(attr === "entryUUID" ? "entryUUID" : "")
        }
        syncUserIdentityVisibility()
    }

    // syncUserIdentityVisibility shows the User identity select only for
    // Generic LDAP.
    // updateTlsWarning shows the unencrypted-transport warning when the
    // server URL is ldap:// and STARTTLS is not enabled.
    function updateTlsWarning() {
        var url = ($("#ldap_server_url").val() || "").trim().toLowerCase()
        var plain = url.indexOf("ldap://") === 0 && !$("#ldap_start_tls").is(":checked")
        $("#ldap_tls_warning").toggle(plain)
    }

    function syncUserIdentityVisibility() {
        if ($("#ldap_directory_type").val() === "generic") {
            $("#ldap_user_identity_group").show()
        } else {
            $("#ldap_user_identity_group").hide()
        }
    }

    function buildConnection(id) {
        var c = {
            name: $("#ldap_name").val(),
            server_url: $("#ldap_server_url").val(),
            base_dn: $("#ldap_base_dn").val(),
            bind_dn: $("#ldap_bind_dn").val(),
            start_tls: $("#ldap_start_tls").is(":checked"),
            ignore_cert_errors: $("#ldap_ignore_cert_errors").is(":checked"),
            directory_id_attr: directoryIdAttrFromUI()
        }
        // A blank password is intentionally omitted so the backend preserves
        // the existing stored credential on edit.
        var pw = $("#ldap_bind_password").val()
        if (pw !== "") {
            c.bind_password = pw
        }
        if (id != -1) {
            c.id = parseInt(id)
        }
        return c
    }

    function saveConnection(id) {
        var c = buildConnection(id)
        if (id != -1) {
            api.ldapConnectionId.put(c)
                .done(function () {
                    dismiss(); load(); hideModal("#ldapModal")
                    ldapFlash("success", 'Connection "' + escapeHtml(c.name) + '" has been updated successfully!')
                })
                .fail(function (data) { ldapModalFlash(data.responseJSON.message) })
        } else {
            api.ldapConnections.post(c)
                .done(function () {
                    dismiss(); load(); hideModal("#ldapModal")
                    ldapFlash("success", 'Connection "' + escapeHtml(c.name) + '" has been created successfully!')
                })
                .fail(function (data) { ldapModalFlash(data.responseJSON.message) })
        }
    }

    // setTestEnabled toggles the modal Test button and its tooltip hint. The
    // backend tests a persisted connection by id, so testing is only possible
    // once the connection exists (edit mode).
    function setTestEnabled(enabled) {
        var btn = document.getElementById("ldapModalTest")
        var wrap = document.getElementById("ldapModalTestWrap")
        if (btn) {
            btn.disabled = !enabled
            // Give the enabled (edit) state an actionable appearance instead
            // of the muted gray that reads as disabled; the disabled (add)
            // state keeps the plain secondary look plus the native disabled
            // dimming.
            if (enabled) {
                btn.classList.remove("btn-secondary")
                btn.classList.add("btn-outline-primary")
            } else {
                btn.classList.remove("btn-outline-primary")
                btn.classList.add("btn-secondary")
            }
        }
        if (wrap) {
            var tip = bootstrap.Tooltip.getInstance(wrap)
            if (enabled) {
                // Remove the "save first" hint once testing is available.
                if (tip) { tip.dispose() }
                wrap.removeAttribute("data-bs-original-title")
            } else {
                // (Re)create the hint tooltip for the disabled state.
                wrap.setAttribute("title", "Save the connection before testing it.")
                bootstrap.Tooltip.getOrCreateInstance(wrap)
            }
        }
    }

    function editConnection(id) {
        $("#ldapModalSubmit").unbind("click").click(function () { saveConnection(id) })
        $("#ldapModalTest").unbind("click").click(function () {
            // Guarded by the disabled state in add mode; still a no-op safety net.
            if (id == -1) { return }
            testConnection(id, $("#ldapModalTest").get(0), true)
        })
        // New connections cannot be tested until saved: disable + show hint.
        setTestEnabled(id != -1)
        if (id != -1) {
            $("#ldapModalLabel").text("Edit Connection")
            // The backend never returns secrets, so the password field stays
            // blank and a blank value on save preserves the stored credential.
            $("#ldap_password_help").text("Leave blank to keep the existing password.")
            api.ldapConnectionId.get(id)
                .done(function (c) {
                    $("#ldap_name").val(c.name)
                    $("#ldap_server_url").val(c.server_url)
                    $("#ldap_base_dn").val(c.base_dn)
                    $("#ldap_bind_dn").val(c.bind_dn)
                    $("#ldap_bind_password").val("")
                    $("#ldap_start_tls").prop("checked", !!c.start_tls)
                    updateTlsWarning()
                    $("#ldap_ignore_cert_errors").prop("checked", !!c.ignore_cert_errors)
                    // Derive Directory type / User identity from the stored
                    // directory_id_attr; an empty value loads as Generic LDAP /
                    // Email fallback, never silently objectGUID.
                    setDirectoryTypeFromAttr(c.directory_id_attr || "")
                    if (c.ignore_cert_errors) {
                        expandAdvanced()
                    }
                })
                .fail(function () { ldapFlash("error", "Error fetching LDAP connection") })
        } else {
            $("#ldapModalLabel").text("Add Connection")
        }
    }

    function testConnection(id, btn, inModal) {
        var original = btn ? btn.innerHTML : ""
        if (btn) {
            btn.disabled = true
            btn.innerHTML = '<i class="fa fa-spinner fa-spin"></i> Testing...'
        }
        api.ldapConnectionId.test(id)
            .done(function (resp) {
                if (btn) { btn.disabled = false; btn.innerHTML = original }
                if (resp && resp.success) {
                    if (inModal) {
                        $("#ldapModalFlashes").empty().append(
                            '<div style="text-align:center" class="alert alert-success">' +
                            '<i class="fa fa-check-circle"></i> Connection successful</div>')
                    } else {
                        ldapFlash("success", "Connection successful")
                    }
                } else {
                    var msg = resp && resp.message ? resp.message : "Connection failed"
                    if (inModal) { ldapModalFlash("Connection failed: " + escapeHtml(msg)) }
                    else { ldapFlash("error", "Connection failed: " + escapeHtml(msg)) }
                }
                load()
            })
            .fail(function (data) {
                if (btn) { btn.disabled = false; btn.innerHTML = original }
                var msg = (data.responseJSON && data.responseJSON.message) ? data.responseJSON.message : "Connection failed"
                if (inModal) { ldapModalFlash("Connection failed: " + escapeHtml(msg)) }
                else { ldapFlash("error", "Connection failed: " + escapeHtml(msg)) }
                load()
            })
    }

    function deleteConnection(id) {
        var c = ldapConnections.find(function (x) { return x.id == id })
        if (!c) { return }
        Swal.fire({
            title: "Are you sure?",
            text: 'This will delete the LDAP connection "' + c.name + '".',
            icon: "warning",
            animation: false,
            showCancelButton: true,
            confirmButtonText: "Delete",
            confirmButtonColor: "#428bca",
            reverseButtons: true,
            allowOutsideClick: false,
            preConfirm: function () {
                return new Promise(function (resolve, reject) {
                    api.ldapConnectionId.delete(id)
                        .done(function () { resolve() })
                        .fail(function (data) { reject(data.responseJSON.message) })
                }).catch(function (error) {
                    Swal.showValidationMessage(error)
                })
            }
        }).then(function (result) {
            if (result.value) {
                Swal.fire("Connection Deleted!", "The LDAP connection has been deleted.", "success")
                load()
            }
        })
    }

    load()
    $("#ldapModal").on("hide.bs.modal", function () { dismiss() })
    $(document).on("click", "button[data-bs-target='#ldapModal'][data-ldap-id='-1']", function () {
        editConnection(-1)
    })
    $("#ldapAdvancedToggle").on("click", function () {
        if ($("#ldapAdvancedArea").is(":visible")) { collapseAdvanced() } else { expandAdvanced() }
    })
    $("#ldap_directory_type").on("change", syncUserIdentityVisibility)
    $("#ldap_server_url").on("input", updateTlsWarning)
    $("#ldap_start_tls").on("change", updateTlsWarning)
    $("#ldapTable").on("click", ".ldap_edit_button", function () {
        editConnection($(this).attr("data-ldap-id"))
    })
    $("#ldapTable").on("click", ".ldap_test_button", function (e) {
        testConnection($(this).attr("data-ldap-id"), e.currentTarget, false)
    })
    $("#ldapTable").on("click", ".ldap_delete_button", function () {
        deleteConnection($(this).attr("data-ldap-id"))
    })
}

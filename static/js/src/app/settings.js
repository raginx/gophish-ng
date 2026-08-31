$(document).ready(function () {
    $('[data-toggle="tooltip"]').tooltip();
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
                    type: "success",
                })
            } else {
                Swal.fire({
                    title: "Failed!",
                    html: "Unable to login to <b>" + escapeHtml($("#imaphost").val()) + "</b>.",
                    type: "error",
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
                type: "error",
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
        $('.nav-tabs a[href="#reportingSettings"]').tab('show')
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
                    $("#oauthstatus").text("Connected").removeClass("label-default label-danger").addClass("label-success")
                    $("#oauthconnect").text("Reconnect Account")
                } else {
                    $("#oauthstatus").text("Not Connected").removeClass("label-success").addClass("label-default")
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
})

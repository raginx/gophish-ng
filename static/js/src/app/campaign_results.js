var map = null
var doPoll = true;

// statuses is a helper map to point result statuses to ui classes
var statuses = {
    "Email Sent": {
        color: "#1abc9c",
        label: "label-success",
        icon: "fa-envelope",
        point: "ct-point-sent"
    },
    "Emails Sent": {
        color: "#1abc9c",
        label: "label-success",
        icon: "fa-envelope",
        point: "ct-point-sent"
    },
    "In progress": {
        label: "label-primary"
    },
    "Queued": {
        label: "label-info"
    },
    "Completed": {
        label: "label-success"
    },
    "Email Opened": {
        color: "#f9bf3b",
        label: "label-warning",
        icon: "fa-envelope-open",
        point: "ct-point-opened"
    },
    "Clicked Link": {
        color: "#F39C12",
        label: "label-clicked",
        icon: "fa-mouse-pointer",
        point: "ct-point-clicked"
    },
    "Success": {
        color: "#f05b4f",
        label: "label-danger",
        icon: "fa-exclamation",
        point: "ct-point-clicked"
    },
    //not a status, but is used for the campaign timeline and user timeline
    "Email Reported": {
        color: "#45d6ef",
        label: "label-info",
        icon: "fa-bullhorn",
        point: "ct-point-reported"
    },
    "Error": {
        color: "#6c7a89",
        label: "label-default",
        icon: "fa-times",
        point: "ct-point-error"
    },
    "Error Sending Email": {
        color: "#6c7a89",
        label: "label-default",
        icon: "fa-times",
        point: "ct-point-error"
    },
    "Submitted Data": {
        color: "#f05b4f",
        label: "label-danger",
        icon: "fa-exclamation",
        point: "ct-point-clicked"
    },
    "Unknown": {
        color: "#6c7a89",
        label: "label-default",
        icon: "fa-question",
        point: "ct-point-error"
    },
    "Sending": {
        color: "#428bca",
        label: "label-primary",
        icon: "fa-spinner",
        point: "ct-point-sending"
    },
    "Retrying": {
        color: "#6c7a89",
        label: "label-default",
        icon: "fa-clock-o",
        point: "ct-point-error"
    },
    "Scheduled": {
        color: "#428bca",
        label: "label-primary",
        icon: "fa-clock-o",
        point: "ct-point-sending"
    },
    "Campaign Created": {
        label: "label-success",
        icon: "fa-rocket"
    }
}

var statusMapping = {
    "Email Sent": "sent",
    "Email Opened": "opened",
    "Clicked Link": "clicked",
    "Submitted Data": "submitted_data",
    "Email Reported": "reported",
}

// This is an underwhelming attempt at an enum
// until I have time to refactor this appropriately.
var progressListing = [
    "Email Sent",
    "Email Opened",
    "Clicked Link",
    "Submitted Data"
]

var campaign = {}
var bubbles = []

function dismiss() {
    $("#modal\\.flashes").empty()
    $("#modal").modal('hide')
    $("#resultsTable").dataTable().DataTable().clear().draw()
}

// Deletes a campaign after prompting the user
function deleteCampaign() {
    Swal.fire({
        title: "Are you sure?",
        text: "This will delete the campaign. This can't be undone!",
        type: "warning",
        animation: false,
        showCancelButton: true,
        confirmButtonText: "Delete Campaign",
        confirmButtonColor: "#428bca",
        reverseButtons: true,
        allowOutsideClick: false,
        showLoaderOnConfirm: true,
        preConfirm: function () {
            return new Promise(function (resolve, reject) {
                api.campaignId.delete(campaign.id)
                    .success(function (msg) {
                        resolve()
                    })
                    .error(function (data) {
                        reject(data.responseJSON.message)
                    })
            })
        }
    }).then(function (result) {
        if(result.value){
            Swal.fire(
                'Campaign Deleted!',
                'This campaign has been deleted!',
                'success'
            );
        }
        $('button:contains("OK")').on('click', function () {
            location.href = '/campaigns'
        })
    })
}

// Completes a campaign after prompting the user
function completeCampaign() {
    Swal.fire({
        title: "Are you sure?",
        text: "Gophish will stop processing events for this campaign",
        type: "warning",
        animation: false,
        showCancelButton: true,
        confirmButtonText: "Complete Campaign",
        confirmButtonColor: "#428bca",
        reverseButtons: true,
        allowOutsideClick: false,
        showLoaderOnConfirm: true,
        preConfirm: function () {
            return new Promise(function (resolve, reject) {
                api.campaignId.complete(campaign.id)
                    .success(function (msg) {
                        resolve()
                    })
                    .error(function (data) {
                        reject(data.responseJSON.message)
                    })
            })
        }
    }).then(function (result) {
        if (result.value){
            Swal.fire(
                'Campaign Completed!',
                'This campaign has been completed!',
                'success'
            );
            $('#complete_button').prop('disabled', true);
            $('#complete_button').text('Completed!')
            doPoll = false;
        }
    })
}

// Exports campaign results as a CSV file
function exportAsCSV(scope) {
    exportHTML = $("#exportButton").html()
    var csvScope = null
    var filename = campaign.name + ' - ' + capitalize(scope) + '.csv'
    switch (scope) {
        case "results":
            csvScope = campaign.results
            break;
        case "events":
            csvScope = campaign.timeline
            break;
    }
    if (!csvScope) {
        return
    }
    $("#exportButton").html('<i class="fa fa-spinner fa-spin"></i>')
    var csvString = Papa.unparse(csvScope, {
        'escapeFormulae': true
    })
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
    $("#exportButton").html(exportHTML)
}

function replay(event_idx) {
    request = campaign.timeline[event_idx]
    details = JSON.parse(request.details)
    url = null
    form = $('<form>').attr({
        method: 'POST',
        target: '_blank',
    })
    /* Create a form object and submit it */
    $.each(Object.keys(details.payload), function (i, param) {
        if (param == "rid") {
            return true;
        }
        if (param == "__original_url") {
            url = details.payload[param];
            return true;
        }
        $('<input>').attr({
            name: param,
        }).val(details.payload[param]).appendTo(form);
    })
    /* Ensure we know where to send the user */
    // Prompt for the URL
    Swal.fire({
        title: 'Where do you want the credentials submitted to?',
        input: 'text',
        showCancelButton: true,
        inputPlaceholder: "http://example.com/login",
        inputValue: url || "",
        inputValidator: function (value) {
            return new Promise(function (resolve, reject) {
                if (value) {
                    resolve();
                } else {
                    reject('Invalid URL.');
                }
            });
        }
    }).then(function (result) {
        if (result.value){
            url = result.value
            submitForm()
        }
    })
    return
    submitForm()

    function submitForm() {
        form.attr({
            action: url
        })
        form.appendTo('body').submit().remove()
    }
}

/**
 * Returns an HTML string that displays the OS and browser that clicked the link
 * or submitted credentials.
 * 
 * @param {object} event_details - The "details" parameter for a campaign
 *  timeline event
 * 
 */
var renderDevice = function (event_details) {
    var ua = UAParser(details.browser['user-agent'])
    var detailsString = '<div class="timeline-device-details">'

    var deviceIcon = 'laptop'
    if (ua.device.type) {
        if (ua.device.type == 'tablet' || ua.device.type == 'mobile') {
            deviceIcon = ua.device.type
        }
    }

    var deviceVendor = ''
    if (ua.device.vendor) {
        deviceVendor = ua.device.vendor.toLowerCase()
        if (deviceVendor == 'microsoft') deviceVendor = 'windows'
    }

    var deviceName = 'Unknown'
    if (ua.os.name) {
        deviceName = ua.os.name
        if (deviceName == "Mac OS") {
            deviceVendor = 'apple'
        } else if (deviceName == "Windows") {
            deviceVendor = 'windows'
        }
        if (ua.device.vendor && ua.device.model) {
            deviceName = ua.device.vendor + ' ' + ua.device.model
        }
    }

    if (ua.os.version) {
        deviceName = deviceName + ' (OS Version: ' + ua.os.version + ')'
    }

    deviceString = '<div class="timeline-device-os"><span class="fa fa-stack">' +
        '<i class="fa fa-' + escapeHtml(deviceIcon) + ' fa-stack-2x"></i>' +
        '<i class="fa fa-vendor-icon fa-' + escapeHtml(deviceVendor) + ' fa-stack-1x"></i>' +
        '</span> ' + escapeHtml(deviceName) + '</div>'

    detailsString += deviceString

    var deviceBrowser = 'Unknown'
    var browserIcon = 'info-circle'
    var browserVersion = ''

    if (ua.browser && ua.browser.name) {
        deviceBrowser = ua.browser.name
        // Handle the "mobile safari" case
        deviceBrowser = deviceBrowser.replace('Mobile ', '')
        if (deviceBrowser) {
            browserIcon = deviceBrowser.toLowerCase()
            if (browserIcon == 'ie') browserIcon = 'internet-explorer'
        }
        browserVersion = '(Version: ' + ua.browser.version + ')'
    }

    var browserString = '<div class="timeline-device-browser"><span class="fa fa-stack">' +
        '<i class="fa fa-' + escapeHtml(browserIcon) + ' fa-stack-1x"></i></span> ' +
        deviceBrowser + ' ' + browserVersion + '</div>'

    detailsString += browserString
    detailsString += '</div>'
    return detailsString
}

function renderTimeline(data) {
    record = {
        "id": data[0],
        "first_name": data[2],
        "last_name": data[3],
        "email": data[4],
        "position": data[5],
        "status": data[6],
        "reported": data[7],
        "send_date": data[8]
    }
    results = '<div class="timeline col-sm-12 well well-lg">' +
        '<h6>Timeline for ' + escapeHtml(record.first_name) + ' ' + escapeHtml(record.last_name) +
        '</h6><span class="subtitle">Email: ' + escapeHtml(record.email) +
        '<br>Result ID: ' + escapeHtml(record.id) + '</span>' +
        '<div class="timeline-graph col-sm-6">'
    $.each(campaign.timeline, function (i, event) {
        if (!event.email || event.email == record.email) {
            // Add the event
            results += '<div class="timeline-entry">' +
                '    <div class="timeline-bar"></div>'
            results +=
                '    <div class="timeline-icon ' + statuses[event.message].label + '">' +
                '    <i class="fa ' + statuses[event.message].icon + '"></i></div>' +
                '    <div class="timeline-message">' + escapeHtml(event.message) +
                '    <span class="timeline-date">' + moment.utc(event.time).local().format('MMMM Do YYYY h:mm:ss a') + '</span>'
            if (event.details) {
                details = JSON.parse(event.details)
                if (event.message == "Clicked Link" || event.message == "Submitted Data") {
                    deviceView = renderDevice(details)
                    if (deviceView) {
                        results += deviceView
                    }
                }
                if (event.message == "Submitted Data") {
                    results += '<div class="timeline-replay-button"><button onclick="replay(' + i + ')" class="btn btn-success">'
                    results += '<i class="fa fa-refresh"></i> Replay Credentials</button></div>'
                    results += '<div class="timeline-event-details"><i class="fa fa-caret-right"></i> View Details</div>'
                }
                if (details.payload) {
                    results += '<div class="timeline-event-results">'
                    results += '    <table class="table table-condensed table-bordered table-striped">'
                    results += '        <thead><tr><th>Parameter</th><th>Value(s)</tr></thead><tbody>'
                    $.each(Object.keys(details.payload), function (i, param) {
                        if (param == "rid") {
                            return true;
                        }
                        results += '    <tr>'
                        results += '        <td>' + escapeHtml(param) + '</td>'
                        results += '        <td>' + escapeHtml(details.payload[param]) + '</td>'
                        results += '    </tr>'
                    })
                    results += '       </tbody></table>'
                    results += '</div>'
                }
                if (details.error) {
                    results += '<div class="timeline-event-details"><i class="fa fa-caret-right"></i> View Details</div>'
                    results += '<div class="timeline-event-results">'
                    results += '<span class="label label-default">Error</span> ' + details.error
                    results += '</div>'
                }
            }
            results += '</div></div>'
        }
    })
    // Add the scheduled send event at the bottom
    if (record.status == "Scheduled" || record.status == "Retrying") {
        results += '<div class="timeline-entry">' +
            '    <div class="timeline-bar"></div>'
        results +=
            '    <div class="timeline-icon ' + statuses[record.status].label + '">' +
            '    <i class="fa ' + statuses[record.status].icon + '"></i></div>' +
            '    <div class="timeline-message">' + "Scheduled to send at " + record.send_date + '</span>'
    }
    results += '</div></div>'
    return results
}

var timelineChart = null

// timelineChartData maps the {x, y, email, message, marker} point shape
// used at call sites
function timelineChartData(points) {
    return points.map(function (p) {
        return {
            value: [p.x, p.y],
            email: p.email,
            message: p.message,
            itemStyle: {
                color: p.marker.fillColor
            }
        }
    })
}

var renderTimelineChart = function (chartopts) {
    timelineChart = echarts.init(document.getElementById('timeline_chart'), null, {
        height: 200
    })
    timelineChart.setOption({
        title: {
            text: 'Campaign Timeline'
        },
        grid: {
            left: '1%',
            right: '2%',
            containLabel: true
        },
        xAxis: {
            type: 'time'
        },
        yAxis: {
            min: 0,
            max: 2,
            show: false
        },
        dataZoom: [{
            type: 'inside',
            filterMode: 'none'
        }],
        tooltip: {
            trigger: 'axis',
            formatter: function (params) {
                var point = params[0]
                return moment(point.value[0]).format('dddd, MMM D h:mm:ss a') +
                    '<br>Event: ' + point.data.message + '<br>Email: <b>' + point.data.email + '</b>'
            }
        },
        legend: {
            show: false
        },
        series: [{
            type: 'line',
            lineStyle: {
                type: 'dashed',
                color: "#cccccc",
                width: 1
            },
            symbol: 'circle',
            symbolSize: 6,
            data: timelineChartData(chartopts['data'])
        }]
    })
    return timelineChart
}

var pieCharts = {}

/* Renders a pie chart using the provided chartopts */
var renderPieChart = function (chartopts) {
    var chart = echarts.init(document.getElementById(chartopts['elemId']))
    chart.setOption({
        title: [{
            text: chartopts['title'],
            left: 'center',
            top: '0%',
            textStyle: {
                fontSize: 14
            }
        }, {
            text: String(chartopts['data'][0].count),
            left: 'center',
            top: '58%',
            textVerticalAlign: 'middle',
            textStyle: {
                fontSize: 24,
                fontWeight: 'bold',
                color: chartopts['colors'][0],
                fontFamily: 'Helvetica,Arial,sans-serif'
            }
        }],
        tooltip: {
            formatter: function (params) {
                if (params.name === '') {
                    return ''
                }
                return '<span style="color:' + params.color + '">\u25CF</span>' + params.name + ': <b>' + params.value + '%</b><br/>'
            }
        },
        series: [{
            type: 'pie',
            center: ['50%', '58%'],
            radius: ['55%', '78%'],
            label: {
                show: false
            },
            data: chartopts['data'].map(function (d, i) {
                return {
                    name: d.name,
                    value: d.y,
                    itemStyle: {
                        color: chartopts['colors'][i]
                    }
                }
            })
        }]
    })
    pieCharts[chartopts['elemId']] = chart
    return chart
}

/* Updates the bubbles on the map

@param {campaign.result[]} results - The campaign results to process
*/
var updateMap = function (results) {
    if (!map) {
        return
    }
    bubbles = []
    $.each(campaign.results, function (i, result) {
        // Check that it wasn't an internal IP
        if (result.latitude == 0 && result.longitude == 0) {
            return true;
        }
        newIP = true
        $.each(bubbles, function (i, bubble) {
            if (bubble.ip == result.ip) {
                bubbles[i].radius += 1
                newIP = false
                return false
            }
        })
        if (newIP) {
            bubbles.push({
                latitude: result.latitude,
                longitude: result.longitude,
                name: result.ip,
                fillKey: "point",
                radius: 2
            })
        }
    })
    map.bubbles(bubbles)
}

/**
 * Creates a status label for use in the results datatable
 * @param {string} status
 * @param {moment(datetime)} send_date
 * @param {string} rid
 */
function createStatusLabel(status, send_date, rid) {
    var label = statuses[status].label || "label-default";
    var statusColumn = "<span class=\"label " + label + "\">" + status + "</span>"
    // Add the tooltip if the email is scheduled to be sent
    if (status == "Scheduled" || status == "Retrying") {
        var sendDateMessage = "Scheduled to send at " + send_date
        statusColumn = "<span class=\"label " + label + "\" data-toggle=\"tooltip\" data-placement=\"top\" data-html=\"true\" title=\"" + sendDateMessage + "\">" + status + "</span>"
    }
    // Let the admin retry a single failed send without recreating the
    // whole campaign
    if (status == "Error" && canModifyObjects()) {
        statusColumn += " <i role=\"button\" class=\"fa fa-repeat text-muted\" data-toggle=\"tooltip\" title=\"Resend\" onclick=\"resend_mail('" + rid + "', '" + campaign.id + "');\"></i>"
    }
    return statusColumn
}

/* poll - Queries the API and updates the UI with the results
 *
 * Updates:
 * * Timeline Chart
 * * Email (Donut) Chart
 * * Map Bubbles
 * * Datatables
 */
function poll() {
    api.campaignId.results(campaign.id)
        .success(function (c) {
            campaign = c
            /* Update the timeline */
            var timeline_series_data = []
            $.each(campaign.timeline, function (i, event) {
                var event_date = moment.utc(event.time).local()
                timeline_series_data.push({
                    email: event.email,
                    message: event.message,
                    x: event_date.valueOf(),
                    y: 1,
                    marker: {
                        fillColor: statuses[event.message].color
                    }
                })
            })
            timelineChart.setOption({
                series: [{
                    data: timelineChartData(timeline_series_data)
                }]
            })
            /* Update the results donut chart */
            var email_series_data = {}
            // Load the initial data
            Object.keys(statusMapping).forEach(function (k) {
                email_series_data[k] = 0
            });
            $.each(campaign.results, function (i, result) {
                email_series_data[result.status]++;
                if (result.reported) {
                    email_series_data['Email Reported']++
                }
                // Backfill status values
                var step = progressListing.indexOf(result.status)
                for (var i = 0; i < step; i++) {
                    email_series_data[progressListing[i]]++
                }
            })
            $.each(email_series_data, function (status, count) {
                var email_data = []
                if (!(status in statusMapping)) {
                    return true
                }
                email_data.push({
                    name: status,
                    y: Math.floor((count / campaign.results.length) * 100),
                    count: count
                })
                email_data.push({
                    name: '',
                    y: 100 - Math.floor((count / campaign.results.length) * 100)
                })
                var chart = pieCharts[statusMapping[status] + '_chart']
                chart.setOption({
                    series: [{
                        data: email_data.map(function (d, i) {
                            return {
                                name: d.name,
                                value: d.y,
                                itemStyle: {
                                    color: [statuses[status].color, '#dddddd'][i]
                                }
                            }
                        })
                    }]
                })
            })

            /* Update the datatable */
            resultsTable = $("#resultsTable").DataTable()
            resultsTable.rows().every(function (i, tableLoop, rowLoop) {
                var row = this.row(i)
                var rowData = row.data()
                var rid = rowData[0]
                $.each(campaign.results, function (j, result) {
                    if (result.id == rid) {
                        rowData[8] = moment(result.send_date).format('MMMM Do YYYY, h:mm:ss a')
                        rowData[7] = result.reported
                        rowData[6] = result.status
                        resultsTable.row(i).data(rowData)
                        if (row.child.isShown()) {
                            $(row.node()).find("#caret").removeClass("fa-caret-right")
                            $(row.node()).find("#caret").addClass("fa-caret-down")
                            row.child(renderTimeline(row.data()))
                        }
                        return false
                    }
                })
            })
            resultsTable.draw(false)
            /* Update the map information */
            updateMap(campaign.results)
            $('[data-toggle="tooltip"]').tooltip()
            $("#refresh_message").hide()
            $("#refresh_btn").show()
        })
}

function load() {
    campaign.id = window.location.pathname.split('/').slice(-1)[0]
    var use_map = JSON.parse(localStorage.getItem('gophish.use_map'))
    api.campaignId.results(campaign.id)
        .success(function (c) {
            campaign = c
            if (campaign) {
                $("title").text(c.name + " - Gophish")
                $("#loading").hide()
                $("#campaignResults").show()
                // Set the title
                $("#page-title").text("Results for " + c.name)
                if (c.status == "Completed") {
                    // .prop() rather than [0].disabled: read-only accounts
                    // never get a complete button rendered, and indexing an
                    // empty jQuery set would throw and abort the rest of
                    // this handler.
                    $('#complete_button').prop('disabled', true);
                    $('#complete_button').text('Completed!');
                    doPoll = false;
                }
                // Setup viewing the details of a result
                $("#resultsTable").on("click", ".timeline-event-details", function () {
                    // Show the parameters
                    payloadResults = $(this).parent().find(".timeline-event-results")
                    if (payloadResults.is(":visible")) {
                        $(this).find("i").removeClass("fa-caret-down")
                        $(this).find("i").addClass("fa-caret-right")
                        payloadResults.hide()
                    } else {
                        $(this).find("i").removeClass("fa-caret-right")
                        $(this).find("i").addClass("fa-caret-down")
                        payloadResults.show()
                    }
                })
                // Setup the results table
                resultsTable = $("#resultsTable").DataTable({
                    destroy: true,
                    "order": [
                        [2, "asc"]
                    ],
                    columnDefs: [{
                            orderable: false,
                            targets: "no-sort"
                        }, {
                            className: "details-control",
                            "targets": [1]
                        }, {
                            "visible": false,
                            "targets": [0, 8]
                        },
                        {
                            "render": function (data, type, row) {
                                return createStatusLabel(data, row[8], row[0])
                            },
                            "targets": [6]
                        },
                        {
                            className: "text-center",
                            "render": function (reported, type, row) {
                                if (type == "display") {
                                    if (reported) {
                                        return "<i class='fa fa-check-circle text-center text-success'></i>"
                                    }
                                    return "<i role='button' class='fa fa-times-circle text-center text-muted' onclick='report_mail(\"" + row[0] + "\", \"" + campaign.id + "\");'></i>"
                                }
                                return reported
                            },
                            "targets": [7]
                        }
                    ]
                });
                resultsTable.clear();
                var email_series_data = {}
                var timeline_series_data = []
                Object.keys(statusMapping).forEach(function (k) {
                    email_series_data[k] = 0
                });
                $.each(campaign.results, function (i, result) {
                    resultsTable.row.add([
                        result.id,
                        "<i id=\"caret\" class=\"fa fa-caret-right\"></i>",
                        escapeHtml(result.first_name) || "",
                        escapeHtml(result.last_name) || "",
                        escapeHtml(result.email) || "",
                        escapeHtml(result.position) || "",
                        result.status,
                        result.reported,
                        moment(result.send_date).format('MMMM Do YYYY, h:mm:ss a')
                    ])
                    email_series_data[result.status]++;
                    if (result.reported) {
                        email_series_data['Email Reported']++
                    }
                    // Backfill status values
                    var step = progressListing.indexOf(result.status)
                    for (var i = 0; i < step; i++) {
                        email_series_data[progressListing[i]]++
                    }
                })
                resultsTable.draw();
                // Setup tooltips
                $('[data-toggle="tooltip"]').tooltip()
                // Setup the individual timelines
                $('#resultsTable tbody').on('click', 'td.details-control', function () {
                    var tr = $(this).closest('tr');
                    var row = resultsTable.row(tr);
                    if (row.child.isShown()) {
                        // This row is already open - close it
                        row.child.hide();
                        tr.removeClass('shown');
                        $(this).find("i").removeClass("fa-caret-down")
                        $(this).find("i").addClass("fa-caret-right")
                    } else {
                        // Open this row
                        $(this).find("i").removeClass("fa-caret-right")
                        $(this).find("i").addClass("fa-caret-down")
                        row.child(renderTimeline(row.data())).show();
                        tr.addClass('shown');
                    }
                });
                // Setup the graphs
                $.each(campaign.timeline, function (i, event) {
                    if (event.message == "Campaign Created") {
                        return true
                    }
                    var event_date = moment.utc(event.time).local()
                    timeline_series_data.push({
                        email: event.email,
                        message: event.message,
                        x: event_date.valueOf(),
                        y: 1,
                        marker: {
                            fillColor: statuses[event.message].color
                        }
                    })
                })
                renderTimelineChart({
                    data: timeline_series_data
                })
                $.each(email_series_data, function (status, count) {
                    var email_data = []
                    if (!(status in statusMapping)) {
                        return true
                    }
                    email_data.push({
                        name: status,
                        y: Math.floor((count / campaign.results.length) * 100),
                        count: count
                    })
                    email_data.push({
                        name: '',
                        y: 100 - Math.floor((count / campaign.results.length) * 100)
                    })
                    var chart = renderPieChart({
                        elemId: statusMapping[status] + '_chart',
                        title: status,
                        name: status,
                        data: email_data,
                        colors: [statuses[status].color, '#dddddd']
                    })
                })

                if (use_map) {
                    $("#resultsMapContainer").show()
                    map = new Datamap({
                        element: document.getElementById("resultsMap"),
                        responsive: true,
                        fills: {
                            defaultFill: "#ffffff",
                            point: "#283F50"
                        },
                        geographyConfig: {
                            highlightFillColor: "#1abc9c",
                            borderColor: "#283F50"
                        },
                        bubblesConfig: {
                            borderColor: "#283F50"
                        }
                    });
                }
                updateMap(campaign.results)
            }
        })
        .error(function () {
            $("#loading").hide()
            errorFlash(" Campaign not found!")
        })
}

var setRefresh

function refresh() {
    if (!doPoll) {
        return;
    }
    $("#refresh_message").show()
    $("#refresh_btn").hide()
    poll()
    clearTimeout(setRefresh)
    setRefresh = setTimeout(refresh, 60000)
};

// localDatetimeInputValue formats a Date the way <input type="datetime-local">
// expects (no timezone suffix), using the browser's local timezone.
function localDatetimeInputValue(date) {
    var offsetMs = date.getTimezoneOffset() * 60000
    return new Date(date.getTime() - offsetMs).toISOString().slice(0, 16)
}

function report_mail(rid, cid) {
    var nowValue = localDatetimeInputValue(new Date())
    Swal.fire({
        title: "Are you sure?",
        html: "This result will be flagged as reported (RID: " + rid + ")<br><br>" +
            "<label for='reportedDate'>Reported at:</label><br>" +
            "<input type='datetime-local' id='reportedDate' class='swal2-input' value='" + nowValue + "' max='" + nowValue + "'>",
        type: "question",
        animation: false,
        showCancelButton: true,
        confirmButtonText: "Continue",
        confirmButtonColor: "#428bca",
        reverseButtons: true,
        allowOutsideClick: false,
        showLoaderOnConfirm: true,
        preConfirm: function () {
            var value = document.getElementById("reportedDate").value
            if (!value) {
                Swal.showValidationMessage("Please provide a reported date/time")
                return false
            }
            var reportedDate = new Date(value)
            if (reportedDate > new Date()) {
                Swal.showValidationMessage("Reported date/time can't be in the future")
                return false
            }
            return reportedDate.toISOString()
        }
    }).then(function (result) {
        if (result.value) {
            api.campaignId.report(cid, rid, result.value)
                .success(function () {
                    refresh()
                })
                .error(function (data) {
                    Swal.fire({
                        title: 'Error',
                        text: data.responseJSON.message,
                        type: 'error',
                        confirmButtonText: 'Close'
                    });
                })
        }
    })
}

// resend_mail resends a single result that previously failed to send
function resend_mail(rid, cid) {
    Swal.fire({
        title: "Are you sure?",
        text: "This will attempt to resend the email for this result (RID: " + rid + ")",
        type: "question",
        animation: false,
        showCancelButton: true,
        confirmButtonText: "Resend",
        confirmButtonColor: "#428bca",
        reverseButtons: true,
        allowOutsideClick: false
    }).then(function (result) {
        if (result.value) {
            api.campaignId.resend(cid, rid)
                .success(function () {
                    successFlash("Result queued for resending")
                    refresh()
                })
                .error(function (data) {
                    Swal.fire({
                        title: 'Error',
                        text: data.responseJSON.message,
                        type: 'error',
                        confirmButtonText: 'Close'
                    });
                })
        }
    })
}

// resend_all_failed resends every result in the campaign that's currently
// in an Error state
function resend_all_failed(cid) {
    Swal.fire({
        title: "Are you sure?",
        text: "This will attempt to resend every failed email in this campaign.",
        type: "question",
        animation: false,
        showCancelButton: true,
        confirmButtonText: "Resend All Failed",
        confirmButtonColor: "#428bca",
        reverseButtons: true,
        allowOutsideClick: false
    }).then(function (result) {
        if (result.value) {
            api.campaignId.resendFailed(cid)
                .success(function (data) {
                    successFlash(data.message)
                    refresh()
                })
                .error(function (data) {
                    Swal.fire({
                        title: 'Error',
                        text: data.responseJSON.message,
                        type: 'error',
                        confirmButtonText: 'Close'
                    });
                })
        }
    })
}

$(window).on('resize', function () {
    if (timelineChart) {
        timelineChart.resize()
    }
    $.each(pieCharts, function (id, chart) {
        chart.resize()
    })
})

$(document).ready(function () {
    load();

    // Start the polling loop
    setRefresh = setTimeout(refresh, 60000)
})

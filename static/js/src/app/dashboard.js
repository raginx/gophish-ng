var campaigns = []
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
        icon: "fa-envelope",
        point: "ct-point-opened"
    },
    "Email Reported": {
        color: "#45d6ef",
        label: "label-warning",
        icon: "fa-bullhorn",
        point: "ct-point-reported"
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
    "Campaign Created": {
        label: "label-success",
        icon: "fa-rocket"
    }
}

var statsMapping = {
    "sent": "Email Sent",
    "opened": "Email Opened",
    "email_reported": "Email Reported",
    "clicked": "Clicked Link",
    "submitted_data": "Submitted Data",
}

function deleteCampaign(idx) {
    if (confirm("Delete " + campaigns[idx].name + "?")) {
        api.campaignId.delete(campaigns[idx].id)
            .done(function (data) {
                successFlash(data.message)
                location.reload()
            })
    }
}

var pieCharts = {}

/* Renders a pie chart using the provided chartopts */
function renderPieChart(chartopts) {
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
                fontSize: 16,
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

function generateStatsPieCharts(campaigns) {
    var stats_data = []
    var stats_series_data = {}
    var total = 0

    $.each(campaigns, function (i, campaign) {
        $.each(campaign.stats, function (status, count) {
            if (status == "total") {
                total += count
                return true
            }
            if (!stats_series_data[status]) {
                stats_series_data[status] = count;
            } else {
                stats_series_data[status] += count;
            }
        })
    })
    $.each(stats_series_data, function (status, count) {
        // I don't like this, but I guess it'll have to work.
        // Turns submitted_data into Submitted Data
        if (!(status in statsMapping)) {
            return true
        }
        status_label = statsMapping[status]
        stats_data.push({
            name: status_label,
            y: Math.floor((count / total) * 100),
            count: count
        })
        stats_data.push({
            name: '',
            y: 100 - Math.floor((count / total) * 100)
        })
        var stats_chart = renderPieChart({
            elemId: status + '_chart',
            title: status_label,
            name: status,
            data: stats_data,
            colors: [statuses[status_label].color, "#dddddd"]
        })

        stats_data = []
    });
}

var overviewChart = null

function generateTimelineChart(campaigns) {
    var overview_data = []
    $.each(campaigns, function (i, campaign) {
        var campaign_date = moment.utc(campaign.created_date).local()
        // Add it to the chart data
        campaign.y = 0
        // Clicked events also contain our data submitted events
        campaign.y += campaign.stats.clicked
        campaign.y = Math.floor((campaign.y / campaign.stats.total) * 100)
        // Add the data to the overview chart
        overview_data.push({
            campaign_id: campaign.id,
            name: campaign.name,
            x: campaign_date.valueOf(),
            y: campaign.y
        })
    })
    overview_data.sort(function (a, b) {
        return a.x - b.x
    })
    overviewChart = echarts.init(document.getElementById('overview_chart'))
    overviewChart.setOption({
        title: {
            text: 'Phishing Success Overview'
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
            name: '% of Success'
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
                    '<br>' + point.data.name + '<br>% Success: <b>' + point.value[1] + '%</b>'
            }
        },
        legend: {
            show: false
        },
        series: [{
            type: 'line',
            areaStyle: {
                opacity: 0.5
            },
            color: "#f05b4f",
            symbol: 'circle',
            symbolSize: 6,
            data: overview_data.map(function (d) {
                return {
                    value: [d.x, d.y],
                    name: d.name,
                    campaign_id: d.campaign_id
                }
            })
        }]
    })
    overviewChart.on('click', function (params) {
        if (params.componentType === 'series') {
            window.location.href = "/campaigns/" + params.data.campaign_id
        }
    })
}

$(window).on('resize', function () {
    if (overviewChart) {
        overviewChart.resize()
    }
    $.each(pieCharts, function (id, chart) {
        chart.resize()
    })
})

$(document).ready(function () {
    api.campaigns.summary()
        .done(function (data) {
            $("#loading").hide()
            campaigns = data.campaigns
            if (campaigns.length > 0) {
                $("#dashboard").show()
                // Create the overview chart data
                campaignTable = $("#campaignTable").DataTable({
                    columnDefs: [{
                            orderable: false,
                            targets: "no-sort"
                        },
                        {
                            className: "color-sent",
                            targets: [2]
                        },
                        {
                            className: "color-opened",
                            targets: [3]
                        },
                        {
                            className: "color-clicked",
                            targets: [4]
                        },
                        {
                            className: "color-success",
                            targets: [5]
                        },
                        {
                            className: "color-reported",
                            targets: [6]
                        }
                    ],
                    order: [
                        [1, "desc"]
                    ]
                });
                campaignRows = []
                $.each(campaigns, function (i, campaign) {
                    var campaign_date = moment(campaign.created_date).format('MMMM Do YYYY, h:mm:ss a')
                    var label = statuses[campaign.status].label || "label-default";
                    //section for tooltips on the status of a campaign to show some quick stats
                    var launchDate;
                    if (moment(campaign.launch_date).isAfter(moment())) {
                        launchDate = "Scheduled to start: " + moment(campaign.launch_date).format('MMMM Do YYYY, h:mm:ss a')
                        var quickStats = launchDate + "<br><br>" + "Number of recipients: " + campaign.stats.total
                    } else {
                        launchDate = "Launch Date: " + moment(campaign.launch_date).format('MMMM Do YYYY, h:mm:ss a')
                        var quickStats = launchDate + "<br><br>" + "Number of recipients: " + campaign.stats.total + "<br><br>" + "Emails opened: " + campaign.stats.opened + "<br><br>" + "Emails clicked: " + campaign.stats.clicked + "<br><br>" + "Submitted Credentials: " + campaign.stats.submitted_data + "<br><br>" + "Errors : " + campaign.stats.error + "<br><br>" + "Reported : " + campaign.stats.email_reported
                    }
                    // Add it to the list
                    campaignRows.push([
                        escapeHtml(campaign.name),
                        campaign_date,
                        campaign.stats.sent,
                        campaign.stats.opened,
                        campaign.stats.clicked,
                        campaign.stats.submitted_data,
                        campaign.stats.email_reported,
                        "<span class=\"label " + label + "\" data-toggle=\"tooltip\" data-placement=\"right\" data-html=\"true\" title=\"" + quickStats + "\">" + campaign.status + "</span>",
                        "<div class='pull-right'><a class='btn btn-primary' href='/campaigns/" + campaign.id + "' data-toggle='tooltip' data-placement='left' title='View Results'>\
                    <i class='fa fa-bar-chart'></i>\
                    </a>" + (canModifyObjects() ? "\
                    <button class='btn btn-danger' onclick='deleteCampaign(" + i + ")' data-toggle='tooltip' data-placement='left' title='Delete Campaign'>\
                    <i class='fa fa-trash-o'></i>\
                    </button>" : "") + "</div>"
                    ])
                    $('[data-toggle="tooltip"]').tooltip()
                })
                campaignTable.rows.add(campaignRows).draw()
                // Build the charts
                generateStatsPieCharts(campaigns)
                generateTimelineChart(campaigns)
            } else {
                $("#emptyMessage").show()
                loadGettingStartedSteps()
            }
        })
        .fail(function () {
            errorFlash("Error fetching campaigns")
        })
})

// loadGettingStartedSteps checks off the getting-started steps
function loadGettingStartedSteps() {
    function markStepComplete(step) {
        $('#gettingStartedSteps li[data-step="' + step + '"] i')
            .removeClass('fa-circle-o')
            .addClass('fa-check-circle text-success')
    }
    api.SMTP.get().done(function (profiles) {
        if (profiles.length > 0) markStepComplete('sendingProfile')
    })
    api.templates.get().done(function (templates) {
        if (templates.length > 0) markStepComplete('template')
    })
    api.pages.get().done(function (pages) {
        if (pages.length > 0) markStepComplete('landingPage')
    })
    api.groups.get().done(function (groups) {
        if (groups.length > 0) markStepComplete('group')
    })
}

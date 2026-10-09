package models

import (
	"sort"
	"time"

	log "github.com/raginx/gophish-ng/logger"
)

// timelineTargetBuckets is the number of buckets computeTimeline aims to
// produce across a campaign's engagement span
const timelineTargetBuckets = 40

// timelineMinInterval is the smallest bucket width the timeline uses
const timelineMinInterval = time.Minute

// CampaignReport is the aggregated, PII-free view of a campaign's outcome. It
// is produced identically for normal and anonymized campaigns
type CampaignReport struct {
	CampaignId int64 `json:"campaign_id"`
	// Anonymized reflects whether the campaign has been scrubbed
	Anonymized bool           `json:"anonymized"`
	Meta       CampaignMeta   `json:"meta"`
	Stats      CampaignStats  `json:"stats"`
	Rates      CampaignRates  `json:"rates"`
	Timing     CampaignTiming `json:"timing"`
	Timeline   []ReportBucket `json:"timeline"`
}

// CampaignMeta carries the descriptive, non-metric context a report needs to
// stand on its own as an evidence artifact: what was tested, when, and whether
// the data has been anonymized. None of it is recipient PII - the template and
// sending-profile names describe the pretext, not any target. TemplateName and
// SMTPName fall back to "[Deleted]" when the referenced object no longer
// exists, so the report never silently omits the pretext it measured.
type CampaignMeta struct {
	Name          string    `json:"name"`
	Status        string    `json:"status"`
	CreatedDate   time.Time `json:"created_date"`
	LaunchDate    time.Time `json:"launch_date"`
	CompletedDate time.Time `json:"completed_date"`
	// ScrubbedDate is the zero value unless the campaign has been anonymized
	ScrubbedDate time.Time `json:"scrubbed_date"`
	TemplateName string    `json:"template_name"`
	SMTPName     string    `json:"smtp_name"`
}

// CampaignRates holds the funnel rates as fractions in the range [0, 1],
// relative to the total number of recipients. Consumers format them as
// percentages. Each rate is cumulative in the same sense as CampaignStats: a
// recipient counted at a later stage is also counted at every earlier one.
type CampaignRates struct {
	Open   float64 `json:"open"`
	Click  float64 `json:"click"`
	Submit float64 `json:"submit"`
	Report float64 `json:"report"`
}

// StageTiming summarizes how long recipients took to reach a funnel stage,
// measured from each recipient's "Email Sent" event. Count is the number of
// recipients with a measurable delta for the stage, which is also the sample
// size behind Median and Average.
type StageTiming struct {
	Count   int           `json:"count"`
	Median  time.Duration `json:"median"`
	Average time.Duration `json:"average"`
}

// CampaignTiming groups the per-stage timing summaries. Because events do not
// cascade (a click is not recorded as an implicit open), each stage reflects
// only recipients with that stage's event actually recorded.
type CampaignTiming struct {
	Open   StageTiming `json:"open"`
	Click  StageTiming `json:"click"`
	Submit StageTiming `json:"submit"`
}

// ReportBucket is one time slice of the engagement timeline. Time is the
// bucket's start; the counts are the engagement events that fell within it.
// Reporting has no timeline: it is a boolean flag on the result with no
// timestamp, so it is reflected in Stats/Rates only.
type ReportBucket struct {
	Time      time.Time `json:"time"`
	Opened    int       `json:"opened"`
	Clicked   int       `json:"clicked"`
	Submitted int       `json:"submitted"`
}

// GetCampaignReport builds the aggregated report for the campaign identified by
// id and visible to teamID. It reuses getCampaignStats for the funnel counts
// and derives rates, timing, and the engagement timeline from the campaign's
// events.
func GetCampaignReport(id int64, teamID int64) (CampaignReport, error) {
	report := CampaignReport{CampaignId: id}

	// Scope to the team and confirm the campaign exists before aggregating.
	c := Campaign{}
	if err := db.Where("id = ? AND team_id = ?", id, teamID).First(&c).Error; err != nil {
		log.Error(err)
		return report, err
	}
	report.Anonymized = !c.ScrubbedDate.IsZero()
	report.Meta = campaignMeta(c)

	stats, err := getCampaignStats(id)
	if err != nil {
		log.Error(err)
		return report, err
	}
	report.Stats = stats
	report.Rates = computeRates(stats)

	events := []Event{}
	if err := db.Where("campaign_id = ?", id).Order("time asc").Find(&events).Error; err != nil {
		log.Error(err)
		return report, err
	}
	report.Timing = computeTiming(events)
	report.Timeline = computeTimeline(events)

	return report, nil
}

// campaignMeta builds the descriptive context for a report from the campaign
// row, resolving the template and sending-profile names (falling back to
// "[Deleted]" when they've since been removed, mirroring getDetails).
func campaignMeta(c Campaign) CampaignMeta {
	meta := CampaignMeta{
		Name:          c.Name,
		Status:        c.Status,
		CreatedDate:   c.CreatedDate,
		LaunchDate:    c.LaunchDate,
		CompletedDate: c.CompletedDate,
		ScrubbedDate:  c.ScrubbedDate,
	}

	tmpl := Template{}
	if err := db.Table("templates").Where("id = ?", c.TemplateId).First(&tmpl).Error; err != nil {
		meta.TemplateName = "[Deleted]"
	} else {
		meta.TemplateName = tmpl.Name
	}

	smtp := SMTP{}
	if err := db.Table("smtp").Where("id = ?", c.SMTPId).First(&smtp).Error; err != nil {
		meta.SMTPName = "[Deleted]"
	} else {
		meta.SMTPName = smtp.Name
	}

	return meta
}

// computeRates derives the funnel rates from the stats, using the total
// recipient count as a single consistent denominator.
func computeRates(s CampaignStats) CampaignRates {
	rates := CampaignRates{}
	if s.Total == 0 {
		return rates
	}
	total := float64(s.Total)
	rates.Open = float64(s.OpenedEmail) / total
	rates.Click = float64(s.ClickedLink) / total
	rates.Submit = float64(s.SubmittedData) / total
	rates.Report = float64(s.EmailReported) / total
	return rates
}

// recipientTimes holds the first timestamp seen for each stage of a single
// recipient's journey. Grouping is keyed by the event's email, which is a
// stable, distinct pseudonym for scrubbed campaigns and the real address
// otherwise - either way each recipient's timeline stays correctly separated.
type recipientTimes struct {
	sent   time.Time
	open   time.Time
	click  time.Time
	submit time.Time
}

// computeTiming measures, per recipient, how long after the "Email Sent" event
// each later stage first occurred, then summarizes the deltas per stage.
func computeTiming(events []Event) CampaignTiming {
	byEmail := map[string]*recipientTimes{}
	// setEarliest keeps the first occurrence, so re-opens or repeat clicks
	// don't move a recipient's recorded time.
	setEarliest := func(dst *time.Time, t time.Time) {
		if dst.IsZero() || t.Before(*dst) {
			*dst = t
		}
	}
	for _, e := range events {
		if e.Email == "" {
			continue
		}
		rt := byEmail[e.Email]
		if rt == nil {
			rt = &recipientTimes{}
			byEmail[e.Email] = rt
		}
		switch e.Message {
		case EventSent:
			setEarliest(&rt.sent, e.Time)
		case EventOpened:
			setEarliest(&rt.open, e.Time)
		case EventClicked:
			setEarliest(&rt.click, e.Time)
		case EventDataSubmit:
			setEarliest(&rt.submit, e.Time)
		}
	}

	var opens, clicks, submits []time.Duration
	for _, rt := range byEmail {
		// Without a send time there's no baseline to measure against.
		if rt.sent.IsZero() {
			continue
		}
		if d, ok := delta(rt.sent, rt.open); ok {
			opens = append(opens, d)
		}
		if d, ok := delta(rt.sent, rt.click); ok {
			clicks = append(clicks, d)
		}
		if d, ok := delta(rt.sent, rt.submit); ok {
			submits = append(submits, d)
		}
	}

	return CampaignTiming{
		Open:   summarizeDurations(opens),
		Click:  summarizeDurations(clicks),
		Submit: summarizeDurations(submits),
	}
}

// delta returns the time from sent to stage, reporting ok=false when the stage
// never occurred or, defensively, predates the send event (clock skew).
func delta(sent, stage time.Time) (time.Duration, bool) {
	if stage.IsZero() || stage.Before(sent) {
		return 0, false
	}
	return stage.Sub(sent), true
}

// summarizeDurations computes the median and average of the given durations.
// An empty input yields a zero-count summary.
func summarizeDurations(ds []time.Duration) StageTiming {
	st := StageTiming{Count: len(ds)}
	if len(ds) == 0 {
		return st
	}
	var sum time.Duration
	for _, d := range ds {
		sum += d
	}
	st.Average = sum / time.Duration(len(ds))

	sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })
	n := len(ds)
	if n%2 == 1 {
		st.Median = ds[n/2]
	} else {
		st.Median = (ds[n/2-1] + ds[n/2]) / 2
	}
	return st
}

// computeTimeline buckets the engagement events (opens, clicks, submissions)
// across the campaign's engagement span. The bucket interval is the span
// divided by timelineTargetBuckets, clamped to timelineMinInterval. It returns
// nil when there are no engagement events to place.
func computeTimeline(events []Event) []ReportBucket {
	relevant := make([]Event, 0, len(events))
	var first, last time.Time
	for _, e := range events {
		switch e.Message {
		case EventOpened, EventClicked, EventDataSubmit:
		default:
			continue
		}
		relevant = append(relevant, e)
		if first.IsZero() || e.Time.Before(first) {
			first = e.Time
		}
		if e.Time.After(last) {
			last = e.Time
		}
	}
	if len(relevant) == 0 {
		return nil
	}

	interval := last.Sub(first) / timelineTargetBuckets
	if interval < timelineMinInterval {
		interval = timelineMinInterval
	}
	bucketIndex := func(t time.Time) int {
		return int(t.Sub(first) / interval)
	}

	buckets := make([]ReportBucket, bucketIndex(last)+1)
	for i := range buckets {
		buckets[i].Time = first.Add(time.Duration(i) * interval)
	}
	for _, e := range relevant {
		b := &buckets[bucketIndex(e.Time)]
		switch e.Message {
		case EventOpened:
			b.Opened++
		case EventClicked:
			b.Clicked++
		case EventDataSubmit:
			b.Submitted++
		}
	}
	return buckets
}

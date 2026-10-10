package models

import (
	"time"

	check "gopkg.in/check.v1"
)

func (s *ModelsSuite) TestComputeRates(c *check.C) {
	// Rates are fractions of the total recipient count.
	rates := computeRates(CampaignStats{
		Total:         4,
		OpenedEmail:   3,
		ClickedLink:   2,
		SubmittedData: 1,
		EmailReported: 1,
	})
	c.Assert(rates.Open, check.Equals, 0.75)
	c.Assert(rates.Click, check.Equals, 0.5)
	c.Assert(rates.Submit, check.Equals, 0.25)
	c.Assert(rates.Report, check.Equals, 0.25)

	// A campaign with no recipients yields zero rates, not a divide-by-zero.
	zero := computeRates(CampaignStats{})
	c.Assert(zero, check.Equals, CampaignRates{})
}

func (s *ModelsSuite) TestSummarizeDurations(c *check.C) {
	// Odd sample size: median is the middle element.
	odd := summarizeDurations([]time.Duration{6 * time.Minute, 2 * time.Minute, 4 * time.Minute})
	c.Assert(odd.Count, check.Equals, 3)
	c.Assert(odd.Median, check.Equals, 4*time.Minute)
	c.Assert(odd.Average, check.Equals, 4*time.Minute)

	// Even sample size: median is the mean of the two middle elements.
	even := summarizeDurations([]time.Duration{1 * time.Minute, 3 * time.Minute})
	c.Assert(even.Count, check.Equals, 2)
	c.Assert(even.Median, check.Equals, 2*time.Minute)
	c.Assert(even.Average, check.Equals, 2*time.Minute)

	// Empty input is a zero-count summary.
	empty := summarizeDurations(nil)
	c.Assert(empty, check.Equals, StageTiming{})
}

func (s *ModelsSuite) TestComputeTiming(c *check.C) {
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	events := []Event{
		// Recipient a: sent, opened +1m, clicked +3m.
		{Email: "a", Message: EventSent, Time: t0},
		{Email: "a", Message: EventOpened, Time: t0.Add(1 * time.Minute)},
		{Email: "a", Message: EventClicked, Time: t0.Add(3 * time.Minute)},
		// Recipient b: sent, opened +5m.
		{Email: "b", Message: EventSent, Time: t0},
		{Email: "b", Message: EventOpened, Time: t0.Add(5 * time.Minute)},
		// Recipient c: opened but never sent - no baseline, excluded.
		{Email: "c", Message: EventOpened, Time: t0.Add(2 * time.Minute)},
		// Campaign-level event with no recipient - ignored.
		{Email: "", Message: "Campaign Created", Time: t0},
	}

	timing := computeTiming(events)

	// Opens: a=1m, b=5m -> median and average both 3m over 2 samples.
	c.Assert(timing.Open.Count, check.Equals, 2)
	c.Assert(timing.Open.Median, check.Equals, 3*time.Minute)
	c.Assert(timing.Open.Average, check.Equals, 3*time.Minute)

	// Clicks: only a=3m.
	c.Assert(timing.Click.Count, check.Equals, 1)
	c.Assert(timing.Click.Median, check.Equals, 3*time.Minute)

	// No submissions were recorded.
	c.Assert(timing.Submit.Count, check.Equals, 0)
}

func (s *ModelsSuite) TestComputeTimingKeepsEarliestOccurrence(c *check.C) {
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	events := []Event{
		{Email: "a", Message: EventSent, Time: t0},
		// A re-open must not move the recorded time off the first open.
		{Email: "a", Message: EventOpened, Time: t0.Add(2 * time.Minute)},
		{Email: "a", Message: EventOpened, Time: t0.Add(9 * time.Minute)},
	}
	timing := computeTiming(events)
	c.Assert(timing.Open.Count, check.Equals, 1)
	c.Assert(timing.Open.Median, check.Equals, 2*time.Minute)
}

func (s *ModelsSuite) TestComputeTimeline(c *check.C) {
	t0 := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	// A 40-minute span over timelineTargetBuckets (40) gives a clean 1m
	// interval, so event offsets map directly to bucket indices.
	events := []Event{
		{Email: "a", Message: EventOpened, Time: t0},
		{Email: "b", Message: EventOpened, Time: t0},
		{Email: "a", Message: EventClicked, Time: t0.Add(1 * time.Minute)},
		{Email: "a", Message: EventDataSubmit, Time: t0.Add(40 * time.Minute)},
		// Non-engagement events never appear in the timeline.
		{Email: "a", Message: EventSent, Time: t0.Add(20 * time.Minute)},
	}

	buckets := computeTimeline(events)
	c.Assert(len(buckets), check.Equals, 41)
	c.Assert(buckets[0].Time.Equal(t0), check.Equals, true)
	c.Assert(buckets[0].Opened, check.Equals, 2)
	c.Assert(buckets[1].Clicked, check.Equals, 1)
	c.Assert(buckets[40].Submitted, check.Equals, 1)

	// An empty engagement set produces no timeline.
	c.Assert(computeTimeline(nil), check.IsNil)
}

// TestGetCampaignReportIsScrubSafe is the core #57 contract for the reporting
// layer: scrubbing a campaign must not change its report. The report is
// recomputed before and after ScrubCampaign and every aggregate is compared.
func (s *ModelsSuite) TestGetCampaignReportIsScrubSafe(c *check.C) {
	campaign := s.createCampaign(c)
	s.recordPIIEvents(c, &campaign)

	before, err := GetCampaignReport(campaign.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)
	c.Assert(before.Anonymized, check.Equals, false)
	c.Assert(before.Meta.ScrubbedDate.IsZero(), check.Equals, true)

	c.Assert(ScrubCampaign(campaign.Id, campaign.UserId), check.Equals, nil)

	after, err := GetCampaignReport(campaign.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)

	// Only the anonymized flag flips; the numbers are untouched.
	c.Assert(after.Anonymized, check.Equals, true)
	c.Assert(after.Stats, check.DeepEquals, before.Stats)
	c.Assert(after.Rates, check.DeepEquals, before.Rates)
	c.Assert(after.Timing, check.DeepEquals, before.Timing)
	c.Assert(after.Timeline, check.DeepEquals, before.Timeline)

	// The descriptive context describes the pretext, not recipients, so it
	// survives scrubbing - except ScrubbedDate, which now records when the
	// anonymization happened (the data-protection evidence).
	c.Assert(after.Meta.Name, check.Equals, before.Meta.Name)
	c.Assert(after.Meta.TemplateName, check.Equals, before.Meta.TemplateName)
	c.Assert(after.Meta.SMTPName, check.Equals, before.Meta.SMTPName)
	c.Assert(after.Meta.ScrubbedDate.IsZero(), check.Equals, false)
}

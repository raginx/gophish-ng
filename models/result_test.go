package models

import (
	"net/mail"
	"regexp"
	"time"

	"gopkg.in/check.v1"
	"gorm.io/gorm"
)

func (s *ModelsSuite) TestGenerateResultId(c *check.C) {
	r := Result{}
	c.Assert(r.GenerateId(db), check.Equals, nil)
	match, err := regexp.Match("[a-zA-Z0-9]{7}", []byte(r.RId))
	c.Assert(err, check.Equals, nil)
	c.Assert(match, check.Equals, true)
}

func (s *ModelsSuite) TestFormatAddress(c *check.C) {
	r := Result{
		BaseRecipient: BaseRecipient{
			FirstName: "John",
			LastName:  "Doe",
			Email:     "johndoe@example.com",
		},
	}
	expected := &mail.Address{
		Name:    "John Doe",
		Address: "johndoe@example.com",
	}
	c.Assert(r.FormatAddress(), check.Equals, expected.String())

	r = Result{
		BaseRecipient: BaseRecipient{Email: "johndoe@example.com"},
	}
	c.Assert(r.FormatAddress(), check.Equals, r.Email)
}

func (s *ModelsSuite) TestResultSendingStatus(ch *check.C) {
	c := s.createCampaignDependencies(ch)
	ch.Assert(PostCampaign(&c, c.UserId, 0), check.Equals, nil)
	// This campaign wasn't scheduled, so we expect the status to
	// be sending
	for _, r := range c.Results {
		ch.Assert(r.Status, check.Equals, StatusSending)
		ch.Assert(r.ModifiedDate, check.Equals, c.CreatedDate)
	}
}
func (s *ModelsSuite) TestResultScheduledStatus(ch *check.C) {
	c := s.createCampaignDependencies(ch)
	c.LaunchDate = time.Now().UTC().Add(time.Hour * time.Duration(1))
	ch.Assert(PostCampaign(&c, c.UserId, 0), check.Equals, nil)
	// This campaign wasn't scheduled, so we expect the status to
	// be sending
	for _, r := range c.Results {
		ch.Assert(r.Status, check.Equals, StatusScheduled)
		ch.Assert(r.ModifiedDate, check.Equals, c.CreatedDate)
	}
}

func (s *ModelsSuite) TestResultVariableStatus(ch *check.C) {
	c := s.createCampaignDependencies(ch)
	c.LaunchDate = time.Now().UTC()
	c.SendByDate = c.LaunchDate.Add(2 * time.Minute)
	ch.Assert(PostCampaign(&c, c.UserId, 0), check.Equals, nil)

	// The campaign has a window smaller than our group size, so we expect some
	// emails to be sent immediately, while others will be scheduled
	for _, r := range c.Results {
		if r.SendDate.Before(c.CreatedDate) || r.SendDate.Equal(c.CreatedDate) {
			ch.Assert(r.Status, check.Equals, StatusSending)
		} else {
			ch.Assert(r.Status, check.Equals, StatusScheduled)
		}
	}
}

func (s *ModelsSuite) TestHandleEmailReport(ch *check.C) {
	c := s.createCampaign(ch)
	r := c.Results[0]

	err := r.HandleEmailReport(EventDetails{})
	ch.Assert(err, check.Equals, nil)
	ch.Assert(r.Reported, check.Equals, true)

	got, err := GetResult(r.RId)
	ch.Assert(err, check.Equals, nil)
	ch.Assert(got.Reported, check.Equals, true)
}

// TestResendNotEligible ensures Resend() refuses to act on a result that
// isn't currently in an Error status
func (s *ModelsSuite) TestResendNotEligible(ch *check.C) {
	c := s.createCampaign(ch)
	r := c.Results[0]
	ch.Assert(r.Status, check.Equals, StatusSending)

	err := r.Resend()
	ch.Assert(err, check.Equals, ErrResultNotEligibleForResend)
}

// TestResendReusesExistingMailLog covers the case where a results mail
// log still exists (eg. it errored out via a repeated transient failure
// hitting MaxSendAttempts). Resend() should reset and reuse it rather
// than creating a duplicate.
func (s *ModelsSuite) TestResendReusesExistingMailLog(ch *check.C) {
	c := s.createCampaign(ch)
	r := c.Results[0]
	r.Status = Error
	ch.Assert(db.Save(&r).Error, check.Equals, nil)

	m, err := GetMailLogByRId(r.RId)
	ch.Assert(err, check.Equals, nil)
	m.SendAttempt = 5
	m.Processing = true
	m.SendDate = time.Now().UTC().Add(-1 * time.Hour)
	ch.Assert(db.Save(m).Error, check.Equals, nil)

	ch.Assert(r.Resend(), check.Equals, nil)

	got, err := GetResult(r.RId)
	ch.Assert(err, check.Equals, nil)
	ch.Assert(got.Status, check.Equals, StatusScheduled)

	ms, err := GetMailLogsByCampaign(c.Id)
	ch.Assert(err, check.Equals, nil)
	found := 0
	for _, m := range ms {
		if m.RId != r.RId {
			continue
		}
		found++
		ch.Assert(m.SendAttempt, check.Equals, 0)
		ch.Assert(m.Processing, check.Equals, false)
		ch.Assert(m.SendDate.After(time.Now().Add(-time.Minute)), check.Equals, true)
	}
	ch.Assert(found, check.Equals, 1)
}

// TestResendGeneratesMailLogWhenDeleted covers the case where a results
// mail log was already deleted (a permanent SMTP failure deletes it, see
// MailLog.Error), Resend() should generate a fresh one rather than
// erroring
func (s *ModelsSuite) TestResendGeneratesMailLogWhenDeleted(ch *check.C) {
	c := s.createCampaign(ch)
	r := c.Results[0]
	r.Status = Error
	ch.Assert(db.Save(&r).Error, check.Equals, nil)

	m, err := GetMailLogByRId(r.RId)
	ch.Assert(err, check.Equals, nil)
	ch.Assert(db.Delete(m).Error, check.Equals, nil)
	_, err = GetMailLogByRId(r.RId)
	ch.Assert(err, check.Equals, gorm.ErrRecordNotFound)

	ch.Assert(r.Resend(), check.Equals, nil)

	got, err := GetResult(r.RId)
	ch.Assert(err, check.Equals, nil)
	ch.Assert(got.Status, check.Equals, StatusScheduled)

	newM, err := GetMailLogByRId(r.RId)
	ch.Assert(err, check.Equals, nil)
	ch.Assert(newM.SendAttempt, check.Equals, 0)
	ch.Assert(newM.Processing, check.Equals, false)
}

func (s *ModelsSuite) TestHandleEmailReportAtCustomTime(ch *check.C) {
	c := s.createCampaign(ch)
	r := c.Results[0]
	reportedTime := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Second)

	err := r.HandleEmailReportAt(EventDetails{}, reportedTime)
	ch.Assert(err, check.Equals, nil)
	ch.Assert(r.Reported, check.Equals, true)
	ch.Assert(r.ModifiedDate.Equal(reportedTime), check.Equals, true)

	got, err := GetResult(r.RId)
	ch.Assert(err, check.Equals, nil)
	ch.Assert(got.Reported, check.Equals, true)
	ch.Assert(got.ModifiedDate.Equal(reportedTime), check.Equals, true)

	campaign, err := GetCampaign(c.Id, c.UserId)
	ch.Assert(err, check.Equals, nil)
	lastEvent := campaign.Events[len(campaign.Events)-1]
	ch.Assert(lastEvent.Message, check.Equals, EventReported)
	ch.Assert(lastEvent.Time.Equal(reportedTime), check.Equals, true)
}

func (s *ModelsSuite) TestHandleEmailReportAtZeroTimeUsesNow(ch *check.C) {
	c := s.createCampaign(ch)
	r := c.Results[0]
	before := time.Now().UTC()

	err := r.HandleEmailReportAt(EventDetails{}, time.Time{})
	ch.Assert(err, check.Equals, nil)
	ch.Assert(r.ModifiedDate.Before(before), check.Equals, false)
}

func (s *ModelsSuite) TestDuplicateResults(ch *check.C) {
	group := Group{Name: "Test Group"}
	group.Targets = []Target{
		Target{BaseRecipient: BaseRecipient{Email: "test1@example.com", FirstName: "First", LastName: "Example"}},
		Target{BaseRecipient: BaseRecipient{Email: "test1@example.com", FirstName: "Duplicate", LastName: "Duplicate"}},
		Target{BaseRecipient: BaseRecipient{Email: "test2@example.com", FirstName: "Second", LastName: "Example"}},
	}
	group.UserId = 1
	ch.Assert(PostGroup(&group), check.Equals, nil)

	// Add a template
	t := Template{Name: "Test Template"}
	t.Subject = "{{.RId}} - Subject"
	t.Text = "{{.RId}} - Text"
	t.HTML = "{{.RId}} - HTML"
	t.UserId = 1
	ch.Assert(PostTemplate(&t), check.Equals, nil)

	// Add a landing page
	p := Page{Name: "Test Page"}
	p.HTML = "<html>Test</html>"
	p.UserId = 1
	ch.Assert(PostPage(&p), check.Equals, nil)

	// Add a sending profile
	smtp := SMTP{Name: "Test Page"}
	smtp.UserId = 1
	smtp.Host = "example.com"
	smtp.FromAddress = "test@test.com"
	ch.Assert(PostSMTP(&smtp), check.Equals, nil)

	c := Campaign{Name: "Test campaign"}
	c.UserId = 1
	c.Template = t
	c.Page = p
	c.SMTP = smtp
	c.Groups = []Group{group}

	ch.Assert(PostCampaign(&c, c.UserId, 0), check.Equals, nil)
	ch.Assert(len(c.Results), check.Equals, 2)
	// Results are shuffled (see TestPostCampaignShufflesTargets in
	// campaign_test.go), so we can't assert on positional order here -
	// just that deduplication produced exactly the expected set of emails.
	gotEmails := map[string]bool{}
	for _, r := range c.Results {
		gotEmails[r.Email] = true
	}
	ch.Assert(gotEmails, check.DeepEquals, map[string]bool{
		"test1@example.com": true,
		"test2@example.com": true,
	})
}

package models

import (
	"fmt"
	"testing"
	"time"

	check "gopkg.in/check.v1"
	"gorm.io/gorm"
)

func (s *ModelsSuite) TestGenerateSendDate(c *check.C) {
	// One set of dependencies is created and reused (by name) across the
	// three campaigns below
	dep := s.createCampaignDependencies(c)

	// Test that if no launch date is provided, the campaign's creation date
	// is used.
	campaign := Campaign{Name: dep.Name, UserId: dep.UserId, Template: dep.Template, Page: dep.Page, SMTP: dep.SMTP, Groups: dep.Groups}
	err := PostCampaign(&campaign, campaign.UserId, 0)
	c.Assert(err, check.Equals, nil)
	c.Assert(campaign.LaunchDate, check.Equals, campaign.CreatedDate)

	// For comparing the dates, we need to fetch the campaign again. This is
	// to solve an issue where the campaign object right now has time down to
	// the microsecond, while in MySQL it's rounded down to the second.
	campaign, _ = GetCampaign(campaign.Id, campaign.UserId)

	ms, err := GetMailLogsByCampaign(campaign.Id)
	c.Assert(err, check.Equals, nil)
	for _, m := range ms {
		c.Assert(m.SendDate, check.Equals, campaign.CreatedDate)
	}

	// Test that if no send date is provided, all the emails are sent at the
	// campaign's launch date
	campaign = Campaign{Name: dep.Name, UserId: dep.UserId, Template: dep.Template, Page: dep.Page, SMTP: dep.SMTP, Groups: dep.Groups}
	campaign.LaunchDate = time.Now().UTC()
	err = PostCampaign(&campaign, campaign.UserId, 0)
	c.Assert(err, check.Equals, nil)

	campaign, _ = GetCampaign(campaign.Id, campaign.UserId)

	ms, err = GetMailLogsByCampaign(campaign.Id)
	c.Assert(err, check.Equals, nil)
	for _, m := range ms {
		c.Assert(m.SendDate, check.Equals, campaign.LaunchDate)
	}

	// Finally, test that if a send date is provided, the emails are staggered
	// correctly.
	campaign = Campaign{Name: dep.Name, UserId: dep.UserId, Template: dep.Template, Page: dep.Page, SMTP: dep.SMTP, Groups: dep.Groups}
	campaign.LaunchDate = time.Now().UTC()
	campaign.SendByDate = campaign.LaunchDate.Add(2 * time.Minute)
	err = PostCampaign(&campaign, campaign.UserId, 0)
	c.Assert(err, check.Equals, nil)

	campaign, _ = GetCampaign(campaign.Id, campaign.UserId)

	ms, err = GetMailLogsByCampaign(campaign.Id)
	c.Assert(err, check.Equals, nil)
	sendingOffset := 2 / float64(len(ms))
	for i, m := range ms {
		expectedOffset := int(sendingOffset * float64(i))
		expectedDate := campaign.LaunchDate.Add(time.Duration(expectedOffset) * time.Minute)
		c.Assert(m.SendDate, check.Equals, expectedDate)
	}
}

// TestPostCampaignShufflesTargets ensures campaign results (and therefore
// send order) aren't created in the same order targets were added to the
// group - see #3167 upstream. With this many targets, a coincidental match
// is astronomically unlikely, so this isn't a flaky test.
func (s *ModelsSuite) TestPostCampaignShufflesTargets(c *check.C) {
	group := Group{Name: "Shuffle Test Group"}
	numTargets := 30
	for i := 0; i < numTargets; i++ {
		group.Targets = append(group.Targets, Target{
			BaseRecipient: BaseRecipient{
				Email:     fmt.Sprintf("target%02d@example.com", i),
				FirstName: "Target",
				LastName:  fmt.Sprintf("%02d", i),
			},
		})
	}
	group.UserId = 1
	c.Assert(PostGroup(&group), check.Equals, nil)

	template := Template{Name: "Shuffle Test Template"}
	template.Subject = "{{.RId}} - Subject"
	template.Text = "{{.RId}} - Text"
	template.HTML = "{{.RId}} - HTML"
	template.UserId = 1
	c.Assert(PostTemplate(&template), check.Equals, nil)

	page := Page{Name: "Shuffle Test Page"}
	page.HTML = "<html>Test</html>"
	page.UserId = 1
	c.Assert(PostPage(&page), check.Equals, nil)

	smtp := SMTP{Name: "Shuffle Test SMTP"}
	smtp.UserId = 1
	smtp.Host = "example.com"
	smtp.FromAddress = "test@test.com"
	c.Assert(PostSMTP(&smtp), check.Equals, nil)

	campaign := Campaign{Name: "Shuffle Test Campaign"}
	campaign.UserId = 1
	campaign.Template = template
	campaign.Page = page
	campaign.SMTP = smtp
	campaign.Groups = []Group{group}

	c.Assert(PostCampaign(&campaign, campaign.UserId, 0), check.Equals, nil)
	c.Assert(len(campaign.Results), check.Equals, numTargets)

	inOriginalOrder := true
	for i, r := range campaign.Results {
		if r.Email != group.Targets[i].Email {
			inOriginalOrder = false
			break
		}
	}
	c.Assert(inOriginalOrder, check.Equals, false)
}

func (s *ModelsSuite) TestCampaignDateValidation(c *check.C) {
	campaign := s.createCampaignDependencies(c)
	// If both are zero, then the campaign should start immediately with no
	// send by date
	err := campaign.Validate()
	c.Assert(err, check.Equals, nil)

	// If the launch date is specified, then the send date is optional
	campaign.LaunchDate = time.Now().UTC()
	err = campaign.Validate()
	c.Assert(err, check.Equals, nil)

	// If the send date is greater than the launch date, then there's no
	//problem
	campaign.SendByDate = campaign.LaunchDate.Add(1 * time.Minute)
	err = campaign.Validate()
	c.Assert(err, check.Equals, nil)

	// If the send date is less than the launch date, then there's an issue
	campaign.SendByDate = campaign.LaunchDate.Add(-1 * time.Minute)
	err = campaign.Validate()
	c.Assert(err, check.Equals, ErrInvalidSendByDate)
}

func (s *ModelsSuite) TestLaunchCampaignMaillogStatus(c *check.C) {
	// For the first test, ensure that campaigns created with the zero date
	// (and therefore are set to launch immediately) have maillogs that are
	// locked to prevent race conditions.
	campaign := s.createCampaign(c)
	ms, err := GetMailLogsByCampaign(campaign.Id)
	c.Assert(err, check.Equals, nil)

	for _, m := range ms {
		c.Assert(m.Processing, check.Equals, true)
	}

	// Next, verify that campaigns scheduled in the future do not lock the
	// maillogs so that they can be picked up by the background worker.
	// Reuses the same dependencies (by name) as above rather than calling
	// createCampaignDependencies again
	newCampaign := Campaign{Name: "New Campaign", UserId: campaign.UserId, Template: campaign.Template, Page: campaign.Page, SMTP: campaign.SMTP, Groups: []Group{{Name: "Test Group"}}}
	newCampaign.LaunchDate = time.Now().Add(1 * time.Hour)
	c.Assert(PostCampaign(&newCampaign, newCampaign.UserId, 0), check.Equals, nil)
	ms, err = GetMailLogsByCampaign(newCampaign.Id)
	c.Assert(err, check.Equals, nil)

	for _, m := range ms {
		c.Assert(m.Processing, check.Equals, false)
	}
}

func (s *ModelsSuite) TestDeleteCampaignAlsoDeletesMailLogs(c *check.C) {
	campaign := s.createCampaign(c)
	ms, err := GetMailLogsByCampaign(campaign.Id)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(ms), check.Equals, len(campaign.Results))

	err = DeleteCampaign(campaign.Id)
	c.Assert(err, check.Equals, nil)

	ms, err = GetMailLogsByCampaign(campaign.Id)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(ms), check.Equals, 0)
}

func (s *ModelsSuite) TestCompleteCampaignAlsoDeletesMailLogs(c *check.C) {
	campaign := s.createCampaign(c)
	ms, err := GetMailLogsByCampaign(campaign.Id)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(ms), check.Equals, len(campaign.Results))

	err = CompleteCampaign(campaign.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)

	ms, err = GetMailLogsByCampaign(campaign.Id)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(ms), check.Equals, 0)
}

func (s *ModelsSuite) TestCampaignGetResults(c *check.C) {
	campaign := s.createCampaign(c)
	got, err := GetCampaign(campaign.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(campaign.Results), check.Equals, len(got.Results))
}

// TestGetCampaignPhishContext verifies that the lightweight context used to
// serve landing pages carries the fields it needs, but - unlike GetCampaign
// - doesn't load the campaign's Results/Events history. That history is
// what made every landing page hit scale with the campaign's size; see
// gophish/gophish#2527.
func (s *ModelsSuite) TestGetCampaignPhishContext(c *check.C) {
	campaign := s.createCampaignDependencies(c)
	campaign.URL = "http://example.com"
	err := PostCampaign(&campaign, campaign.UserId, 0)
	c.Assert(err, check.Equals, nil)

	got, err := GetCampaignPhishContext(campaign.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)
	c.Assert(got.Status, check.Equals, campaign.Status)
	c.Assert(got.PageId, check.Equals, campaign.PageId)
	c.Assert(got.URL, check.Equals, "http://example.com")
	c.Assert(got.SMTP.FromAddress, check.Equals, campaign.SMTP.FromAddress)
	c.Assert(len(got.Results), check.Equals, 0)
	c.Assert(len(got.Events), check.Equals, 0)

	// Sanity check that this campaign actually has results/events, so the
	// assertions above are meaningful rather than vacuously true.
	full, err := GetCampaign(campaign.Id, campaign.UserId)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(full.Results) > 0, check.Equals, true)
	c.Assert(len(full.Events) > 0, check.Equals, true)
}

func setupCampaignDependencies(b *testing.B, size int) {
	group := Group{Name: "Test Group"}
	// Create a large group of 5000 members
	for i := 0; i < size; i++ {
		group.Targets = append(group.Targets, Target{BaseRecipient: BaseRecipient{Email: fmt.Sprintf("test%d@example.com", i), FirstName: "User", LastName: fmt.Sprintf("%d", i)}})
	}
	group.UserId = 1
	err := PostGroup(&group)
	if err != nil {
		b.Fatalf("error posting group: %v", err)
	}

	// Add a template
	template := Template{Name: "Test Template"}
	template.Subject = "{{.RId}} - Subject"
	template.Text = "{{.RId}} - Text"
	template.HTML = "{{.RId}} - HTML"
	template.UserId = 1
	err = PostTemplate(&template)
	if err != nil {
		b.Fatalf("error posting template: %v", err)
	}

	// Add a landing page
	p := Page{Name: "Test Page"}
	p.HTML = "<html>Test</html>"
	p.UserId = 1
	err = PostPage(&p)
	if err != nil {
		b.Fatalf("error posting page: %v", err)
	}

	// Add a sending profile
	smtp := SMTP{Name: "Test Page"}
	smtp.UserId = 1
	smtp.Host = "example.com"
	smtp.FromAddress = "test@test.com"
	err = PostSMTP(&smtp)
	if err != nil {
		b.Fatalf("error posting smtp: %v", err)
	}
}

// setupCampaign sets up the campaign dependencies as well as posting the
// actual campaign
func setupCampaign(b *testing.B, size int) Campaign {
	setupCampaignDependencies(b, size)
	campaign := Campaign{Name: "Test campaign"}
	campaign.UserId = 1
	campaign.Template = Template{Name: "Test Template"}
	campaign.Page = Page{Name: "Test Page"}
	campaign.SMTP = SMTP{Name: "Test Page"}
	campaign.Groups = []Group{Group{Name: "Test Group"}}
	err := PostCampaign(&campaign, 1, 0)
	if err != nil {
		b.Fatalf("error posting campaign: %v", err)
	}
	return campaign
}

func BenchmarkCampaign100(b *testing.B) {
	setupBenchmark(b)
	setupCampaignDependencies(b, 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		campaign := Campaign{Name: "Test campaign"}
		campaign.UserId = 1
		campaign.Template = Template{Name: "Test Template"}
		campaign.Page = Page{Name: "Test Page"}
		campaign.SMTP = SMTP{Name: "Test Page"}
		campaign.Groups = []Group{Group{Name: "Test Group"}}

		b.StartTimer()
		err := PostCampaign(&campaign, 1, 0)
		if err != nil {
			b.Fatalf("error posting campaign: %v", err)
		}
		b.StopTimer()
		gdb := db.Session(&gorm.Session{AllowGlobalUpdate: true})
		gdb.Delete(Result{})
		gdb.Delete(MailLog{})
		gdb.Delete(Campaign{})
	}
	tearDownBenchmark(b)
}

func BenchmarkCampaign1000(b *testing.B) {
	setupBenchmark(b)
	setupCampaignDependencies(b, 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		campaign := Campaign{Name: "Test campaign"}
		campaign.UserId = 1
		campaign.Template = Template{Name: "Test Template"}
		campaign.Page = Page{Name: "Test Page"}
		campaign.SMTP = SMTP{Name: "Test Page"}
		campaign.Groups = []Group{Group{Name: "Test Group"}}

		b.StartTimer()
		err := PostCampaign(&campaign, 1, 0)
		if err != nil {
			b.Fatalf("error posting campaign: %v", err)
		}
		b.StopTimer()
		gdb := db.Session(&gorm.Session{AllowGlobalUpdate: true})
		gdb.Delete(Result{})
		gdb.Delete(MailLog{})
		gdb.Delete(Campaign{})
	}
	tearDownBenchmark(b)
}

func BenchmarkCampaign10000(b *testing.B) {
	setupBenchmark(b)
	setupCampaignDependencies(b, 10000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		campaign := Campaign{Name: "Test campaign"}
		campaign.UserId = 1
		campaign.Template = Template{Name: "Test Template"}
		campaign.Page = Page{Name: "Test Page"}
		campaign.SMTP = SMTP{Name: "Test Page"}
		campaign.Groups = []Group{Group{Name: "Test Group"}}

		b.StartTimer()
		err := PostCampaign(&campaign, 1, 0)
		if err != nil {
			b.Fatalf("error posting campaign: %v", err)
		}
		b.StopTimer()
		gdb := db.Session(&gorm.Session{AllowGlobalUpdate: true})
		gdb.Delete(Result{})
		gdb.Delete(MailLog{})
		gdb.Delete(Campaign{})
	}
	tearDownBenchmark(b)
}

func BenchmarkGetCampaign100(b *testing.B) {
	setupBenchmark(b)
	campaign := setupCampaign(b, 100)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GetCampaign(campaign.Id, campaign.UserId)
		if err != nil {
			b.Fatalf("error getting campaign: %v", err)
		}
	}
	tearDownBenchmark(b)
}

func BenchmarkGetCampaign1000(b *testing.B) {
	setupBenchmark(b)
	campaign := setupCampaign(b, 1000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GetCampaign(campaign.Id, campaign.UserId)
		if err != nil {
			b.Fatalf("error getting campaign: %v", err)
		}
	}
	tearDownBenchmark(b)
}

func BenchmarkGetCampaign5000(b *testing.B) {
	setupBenchmark(b)
	campaign := setupCampaign(b, 5000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GetCampaign(campaign.Id, campaign.UserId)
		if err != nil {
			b.Fatalf("error getting campaign: %v", err)
		}
	}
	tearDownBenchmark(b)
}

func BenchmarkGetCampaign10000(b *testing.B) {
	setupBenchmark(b)
	campaign := setupCampaign(b, 10000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := GetCampaign(campaign.Id, campaign.UserId)
		if err != nil {
			b.Fatalf("error getting campaign: %v", err)
		}
	}
	tearDownBenchmark(b)
}

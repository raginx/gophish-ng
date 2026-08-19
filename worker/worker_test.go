package worker

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gophish/gophish/config"
	"github.com/gophish/gophish/mailer"
	"github.com/gophish/gophish/models"
)

type logMailer struct {
	queue chan []mailer.Mail
}

func (m *logMailer) Start(ctx context.Context) {}

func (m *logMailer) Queue(ms []mailer.Mail) {
	m.queue <- ms
}

// testContext is context to cover API related functions
type testContext struct {
	config *config.Config
}

func setupTest(t *testing.T) *testContext {
	conf := &config.Config{
		DBName:         "sqlite3",
		DBPath:         ":memory:",
		MigrationsPath: "../db/db_sqlite3/migrations/",
	}
	err := models.Setup(conf)
	if err != nil {
		t.Fatalf("Failed creating database: %v", err)
	}
	ctx := &testContext{}
	ctx.config = conf
	createTestData(t, ctx)
	return ctx
}

func createTestData(t *testing.T, ctx *testContext) {
	ctx.config.TestFlag = true
	// Add a group
	group := models.Group{Name: "Test Group"}
	for i := 0; i < 10; i++ {
		group.Targets = append(group.Targets, models.Target{
			BaseRecipient: models.BaseRecipient{
				Email:     fmt.Sprintf("test%d@example.com", i),
				FirstName: "First",
				LastName:  "Example"}})
	}
	group.UserId = 1
	if err := models.PostGroup(&group); err != nil {
		t.Fatalf("error posting group: %v", err)
	}

	// Add a template
	template := models.Template{Name: "Test Template"}
	template.Subject = "Test subject"
	template.Text = "Text text"
	template.HTML = "<html>Test</html>"
	template.UserId = 1
	if err := models.PostTemplate(&template); err != nil {
		t.Fatalf("error posting template: %v", err)
	}

	// Add a landing page
	p := models.Page{Name: "Test Page"}
	p.HTML = "<html>Test</html>"
	p.UserId = 1
	if err := models.PostPage(&p); err != nil {
		t.Fatalf("error posting page: %v", err)
	}

	// Add a sending profile
	smtp := models.SMTP{Name: "Test Page"}
	smtp.UserId = 1
	smtp.Host = "example.com"
	smtp.FromAddress = "test@test.com"
	if err := models.PostSMTP(&smtp); err != nil {
		t.Fatalf("error posting SMTP profile: %v", err)
	}
}

func setupCampaign(id int) (*models.Campaign, error) {
	// Setup and "launch" our campaign
	// Set the status such that no emails are attempted
	c := models.Campaign{Name: fmt.Sprintf("Test campaign - %d", id)}
	c.UserId = 1
	template, err := models.GetTemplate(1, 1)
	if err != nil {
		return nil, err
	}
	c.Template = template

	page, err := models.GetPage(1, 1)
	if err != nil {
		return nil, err
	}
	c.Page = page

	smtp, err := models.GetSMTP(1, 1)
	if err != nil {
		return nil, err
	}
	c.SMTP = smtp

	group, err := models.GetGroup(1, 1)
	if err != nil {
		return nil, err
	}
	c.Groups = []models.Group{group}
	err = models.PostCampaign(&c, c.UserId, 0)
	if err != nil {
		return nil, err
	}
	err = c.UpdateStatus(models.CampaignEmailsSent)
	return &c, err
}

func TestMailLogGrouping(t *testing.T) {
	setupTest(t)

	// Create the campaigns and unlock the maillogs so that they're picked up
	// by the worker
	for i := 0; i < 10; i++ {
		campaign, err := setupCampaign(i)
		if err != nil {
			t.Fatalf("error creating campaign: %v", err)
		}
		ms, err := models.GetMailLogsByCampaign(campaign.Id)
		if err != nil {
			t.Fatalf("error getting maillogs for campaign: %v", err)
		}
		for _, m := range ms {
			if err := m.Unlock(); err != nil {
				t.Fatalf("error unlocking maillog: %v", err)
			}
		}
	}

	lm := &logMailer{queue: make(chan []mailer.Mail)}
	worker := &DefaultWorker{}
	worker.mailer = lm

	// Trigger the worker, generating the maillogs and sending them to the
	// mailer
	if err := worker.processCampaigns(time.Now()); err != nil {
		t.Fatalf("error processing campaigns: %v", err)
	}

	// Verify that each slice of maillogs received belong to the same campaign
	for i := 0; i < 10; i++ {
		ms := <-lm.queue
		maillog, ok := ms[0].(*models.MailLog)
		if !ok {
			t.Fatalf("unable to cast mail to models.MailLog")
		}
		expected := maillog.CampaignId
		for _, m := range ms {
			maillog, ok = m.(*models.MailLog)
			if !ok {
				t.Fatalf("unable to cast mail to models.MailLog")
			}
			got := maillog.CampaignId
			if got != expected {
				t.Fatalf("unexpected campaign ID received for maillog: got %d expected %d", got, expected)
			}
		}
	}
}

// TestProcessCampaignsSkipsFailedCampaignContext verifies that if
// GetCampaignMailContext fails for one campaign (e.g. its sending profile
// was deleted out from under it), processCampaigns still sends mail for
// every other campaign, and unlocks the failed campaign's maillogs instead
// of leaving them stuck until a full service restart.
func TestProcessCampaignsSkipsFailedCampaignContext(t *testing.T) {
	setupTest(t)

	// A second, independent sending profile so we can break just one
	// campaign's context without affecting the others.
	smtp := models.SMTP{Name: "Broken Profile"}
	smtp.UserId = 1
	smtp.Host = "example.com"
	smtp.FromAddress = "broken@test.com"
	if err := models.PostSMTP(&smtp); err != nil {
		t.Fatalf("error creating second sending profile: %v", err)
	}

	goodCampaign, err := setupCampaign(0)
	if err != nil {
		t.Fatalf("error creating good campaign: %v", err)
	}

	template, err := models.GetTemplate(1, 1)
	if err != nil {
		t.Fatalf("error getting template: %v", err)
	}
	page, err := models.GetPage(1, 1)
	if err != nil {
		t.Fatalf("error getting page: %v", err)
	}
	group, err := models.GetGroup(1, 1)
	if err != nil {
		t.Fatalf("error getting group: %v", err)
	}
	badCampaign := models.Campaign{Name: "Bad campaign"}
	badCampaign.UserId = 1
	badCampaign.Template = template
	badCampaign.Page = page
	badCampaign.SMTP = smtp
	badCampaign.Groups = []models.Group{group}
	if err := models.PostCampaign(&badCampaign, badCampaign.UserId, 0); err != nil {
		t.Fatalf("error creating bad campaign: %v", err)
	}
	if err := badCampaign.UpdateStatus(models.CampaignEmailsSent); err != nil {
		t.Fatalf("error updating bad campaign status: %v", err)
	}

	// Unlock the maillogs for both campaigns so the worker picks them up.
	for _, c := range []*models.Campaign{goodCampaign, &badCampaign} {
		ms, err := models.GetMailLogsByCampaign(c.Id)
		if err != nil {
			t.Fatalf("error getting maillogs for campaign: %v", err)
		}
		for _, m := range ms {
			if err := m.Unlock(); err != nil {
				t.Fatalf("error unlocking maillog: %v", err)
			}
		}
	}

	// Break the bad campaign's context by deleting its sending profile.
	if err := models.DeleteSMTP(smtp.Id, 1); err != nil {
		t.Fatalf("error deleting sending profile: %v", err)
	}

	lm := &logMailer{queue: make(chan []mailer.Mail, 10)}
	worker := &DefaultWorker{}
	worker.mailer = lm

	if err := worker.processCampaigns(time.Now()); err != nil {
		t.Fatalf("processCampaigns returned an error: %v", err)
	}

	// The good campaign's mail should still have been queued for sending.
	select {
	case ms := <-lm.queue:
		maillog, ok := ms[0].(*models.MailLog)
		if !ok {
			t.Fatalf("unable to cast mail to models.MailLog")
		}
		if maillog.CampaignId != goodCampaign.Id {
			t.Fatalf("unexpected campaign ID received for maillog: got %d expected %d", maillog.CampaignId, goodCampaign.Id)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for the good campaign's mail to be queued")
	}

	// The bad campaign's maillogs must be unlocked again, not stranded.
	badMs, err := models.GetMailLogsByCampaign(badCampaign.Id)
	if err != nil {
		t.Fatalf("error getting maillogs for bad campaign: %v", err)
	}
	for _, m := range badMs {
		if m.Processing {
			t.Fatalf("expected maillog %s to be unlocked after a failed campaign context, but it's still locked", m.RId)
		}
	}
}

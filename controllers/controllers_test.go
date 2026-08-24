package controllers

import (
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/raginx/gophish-ng/auth"
	"github.com/raginx/gophish-ng/config"
	"github.com/raginx/gophish-ng/models"
)

// testContext is the data required to test API related functions
type testContext struct {
	apiKey      string
	config      *config.Config
	adminServer *httptest.Server
	phishServer *httptest.Server
	origPath    string
}

func setupTest(t *testing.T) *testContext {
	wd, _ := os.Getwd()
	fmt.Println(wd)
	conf := &config.Config{
		DBName:         "sqlite3",
		DBPath:         ":memory:",
		MigrationsPath: "../db/db_sqlite3/migrations/",
	}
	abs, _ := filepath.Abs("../db/db_sqlite3/migrations/")
	fmt.Printf("in controllers_test.go: %s\n", abs)
	err := models.Setup(conf)
	if err != nil {
		t.Fatalf("error setting up database: %v", err)
	}
	ctx := &testContext{}
	ctx.config = conf
	ctx.adminServer = httptest.NewUnstartedServer(NewAdminServer(ctx.config.AdminConf).server.Handler)
	ctx.adminServer.Config.Addr = ctx.config.AdminConf.ListenURL
	ctx.adminServer.Start()
	// Get the API key to use for these tests
	u, err := models.GetUser(1)
	if err != nil {
		t.Fatalf("error getting first user from database: %v", err)
	}
	// Reset the temporary password for the admin user to a value we control
	hash, err := auth.GeneratePasswordHash("gophish")
	if err != nil {
		t.Fatalf("error hashing password: %v", err)
	}
	u.Hash = hash
	if err := models.PutUser(&u); err != nil {
		t.Fatalf("error updating first user: %v", err)
	}

	// Create a second user to test account locked status
	u2 := models.User{Username: "houdini", Hash: hash, AccountLocked: true}
	if err := models.PutUser(&u2); err != nil {
		t.Fatalf("error creating new user: %v", err)
	}

	ctx.apiKey = u.ApiKey
	// Start the phishing server
	ctx.phishServer = httptest.NewUnstartedServer(NewPhishingServer(ctx.config.PhishConf).server.Handler)
	ctx.phishServer.Config.Addr = ctx.config.PhishConf.ListenURL
	ctx.phishServer.Start()
	// Move our cwd up to the project root for help with resolving
	// static assets
	origPath, _ := os.Getwd()
	ctx.origPath = origPath
	err = os.Chdir("../")
	if err != nil {
		t.Fatalf("error changing directories to setup asset discovery: %v", err)
	}
	createTestData(t)
	return ctx
}

func tearDown(t *testing.T, ctx *testContext) {
	// Tear down the admin and phishing servers
	ctx.adminServer.Close()
	ctx.phishServer.Close()
	// Reset the path for the next test
	if err := os.Chdir(ctx.origPath); err != nil {
		t.Fatalf("error restoring working directory: %v", err)
	}
}

func createTestData(t *testing.T) {
	// Add a group
	group := models.Group{Name: "Test Group"}
	group.Targets = []models.Target{
		models.Target{BaseRecipient: models.BaseRecipient{Email: "test1@example.com", FirstName: "First", LastName: "Example"}},
		models.Target{BaseRecipient: models.BaseRecipient{Email: "test2@example.com", FirstName: "Second", LastName: "Example"}},
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

	// Setup and "launch" our campaign
	// Set the status such that no emails are attempted
	c := models.Campaign{Name: "Test campaign"}
	c.UserId = 1
	c.Template = template
	c.Page = p
	c.SMTP = smtp
	c.Groups = []models.Group{group}
	if err := models.PostCampaign(&c, c.UserId, 0); err != nil {
		t.Fatalf("error posting campaign: %v", err)
	}
	if err := c.UpdateStatus(models.CampaignEmailsSent); err != nil {
		t.Fatalf("error updating campaign status: %v", err)
	}
}

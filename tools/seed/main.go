// Command seed populates a Gophish database with realistic-looking demo
// data (groups, sending profiles, landing pages, templates, and campaigns
// with varied results) - useful for taking screenshots or otherwise poking
// around a UI that isn't empty.
//
// It goes through the same models.Post*/Handle* functions the application
// itself uses (not raw SQL), so the data is exactly as valid as anything
// created through the UI.
//
// Safe to re-run: each entity is looked up by name first and reused if it
// already exists, so running this twice won't create duplicates.
//
// Usage:
//
//	go run ./tools/seed -config path/to/config.json
//
// Point -config at a scratch/dev config, not a production database.
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/url"
	"time"

	"github.com/raginx/gophish-ng/config"
	"github.com/raginx/gophish-ng/models"
)

const demoDomain = "example.com"

func main() {
	configPath := flag.String("config", "config.json", "Path to config.json")
	flag.Parse()

	conf, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("error loading config: %v", err)
	}
	if err := models.Setup(conf); err != nil {
		log.Fatalf("error setting up database: %v", err)
	}

	// db connection for timestamp backdating
	rawDB, err := openRawDB(conf)
	if err != nil {
		log.Fatalf("error opening raw db connection: %v", err)
	}

	defer func() { _ = rawDB.Close() }()

	admin, err := models.GetUser(1)
	if err != nil {
		log.Fatalf("error getting admin user (expected user id 1 to exist - has the app been started at least once?): %v", err)
	}
	teamID := admin.TeamID

	rng := rand.New(rand.NewSource(42))

	marketing := getOrCreateGroup(admin.Id, teamID, "Marketing Team", []models.Target{
		target("Alex", "Morgan", "Marketing Manager"),
		target("Jamie", "Chen", "Content Strategist"),
		target("Taylor", "Reyes", "Social Media Coordinator"),
		target("Morgan", "Patel", "Brand Designer"),
		target("Casey", "Nguyen", "Marketing Analyst"),
		target("Jordan", "Kim", "Campaign Manager"),
		target("Riley", "Osei", "SEO Specialist"),
		target("Drew", "Fischer", "Marketing Intern"),
	})

	finance := getOrCreateGroup(admin.Id, teamID, "Finance Team", []models.Target{
		target("Sam", "Okafor", "Financial Controller"),
		target("Robin", "Alvarez", "Accounts Payable"),
		target("Charlie", "Novak", "Payroll Specialist"),
		target("Avery", "Silva", "Financial Analyst"),
		target("Quinn", "Larsen", "Accounts Receivable"),
		target("Skyler", "Haddad", "Finance Intern"),
	})

	engineering := getOrCreateGroup(admin.Id, teamID, "Engineering", bulkTargets(220, []string{
		"Software Engineer", "Senior Software Engineer", "Staff Engineer",
		"Engineering Manager", "QA Engineer", "DevOps Engineer",
		"Site Reliability Engineer", "Data Engineer", "Security Engineer",
	}))

	sales := getOrCreateGroup(admin.Id, teamID, "Sales", bulkTargets(140, []string{
		"Account Executive", "Sales Development Rep", "Sales Manager",
		"Solutions Engineer", "Customer Success Manager", "Regional Sales Director",
	}))

	support := getOrCreateGroup(admin.Id, teamID, "Customer Support", bulkTargets(90, []string{
		"Support Engineer", "Support Team Lead", "Technical Support Specialist",
		"Customer Support Representative",
	}))

	hr := getOrCreateGroup(admin.Id, teamID, "Human Resources", bulkTargets(35, []string{
		"HR Business Partner", "Recruiter", "HR Generalist", "Benefits Administrator",
	}))

	allEmployees := getOrCreateGroup(admin.Id, teamID, "All Employees", bulkTargets(1500, []string{
		"Software Engineer", "Account Executive", "Support Engineer", "Marketing Manager",
		"Financial Analyst", "HR Business Partner", "Product Manager", "Data Analyst",
		"Operations Manager", "Recruiter", "Sales Manager", "Customer Success Manager",
	}))

	o365 := getOrCreateSMTP(admin.Id, teamID, "Corporate O365 Relay", func(s *models.SMTP) {
		s.Host = "smtp.office365.com:587"
		s.FromAddress = fmt.Sprintf("it-support@%s", demoDomain)
		s.SendRate = 10
	})

	internalRelay := getOrCreateSMTP(admin.Id, teamID, "Internal Relay (Rate Limited)", func(s *models.SMTP) {
		s.Host = fmt.Sprintf("mail.%s:25", demoDomain)
		s.FromAddress = fmt.Sprintf("notifications@%s", demoDomain)
		s.SendRate = 2
		s.CC = fmt.Sprintf("security-team@%s", demoDomain)
	})

	pwPage := getOrCreatePage(admin.Id, teamID, "Password Reset Portal", func(p *models.Page) {
		p.HTML = polishedLandingPageHTML()
		p.CaptureCredentials = true
		p.CapturePasswords = true
	})

	docPage := getOrCreatePage(admin.Id, teamID, "Shared Document Login", func(p *models.Page) {
		p.HTML = landingPageHTML("Document Access", "Sign in to view the document that was shared with you.")
		p.CaptureCredentials = true
		p.CapturePasswords = true
	})

	portalPage := getOrCreatePage(admin.Id, teamID, "Employee Portal Login", func(p *models.Page) {
		p.HTML = landingPageHTML("Employee Portal", "Enter your employee ID to continue.")
		p.CaptureCredentials = true
		p.CapturePasswords = false
	})

	securityTemplate := getOrCreateTemplate(admin.Id, teamID, "IT Security Alert - Password Expiring", func(t *models.Template) {
		t.Subject = "Action Required: Your password expires in 24 hours"
		t.HTML = polishedEmailHTML()
	})

	docTemplate := getOrCreateTemplate(admin.Id, teamID, "Shared Document Notification", func(t *models.Template) {
		t.EnvelopeSender = fmt.Sprintf("notifications@%s", demoDomain)
		t.Subject = "A document has been shared with you: Q3_Budget_Review.xlsx"
		t.HTML = emailHTML(
			"Hi {{.FirstName}},",
			`{{.From}} shared a spreadsheet with you. Click below to view "Q3_Budget_Review.xlsx".`,
			"View Document",
		)
	})

	resetTemplate := getOrCreateTemplate(admin.Id, teamID, "Password Reset Confirmation", func(t *models.Template) {
		t.Subject = "Please confirm your password reset request"
		t.HTML = emailHTML(
			"Hi {{.FirstName}},",
			`We received a request to reset the password for your account ({{.Email}}). If you
			made this request, confirm it below.`,
			"Confirm Reset",
		)
	})

	// Completed campaign, good defense (moderate report rate)
	seedCampaign(admin.Id, teamID, campaignSpec{
		name:       "Q3 Security Awareness - Marketing",
		group:      marketing,
		template:   securityTemplate,
		page:       pwPage,
		smtp:       o365,
		launchDate: time.Now().Add(-14 * 24 * time.Hour),
		openRate:   0.70,
		clickRate:  0.45,
		submitRate: 0.40,
		reportRate: 0.15,
		complete:   true,
	}, rng, rawDB)

	// Completed campaign, strong defense (finance team reports well)
	seedCampaign(admin.Id, teamID, campaignSpec{
		name:       "Q3 Security Awareness - Finance",
		group:      finance,
		template:   docTemplate,
		page:       docPage,
		smtp:       o365,
		launchDate: time.Now().Add(-10 * 24 * time.Hour),
		openRate:   0.60,
		clickRate:  0.20,
		submitRate: 0.10,
		reportRate: 0.35,
		complete:   true,
	}, rng, rawDB)

	// In-progress campaign, launched recently, not yet complete
	seedCampaign(admin.Id, teamID, campaignSpec{
		name:       "Password Reset Phish - Follow-up",
		group:      marketing,
		template:   resetTemplate,
		page:       pwPage,
		smtp:       internalRelay,
		launchDate: time.Now().Add(-2 * 24 * time.Hour),
		openRate:   0.50,
		clickRate:  0.10,
		submitRate: 0.0,
		reportRate: 0.10,
		complete:   false,
	}, rng, rawDB)

	// Scheduled for the future - nothing simulated, shows the "Queued" state
	seedCampaign(admin.Id, teamID, campaignSpec{
		name:       "New Hire Onboarding Test",
		group:      finance,
		template:   securityTemplate,
		page:       portalPage,
		smtp:       internalRelay,
		launchDate: time.Now().Add(2 * 24 * time.Hour),
	}, rng, rawDB)

	// Large-scale campaign demonstrate that Gophish holds up
	// at real organization size
	seedCampaign(admin.Id, teamID, campaignSpec{
		name:       "Annual Company-Wide Phishing Assessment",
		group:      allEmployees,
		template:   securityTemplate,
		page:       pwPage,
		smtp:       o365,
		launchDate: time.Now().Add(-30 * 24 * time.Hour),
		openRate:   0.55,
		clickRate:  0.30,
		submitRate: 0.20,
		reportRate: 0.20,
		complete:   true,
	}, rng, rawDB)

	seedCampaign(admin.Id, teamID, campaignSpec{
		name:       "Engineering Security Refresher",
		group:      engineering,
		template:   docTemplate,
		page:       docPage,
		smtp:       o365,
		launchDate: time.Now().Add(-20 * 24 * time.Hour),
		openRate:   0.45,
		clickRate:  0.15,
		submitRate: 0.05,
		reportRate: 0.45,
		complete:   true,
	}, rng, rawDB)

	seedCampaign(admin.Id, teamID, campaignSpec{
		name:       "Sales Team Social Engineering Test",
		group:      sales,
		template:   resetTemplate,
		page:       pwPage,
		smtp:       internalRelay,
		launchDate: time.Now().Add(-3 * 24 * time.Hour),
		openRate:   0.65,
		clickRate:  0.40,
		submitRate: 0.25,
		reportRate: 0.05,
		complete:   false,
	}, rng, rawDB)

	seedCampaign(admin.Id, teamID, campaignSpec{
		name:       "Support Team Phishing Drill",
		group:      support,
		template:   docTemplate,
		page:       docPage,
		smtp:       o365,
		launchDate: time.Now().Add(-7 * 24 * time.Hour),
		openRate:   0.50,
		clickRate:  0.25,
		submitRate: 0.10,
		reportRate: 0.25,
		complete:   true,
	}, rng, rawDB)

	seedCampaign(admin.Id, teamID, campaignSpec{
		name:       "HR Onboarding Documents Phish",
		group:      hr,
		template:   resetTemplate,
		page:       portalPage,
		smtp:       internalRelay,
		launchDate: time.Now().Add(-5 * 24 * time.Hour),
		openRate:   0.60,
		clickRate:  0.20,
		submitRate: 0.15,
		reportRate: 0.20,
		complete:   true,
	}, rng, rawDB)

	fmt.Println("\nDemo data seeded. Log in as the admin user Gophish created on first startup to view it.")
}

func target(first, last, position string) models.Target {
	email := fmt.Sprintf("%s.%s@%s", first, last, demoDomain)
	return models.Target{
		BaseRecipient: models.BaseRecipient{
			FirstName: first,
			LastName:  last,
			Email:     email,
			Position:  position,
		},
	}
}

// firstNames and lastNames are combined by bulkTargets to generate n
// distinct people without listing them all
var firstNames = []string{
	"Alex", "Jamie", "Taylor", "Morgan", "Casey", "Jordan", "Riley", "Drew",
	"Sam", "Robin", "Charlie", "Avery", "Quinn", "Skyler", "Cameron", "Reese",
	"Rowan", "Emerson", "Finley", "Harper", "Hayden", "Jesse", "Kendall",
	"Logan", "Micah", "Parker", "Peyton", "Sage", "Shawn", "Sydney", "Blake",
	"Dakota", "Elliot", "Frankie", "Gray", "Kai", "Lane", "Marley", "Noel",
	"Remy", "Ariel", "Bailey", "Corey", "Devon", "Ellis", "Frankie", "Grier",
	"Hollis", "Indigo", "Justice", "Kris", "Lior", "Milan", "Nico",
}

var lastNames = []string{
	"Morgan", "Chen", "Reyes", "Patel", "Nguyen", "Kim", "Osei", "Fischer",
	"Okafor", "Alvarez", "Novak", "Silva", "Larsen", "Haddad", "Johansson",
	"Petrov", "Delgado", "Yamamoto", "Kowalski", "Mensah", "Andersen",
	"Ibrahim", "Costa", "Tanaka", "Schmidt", "Rossi", "Dubois", "Muller",
	"Nakamura", "Santos", "Kovac", "Volkov", "Haas", "Berg", "Fontaine",
	"Adeyemi", "Choi", "Marin", "Weiss", "Lindqvist", "Sorensen", "Park",
	"Hassan", "Moreau", "Ferreira",
}

// bulkTargets procedurally generates n targets with distinct name pairs
// and positions cycled from the given pool
func bulkTargets(n int, positions []string) []models.Target {
	maxUnique := len(firstNames) * len(lastNames)
	if n > maxUnique {
		log.Fatalf("bulkTargets: requested %d targets but only %d unique names available", n, maxUnique)
	}
	targets := make([]models.Target, n)
	for i := 0; i < n; i++ {
		first := firstNames[i%len(firstNames)]
		last := lastNames[(i/len(firstNames))%len(lastNames)]
		position := positions[i%len(positions)]
		targets[i] = target(first, last, position)
	}
	return targets
}

func getOrCreateGroup(uid, teamID int64, name string, targets []models.Target) models.Group {
	if g, err := models.GetGroupByName(name, teamID); err == nil {
		fmt.Printf("group %q already exists, reusing\n", name)
		return g
	}
	g := models.Group{Name: name, UserId: uid, Targets: targets}
	if err := models.PostGroup(&g); err != nil {
		log.Fatalf("error creating group %q: %v", name, err)
	}
	fmt.Printf("created group %q (%d targets)\n", name, len(targets))
	return g
}

func getOrCreateSMTP(uid, teamID int64, name string, configure func(*models.SMTP)) models.SMTP {
	if s, err := models.GetSMTPByName(name, teamID); err == nil {
		fmt.Printf("sending profile %q already exists, reusing\n", name)
		return s
	}
	s := models.SMTP{Name: name, UserId: uid}
	configure(&s)
	if err := models.PostSMTP(&s); err != nil {
		log.Fatalf("error creating sending profile %q: %v", name, err)
	}
	fmt.Printf("created sending profile %q\n", name)
	return s
}

func getOrCreatePage(uid, teamID int64, name string, configure func(*models.Page)) models.Page {
	if p, err := models.GetPageByName(name, teamID); err == nil {
		fmt.Printf("landing page %q already exists, reusing\n", name)
		return p
	}
	p := models.Page{Name: name, UserId: uid}
	configure(&p)
	if err := models.PostPage(&p); err != nil {
		log.Fatalf("error creating landing page %q: %v", name, err)
	}
	fmt.Printf("created landing page %q\n", name)
	return p
}

func getOrCreateTemplate(uid, teamID int64, name string, configure func(*models.Template)) models.Template {
	if t, err := models.GetTemplateByName(name, teamID); err == nil {
		fmt.Printf("template %q already exists, reusing\n", name)
		return t
	}
	t := models.Template{Name: name, UserId: uid}
	configure(&t)
	if err := models.PostTemplate(&t); err != nil {
		log.Fatalf("error creating template %q: %v", name, err)
	}
	fmt.Printf("created template %q\n", name)
	return t
}

type campaignSpec struct {
	name       string
	group      models.Group
	template   models.Template
	page       models.Page
	smtp       models.SMTP
	launchDate time.Time
	openRate   float64
	clickRate  float64
	submitRate float64
	reportRate float64
	complete   bool
}

func seedCampaign(uid, teamID int64, spec campaignSpec, rng *rand.Rand, rawDB *sql.DB) {
	existing, err := models.GetCampaigns(teamID)
	if err != nil {
		log.Fatalf("error listing campaigns: %v", err)
	}
	for _, c := range existing {
		if c.Name == spec.name {
			fmt.Printf("campaign %q already exists, skipping\n", spec.name)
			return
		}
	}

	c := models.Campaign{
		Name:       spec.name,
		LaunchDate: spec.launchDate,
		Groups:     []models.Group{{Name: spec.group.Name}},
		Template:   models.Template{Name: spec.template.Name},
		Page:       models.Page{Name: spec.page.Name},
		SMTP:       models.SMTP{Name: spec.smtp.Name},
	}
	if err := models.PostCampaign(&c, uid, teamID); err != nil {
		log.Fatalf("error creating campaign %q: %v", spec.name, err)
	}

	full, err := models.GetCampaign(c.Id, teamID)
	if err != nil {
		log.Fatalf("error fetching created campaign %q: %v", spec.name, err)
	}

	// A campaign launched in the future hasn't sent anything yet
	if spec.launchDate.After(time.Now()) {
		fmt.Printf("created campaign %q (scheduled, launch date in the future)\n", spec.name)
		return
	}

	if err := backdateCampaignCreated(rawDB, full.Id, spec.launchDate); err != nil {
		log.Fatalf("error backdating campaign %q created_date: %v", spec.name, err)
	}

	sent, opened, clicked, submitted, reported := 0, 0, 0, 0, 0
	for i := range full.Results {
		r := &full.Results[i]

		sentAt := spec.launchDate.Add(randDuration(rng, 0, 3*time.Hour))
		if err := r.HandleEmailSent(); err != nil {
			log.Fatalf("error marking result sent: %v", err)
		}
		if err := backdateEvent(rawDB, full.Id, r.Email, models.EventSent, sentAt); err != nil {
			log.Fatalf("error backdating sent event: %v", err)
		}
		sent++
		lastAt := sentAt

		didOpen := rng.Float64() < spec.openRate
		didClick := didOpen && rng.Float64() < spec.clickRate
		didSubmit := didClick && rng.Float64() < spec.submitRate
		didReport := !didClick && rng.Float64() < spec.reportRate

		if didOpen {
			openAt := sentAt.Add(randDuration(rng, 5*time.Minute, 48*time.Hour))
			if err := r.HandleEmailOpened(models.EventDetails{}); err != nil {
				log.Fatalf("error marking result opened: %v", err)
			}
			if err := backdateEvent(rawDB, full.Id, r.Email, models.EventOpened, openAt); err != nil {
				log.Fatalf("error backdating opened event: %v", err)
			}
			lastAt = openAt
			opened++
		}
		if didClick {
			clickAt := lastAt.Add(randDuration(rng, 1*time.Minute, 30*time.Minute))
			if err := r.HandleClickedLink(models.EventDetails{}); err != nil {
				log.Fatalf("error marking result clicked: %v", err)
			}
			if err := backdateEvent(rawDB, full.Id, r.Email, models.EventClicked, clickAt); err != nil {
				log.Fatalf("error backdating clicked event: %v", err)
			}
			lastAt = clickAt
			clicked++
		}
		if didSubmit {
			details := models.EventDetails{Payload: url.Values{
				"username": {r.Email},
				"password": {"SummerVacation2026!"},
			}}
			submitAt := lastAt.Add(randDuration(rng, 1*time.Minute, 10*time.Minute))
			if err := r.HandleFormSubmit(details); err != nil {
				log.Fatalf("error marking result submitted: %v", err)
			}
			if err := backdateEvent(rawDB, full.Id, r.Email, models.EventDataSubmit, submitAt); err != nil {
				log.Fatalf("error backdating submitted event: %v", err)
			}
			lastAt = submitAt
			submitted++
		}
		if didReport {
			reportAt := sentAt.Add(randDuration(rng, 10*time.Minute, 72*time.Hour))
			if err := r.HandleEmailReport(models.EventDetails{}); err != nil {
				log.Fatalf("error marking result reported: %v", err)
			}
			if err := backdateEvent(rawDB, full.Id, r.Email, models.EventReported, reportAt); err != nil {
				log.Fatalf("error backdating reported event: %v", err)
			}
			if reportAt.After(lastAt) {
				lastAt = reportAt
			}
			reported++
		}

		if err := backdateResult(rawDB, r.Id, sentAt, lastAt); err != nil {
			log.Fatalf("error backdating result %d: %v", r.Id, err)
		}
	}

	if spec.complete {
		if err := models.CompleteCampaign(full.Id, teamID); err != nil {
			log.Fatalf("error completing campaign %q: %v", spec.name, err)
		}
		completedAt := spec.launchDate.Add(randDuration(rng, 3*24*time.Hour, 8*24*time.Hour))
		if completedAt.After(time.Now()) {
			completedAt = time.Now()
		}
		if err := backdateCampaignCompleted(rawDB, full.Id, completedAt); err != nil {
			log.Fatalf("error backdating campaign %q completed_date: %v", spec.name, err)
		}
	}

	fmt.Printf("created campaign %q (%d sent, %d opened, %d clicked, %d submitted, %d reported)\n",
		spec.name, sent, opened, clicked, submitted, reported)
}

func openRawDB(conf *config.Config) (*sql.DB, error) {
	driver := "sqlite3"
	if conf.DBName == "mysql" {
		driver = "mysql"
	}
	return sql.Open(driver, conf.DBPath)
}

// randDuration returns a random duration in [min, max). Returns min if the
// range is empty or inverted.
func randDuration(rng *rand.Rand, min, max time.Duration) time.Duration {
	if max <= min {
		return min
	}
	return min + time.Duration(rng.Int63n(int64(max-min)))
}

// backdateEvent rewrites the timestamp of the most recently inserted event
// matching (campaign, email, message)
func backdateEvent(rawDB *sql.DB, campaignID int64, email, message string, t time.Time) error {
	_, err := rawDB.Exec(
		`UPDATE events SET time = ? WHERE id = (
			SELECT id FROM (
				SELECT id FROM events WHERE campaign_id = ? AND email = ? AND message = ? ORDER BY id DESC LIMIT 1
			) AS latest
		)`,
		t.UTC(), campaignID, email, message,
	)
	return err
}

// backdateResult rewrites a result's send_date/modified_date to match the
// backdated timestamps of its own events
func backdateResult(rawDB *sql.DB, id int64, sendDate, modifiedDate time.Time) error {
	_, err := rawDB.Exec(`UPDATE results SET send_date = ?, modified_date = ? WHERE id = ?`,
		sendDate.UTC(), modifiedDate.UTC(), id)
	return err
}

// backdateCampaignCreated rewrites a campaign's created_date (PostCampaign
// always sets it to time.Now()) along with its auto-generated "Campaign
// Created" event.
func backdateCampaignCreated(rawDB *sql.DB, campaignID int64, t time.Time) error {
	if _, err := rawDB.Exec(`UPDATE campaigns SET created_date = ? WHERE id = ?`, t.UTC(), campaignID); err != nil {
		return err
	}
	_, err := rawDB.Exec(
		`UPDATE events SET time = ? WHERE id = (
			SELECT id FROM (
				SELECT id FROM events WHERE campaign_id = ? AND message = 'Campaign Created' ORDER BY id DESC LIMIT 1
			) AS latest
		)`,
		t.UTC(), campaignID,
	)
	return err
}

// backdateCampaignCompleted rewrites a campaign's completed_date
// (CompleteCampaign always sets it to time.Now()).
func backdateCampaignCompleted(rawDB *sql.DB, campaignID int64, t time.Time) error {
	_, err := rawDB.Exec(`UPDATE campaigns SET completed_date = ? WHERE id = ?`, t.UTC(), campaignID)
	return err
}

func landingPageHTML(title, body string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head><meta charset="utf-8"><title>%s</title></head>
<body style="font-family: sans-serif; max-width: 400px; margin: 80px auto;">
	<h2>%s</h2>
	<p>%s</p>
	<form method="post">
		<p><label>Username<br><input type="text" name="username" style="width:100%%"></label></p>
		<p><label>Password<br><input type="password" name="password" style="width:100%%"></label></p>
		<button type="submit">Sign In</button>
	</form>
</body>
</html>`, title, title, body)
}

func emailHTML(greeting, body, buttonText string) string {
	return fmt.Sprintf(`<html>
<body style="font-family: sans-serif;">
	<p>%s</p>
	<p>%s</p>
	<p><a href="{{.URL}}" style="background:#2c3e50;color:#fff;padding:10px 16px;text-decoration:none;border-radius:4px;">%s</a></p>
	<p style="color:#888;font-size:12px;">{{.Tracker}}</p>
</body>
</html>`, greeting, body, buttonText)
}

// polishedLandingPageHTML is a genuinely styled corporate-SSO-style login
// page, for the one landing page worth showing off in a screenshot
func polishedLandingPageHTML() string {
	return `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>Example Corp - Sign In</title>
<style>
	* { box-sizing: border-box; }
	body {
		margin: 0;
		min-height: 100vh;
		display: flex;
		align-items: center;
		justify-content: center;
		background: linear-gradient(160deg, #eef2f7 0%, #e2e8f0 100%);
		font-family: -apple-system, "Segoe UI", Roboto, Helvetica, Arial, sans-serif;
	}
	.card {
		width: 100%;
		max-width: 400px;
		background: #ffffff;
		border-radius: 12px;
		box-shadow: 0 10px 40px rgba(15, 23, 42, 0.12);
		padding: 40px 36px 32px;
		margin: 24px;
	}
	.brand {
		display: flex;
		align-items: center;
		gap: 10px;
		margin-bottom: 28px;
	}
	.brand-mark {
		width: 32px;
		height: 32px;
		border-radius: 8px;
		background: linear-gradient(135deg, #2563eb, #1d4ed8);
		display: flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
	}
	.brand-name {
		font-size: 15px;
		font-weight: 600;
		color: #1e293b;
		letter-spacing: 0.01em;
	}
	h1 {
		font-size: 21px;
		font-weight: 600;
		color: #0f172a;
		margin: 0 0 6px;
	}
	.subtitle {
		font-size: 14px;
		color: #64748b;
		margin: 0 0 28px;
	}
	label {
		display: block;
		font-size: 13px;
		font-weight: 500;
		color: #334155;
		margin-bottom: 6px;
	}
	.field {
		margin-bottom: 18px;
	}
	input[type="text"], input[type="password"] {
		width: 100%;
		padding: 10px 12px;
		font-size: 14px;
		border: 1px solid #d1d5db;
		border-radius: 8px;
		color: #0f172a;
		background: #f9fafb;
	}
	input[type="text"]:focus, input[type="password"]:focus {
		outline: none;
		border-color: #2563eb;
		background: #fff;
	}
	button {
		width: 100%;
		padding: 12px;
		font-size: 14px;
		font-weight: 600;
		color: #fff;
		background: linear-gradient(135deg, #2563eb, #1d4ed8);
		border: none;
		border-radius: 8px;
		cursor: pointer;
		margin-top: 4px;
	}
	.footer {
		margin-top: 24px;
		text-align: center;
		font-size: 12px;
		color: #94a3b8;
	}
</style>
</head>
<body>
	<div class="card">
		<div class="brand">
			<div class="brand-mark">
				<svg width="16" height="16" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
					<path d="M12 2L3 6V11C3 16.5 6.8 21.6 12 23C17.2 21.6 21 16.5 21 11V6L12 2Z" fill="white" fill-opacity="0.9"/>
				</svg>
			</div>
			<div class="brand-name">Example Corp</div>
		</div>
		<h1>Reset your password</h1>
		<p class="subtitle">Please sign in with your corporate credentials to continue.</p>
		<form method="post">
			<div class="field">
				<label for="username">Username</label>
				<input type="text" id="username" name="username" autocomplete="username">
			</div>
			<div class="field">
				<label for="password">Password</label>
				<input type="password" id="password" name="password" autocomplete="current-password">
			</div>
			<button type="submit">Sign In</button>
		</form>
		<div class="footer">Protected by Example Corp Identity Services<br>&copy; 2026 Example Corp. All rights reserved.</div>
	</div>
</body>
</html>`
}

// polishedEmailHTML is a genuinely styled, table-based HTML email for the one template
// worth showing off in a screenshot.
func polishedEmailHTML() string {
	return `<html>
<body style="margin:0; padding:0; background-color:#f1f5f9;">
<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color:#f1f5f9;">
<tr><td align="center" style="padding:40px 16px;">

<table role="presentation" width="600" cellpadding="0" cellspacing="0" style="width:600px; max-width:100%; background-color:#ffffff; border-radius:8px; overflow:hidden; font-family:Helvetica, Arial, sans-serif;">

	<tr>
		<td style="background-color:#1e293b; padding:22px 32px;">
			<span style="color:#ffffff; font-size:16px; font-weight:bold; letter-spacing:0.04em;">EXAMPLE CORP</span>
			<span style="color:#94a3b8; font-size:13px; margin-left:10px;">IT Security</span>
		</td>
	</tr>

	<tr>
		<td style="padding:32px 32px 8px;">
			<p style="margin:0 0 16px; font-size:15px; color:#0f172a;">Hi {{.FirstName}},</p>
			<p style="margin:0 0 20px; font-size:14px; line-height:1.6; color:#334155;">
				Our records show the password for your account will expire soon. To avoid
				losing access to your email, calendar, and shared drives, please confirm
				your password using the button below.
			</p>
		</td>
	</tr>

	<tr>
		<td style="padding:0 32px 20px;">
			<table role="presentation" width="100%" cellpadding="0" cellspacing="0" style="background-color:#fff7ed; border-left:4px solid #f59e0b; border-radius:4px;">
				<tr>
					<td style="padding:14px 18px; font-size:13px; color:#92400e;">
						<strong>Account:</strong> {{.Email}}<br>
						<strong>Expires:</strong> within 24 hours
					</td>
				</tr>
			</table>
		</td>
	</tr>

	<tr>
		<td align="center" style="padding:8px 32px 28px;">
			<table role="presentation" cellpadding="0" cellspacing="0">
				<tr>
					<td style="background-color:#2563eb; border-radius:6px;">
						<a href="{{.URL}}" style="display:inline-block; padding:13px 34px; font-size:14px; font-weight:bold; color:#ffffff; text-decoration:none;">Confirm Password</a>
					</td>
				</tr>
			</table>
			<p style="margin:16px 0 0; font-size:12px; color:#94a3b8;">
				Or copy this link into your browser:<br>{{.URL}}
			</p>
		</td>
	</tr>

	<tr>
		<td style="background-color:#f8fafc; padding:20px 32px; border-top:1px solid #e2e8f0;">
			<p style="margin:0; font-size:12px; color:#94a3b8; line-height:1.6;">
				This is an automated message from Example Corp IT Security. If you did not
				expect this email, please contact the helpdesk.<br>
				Example Corp, 1 Market Street, Springfield
			</p>
		</td>
	</tr>

</table>
{{.Tracker}}
</td></tr>
</table>
</body>
</html>`
}

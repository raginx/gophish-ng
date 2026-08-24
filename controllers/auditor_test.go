package controllers

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/raginx/gophish-ng/auth"
	"github.com/raginx/gophish-ng/models"
)

// sessionCookieFor logs the given account in and returns its session cookie,
// following the same pattern as loginSessionCookie but for arbitrary users.
func sessionCookieFor(t *testing.T, ctx *testContext, username, password string) string {
	t.Helper()
	resp := attemptLogin(t, ctx, noRedirectClient(), username, password, "")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("login as %s did not redirect as expected: got status %d", username, resp.StatusCode)
	}
	cookie := gophishSessionCookie(resp)
	if cookie == "" {
		t.Fatalf("no session cookie set when logging in as %s", username)
	}
	return cookie
}

// fetchPage retrieves an admin page using the given session cookie.
func fetchPage(t *testing.T, ctx *testContext, cookie, path string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, ctx.adminServer.URL+path, nil)
	if err != nil {
		t.Fatalf("error building request for %s: %v", path, err)
	}
	req.Header.Set("Cookie", cookie)
	resp, err := noRedirectClient().Do(req)
	if err != nil {
		t.Fatalf("error requesting %s: %v", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("%s: expected 200 got %d (redirect target %q)", path, resp.StatusCode, resp.Header.Get("Location"))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("error reading %s body: %v", path, err)
	}
	return string(body)
}

// TestAuditorUIRendering confirms the rendered admin pages actually omit the
// controls a read-only account cannot use, and that a normal user still gets
// them.
func TestAuditorUIRendering(t *testing.T) {
	ctx := setupTest(t)
	defer tearDown(t, ctx)

	hash, err := auth.GeneratePasswordHash("gophish")
	if err != nil {
		t.Fatalf("error hashing password: %v", err)
	}
	auditorRole, err := models.GetRoleBySlug(models.RoleAuditor)
	if err != nil {
		t.Fatalf("auditor role missing: %v", err)
	}
	userRole, err := models.GetRoleBySlug(models.RoleUser)
	if err != nil {
		t.Fatalf("user role missing: %v", err)
	}
	admin, err := models.GetUser(1)
	if err != nil {
		t.Fatalf("error getting admin: %v", err)
	}
	for _, u := range []models.User{
		{Username: "ui-auditor", Hash: hash, ApiKey: "ui-auditor-key", RoleID: auditorRole.ID, TeamID: admin.TeamID},
		{Username: "ui-user", Hash: hash, ApiKey: "ui-user-key", RoleID: userRole.ID, TeamID: admin.TeamID},
	} {
		user := u
		if err := models.PutUser(&user); err != nil {
			t.Fatalf("error creating %s: %v", user.Username, err)
		}
	}

	auditorCookie := sessionCookieFor(t, ctx, "ui-auditor", "gophish")
	userCookie := sessionCookieFor(t, ctx, "ui-user", "gophish")

	// Controls that must disappear for a read-only account, by page. These
	// match the create button itself rather than the bare label, since the
	// (unreachable) edit modals keep the same wording in their titles.
	markers := []struct {
		path   string
		marker string
	}{
		{"/campaigns", "New Campaign</button>"},
		{"/templates", "New Template</button>"},
		{"/landing_pages", "New Page</button>"},
		{"/sending_profiles", "New Profile</button>"},
		{"/groups", "New Group</button>"},
		{"/settings", "Reporting Settings</a>"},
		{"/campaigns/1", "fa-trash-o fa-lg\"></i> Delete"},
	}

	for _, m := range markers {
		auditorBody := fetchPage(t, ctx, auditorCookie, m.path)
		if strings.Contains(auditorBody, m.marker) {
			t.Errorf("auditor still sees %q on %s", m.marker, m.path)
		}
		userBody := fetchPage(t, ctx, userCookie, m.path)
		if !strings.Contains(userBody, m.marker) {
			t.Errorf("regular user lost %q on %s", m.marker, m.path)
		}
	}

	// The permission flag itself must reach the client-side user object,
	// since the per-row action buttons are gated in JS off of it.
	dashboardAuditor := fetchPage(t, ctx, auditorCookie, "/")
	if !strings.Contains(dashboardAuditor, "modify_objects") {
		t.Error("modify_objects flag missing from the rendered page")
	}
	if !strings.Contains(strings.Join(strings.Fields(dashboardAuditor), " "), "modify_objects : false") {
		t.Error("auditor page did not render modify_objects as false")
	}
	dashboardUser := fetchPage(t, ctx, userCookie, "/")
	if !strings.Contains(strings.Join(strings.Fields(dashboardUser), " "), "modify_objects : true") {
		t.Error("regular user page did not render modify_objects as true")
	}
}

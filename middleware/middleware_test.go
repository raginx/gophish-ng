package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/raginx/gophish-ng/config"
	ctx "github.com/raginx/gophish-ng/context"
	"github.com/raginx/gophish-ng/models"
)

var successHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte("success"))
})

type testContext struct {
	apiKey string
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
	// Get the API key to use for these tests
	u, err := models.GetUser(1)
	if err != nil {
		t.Fatalf("error getting user: %v", err)
	}
	ctx := &testContext{}
	ctx.apiKey = u.ApiKey
	return ctx
}

// MiddlewarePermissionTest maps an expected HTTP Method to an expected HTTP
// status code
type MiddlewarePermissionTest map[string]int

// TestEnforceViewOnly ensures that only users with the ModifyObjects
// permission have the ability to send non-GET requests.
func TestEnforceViewOnly(t *testing.T) {
	setupTest(t)
	permissionTests := map[string]MiddlewarePermissionTest{
		models.RoleAdmin: MiddlewarePermissionTest{
			http.MethodGet:     http.StatusOK,
			http.MethodHead:    http.StatusOK,
			http.MethodOptions: http.StatusOK,
			http.MethodPost:    http.StatusOK,
			http.MethodPut:     http.StatusOK,
			http.MethodDelete:  http.StatusOK,
		},
		models.RoleUser: MiddlewarePermissionTest{
			http.MethodGet:     http.StatusOK,
			http.MethodHead:    http.StatusOK,
			http.MethodOptions: http.StatusOK,
			http.MethodPost:    http.StatusOK,
			http.MethodPut:     http.StatusOK,
			http.MethodDelete:  http.StatusOK,
		},
		models.RoleAuditor: MiddlewarePermissionTest{
			http.MethodGet:     http.StatusOK,
			http.MethodHead:    http.StatusOK,
			http.MethodOptions: http.StatusOK,
			http.MethodPost:    http.StatusForbidden,
			http.MethodPut:     http.StatusForbidden,
			http.MethodDelete:  http.StatusForbidden,
		},
	}
	for r, checks := range permissionTests {
		role, err := models.GetRoleBySlug(r)
		if err != nil {
			t.Fatalf("error getting role by slug: %v", err)
		}

		for method, expected := range checks {
			req := httptest.NewRequest(method, "/", nil)
			response := httptest.NewRecorder()

			req = ctx.Set(req, "user", models.User{
				Role:   role,
				RoleID: role.ID,
			})

			EnforceViewOnly(successHandler).ServeHTTP(response, req)
			got := response.Code
			if got != expected {
				t.Fatalf("incorrect status code received. expected %d got %d", expected, got)
			}
		}
	}
}

// TestEnforceViewOnlySelfService ensures that a read-only account can still
// POST to the endpoints that only act on its own account, so that an auditor
// is able to rotate their own API key.
func TestEnforceViewOnlySelfService(t *testing.T) {
	setupTest(t)
	role, err := models.GetRoleBySlug(models.RoleAuditor)
	if err != nil {
		t.Fatalf("error getting role by slug: %v", err)
	}
	for path, expected := range map[string]int{
		"/api/reset":    http.StatusOK,
		"/api/imap/":    http.StatusForbidden,
		"/api/reset/":   http.StatusForbidden,
		"/api/campaign": http.StatusForbidden,
	} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		response := httptest.NewRecorder()
		req = ctx.Set(req, "user", models.User{
			Role:   role,
			RoleID: role.ID,
		})

		EnforceViewOnly(successHandler).ServeHTTP(response, req)
		if got := response.Code; got != expected {
			t.Fatalf("incorrect status code received for %s. expected %d got %d", path, expected, got)
		}
	}
}

func TestRequirePermission(t *testing.T) {
	setupTest(t)
	middleware := RequirePermission(models.PermissionModifySystem)
	handler := middleware(successHandler)

	permissionTests := map[string]int{
		models.RoleUser:  http.StatusForbidden,
		models.RoleAdmin: http.StatusOK,
	}

	for role, expected := range permissionTests {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		response := httptest.NewRecorder()
		// Test that with the requested permission, the request succeeds
		role, err := models.GetRoleBySlug(role)
		if err != nil {
			t.Fatalf("error getting role by slug: %v", err)
		}
		req = ctx.Set(req, "user", models.User{
			Role:   role,
			RoleID: role.ID,
		})
		handler.ServeHTTP(response, req)
		got := response.Code
		if got != expected {
			t.Fatalf("incorrect status code received. expected %d got %d", expected, got)
		}
	}
}

func TestRequireAPIKey(t *testing.T) {
	setupTest(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	// Test that making a request without an API key is denied
	RequireAPIKey(successHandler).ServeHTTP(response, req)
	expected := http.StatusUnauthorized
	got := response.Code
	if got != expected {
		t.Fatalf("incorrect status code received. expected %d got %d", expected, got)
	}
}

// TestRequireAPIKeyLockedAccount guards against a regression where a locked
// account's API key kept working - RequireLogin blocks a locked-out web
// session (indirectly, via Login refusing to start one), but RequireAPIKey
// didn't check AccountLocked at all, so a locked user's API key remained a
// fully working bearer credential regardless of the lock. See upstream
// issue #9440.
func TestRequireAPIKeyLockedAccount(t *testing.T) {
	testCtx := setupTest(t)
	u, err := models.GetUserByAPIKey(testCtx.apiKey)
	if err != nil {
		t.Fatalf("error getting user: %v", err)
	}
	u.AccountLocked = true
	if err := models.PutUser(&u); err != nil {
		t.Fatalf("error locking account: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	query := req.URL.Query()
	query.Set("api_key", testCtx.apiKey)
	req.URL.RawQuery = query.Encode()
	response := httptest.NewRecorder()
	RequireAPIKey(successHandler).ServeHTTP(response, req)
	expected := http.StatusUnauthorized
	got := response.Code
	if got != expected {
		t.Fatalf("locked account's API key should be rejected. expected status %d got %d", expected, got)
	}
}

func TestCORSHeaders(t *testing.T) {
	setupTest(t)
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	response := httptest.NewRecorder()
	RequireAPIKey(successHandler).ServeHTTP(response, req)
	expected := "POST, GET, OPTIONS, PUT, DELETE"
	got := response.Result().Header.Get("Access-Control-Allow-Methods")
	if got != expected {
		t.Fatalf("incorrect cors options received. expected %s got %s", expected, got)
	}
}

func TestInvalidAPIKey(t *testing.T) {
	setupTest(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	query := req.URL.Query()
	query.Set("api_key", "bogus-api-key")
	req.URL.RawQuery = query.Encode()
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	RequireAPIKey(successHandler).ServeHTTP(response, req)
	expected := http.StatusUnauthorized
	got := response.Code
	if got != expected {
		t.Fatalf("incorrect status code received. expected %d got %d", expected, got)
	}
}

func TestBearerToken(t *testing.T) {
	testCtx := setupTest(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", testCtx.apiKey))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	RequireAPIKey(successHandler).ServeHTTP(response, req)
	expected := http.StatusOK
	got := response.Code
	if got != expected {
		t.Fatalf("incorrect status code received. expected %d got %d", expected, got)
	}
}

func TestPasswordResetRequired(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = ctx.Set(req, "user", models.User{
		PasswordChangeRequired: true,
	})
	response := httptest.NewRecorder()
	RequireLogin(successHandler).ServeHTTP(response, req)
	gotStatus := response.Code
	expectedStatus := http.StatusTemporaryRedirect
	if gotStatus != expectedStatus {
		t.Fatalf("incorrect status code received. expected %d got %d", expectedStatus, gotStatus)
	}
	expectedLocation := "/reset_password?next=%2F"
	gotLocation := response.Header().Get("Location")
	if gotLocation != expectedLocation {
		t.Fatalf("incorrect location header received. expected %s got %s", expectedLocation, gotLocation)
	}
}

func TestApplySecurityHeaders(t *testing.T) {
	expected := map[string]string{
		"Content-Security-Policy": "frame-ancestors 'none';",
		"X-Frame-Options":         "DENY",
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	ApplySecurityHeaders(successHandler).ServeHTTP(response, req)
	for header, value := range expected {
		got := response.Header().Get(header)
		if got != value {
			t.Fatalf("incorrect security header received for %s: expected %s got %s", header, value, got)
		}
	}
}

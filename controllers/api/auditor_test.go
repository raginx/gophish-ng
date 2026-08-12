package api

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gophish/gophish/models"
)

// TestAuditorEndToEnd drives the real API server with a real auditor account
// to confirm the role behaves as intended end-to-end.
func TestAuditorEndToEnd(t *testing.T) {
	ctx := setupTest(t)
	role, err := models.GetRoleBySlug(models.RoleAuditor)
	if err != nil {
		t.Fatalf("auditor role missing from migrations: %v", err)
	}
	auditor := models.User{
		Username: "auditor-e2e",
		Hash:     "hash",
		ApiKey:   "auditor-e2e-key",
		RoleID:   role.ID,
		TeamID:   ctx.admin.TeamID,
	}
	if err := models.PutUser(&auditor); err != nil {
		t.Fatalf("error creating auditor: %v", err)
	}

	cases := []struct {
		method   string
		path     string
		body     string
		expected int
		why      string
	}{
		{http.MethodGet, "/api/campaigns/", "", http.StatusOK, "auditor must be able to read campaigns"},
		{http.MethodGet, "/api/groups/", "", http.StatusOK, "auditor must be able to read groups"},
		{http.MethodGet, "/api/templates/", "", http.StatusOK, "auditor must be able to read templates"},
		{http.MethodPost, "/api/campaigns/", `{"name":"nope"}`, http.StatusForbidden, "auditor must not create campaigns"},
		{http.MethodPost, "/api/groups/", `{"name":"nope"}`, http.StatusForbidden, "auditor must not create groups"},
		{http.MethodPost, "/api/templates/", `{"name":"nope"}`, http.StatusForbidden, "auditor must not create templates"},
		{http.MethodDelete, "/api/campaigns/1", "", http.StatusForbidden, "auditor must not delete campaigns"},
		{http.MethodPost, "/api/users/", `{"username":"x","role":"admin"}`, http.StatusForbidden, "auditor must not create users"},
		{http.MethodPost, "/api/reset", "", http.StatusOK, "auditor must be able to rotate its own API key"},
	}

	for _, tc := range cases {
		r := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", auditor.ApiKey))
		w := httptest.NewRecorder()
		ctx.apiServer.ServeHTTP(w, r)
		if w.Code != tc.expected {
			t.Errorf("%s %s: expected %d got %d (%s)", tc.method, tc.path, tc.expected, w.Code, tc.why)
		}
	}

	// The key rotation above must have actually taken effect.
	reloaded, err := models.GetUser(auditor.Id)
	if err != nil {
		t.Fatalf("error reloading auditor: %v", err)
	}
	if reloaded.ApiKey == "auditor-e2e-key" {
		t.Error("POST /api/reset returned 200 but did not rotate the API key")
	}

	// A regular user must still be able to write, i.e. the middleware change
	// didn't accidentally lock everyone out.
	userRole, err := models.GetRoleBySlug(models.RoleUser)
	if err != nil {
		t.Fatalf("error getting user role: %v", err)
	}
	regular := models.User{
		Username: "regular-e2e",
		Hash:     "hash",
		ApiKey:   "regular-e2e-key",
		RoleID:   userRole.ID,
		TeamID:   ctx.admin.TeamID,
	}
	if err := models.PutUser(&regular); err != nil {
		t.Fatalf("error creating user: %v", err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/groups/", bytes.NewBufferString(
		`{"name":"e2e group","targets":[{"email":"a@example.com"}]}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", regular.ApiKey))
	w := httptest.NewRecorder()
	ctx.apiServer.ServeHTTP(w, r)
	if w.Code != http.StatusCreated && w.Code != http.StatusOK {
		t.Errorf("regular user POST /api/groups/: expected success, got %d: %s", w.Code, w.Body.String())
	}
}

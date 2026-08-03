package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gophish/gophish/models"
	"golang.org/x/oauth2"
)

var fakeOAuthToken = oauth2.Token{
	AccessToken:  "fake-access-token",
	RefreshToken: "fake-refresh-token",
	Expiry:       time.Now().Add(time.Hour),
}

func postIMAP(t *testing.T, testCtx *testContext, payload map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("error marshaling payload: %v", err)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/imap/", bytes.NewBuffer(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testCtx.apiKey))
	w := httptest.NewRecorder()
	testCtx.apiServer.ServeHTTP(w, r)
	return w
}

func getIMAP(t *testing.T, testCtx *testContext) []map[string]interface{} {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/api/imap/", nil)
	r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testCtx.apiKey))
	w := httptest.NewRecorder()
	testCtx.apiServer.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status code from GET /api/imap/: %d: %s", w.Code, w.Body.String())
	}
	var got []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("error unmarshaling response: %v", err)
	}
	return got
}

func TestIMAPGetNeverReturnsSecrets(t *testing.T) {
	testCtx := setupTest(t)

	w := postIMAP(t, testCtx, map[string]interface{}{
		"host":     "127.0.0.1",
		"port":     "993",
		"username": "user@example.com",
		"password": "super-secret-password",
		"tls":      true,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("unexpected status code saving IMAP config: %d: %s", w.Code, w.Body.String())
	}

	got := getIMAP(t, testCtx)
	if len(got) != 1 {
		t.Fatalf("expected exactly one IMAP config, got %d", len(got))
	}
	if pw, ok := got[0]["password"]; !ok || pw != "" {
		t.Fatalf("expected password to be empty in the API response, got %v", pw)
	}
	if _, ok := got[0]["oauth_client_secret"]; ok && got[0]["oauth_client_secret"] != "" {
		t.Fatalf("expected oauth_client_secret to be empty in the API response, got %v", got[0]["oauth_client_secret"])
	}
	if connected, ok := got[0]["oauth_connected"]; !ok || connected != false {
		t.Fatalf("expected oauth_connected to be false for a Basic Auth config, got %v", connected)
	}

	// The actual password must still be intact in the database, even
	// though the API never shows it again.
	stored, err := models.GetIMAP(1)
	if err != nil {
		t.Fatalf("error getting IMAP config from the database: %v", err)
	}
	if stored[0].Password != "super-secret-password" {
		t.Fatalf("expected the stored password to be preserved, got %q", stored[0].Password)
	}
}

func TestIMAPPostBlankPasswordPreservesExisting(t *testing.T) {
	testCtx := setupTest(t)

	w := postIMAP(t, testCtx, map[string]interface{}{
		"host":     "127.0.0.1",
		"port":     "993",
		"username": "user@example.com",
		"password": "original-password",
		"tls":      true,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("unexpected status code saving IMAP config: %d: %s", w.Code, w.Body.String())
	}

	// Simulate the settings page re-saving without the user having typed
	// new password (the browser never gets the old one back to resubmit)
	w = postIMAP(t, testCtx, map[string]interface{}{
		"host":     "127.0.0.1",
		"port":     "993",
		"username": "user@example.com",
		"password": "",
		"tls":      true,
		"enabled":  true,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("unexpected status code re-saving IMAP config: %d: %s", w.Code, w.Body.String())
	}

	stored, err := models.GetIMAP(1)
	if err != nil {
		t.Fatalf("error getting IMAP config from the database: %v", err)
	}
	if stored[0].Password != "original-password" {
		t.Fatalf("expected the original password to be preserved, got %q", stored[0].Password)
	}
	if !stored[0].Enabled {
		t.Fatalf("expected the unrelated field change (enabled) to have been applied")
	}
}

func TestIMAPPostPreservesOAuthTokenAcrossUnrelatedUpdate(t *testing.T) {
	testCtx := setupTest(t)
	testCtx.config.SecretKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="

	w := postIMAP(t, testCtx, map[string]interface{}{
		"host":                "127.0.0.1",
		"port":                "993",
		"username":            "user@example.com",
		"auth_type":           models.IMAPAuthTypeOAuth2,
		"oauth_provider":      models.OAuthProviderCustom,
		"oauth_client_id":     "client-id",
		"oauth_client_secret": "client-secret",
		"oauth_auth_url":      "https://idp.example.com/authorize",
		"oauth_token_url":     "https://idp.example.com/token",
		"oauth_scopes":        "scope",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("unexpected status code saving OAuth2 IMAP config: %d: %s", w.Code, w.Body.String())
	}

	im, err := models.GetIMAP(1)
	if err != nil || len(im) != 1 {
		t.Fatalf("error getting IMAP config: %v", err)
	}
	imCopy := im[0]
	if err := models.StoreOAuthToken(&imCopy, &fakeOAuthToken); err != nil {
		t.Fatalf("error storing a fake OAuth token: %v", err)
	}

	got := getIMAP(t, testCtx)
	if connected, ok := got[0]["oauth_connected"]; !ok || connected != true {
		t.Fatalf("expected oauth_connected to be true after StoreOAuthToken, got %v", connected)
	}

	// Re-save via the API without touching the client secret or tokens
	// e.g. the user just changed the polling frequency
	w = postIMAP(t, testCtx, map[string]interface{}{
		"host":                "127.0.0.1",
		"port":                "993",
		"username":            "user@example.com",
		"auth_type":           models.IMAPAuthTypeOAuth2,
		"oauth_provider":      models.OAuthProviderCustom,
		"oauth_client_id":     "client-id",
		"oauth_client_secret": "",
		"oauth_auth_url":      "https://idp.example.com/authorize",
		"oauth_token_url":     "https://idp.example.com/token",
		"oauth_scopes":        "scope",
		"imap_freq":           "120",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("unexpected status code re-saving OAuth2 IMAP config: %d: %s", w.Code, w.Body.String())
	}

	stored, err := models.GetIMAP(1)
	if err != nil {
		t.Fatalf("error getting IMAP config from the database: %v", err)
	}
	if stored[0].OAuthClientSecret != "client-secret" {
		t.Fatalf("expected the client secret to be preserved, got %q", stored[0].OAuthClientSecret)
	}
	if stored[0].OAuthRefreshTokenEnc == "" {
		t.Fatalf("expected the OAuth refresh token to survive an unrelated settings update")
	}
	if stored[0].IMAPFreq != 120 {
		t.Fatalf("expected the unrelated field change (imap_freq) to have been applied, got %d", stored[0].IMAPFreq)
	}
}

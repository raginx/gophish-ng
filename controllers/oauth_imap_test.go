package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gophish/gophish/models"
)

// loginSessionCookie logs in as admin and returns just the session Cookie
// header value
func loginSessionCookie(t *testing.T, ctx *testContext) string {
	t.Helper()
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}}
	resp := attemptLogin(t, ctx, client, "admin", "gophish", "")
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("login did not redirect as expected: got status %d", resp.StatusCode)
	}
	cookie := gophishSessionCookie(resp)
	if cookie == "" {
		t.Fatalf("expected a session cookie to be set on successful login")
	}
	return cookie
}

// clearPasswordChangeRequired removes the "must change password" flag the
// test admin starts with, since it would otherwise redirect every request
// (including OAuth routes) to /reset_password first
func clearPasswordChangeRequired(t *testing.T) {
	t.Helper()
	u, err := models.GetUser(1)
	if err != nil {
		t.Fatalf("error getting admin user: %v", err)
	}
	u.PasswordChangeRequired = false
	if err := models.PutUser(&u); err != nil {
		t.Fatalf("error updating admin user: %v", err)
	}
}

func setupOAuthIMAPConfig(t *testing.T, testCtx *testContext, tokenURL string) {
	t.Helper()
	// OAuth2 IMAP configs now require a configured secret_key to save at
	// all (models.IMAP.Validate rejects them otherwise, since tokens are
	// encrypted at rest with it )
	testCtx.config.SecretKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	im := &models.IMAP{
		UserId:            1,
		Host:              "127.0.0.1",
		Port:              993,
		Username:          "user@example.com",
		AuthType:          models.IMAPAuthTypeOAuth2,
		OAuthProvider:     models.OAuthProviderCustom,
		OAuthClientID:     "client-id",
		OAuthClientSecret: "client-secret",
		OAuthAuthURL:      "https://idp.example.com/authorize",
		OAuthTokenURL:     tokenURL,
		OAuthScopes:       "scope",
	}
	if err := models.PostIMAP(im, 1); err != nil {
		t.Fatalf("error saving OAuth2 IMAP config: %v", err)
	}
}

// gophishSessionCookie picks the "gophish" session cookie out of a
// response's Set-Cookie headers
func gophishSessionCookie(resp *http.Response) string {
	for _, sc := range resp.Header.Values("Set-Cookie") {
		if strings.HasPrefix(sc, "gophish=") {
			return sc
		}
	}
	return ""
}

func noRedirectClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func TestOAuthIMAPAuthorizeRedirectsWithState(t *testing.T) {
	testCtx := setupTest(t)
	defer tearDown(t, testCtx)
	clearPasswordChangeRequired(t)
	setupOAuthIMAPConfig(t, testCtx, "https://idp.example.com/token")
	cookie := loginSessionCookie(t, testCtx)

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/oauth/imap/authorize", testCtx.adminServer.URL), nil)
	if err != nil {
		t.Fatalf("error creating request: %v", err)
	}
	req.Header.Set("Cookie", cookie)
	resp, err := noRedirectClient().Do(req)
	if err != nil {
		t.Fatalf("error requesting /oauth/imap/authorize: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected a redirect, got status %d", resp.StatusCode)
	}
	loc, err := resp.Location()
	if err != nil {
		t.Fatalf("error parsing Location header: %v", err)
	}
	if loc.Query().Get("state") == "" {
		t.Fatalf("expected a state parameter in the authorize redirect, got %s", loc.String())
	}
	if loc.Query().Get("client_id") != "client-id" {
		t.Fatalf("expected client_id=client-id in the authorize redirect, got %s", loc.String())
	}
}

func TestOAuthIMAPCallbackRejectsMismatchedState(t *testing.T) {
	testCtx := setupTest(t)
	defer tearDown(t, testCtx)
	clearPasswordChangeRequired(t)
	setupOAuthIMAPConfig(t, testCtx, "https://idp.example.com/token")
	cookie := loginSessionCookie(t, testCtx)
	client := noRedirectClient()

	// Start the flow so a real state ends up in the session...
	authReq, _ := http.NewRequest("GET", fmt.Sprintf("%s/oauth/imap/authorize", testCtx.adminServer.URL), nil)
	authReq.Header.Set("Cookie", cookie)
	authResp, err := client.Do(authReq)
	if err != nil {
		t.Fatalf("error requesting /oauth/imap/authorize: %v", err)
	}
	if sc := gophishSessionCookie(authResp); sc != "" {
		cookie = sc
	}

	// ...then hit the callback with a state that doesn't match.
	callbackURL := fmt.Sprintf("%s/oauth/imap/callback?state=%s&code=irrelevant", testCtx.adminServer.URL, url.QueryEscape("wrong-state"))
	cbReq, _ := http.NewRequest("GET", callbackURL, nil)
	cbReq.Header.Set("Cookie", cookie)
	cbResp, err := client.Do(cbReq)
	if err != nil {
		t.Fatalf("error requesting /oauth/imap/callback: %v", err)
	}
	if cbResp.StatusCode != http.StatusFound {
		t.Fatalf("expected a redirect back to /settings, got status %d", cbResp.StatusCode)
	}
	loc, _ := cbResp.Location()
	if loc.Path != "/settings" {
		t.Fatalf("expected redirect to /settings, got %s", loc.Path)
	}

	got, err := models.GetIMAP(1)
	if err != nil {
		t.Fatalf("error getting IMAP config: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly one IMAP config, got %d", len(got))
	}
	if got[0].OAuthRefreshTokenEnc != "" {
		t.Fatalf("no token should have been stored for a mismatched state")
	}
}

func TestOAuthIMAPCallbackPersistsToken(t *testing.T) {
	testCtx := setupTest(t)
	defer tearDown(t, testCtx)
	clearPasswordChangeRequired(t)

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "issued-access-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"refresh_token": "issued-refresh-token",
		})
	}))
	defer tokenServer.Close()

	setupOAuthIMAPConfig(t, testCtx, tokenServer.URL)
	cookie := loginSessionCookie(t, testCtx)
	client := noRedirectClient()

	authReq, _ := http.NewRequest("GET", fmt.Sprintf("%s/oauth/imap/authorize", testCtx.adminServer.URL), nil)
	authReq.Header.Set("Cookie", cookie)
	authResp, err := client.Do(authReq)
	if err != nil {
		t.Fatalf("error requesting /oauth/imap/authorize: %v", err)
	}
	if sc := gophishSessionCookie(authResp); sc != "" {
		cookie = sc
	}
	loc, err := authResp.Location()
	if err != nil {
		t.Fatalf("error parsing Location header: %v", err)
	}
	state := loc.Query().Get("state")
	if state == "" {
		t.Fatalf("expected a state parameter, got %s", loc.String())
	}

	callbackURL := fmt.Sprintf("%s/oauth/imap/callback?state=%s&code=fake-code", testCtx.adminServer.URL, url.QueryEscape(state))
	cbReq, _ := http.NewRequest("GET", callbackURL, nil)
	cbReq.Header.Set("Cookie", cookie)
	cbResp, err := client.Do(cbReq)
	if err != nil {
		t.Fatalf("error requesting /oauth/imap/callback: %v", err)
	}
	if cbResp.StatusCode != http.StatusFound {
		t.Fatalf("expected a redirect back to /settings, got status %d", cbResp.StatusCode)
	}

	got, err := models.GetIMAP(1)
	if err != nil {
		t.Fatalf("error getting IMAP config: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected exactly one IMAP config, got %d", len(got))
	}
	if got[0].OAuthRefreshTokenEnc == "" {
		t.Fatalf("expected an encrypted refresh token to have been persisted")
	}
}

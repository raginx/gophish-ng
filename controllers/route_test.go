package controllers

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func attemptLogin(t *testing.T, ctx *testContext, client *http.Client, username, password, optionalPath string) *http.Response {
	resp, err := http.Get(fmt.Sprintf("%s/login", ctx.adminServer.URL))
	if err != nil {
		t.Fatalf("error requesting the /login endpoint: %v", err)
	}
	got := resp.StatusCode
	expected := http.StatusOK
	if got != expected {
		t.Fatalf("invalid status code received. expected %d got %d", expected, got)
	}

	doc, err := goquery.NewDocumentFromResponse(resp)
	if err != nil {
		t.Fatalf("error parsing /login response body")
	}
	elem := doc.Find("input[name='csrf_token']").First()
	token, ok := elem.Attr("value")
	if !ok {
		t.Fatal("unable to find csrf_token value in login response")
	}
	if client == nil {
		client = &http.Client{}
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("%s/login%s", ctx.adminServer.URL, optionalPath), strings.NewReader(url.Values{
		"username":   {username},
		"password":   {password},
		"csrf_token": {token},
	}.Encode()))
	if err != nil {
		t.Fatalf("error creating new /login request: %v", err)
	}

	req.Header.Set("Cookie", resp.Header.Get("Set-Cookie"))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	// A real browser submitting this same-origin form sends a matching
	// Referer; gorilla/csrf now enforces that (fixed Referer validation,
	// GO-2025-3607), so the test client must mimic it too.
	req.Header.Set("Referer", fmt.Sprintf("%s/login", ctx.adminServer.URL))

	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("error requesting the /login endpoint: %v", err)
	}
	return resp
}

func TestLoginCSRF(t *testing.T) {
	ctx := setupTest(t)
	defer tearDown(t, ctx)
	resp, err := http.PostForm(fmt.Sprintf("%s/login", ctx.adminServer.URL),
		url.Values{
			"username": {"admin"},
			"password": {"gophish"},
		})

	if err != nil {
		t.Fatalf("error requesting the /login endpoint: %v", err)
	}

	got := resp.StatusCode
	expected := http.StatusForbidden
	if got != expected {
		t.Fatalf("invalid status code received. expected %d got %d", expected, got)
	}
}

func TestInvalidCredentials(t *testing.T) {
	ctx := setupTest(t)
	defer tearDown(t, ctx)
	resp := attemptLogin(t, ctx, nil, "admin", "bogus", "")
	got := resp.StatusCode
	expected := http.StatusUnauthorized
	if got != expected {
		t.Fatalf("invalid status code received. expected %d got %d", expected, got)
	}
}

func TestSuccessfulLogin(t *testing.T) {
	ctx := setupTest(t)
	defer tearDown(t, ctx)
	resp := attemptLogin(t, ctx, nil, "admin", "gophish", "")
	got := resp.StatusCode
	expected := http.StatusOK
	if got != expected {
		t.Fatalf("invalid status code received. expected %d got %d", expected, got)
	}
}

func TestSuccessfulRedirect(t *testing.T) {
	ctx := setupTest(t)
	defer tearDown(t, ctx)
	next := "/campaigns"
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}}
	resp := attemptLogin(t, ctx, client, "admin", "gophish", fmt.Sprintf("?next=%s", next))
	got := resp.StatusCode
	expected := http.StatusFound
	if got != expected {
		t.Fatalf("invalid status code received. expected %d got %d", expected, got)
	}
	url, err := resp.Location()
	if err != nil {
		t.Fatalf("error parsing response Location header: %v", err)
	}
	if url.Path != next {
		t.Fatalf("unexpected Location header received. expected %s got %s", next, url.Path)
	}
}

// TestAdminPageNoStore verifies that authenticated admin pages tell the
// browser not to cache/bfcache them - otherwise a stale page can be shown
// after switching accounts in the same browser (ref gophish/gophish#2022).
func TestAdminPageNoStore(t *testing.T) {
	ctx := setupTest(t)
	defer tearDown(t, ctx)
	// Stop at the first redirect so we get the POST /login response itself
	// (and its Set-Cookie for the newly-authenticated session), rather than
	// following on to further hops that only ever see an unauthenticated
	// request - this http.Client has no CookieJar, so it can't carry a
	// Secure session cookie across a plain-HTTP redirect chain, same as a
	// real browser wouldn't either.
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}}
	loginResp := attemptLogin(t, ctx, client, "admin", "gophish", "")
	if loginResp.StatusCode != http.StatusFound {
		t.Fatalf("invalid status code received. expected %d got %d", http.StatusFound, loginResp.StatusCode)
	}
	sessionCookie := loginResp.Header.Get("Set-Cookie")
	if sessionCookie == "" {
		t.Fatalf("expected a session cookie to be set on successful login")
	}

	// Forward the session cookie manually (as attemptLogin itself does for
	// the CSRF cookie), since it's Secure and this test server is plain
	// HTTP - a CookieJar would (correctly) refuse to send it back.
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/campaigns", ctx.adminServer.URL), nil)
	if err != nil {
		t.Fatalf("error creating /campaigns request: %v", err)
	}
	req.Header.Set("Cookie", sessionCookie)
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatalf("error requesting /campaigns: %v", err)
	}
	// The freshly-created test admin has PasswordChangeRequired set, so the
	// authenticated request above actually lands on /reset_password rather
	// than /campaigns itself - that's still a getTemplate-rendered admin
	// page, which is what this test cares about.
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected authenticated request to succeed, got status %d", resp.StatusCode)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("unexpected Cache-Control header on admin page: got %q, want %q", cc, "no-store")
	}
}

func TestAccountLocked(t *testing.T) {
	ctx := setupTest(t)
	defer tearDown(t, ctx)
	resp := attemptLogin(t, ctx, nil, "houdini", "gophish", "")
	got := resp.StatusCode
	expected := http.StatusUnauthorized
	if got != expected {
		t.Fatalf("invalid status code received. expected %d got %d", expected, got)
	}
}

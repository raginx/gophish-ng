package models

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"

	gophishcrypto "github.com/gophish/gophish/crypto"
	"golang.org/x/oauth2"
	check "gopkg.in/check.v1"
)

func (s *ModelsSuite) TestOAuthEndpointProviders(c *check.C) {
	microsoftEP, err := oauthEndpoint(&IMAP{OAuthProvider: OAuthProviderMicrosoft, OAuthTenantID: "my-tenant"})
	c.Assert(err, check.Equals, nil)
	c.Assert(microsoftEP.AuthURL, check.Equals, "https://login.microsoftonline.com/my-tenant/oauth2/v2.0/authorize")

	googleEP, err := oauthEndpoint(&IMAP{OAuthProvider: OAuthProviderGoogle})
	c.Assert(err, check.Equals, nil)
	c.Assert(googleEP.AuthURL, check.Equals, "https://accounts.google.com/o/oauth2/auth")

	customEP, err := oauthEndpoint(&IMAP{
		OAuthProvider: OAuthProviderCustom,
		OAuthAuthURL:  "https://idp.example.com/authorize",
		OAuthTokenURL: "https://idp.example.com/token",
	})
	c.Assert(err, check.Equals, nil)
	c.Assert(customEP.AuthURL, check.Equals, "https://idp.example.com/authorize")
	c.Assert(customEP.TokenURL, check.Equals, "https://idp.example.com/token")

	_, err = oauthEndpoint(&IMAP{OAuthProvider: "not-a-real-provider"})
	c.Assert(err, check.Equals, ErrInvalidOAuthProvider)
}

func (s *ModelsSuite) TestOAuthScopesDefaultAndOverride(c *check.C) {
	c.Assert(oauthScopes(&IMAP{OAuthProvider: OAuthProviderGoogle}), check.DeepEquals, []string{"https://mail.google.com/"})
	c.Assert(oauthScopes(&IMAP{OAuthProvider: OAuthProviderMicrosoft}), check.DeepEquals,
		[]string{"https://outlook.office.com/IMAP.AccessAsUser.All", "offline_access"})

	// An explicit OAuthScopes value overrides the provider default.
	overridden := oauthScopes(&IMAP{OAuthProvider: OAuthProviderGoogle, OAuthScopes: "scope-a scope-b"})
	c.Assert(overridden, check.DeepEquals, []string{"scope-a", "scope-b"})

	// "custom" has no built-in default.
	c.Assert(oauthScopes(&IMAP{OAuthProvider: OAuthProviderCustom}), check.HasLen, 0)
}

func (s *ModelsSuite) TestOAuthConfig(c *check.C) {
	im := &IMAP{
		OAuthProvider:     OAuthProviderGoogle,
		OAuthClientID:     "client-id",
		OAuthClientSecret: "client-secret",
	}
	oc, err := OAuthConfig(im, "https://admin.example.com/oauth/imap/callback")
	c.Assert(err, check.Equals, nil)
	c.Assert(oc.ClientID, check.Equals, "client-id")
	c.Assert(oc.ClientSecret, check.Equals, "client-secret")
	c.Assert(oc.RedirectURL, check.Equals, "https://admin.example.com/oauth/imap/callback")
	c.Assert(oc.Scopes, check.DeepEquals, []string{"https://mail.google.com/"})
}

func (s *ModelsSuite) TestGetValidAccessTokenNotConnected(c *check.C) {
	origKey := conf.SecretKey
	conf.SecretKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	defer func() { conf.SecretKey = origKey }()

	im := &IMAP{OAuthProvider: OAuthProviderGoogle}
	_, err := GetValidAccessToken(context.Background(), im)
	c.Assert(err, check.Equals, ErrOAuthNotConnected)
}

func (s *ModelsSuite) TestGetValidAccessTokenNoSecretKey(c *check.C) {
	origKey := conf.SecretKey
	conf.SecretKey = ""
	defer func() { conf.SecretKey = origKey }()

	im := &IMAP{OAuthProvider: OAuthProviderGoogle, OAuthRefreshTokenEnc: "irrelevant"}
	_, err := GetValidAccessToken(context.Background(), im)
	c.Assert(err, check.NotNil)
}

func (s *ModelsSuite) TestGetValidAccessTokenCacheHit(c *check.C) {
	origKey := conf.SecretKey
	conf.SecretKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	defer func() { conf.SecretKey = origKey }()

	var tokenEndpointHits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&tokenEndpointHits, 1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "should-not-be-used",
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
	}))
	defer server.Close()

	im := &IMAP{
		UserId:            1,
		Host:              "127.0.0.1",
		Port:              993,
		Username:          "user@example.com",
		AuthType:          IMAPAuthTypeOAuth2,
		OAuthProvider:     OAuthProviderCustom,
		OAuthClientID:     "client-id",
		OAuthClientSecret: "client-secret",
		OAuthAuthURL:      server.URL + "/authorize",
		OAuthTokenURL:     server.URL + "/token",
		OAuthScopes:       "scope",
	}
	// StoreOAuthToken targets an existing row by user_id (IMAP has no
	// primary key - see PostIMAP/StoreOAuthToken), so the config needs to
	// actually be persisted first, same as it would be via a real
	// POST /api/imap/ before ever reaching the OAuth callback.
	c.Assert(PostIMAP(im, 1), check.Equals, nil)

	tok := &oauth2.Token{
		AccessToken:  "cached-access-token",
		RefreshToken: "cached-refresh-token",
		Expiry:       time.Now().Add(1 * time.Hour),
	}
	c.Assert(StoreOAuthToken(im, tok), check.Equals, nil)

	got, err := GetValidAccessToken(context.Background(), im)
	c.Assert(err, check.Equals, nil)
	c.Assert(got, check.Equals, "cached-access-token")
	c.Assert(atomic.LoadInt32(&tokenEndpointHits), check.Equals, int32(0))
}

func (s *ModelsSuite) TestGetValidAccessTokenRefreshesWhenExpired(c *check.C) {
	origKey := conf.SecretKey
	conf.SecretKey = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	defer func() { conf.SecretKey = origKey }()

	var tokenEndpointHits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&tokenEndpointHits, 1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token":  "refreshed-access-token",
			"token_type":    "Bearer",
			"expires_in":    3600,
			"refresh_token": "rotated-refresh-token",
		})
	}))
	defer server.Close()

	im := &IMAP{
		UserId:            1,
		Host:              "127.0.0.1",
		Port:              993,
		Username:          "user@example.com",
		AuthType:          IMAPAuthTypeOAuth2,
		OAuthProvider:     OAuthProviderCustom,
		OAuthClientID:     "client-id",
		OAuthClientSecret: "client-secret",
		OAuthAuthURL:      server.URL + "/authorize",
		OAuthTokenURL:     server.URL + "/token",
		OAuthScopes:       "scope",
	}
	c.Assert(PostIMAP(im, 1), check.Equals, nil)

	// Already-expired cached token forces a refresh.
	tok := &oauth2.Token{
		AccessToken:  "stale-access-token",
		RefreshToken: "original-refresh-token",
		Expiry:       time.Now().Add(-1 * time.Hour),
	}
	c.Assert(StoreOAuthToken(im, tok), check.Equals, nil)

	got, err := GetValidAccessToken(context.Background(), im)
	c.Assert(err, check.Equals, nil)
	c.Assert(got, check.Equals, "refreshed-access-token")
	c.Assert(atomic.LoadInt32(&tokenEndpointHits), check.Equals, int32(1))

	// The refreshed (rotated) refresh token should have been persisted.
	key, err := conf.SecretKeyBytes()
	c.Assert(err, check.Equals, nil)
	stored, err := gophishcrypto.Decrypt(im.OAuthRefreshTokenEnc, key)
	c.Assert(err, check.Equals, nil)
	c.Assert(string(stored), check.Equals, "rotated-refresh-token")
}

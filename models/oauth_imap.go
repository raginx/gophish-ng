package models

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	gophishcrypto "github.com/gophish/gophish/crypto"
	"github.com/gophish/gophish/dialer"
	log "github.com/gophish/gophish/logger"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/microsoft"
)

// ErrOAuthNotConnected is returned by GetValidAccessToken when the IMAP
// config is set to OAuth2 auth but no account has been connected yet (no
// refresh token stored) - the admin needs to complete the
// /oauth/imap/authorize flow first.
var ErrOAuthNotConnected = errors.New("no OAuth2 account connected - complete the OAuth authorization flow first")

// tokenExpiryMargin is how far ahead of the stored expiry we proactively
// refresh, so a token doesn't expire mid-request.
const tokenExpiryMargin = 2 * time.Minute

// defaultOAuthScopes are the well-known IMAP OAuth2 scopes for the
// built-in providers. "custom" has no sane default and always requires
// IMAP.OAuthScopes to be set explicitly.
var defaultOAuthScopes = map[string][]string{
	OAuthProviderMicrosoft: {"https://outlook.office.com/IMAP.AccessAsUser.All", "offline_access"},
	OAuthProviderGoogle:    {"https://mail.google.com/"},
}

// oauthEndpoint returns the OAuth2 authorization/token endpoint for the
// IMAP config's provider.
func oauthEndpoint(im *IMAP) (oauth2.Endpoint, error) {
	switch im.OAuthProvider {
	case OAuthProviderMicrosoft:
		return microsoft.AzureADEndpoint(im.OAuthTenantID), nil
	case OAuthProviderGoogle:
		return google.Endpoint, nil
	case OAuthProviderCustom:
		return oauth2.Endpoint{AuthURL: im.OAuthAuthURL, TokenURL: im.OAuthTokenURL}, nil
	default:
		return oauth2.Endpoint{}, ErrInvalidOAuthProvider
	}
}

// oauthScopes returns the OAuth2 scopes to request for the IMAP config's
// provider - IMAP.OAuthScopes if set, otherwise the built-in default for
// well-known providers.
func oauthScopes(im *IMAP) []string {
	if im.OAuthScopes != "" {
		return strings.Fields(im.OAuthScopes)
	}
	return defaultOAuthScopes[im.OAuthProvider]
}

// oauthHTTPClient returns an *http.Client dialing through the same
// SSRF-restricted dialer used for SMTP/IMAP connections (dialer.Dialer()).
// The "custom" OAuth provider's endpoint URLs are admin-configured, and
// thus - from this app's threat model - not fully trusted, so
// token-endpoint requests get the same treatment.
func oauthHTTPClient() *http.Client {
	d := dialer.Dialer()
	return &http.Client{
		Transport: &http.Transport{
			DialContext: d.DialContext,
		},
	}
}

// oauthContext returns a context carrying the SSRF-restricted HTTP client
// for use with oauth2 package calls (AuthCodeURL doesn't need it, but
// Exchange/TokenSource do).
func oauthContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, oauth2.HTTPClient, oauthHTTPClient())
}

// OAuthConfig builds the oauth2.Config for the IMAP config's provider,
// ready to use for the authorization-code flow.
func OAuthConfig(im *IMAP, redirectURL string) (*oauth2.Config, error) {
	endpoint, err := oauthEndpoint(im)
	if err != nil {
		return nil, err
	}
	return &oauth2.Config{
		ClientID:     im.OAuthClientID,
		ClientSecret: im.OAuthClientSecret,
		RedirectURL:  redirectURL,
		Scopes:       oauthScopes(im),
		Endpoint:     endpoint,
	}, nil
}

// ExchangeOAuthCode exchanges an OAuth2 authorization code (as received on
// the /oauth/imap/callback redirect) for a token, via the SSRF-restricted
// HTTP client.
func ExchangeOAuthCode(ctx context.Context, im *IMAP, redirectURL, code string) (*oauth2.Token, error) {
	oauthConf, err := OAuthConfig(im, redirectURL)
	if err != nil {
		return nil, err
	}
	return oauthConf.Exchange(oauthContext(ctx), code)
}

// StoreOAuthToken encrypts and persists the given token onto the IMAP
// config, as a result of either the initial authorization-code exchange or
// a later refresh.
//
// IMAP has no primary key (see PostIMAP), so this can't go through
// db.Save() - like SuccessfulLogin, it targets the row explicitly by
// user_id via an Updates map (whose keys are literal column names, not Go
// field names).
func StoreOAuthToken(im *IMAP, tok *oauth2.Token) error {
	key, err := conf.SecretKeyBytes()
	if err != nil {
		return err
	}
	accessEnc, err := gophishcrypto.Encrypt([]byte(tok.AccessToken), key)
	if err != nil {
		return err
	}
	updates := map[string]interface{}{
		"oauth_access_token_enc": accessEnc,
		"oauth_token_expiry":     tok.Expiry,
	}
	// Not every refresh response includes a new refresh token - some
	// providers only rotate it occasionally. Only overwrite ours if we
	// actually got a new one.
	refreshEnc := im.OAuthRefreshTokenEnc
	if tok.RefreshToken != "" {
		refreshEnc, err = gophishcrypto.Encrypt([]byte(tok.RefreshToken), key)
		if err != nil {
			return err
		}
		updates["oauth_refresh_token_enc"] = refreshEnc
	}
	if err := db.Model(&IMAP{}).Where("user_id = ?", im.UserId).Updates(updates).Error; err != nil {
		return err
	}
	// Keep the in-memory struct in sync with what was just persisted.
	im.OAuthAccessTokenEnc = accessEnc
	im.OAuthTokenExpiry = tok.Expiry
	im.OAuthRefreshTokenEnc = refreshEnc
	return nil
}

// GetValidAccessToken returns a currently-valid OAuth2 access token for the
// IMAP config, transparently refreshing (and persisting the refreshed
// token) if the cached one is expired or about to expire. Returns
// ErrOAuthNotConnected if no account has been connected yet.
func GetValidAccessToken(ctx context.Context, im *IMAP) (string, error) {
	if im.OAuthRefreshTokenEnc == "" {
		return "", ErrOAuthNotConnected
	}
	key, err := conf.SecretKeyBytes()
	if err != nil {
		return "", err
	}

	if im.OAuthAccessTokenEnc != "" && time.Until(im.OAuthTokenExpiry) > tokenExpiryMargin {
		access, err := gophishcrypto.Decrypt(im.OAuthAccessTokenEnc, key)
		if err != nil {
			return "", err
		}
		return string(access), nil
	}

	refreshPlain, err := gophishcrypto.Decrypt(im.OAuthRefreshTokenEnc, key)
	if err != nil {
		return "", err
	}
	oauthConf, err := OAuthConfig(im, "")
	if err != nil {
		return "", err
	}
	ts := oauthConf.TokenSource(oauthContext(ctx), &oauth2.Token{RefreshToken: string(refreshPlain)})
	tok, err := ts.Token()
	if err != nil {
		return "", err
	}
	if err := StoreOAuthToken(im, tok); err != nil {
		// The refreshed token is still good to use for this poll even if
		// we failed to persist it - log and continue, we'll just refresh
		// again next time.
		log.Error("Unable to persist refreshed OAuth token: ", err.Error())
	}
	return tok.AccessToken, nil
}

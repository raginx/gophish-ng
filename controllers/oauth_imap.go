package controllers

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"

	"github.com/gorilla/sessions"
	ctx "github.com/raginx/gophish-ng/context"
	log "github.com/raginx/gophish-ng/logger"
	"github.com/raginx/gophish-ng/models"
	"golang.org/x/oauth2"
)

// oauthStateSessionKey is where the OAuth2 CSRF-defense nonce is stashed
// in the session between OAuthIMAPAuthorize and OAuthIMAPCallback
const oauthStateSessionKey = "imap_oauth_state"

// oauthRedirectURL derives the redirect_uri for the OAuth2 dance from the
// incoming requests own host
func (as *AdminServer) oauthRedirectURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || as.config.UseTLS {
		scheme = "https"
	}
	return scheme + "://" + r.Host + "/oauth/imap/callback"
}

func generateOAuthState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// currentUserIMAPConfig loads the current users IMAP config and confirms
// its set up for OAuth2
func currentUserIMAPConfig(uid int64) (models.IMAP, error) {
	ims, err := models.GetIMAP(uid)
	if err != nil {
		return models.IMAP{}, err
	}
	if len(ims) == 0 || ims[0].AuthType != models.IMAPAuthTypeOAuth2 {
		return models.IMAP{}, models.ErrOAuthProviderNotSpecified
	}
	return ims[0], nil
}

// OAuthIMAPAuthorize starts the OAuth2 authorization-code flow for IMAP
// email reporting
func (as *AdminServer) OAuthIMAPAuthorize(w http.ResponseWriter, r *http.Request) {
	u := ctx.Get(r, "user").(models.User)
	im, err := currentUserIMAPConfig(u.Id)
	if err != nil {
		Flash(w, r, "danger", "Save an OAuth2 IMAP configuration before connecting an account")
		http.Redirect(w, r, "/settings#reportingSettings", http.StatusFound)
		return
	}

	oauthConf, err := models.OAuthConfig(&im, as.oauthRedirectURL(r))
	if err != nil {
		log.Error(err)
		Flash(w, r, "danger", "Invalid OAuth configuration: "+err.Error())
		http.Redirect(w, r, "/settings#reportingSettings", http.StatusFound)
		return
	}

	state, err := generateOAuthState()
	if err != nil {
		log.Error(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	session := ctx.Get(r, "session").(*sessions.Session)
	session.Values[oauthStateSessionKey] = state
	if err := session.Save(r, w); err != nil {
		log.Error(err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, oauthConf.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce), http.StatusFound)
}

// OAuthIMAPCallback completes the OAuth2 authorization-code flow started by
// OAuthIMAPAuthorize.
func (as *AdminServer) OAuthIMAPCallback(w http.ResponseWriter, r *http.Request) {
	u := ctx.Get(r, "user").(models.User)
	session := ctx.Get(r, "session").(*sessions.Session)

	expectedState, _ := session.Values[oauthStateSessionKey].(string)
	delete(session.Values, oauthStateSessionKey)
	if err := session.Save(r, w); err != nil {
		log.Error(err)
	}

	if err := r.ParseForm(); err != nil {
		log.Error(err)
		Flash(w, r, "danger", "Invalid OAuth callback request")
		http.Redirect(w, r, "/settings#reportingSettings", http.StatusFound)
		return
	}

	if expectedState == "" || r.FormValue("state") != expectedState {
		Flash(w, r, "danger", "Invalid or expired OAuth state - please try connecting again")
		http.Redirect(w, r, "/settings#reportingSettings", http.StatusFound)
		return
	}

	if errMsg := r.FormValue("error"); errMsg != "" {
		Flash(w, r, "danger", "OAuth authorization failed: "+errMsg)
		http.Redirect(w, r, "/settings#reportingSettings", http.StatusFound)
		return
	}

	im, err := currentUserIMAPConfig(u.Id)
	if err != nil {
		Flash(w, r, "danger", "No OAuth2 IMAP configuration found")
		http.Redirect(w, r, "/settings#reportingSettings", http.StatusFound)
		return
	}

	tok, err := models.ExchangeOAuthCode(r.Context(), &im, as.oauthRedirectURL(r), r.FormValue("code"))
	if err != nil {
		log.Error(err)
		Flash(w, r, "danger", "Unable to complete OAuth authorization")
		http.Redirect(w, r, "/settings#reportingSettings", http.StatusFound)
		return
	}

	if err := models.StoreOAuthToken(&im, tok); err != nil {
		log.Error(err)
		Flash(w, r, "danger", "Unable to save OAuth token: "+err.Error())
		http.Redirect(w, r, "/settings#reportingSettings", http.StatusFound)
		return
	}

	Flash(w, r, "success", "Successfully connected your account for IMAP reporting")
	http.Redirect(w, r, "/settings#reportingSettings", http.StatusFound)
}

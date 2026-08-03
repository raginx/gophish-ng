package api

import (
	"encoding/json"
	"net/http"
	"time"

	ctx "github.com/gophish/gophish/context"
	"github.com/gophish/gophish/imap"
	"github.com/gophish/gophish/models"
)

// imapResponse is what GET /api/imap/ actually returns
type imapResponse struct {
	models.IMAP
	OAuthConnected bool `json:"oauth_connected"`
}

func newIMAPResponse(im models.IMAP) imapResponse {
	connected := im.OAuthRefreshTokenEnc != ""
	im.Password = ""
	im.OAuthClientSecret = ""
	return imapResponse{IMAP: im, OAuthConnected: connected}
}

// IMAPServerValidate handles requests for the /api/imapserver/validate endpoint
func (as *Server) IMAPServerValidate(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "GET":
		JSONResponse(w, models.Response{Success: false, Message: "Only POSTs allowed"}, http.StatusBadRequest)
	case r.Method == "POST":
		im := models.IMAP{}
		err := json.NewDecoder(r.Body).Decode(&im)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Invalid request"}, http.StatusBadRequest)
			return
		}
		if im.AuthType == models.IMAPAuthTypeOAuth2 {
			// OAuth2 secrets (client secret, tokens) never round-trip to
			// the browser (see newIMAPResponse), so there's nothing
			// meaningful in the request body to validate against - test
			// the already-saved, already-connected config instead.
			uid := ctx.Get(r, "user_id").(int64)
			saved, err := models.GetIMAP(uid)
			if err != nil || len(saved) == 0 {
				JSONResponse(w, models.Response{Success: false, Message: "Save your IMAP settings first"}, http.StatusOK)
				return
			}
			im = saved[0]
		}
		err = imap.Validate(&im)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusOK)
			return
		}
		JSONResponse(w, models.Response{Success: true, Message: "Successful login."}, http.StatusCreated)
	}
}

// IMAPServer handles requests for the /api/imapserver/ endpoint
func (as *Server) IMAPServer(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "GET":
		ims, err := models.GetIMAP(ctx.Get(r, "user_id").(int64))
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
			return
		}
		resp := make([]imapResponse, len(ims))
		for i, im := range ims {
			resp[i] = newIMAPResponse(im)
		}
		JSONResponse(w, resp, http.StatusOK)

	// POST: Update database
	case r.Method == "POST":
		im := models.IMAP{}
		err := json.NewDecoder(r.Body).Decode(&im)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Invalid data. Please check your IMAP settings."}, http.StatusBadRequest)
			return
		}
		uid := ctx.Get(r, "user_id").(int64)
		// Secrets never round-trip to the browser (see newIMAPResponse),
		// so a blank value here means "unchanged" and
		// the OAuth tokens are always blank on the wire (json:"-"), since
		// they're only ever written by the OAuth callback flow /
		// GetValidAccessToken's refresh path
		if existing, err := models.GetIMAP(uid); err == nil && len(existing) > 0 {
			if im.Password == "" {
				im.Password = existing[0].Password
			}
			if im.OAuthClientSecret == "" {
				im.OAuthClientSecret = existing[0].OAuthClientSecret
			}
			im.OAuthRefreshTokenEnc = existing[0].OAuthRefreshTokenEnc
			im.OAuthAccessTokenEnc = existing[0].OAuthAccessTokenEnc
			im.OAuthTokenExpiry = existing[0].OAuthTokenExpiry
		}
		im.ModifiedDate = time.Now().UTC()
		im.UserId = uid
		err = models.PostIMAP(&im, uid)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, models.Response{Success: true, Message: "Successfully saved IMAP settings."}, http.StatusCreated)
	}
}

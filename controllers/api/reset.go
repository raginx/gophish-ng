package api

import (
	"net/http"

	"github.com/raginx/gophish-ng/audit"
	"github.com/raginx/gophish-ng/auth"
	ctx "github.com/raginx/gophish-ng/context"
	"github.com/raginx/gophish-ng/models"
)

// Reset (/api/reset) resets the currently authenticated user's API key
func (as *Server) Reset(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "POST":
		u := ctx.Get(r, "user").(models.User)
		u.ApiKey = auth.GenerateSecureKey(auth.APIKeyLength)
		err := models.PutUser(&u)
		if err != nil {
			http.Error(w, "Error setting API Key", http.StatusInternalServerError)
		} else {
			audit.Record(audit.Event{
				Event:    audit.EventAPIKeyReset,
				Outcome:  audit.OutcomeSuccess,
				Actor:    &audit.Actor{ID: u.Id, Username: u.Username},
				SourceIP: audit.SourceIP(r),
			})
			JSONResponse(w, models.Response{Success: true, Message: "API Key successfully reset!", Data: u.ApiKey}, http.StatusOK)
		}
	}
}

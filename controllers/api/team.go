package api

import (
	"encoding/json"
	"net/http"

	log "github.com/gophish/gophish/logger"
	"github.com/gophish/gophish/models"
)

// Teams handles requests for the /api/teams/ endpoint. Team management is
// admin-only (see the PermissionModifySystem guard in server.go) the
// same team assignment happens through the Users endpoints for everyone
// else
func (as *Server) Teams(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		ts, err := models.GetTeams()
		if err != nil {
			log.Error(err)
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, ts, http.StatusOK)

	case "POST":
		t := models.Team{}
		err := json.NewDecoder(r.Body).Decode(&t)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Invalid JSON structure"}, http.StatusBadRequest)
			return
		}
		err = models.PostTeam(&t)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusBadRequest)
			return
		}
		JSONResponse(w, t, http.StatusCreated)
	}
}

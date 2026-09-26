package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	ctx "github.com/raginx/gophish-ng/context"
	log "github.com/raginx/gophish-ng/logger"
	"github.com/raginx/gophish-ng/models"
	"gorm.io/gorm"
)

// Tags handles the functionality for the /api/tags/ endpoint
func (as *Server) Tags(w http.ResponseWriter, r *http.Request) {
	teamID := ctx.Get(r, "team_id").(int64)
	switch r.Method {
	case "GET":
		tags, err := models.GetTags(teamID)
		if err != nil {
			log.Error(err)
		}
		JSONResponse(w, tags, http.StatusOK)
	case "POST":
		t := models.Tag{}
		if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Invalid JSON structure"}, http.StatusBadRequest)
			return
		}
		// The id and team are never taken from the client.
		t.Id = 0
		t.TeamId = teamID
		t.ModifiedDate = time.Now().UTC()
		err := models.PostTag(&t)
		if errors.Is(err, models.ErrTagNameNotSpecified) {
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusBadRequest)
			return
		}
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			JSONResponse(w, models.Response{Success: false, Message: "Tag name already in use"}, http.StatusConflict)
			return
		}
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, t, http.StatusCreated)
	}
}

// Tag handles the functionality for the /api/tags/:id endpoint
func (as *Server) Tag(w http.ResponseWriter, r *http.Request) {
	teamID := ctx.Get(r, "team_id").(int64)
	vars := mux.Vars(r)
	id, _ := strconv.ParseInt(vars["id"], 0, 64)
	t, err := models.GetTag(id, teamID)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Tag not found"}, http.StatusNotFound)
		return
	}
	switch r.Method {
	case "GET":
		JSONResponse(w, t, http.StatusOK)
	case "PUT":
		payload := models.Tag{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Invalid JSON structure"}, http.StatusBadRequest)
			return
		}
		// Only the name and color are editable; id and team stay fixed.
		t.Name = payload.Name
		t.Color = payload.Color
		err = models.PutTag(&t)
		if errors.Is(err, models.ErrTagNameNotSpecified) {
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusBadRequest)
			return
		}
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			JSONResponse(w, models.Response{Success: false, Message: "Tag name already in use"}, http.StatusConflict)
			return
		}
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, t, http.StatusOK)
	case "DELETE":
		if err := models.DeleteTag(id, teamID); err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Error deleting tag"}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, models.Response{Success: true, Message: "Tag deleted successfully!"}, http.StatusOK)
	}
}

// tagFilter returns the set of object ids (of the given taggable type) that
// carry at least one of the tags named in the request's repeated ?tag=
// query params, plus whether any tag filter was requested at all. When no
// ?tag= param is present, ok is false and callers should skip filtering.
func tagFilter(r *http.Request, taggableType string, teamID int64) (allowed map[int64]bool, ok bool) {
	names := r.URL.Query()["tag"]
	if len(names) == 0 {
		return nil, false
	}
	ids, err := models.TaggedIDs(taggableType, teamID, names)
	if err != nil {
		log.Error(err)
	}
	allowed = make(map[int64]bool, len(ids))
	for _, id := range ids {
		allowed[id] = true
	}
	return allowed, true
}

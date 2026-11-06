package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	ctx "github.com/raginx/gophish-ng/context"
	log "github.com/raginx/gophish-ng/logger"
	"github.com/raginx/gophish-ng/models"
	"gorm.io/gorm"
)

// Groups returns a list of groups if requested via GET.
// If requested via POST, APIGroups creates a new group and returns a reference to it.
func (as *Server) Groups(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "GET":
		teamID := ctx.Get(r, "team_id").(int64)
		gs, err := models.GetGroups(teamID)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "No groups found"}, http.StatusNotFound)
			return
		}
		if allowed, ok := tagFilter(r, models.TaggableGroup, teamID); ok {
			filtered := []models.Group{}
			for _, g := range gs {
				if allowed[g.Id] {
					filtered = append(filtered, g)
				}
			}
			gs = filtered
		}
		JSONResponse(w, gs, http.StatusOK)
	//POST: Create a new group and return it as JSON
	case r.Method == "POST":
		// groupRequest extends the group payload with an optional LDAP
		// synchronization mapping. When ldap_sync is present, the group is
		// created as an LDAP-managed group (atomically, with the mapping) and
		// does not require manually supplied members. When absent, this is an
		// ordinary manual group and behaves exactly as before.
		type groupRequest struct {
			models.Group
			LDAPSync *models.LDAPGroupSync `json:"ldap_sync,omitempty"`
		}
		req := groupRequest{}
		// Put the request into a group
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Invalid JSON structure"}, http.StatusBadRequest)
			return
		}
		g := req.Group
		if req.LDAPSync != nil {
			// The managed-group name is auto-filled from the selected AD group
			// by the UI; trim it before the uniqueness check and persistence.
			g.Name = strings.TrimSpace(g.Name)
		}
		_, err = models.GetGroupByName(g.Name, ctx.Get(r, "team_id").(int64))
		if err != gorm.ErrRecordNotFound {
			JSONResponse(w, models.Response{Success: false, Message: "Group name already in use"}, http.StatusConflict)
			return
		}
		g.ModifiedDate = time.Now().UTC()
		g.UserId = ctx.Get(r, "user_id").(int64)
		g.TeamId = ctx.Get(r, "team_id").(int64)
		if req.LDAPSync != nil {
			// LDAP-managed group: create the group and its mapping atomically.
			// Any manually supplied targets are ignored - membership is owned
			// by LDAP synchronization.
			g.Targets = nil
			// Resolve the selected AD group server-side BEFORE creating
			// anything: a group that does not resolve leaves no orphan.
			req.LDAPSync.UserId = g.UserId
			req.LDAPSync.TeamId = g.TeamId
			if status, msg := resolveMappingGroup(req.LDAPSync); msg != "" {
				JSONResponse(w, models.Response{Success: false, Message: msg}, status)
				return
			}
			err = models.PostLDAPManagedGroup(&g, req.LDAPSync)
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				JSONResponse(w, models.Response{Success: false, Message: "Group name already in use"}, http.StatusConflict)
				return
			}
			if err != nil {
				JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusBadRequest)
				return
			}
			JSONResponse(w, g, http.StatusCreated)
			return
		}
		err = models.PostGroup(&g)
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			// Two concurrent requests both passed the check above before
			// either finished inserting - the DB-level unique constraint
			// is what actually stops the race (see #3278).
			JSONResponse(w, models.Response{Success: false, Message: "Group name already in use"}, http.StatusConflict)
			return
		}
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusBadRequest)
			return
		}
		JSONResponse(w, g, http.StatusCreated)
	}
}

// GroupsSummary returns a summary of the groups owned by the current user.
func (as *Server) GroupsSummary(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "GET":
		gs, err := models.GetGroupSummaries(ctx.Get(r, "team_id").(int64))
		if err != nil {
			log.Error(err)
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, gs, http.StatusOK)
	}
}

// Group returns details about the requested group.
// If the group is not valid, Group returns null.
func (as *Server) Group(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, _ := strconv.ParseInt(vars["id"], 0, 64)
	g, err := models.GetGroup(id, ctx.Get(r, "team_id").(int64))
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Group not found"}, http.StatusNotFound)
		return
	}
	switch {
	case r.Method == "GET":
		JSONResponse(w, g, http.StatusOK)
	case r.Method == "DELETE":
		err = models.DeleteGroup(&g)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Error deleting group"}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, models.Response{Success: true, Message: "Group deleted successfully!"}, http.StatusOK)
	case r.Method == "PUT":
		// Change this to get from URL and uid (don't bother with id in r.Body)
		g = models.Group{}
		err = json.NewDecoder(r.Body).Decode(&g)
		if err != nil {
			log.Errorf("error decoding group: %v", err)
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
			return
		}
		if g.Id != id {
			JSONResponse(w, models.Response{Success: false, Message: "Error: /:id and group_id mismatch"}, http.StatusInternalServerError)
			return
		}
		teamID := ctx.Get(r, "team_id").(int64)
		// Membership of an LDAP-managed group is owned exclusively by LDAP
		// synchronization. Reject any attempt to edit it manually through the
		// group API, regardless of what the UI allows. (Manual groups are
		// unaffected.)
		managed, mErr := models.IsGroupLDAPManaged(id, teamID)
		if mErr != nil {
			JSONResponse(w, models.Response{Success: false, Message: mErr.Error()}, http.StatusInternalServerError)
			return
		}
		if managed {
			JSONResponse(w, models.Response{Success: false, Message: "This group's membership is managed by LDAP synchronization and cannot be modified manually."}, http.StatusConflict)
			return
		}
		g.ModifiedDate = time.Now().UTC()
		g.UserId = ctx.Get(r, "user_id").(int64)
		g.TeamId = teamID
		err = models.PutGroup(&g)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusBadRequest)
			return
		}
		JSONResponse(w, g, http.StatusOK)
	}
}

// GroupSummary returns a summary of the groups owned by the current user.
func (as *Server) GroupSummary(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == "GET":
		vars := mux.Vars(r)
		id, _ := strconv.ParseInt(vars["id"], 0, 64)
		g, err := models.GetGroupSummary(id, ctx.Get(r, "team_id").(int64))
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Group not found"}, http.StatusNotFound)
			return
		}
		JSONResponse(w, g, http.StatusOK)
	}
}

package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	ctx "github.com/raginx/gophish-ng/context"
	"github.com/raginx/gophish-ng/ldap"
	log "github.com/raginx/gophish-ng/logger"
	"github.com/raginx/gophish-ng/models"
)

// sanitizeConnection strips the write-only bind password before returning a
// connection over the API, mirroring how the IMAP handler blanks secrets.
func sanitizeConnection(c models.LDAPConnection) models.LDAPConnection {
	c.BindPassword = ""
	c.BindPasswordEnc = ""
	return c
}

// LDAPConnections handles GET (list) and POST (create) for LDAP connections.
func (as *Server) LDAPConnections(w http.ResponseWriter, r *http.Request) {
	teamID := ctx.Get(r, "team_id").(int64)
	switch r.Method {
	case http.MethodGet:
		cs, err := models.GetLDAPConnections(teamID)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
			return
		}
		for i := range cs {
			cs[i] = sanitizeConnection(cs[i])
		}
		JSONResponse(w, cs, http.StatusOK)
	case http.MethodPost:
		c := models.LDAPConnection{}
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Invalid JSON structure"}, http.StatusBadRequest)
			return
		}
		c.UserId = ctx.Get(r, "user_id").(int64)
		c.TeamId = teamID
		if err := models.PostLDAPConnection(&c); err != nil {
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusBadRequest)
			return
		}
		JSONResponse(w, sanitizeConnection(c), http.StatusCreated)
	}
}

// LDAPConnection handles GET, PUT and DELETE for a single connection.
func (as *Server) LDAPConnection(w http.ResponseWriter, r *http.Request) {
	teamID := ctx.Get(r, "team_id").(int64)
	vars := mux.Vars(r)
	id, _ := strconv.ParseInt(vars["id"], 0, 64)
	c, err := models.GetLDAPConnection(id, teamID)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "LDAP connection not found"}, http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		JSONResponse(w, sanitizeConnection(c), http.StatusOK)
	case http.MethodDelete:
		if derr := models.DeleteLDAPConnection(id, teamID); derr != nil {
			if errors.Is(derr, models.ErrLDAPConnectionInUse) {
				JSONResponse(w, models.Response{Success: false, Message: derr.Error()}, http.StatusConflict)
				return
			}
			JSONResponse(w, models.Response{Success: false, Message: derr.Error()}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, models.Response{Success: true, Message: "LDAP connection deleted"}, http.StatusOK)
	case http.MethodPut:
		updated := models.LDAPConnection{}
		if derr := json.NewDecoder(r.Body).Decode(&updated); derr != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Invalid JSON structure"}, http.StatusBadRequest)
			return
		}
		if updated.Id != id {
			JSONResponse(w, models.Response{Success: false, Message: "Error: /:id and connection id mismatch"}, http.StatusBadRequest)
			return
		}
		updated.UserId = ctx.Get(r, "user_id").(int64)
		updated.TeamId = teamID
		if perr := models.PutLDAPConnection(&updated); perr != nil {
			JSONResponse(w, models.Response{Success: false, Message: perr.Error()}, http.StatusBadRequest)
			return
		}
		JSONResponse(w, sanitizeConnection(updated), http.StatusOK)
	}
}

// LDAPConnectionTest binds to the directory to verify the stored connection
// settings. It never reads or mutates a group.
func (as *Server) LDAPConnectionTest(w http.ResponseWriter, r *http.Request) {
	teamID := ctx.Get(r, "team_id").(int64)
	vars := mux.Vars(r)
	id, _ := strconv.ParseInt(vars["id"], 0, 64)
	c, err := models.GetLDAPConnection(id, teamID)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "LDAP connection not found"}, http.StatusNotFound)
		return
	}
	testErr := ldap.TestConnection(&c)
	if rerr := models.RecordLDAPTestResult(id, teamID, testErr); rerr != nil {
		log.Error(rerr)
	}
	if testErr != nil {
		JSONResponse(w, models.Response{Success: false, Message: testErr.Error()}, http.StatusOK)
		return
	}
	JSONResponse(w, models.Response{Success: true, Message: "Successfully connected to the LDAP server."}, http.StatusOK)
}

// LDAPConnectionGroups searches the connection's Active Directory for groups
// whose name contains the ?q= text, for selecting the directory group an
// LDAP-managed Gophish group synchronizes from.
//
// Object-tier (modify_objects): it only consumes an already-configured,
// team-scoped connection and its stored encrypted bind credential. The server,
// bind identity, BaseDN and TLS settings always come from the stored
// connection; the request can only supply a short search text, which is
// bounded and escaped server-side. The response carries only group name, DN
// and normalized stable identifier - never members, user data or credentials.
func (as *Server) LDAPConnectionGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		JSONResponse(w, models.Response{Success: false, Message: "Method not allowed"}, http.StatusMethodNotAllowed)
		return
	}
	teamID := ctx.Get(r, "team_id").(int64)
	vars := mux.Vars(r)
	id, _ := strconv.ParseInt(vars["id"], 0, 64)
	c, err := models.GetLDAPConnection(id, teamID)
	if err != nil {
		// Not found for this team - do not reveal whether it exists elsewhere.
		JSONResponse(w, models.Response{Success: false, Message: "LDAP connection not found"}, http.StatusNotFound)
		return
	}
	items, serr := ldap.SearchGroups(&c, r.URL.Query().Get("q"))
	if serr != nil {
		switch {
		case errors.Is(serr, ldap.ErrNotActiveDirectory), errors.Is(serr, ldap.ErrGroupQueryTooLong):
			// Client-correctable request errors (safe messages, no infra detail).
			JSONResponse(w, models.Response{Success: false, Message: serr.Error()}, http.StatusBadRequest)
		default:
			// Connection/bind/search/backend failure: generic sanitized message;
			// the raw error (which may include the server address) is logged
			// server-side only.
			log.Errorf("LDAP group search failed for connection %d (team %d): %s", id, teamID, serr.Error())
			JSONResponse(w, models.Response{Success: false, Message: "Unable to search Active Directory groups. Verify the connection configuration and try again."}, http.StatusInternalServerError)
		}
		return
	}
	JSONResponse(w, items, http.StatusOK)
}

// LDAPConnectionsSummary returns a minimal, non-sensitive list of the team's
// LDAP connections (id/name/base_dn only) for the Groups UI connection picker.
// It is object-tier (modify_objects) and never exposes credentials, server
// URL, TLS/security settings, or the directory identity attribute. Connection
// administration stays on the modify_system endpoints.
func (as *Server) LDAPConnectionsSummary(w http.ResponseWriter, r *http.Request) {
	teamID := ctx.Get(r, "team_id").(int64)
	summaries, err := models.GetLDAPConnectionSummaries(teamID)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
		return
	}
	JSONResponse(w, summaries, http.StatusOK)
}

// resolveDirectoryGroup locates a directory group by its stable identity on a
// stored connection. It is a package variable so tests can substitute an
// in-memory resolver; production always uses ldap.ResolveGroupByID.
var resolveDirectoryGroup = ldap.ResolveGroupByID

// msgResolveGroupBackend is the sanitized message returned when the directory
// could not be contacted/searched while resolving the selected group. The raw
// error may carry the server address and is logged server-side only.
const msgResolveGroupBackend = "Unable to resolve the selected Active Directory group. Verify the connection configuration and try again."

// resolveMappingGroup makes the stored connection authoritative for the group
// selected on a NEW mapping. The client identifies the group only by
// group_directory_id; any client-supplied group_dn/group_name is discarded.
// It loads the team-scoped connection, requires Active Directory, resolves the
// group by objectGUID under the connection BaseDN (ldap.ResolveGroupByID also
// enforces containment), and stores the server-observed identifier, DN and
// name on the mapping. On failure it returns the HTTP status to answer with
// and a safe message for the client (empty on success); no mapping or group
// must be created in that case.
func resolveMappingGroup(s *models.LDAPGroupSync) (int, string) {
	if s.ConnectionId == 0 {
		return http.StatusBadRequest, models.ErrLDAPConnectionNotSpecified.Error()
	}
	conn, err := models.GetLDAPConnection(s.ConnectionId, s.TeamId)
	if err != nil {
		return http.StatusBadRequest, models.ErrLDAPConnectionNotFound.Error()
	}
	if !conn.IsActiveDirectory() {
		return http.StatusBadRequest, models.ErrLDAPConnectionNotActiveDirectory.Error()
	}
	id := strings.TrimSpace(s.GroupDirectoryId)
	if id == "" {
		return http.StatusBadRequest, models.ErrLDAPGroupDirectoryIDNotSpecified.Error()
	}
	g, rerr := resolveDirectoryGroup(&conn, id)
	if rerr != nil {
		switch {
		case errors.Is(rerr, ldap.ErrGroupIdentityInvalid):
			return http.StatusBadRequest, models.ErrLDAPGroupDirectoryIDInvalid.Error()
		case errors.Is(rerr, ldap.ErrNotActiveDirectory):
			return http.StatusBadRequest, models.ErrLDAPConnectionNotActiveDirectory.Error()
		case errors.Is(rerr, ldap.ErrGroupNotFound), errors.Is(rerr, ldap.ErrGroupAmbiguous), errors.Is(rerr, ldap.ErrGroupOutsideBase):
			// Client-correctable: the selected group does not resolve on this
			// connection. Safe messages, no directory values.
			return http.StatusBadRequest, rerr.Error()
		default:
			log.Errorf("LDAP group resolution failed for connection %d (team %d): %s", conn.Id, s.TeamId, rerr.Error())
			return http.StatusInternalServerError, msgResolveGroupBackend
		}
	}
	// Server-observed values replace whatever the client sent.
	s.GroupDirectoryId = g.DirectoryId
	s.GroupDN = g.DN
	s.GroupName = g.Name
	return http.StatusOK, ""
}

func (as *Server) LDAPGroupSyncs(w http.ResponseWriter, r *http.Request) {
	teamID := ctx.Get(r, "team_id").(int64)
	switch r.Method {
	case http.MethodGet:
		// Optional ?group_id= filter: return only the mapping (if any) for
		// that group within the team. A missing/invalid/unmanaged group
		// yields an empty list - the architecture-consistent result for a
		// filtered list endpoint. The model guarantees at most one mapping
		// per group (unique index on group_id).
		if raw := r.URL.Query().Get("group_id"); raw != "" {
			gid, perr := strconv.ParseInt(raw, 10, 64)
			if perr != nil {
				JSONResponse(w, []models.LDAPGroupSync{}, http.StatusOK)
				return
			}
			s, gerr := models.GetLDAPGroupSyncByGroup(gid, teamID)
			if gerr != nil {
				// Not managed / not found for this team: empty list.
				JSONResponse(w, []models.LDAPGroupSync{}, http.StatusOK)
				return
			}
			JSONResponse(w, []models.LDAPGroupSync{s}, http.StatusOK)
			return
		}
		ss, err := models.GetLDAPGroupSyncs(teamID)
		if err != nil {
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, ss, http.StatusOK)
	case http.MethodPost:
		s := models.LDAPGroupSync{}
		if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Invalid JSON structure"}, http.StatusBadRequest)
			return
		}
		s.UserId = ctx.Get(r, "user_id").(int64)
		s.TeamId = teamID
		// The directory is authoritative for the selected group: resolve it
		// by stable identity before anything is persisted.
		if status, msg := resolveMappingGroup(&s); msg != "" {
			JSONResponse(w, models.Response{Success: false, Message: msg}, status)
			return
		}
		if err := models.PostLDAPGroupSync(&s); err != nil {
			JSONResponse(w, models.Response{Success: false, Message: err.Error()}, http.StatusBadRequest)
			return
		}
		JSONResponse(w, s, http.StatusCreated)
	}
}

// LDAPGroupSync handles GET, PUT and DELETE for a single mapping.
func (as *Server) LDAPGroupSync(w http.ResponseWriter, r *http.Request) {
	teamID := ctx.Get(r, "team_id").(int64)
	vars := mux.Vars(r)
	id, _ := strconv.ParseInt(vars["id"], 0, 64)
	s, err := models.GetLDAPGroupSync(id, teamID)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "LDAP sync mapping not found"}, http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		JSONResponse(w, s, http.StatusOK)
	case http.MethodDelete:
		if derr := models.DeleteLDAPGroupSync(id, teamID); derr != nil {
			JSONResponse(w, models.Response{Success: false, Message: derr.Error()}, http.StatusInternalServerError)
			return
		}
		JSONResponse(w, models.Response{Success: true, Message: "LDAP sync mapping deleted"}, http.StatusOK)
	case http.MethodPut:
		updated := models.LDAPGroupSync{}
		if derr := json.NewDecoder(r.Body).Decode(&updated); derr != nil {
			JSONResponse(w, models.Response{Success: false, Message: "Invalid JSON structure"}, http.StatusBadRequest)
			return
		}
		if updated.Id != id {
			JSONResponse(w, models.Response{Success: false, Message: "Error: /:id and mapping id mismatch"}, http.StatusBadRequest)
			return
		}
		updated.UserId = ctx.Get(r, "user_id").(int64)
		updated.TeamId = teamID
		if perr := models.PutLDAPGroupSync(&updated); perr != nil {
			JSONResponse(w, models.Response{Success: false, Message: perr.Error()}, http.StatusBadRequest)
			return
		}
		JSONResponse(w, updated, http.StatusOK)
	}
}

// LDAPGroupSyncRun triggers a manual synchronization for a mapping now.
func (as *Server) LDAPGroupSyncRun(w http.ResponseWriter, r *http.Request) {
	teamID := ctx.Get(r, "team_id").(int64)
	vars := mux.Vars(r)
	id, _ := strconv.ParseInt(vars["id"], 0, 64)
	s, err := models.GetLDAPGroupSync(id, teamID)
	if err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "LDAP sync mapping not found"}, http.StatusNotFound)
		return
	}
	rr, syncErr := ldap.Sync(&s)
	if syncErr != nil {
		JSONResponse(w, models.Response{Success: false, Message: syncErr.Error()}, http.StatusOK)
		return
	}
	JSONResponse(w, map[string]interface{}{
		"success":    true,
		"message":    "Synchronization completed.",
		"discovered": rr.Discovered,
		"added":      rr.Added,
		"updated":    rr.Updated,
		"removed":    rr.Removed,
		"skipped":    rr.Skipped,
	}, http.StatusOK)
}

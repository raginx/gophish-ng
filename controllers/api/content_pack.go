package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	ctx "github.com/raginx/gophish-ng/context"
	"github.com/raginx/gophish-ng/models"
	"gorm.io/gorm"
)

// contentPackFormatVersion is the only format_version this server accepts,
// on both export and import.
const contentPackFormatVersion = "1"

// ContentPack is the export/import bundle format for sharing Templates and
// Pages between Gophish instances. Templates and Pages serialize cleanly
// via their own json tags (UserId/TeamId are already `json:"-"`).
type ContentPack struct {
	FormatVersion string            `json:"format_version"`
	Name          string            `json:"name,omitempty"`
	Templates     []models.Template `json:"templates"`
	Pages         []models.Page     `json:"pages"`
}

type contentPackExportRequest struct {
	TemplateIDs []int64 `json:"template_ids"`
	PageIDs     []int64 `json:"page_ids"`
}

type contentPackRenameEntry struct {
	Original string `json:"original"`
	New      string `json:"new"`
}

type contentPackImportResult struct {
	TemplatesImported []string                 `json:"templates_imported"`
	PagesImported     []string                 `json:"pages_imported"`
	Renamed           []contentPackRenameEntry `json:"renamed"`
	Errors            []string                 `json:"errors"`
}

// ContentPackExport bundles the requested Templates and Pages (scoped to
// the caller's team) into a single Content Pack document.
func (as *Server) ContentPackExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		JSONResponse(w, models.Response{Success: false, Message: "Method not allowed"}, http.StatusBadRequest)
		return
	}
	req := contentPackExportRequest{}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Invalid JSON structure"}, http.StatusBadRequest)
		return
	}
	teamID := ctx.Get(r, "team_id").(int64)
	pack := ContentPack{
		FormatVersion: contentPackFormatVersion,
		Templates:     []models.Template{},
		Pages:         []models.Page{},
	}
	for _, id := range req.TemplateIDs {
		t, err := models.GetTemplate(id, teamID)
		if err != nil {
			continue
		}
		pack.Templates = append(pack.Templates, t)
	}
	for _, id := range req.PageIDs {
		p, err := models.GetPage(id, teamID)
		if err != nil {
			continue
		}
		pack.Pages = append(pack.Pages, p)
	}
	JSONResponse(w, pack, http.StatusOK)
}

// ContentPackImport creates a new Template/Page for every entry in the
// uploaded Content Pack, scoped to the caller's team. Name collisions are
// resolved by appending a numbered suffix rather than failing the import,
// and a failure on one item doesn't abort the rest.
func (as *Server) ContentPackImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		JSONResponse(w, models.Response{Success: false, Message: "Method not allowed"}, http.StatusBadRequest)
		return
	}
	pack := ContentPack{}
	if err := json.NewDecoder(r.Body).Decode(&pack); err != nil {
		JSONResponse(w, models.Response{Success: false, Message: "Invalid JSON structure"}, http.StatusBadRequest)
		return
	}
	if pack.FormatVersion != contentPackFormatVersion {
		JSONResponse(w, models.Response{Success: false, Message: "Unsupported content pack format_version"}, http.StatusBadRequest)
		return
	}
	teamID := ctx.Get(r, "team_id").(int64)
	userID := ctx.Get(r, "user_id").(int64)
	result := contentPackImportResult{
		TemplatesImported: []string{},
		PagesImported:     []string{},
		Renamed:           []contentPackRenameEntry{},
		Errors:            []string{},
	}

	for _, t := range pack.Templates {
		original := t.Name
		// A pack from another instance may carry IDs that happen to
		// collide with rows belonging to a different team - Post*
		// saves by primary key with no ownership check, so the ID
		// must never come from the client.
		t.Id = 0
		t.UserId = userID
		t.TeamId = teamID
		t.ModifiedDate = time.Now().UTC()
		t.Name = uniqueTemplateName(original, teamID)
		if t.Name != original {
			result.Renamed = append(result.Renamed, contentPackRenameEntry{Original: original, New: t.Name})
		}
		if err := models.PostTemplate(&t); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("template %q: %s", original, err.Error()))
			continue
		}
		result.TemplatesImported = append(result.TemplatesImported, t.Name)
	}

	for _, p := range pack.Pages {
		original := p.Name
		p.Id = 0
		p.UserId = userID
		p.TeamId = teamID
		p.ModifiedDate = time.Now().UTC()
		p.Name = uniquePageName(original, teamID)
		if p.Name != original {
			result.Renamed = append(result.Renamed, contentPackRenameEntry{Original: original, New: p.Name})
		}
		if err := models.PostPage(&p); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("page %q: %s", original, err.Error()))
			continue
		}
		result.PagesImported = append(result.PagesImported, p.Name)
	}

	JSONResponse(w, result, http.StatusOK)
}

// uniqueTemplateName returns name, or name suffixed with " (n)" for the
// smallest n that isn't already used by the team, so an import never
// fails outright on a name collision.
func uniqueTemplateName(name string, teamID int64) string {
	candidate := name
	for i := 2; ; i++ {
		_, err := models.GetTemplateByName(candidate, teamID)
		if err == gorm.ErrRecordNotFound {
			return candidate
		}
		candidate = fmt.Sprintf("%s (%d)", name, i)
	}
}

// uniquePageName is the Page equivalent of uniqueTemplateName.
func uniquePageName(name string, teamID int64) string {
	candidate := name
	for i := 2; ; i++ {
		_, err := models.GetPageByName(candidate, teamID)
		if err == gorm.ErrRecordNotFound {
			return candidate
		}
		candidate = fmt.Sprintf("%s (%d)", name, i)
	}
}

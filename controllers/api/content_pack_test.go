package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	ctx "github.com/raginx/gophish-ng/context"
	"github.com/raginx/gophish-ng/models"
)

func makeContentPackRequest(path string, teamID, userID int64, body interface{}) *http.Request {
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req = ctx.Set(req, "team_id", teamID)
	req = ctx.Set(req, "user_id", userID)
	return req
}

func TestContentPackExportImportRoundtrip(t *testing.T) {
	tc := setupTest(t)
	template := models.Template{Name: "Roundtrip Template", Subject: "Subject", HTML: "<html>Hi {{.FirstName}}</html>", UserId: tc.admin.Id}
	if err := models.PostTemplate(&template); err != nil {
		t.Fatalf("error creating template: %v", err)
	}
	page := models.Page{Name: "Roundtrip Page", HTML: "<html>Test</html>", UserId: tc.admin.Id}
	if err := models.PostPage(&page); err != nil {
		t.Fatalf("error creating page: %v", err)
	}

	exportReq := makeContentPackRequest("/api/content-packs/export", tc.admin.TeamID, tc.admin.Id, contentPackExportRequest{
		TemplateIDs: []int64{template.Id},
		PageIDs:     []int64{page.Id},
	})
	exportResp := httptest.NewRecorder()
	tc.apiServer.ContentPackExport(exportResp, exportReq)
	if exportResp.Code != http.StatusOK {
		t.Fatalf("unexpected export status: %d, body: %s", exportResp.Code, exportResp.Body.String())
	}
	pack := ContentPack{}
	if err := json.NewDecoder(exportResp.Body).Decode(&pack); err != nil {
		t.Fatalf("error decoding export response: %v", err)
	}
	if len(pack.Templates) != 1 || len(pack.Pages) != 1 {
		t.Fatalf("expected 1 template and 1 page in pack, got %d templates and %d pages", len(pack.Templates), len(pack.Pages))
	}

	// Import the exported pack into a different, freshly created team - the
	// names don't collide there, so both items should be created as-is.
	otherTeam, err := models.GetOrCreateTeamByName("Other Team")
	if err != nil {
		t.Fatalf("error creating other team: %v", err)
	}
	importReq := makeContentPackRequest("/api/content-packs/import", otherTeam.Id, tc.admin.Id, pack)
	importResp := httptest.NewRecorder()
	tc.apiServer.ContentPackImport(importResp, importReq)
	if importResp.Code != http.StatusOK {
		t.Fatalf("unexpected import status: %d, body: %s", importResp.Code, importResp.Body.String())
	}
	result := contentPackImportResult{}
	if err := json.NewDecoder(importResp.Body).Decode(&result); err != nil {
		t.Fatalf("error decoding import response: %v", err)
	}
	if len(result.TemplatesImported) != 1 || result.TemplatesImported[0] != "Roundtrip Template" {
		t.Fatalf("unexpected templates_imported: %+v", result.TemplatesImported)
	}
	if len(result.PagesImported) != 1 || result.PagesImported[0] != "Roundtrip Page" {
		t.Fatalf("unexpected pages_imported: %+v", result.PagesImported)
	}
	if len(result.Renamed) != 0 || len(result.Errors) != 0 {
		t.Fatalf("expected no renames or errors, got renamed=%+v errors=%+v", result.Renamed, result.Errors)
	}

	imported, err := models.GetTemplateByName("Roundtrip Template", otherTeam.Id)
	if err != nil {
		t.Fatalf("error fetching imported template: %v", err)
	}
	if imported.Id == template.Id {
		t.Fatalf("imported template reused the source template's ID (%d) instead of getting a new one", imported.Id)
	}
}

func TestContentPackImportRenamesOnNameCollision(t *testing.T) {
	tc := setupTest(t)
	pack := ContentPack{
		FormatVersion: contentPackFormatVersion,
		Templates: []models.Template{
			{Name: "Duplicate Template", HTML: "<html>Test</html>"},
		},
	}

	for i := 0; i < 2; i++ {
		req := makeContentPackRequest("/api/content-packs/import", tc.admin.TeamID, tc.admin.Id, pack)
		resp := httptest.NewRecorder()
		tc.apiServer.ContentPackImport(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("unexpected import status on iteration %d: %d, body: %s", i, resp.Code, resp.Body.String())
		}
	}

	renamed, err := models.GetTemplateByName("Duplicate Template (2)", tc.admin.TeamID)
	if err != nil {
		t.Fatalf("expected second import to be renamed to 'Duplicate Template (2)': %v", err)
	}
	if renamed.Name != "Duplicate Template (2)" {
		t.Fatalf("unexpected renamed template name: %s", renamed.Name)
	}
}

func TestContentPackImportRejectsUnsupportedVersion(t *testing.T) {
	tc := setupTest(t)
	pack := ContentPack{
		FormatVersion: "999",
		Templates:     []models.Template{{Name: "Whatever", HTML: "<html></html>"}},
	}
	req := makeContentPackRequest("/api/content-packs/import", tc.admin.TeamID, tc.admin.Id, pack)
	resp := httptest.NewRecorder()
	tc.apiServer.ContentPackImport(resp, req)
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unsupported format_version, got %d", resp.Code)
	}
}

// TestContentPackImportIgnoresClientSuppliedID guards the cross-tenant
// integrity fix in ContentPackImport: PostTemplate/PostPage save by
// primary key with no ownership check, so a pack claiming another team's
// row ID must never be allowed to overwrite it.
func TestContentPackImportIgnoresClientSuppliedID(t *testing.T) {
	tc := setupTest(t)
	original := models.Template{Name: "Team One Template", HTML: "<html>original</html>", UserId: tc.admin.Id}
	if err := models.PostTemplate(&original); err != nil {
		t.Fatalf("error creating original template: %v", err)
	}

	otherTeam, err := models.GetOrCreateTeamByName("Another Team")
	if err != nil {
		t.Fatalf("error creating other team: %v", err)
	}
	pack := ContentPack{
		FormatVersion: contentPackFormatVersion,
		Templates: []models.Template{
			{Id: original.Id, Name: "Malicious Template", HTML: "<html>overwritten</html>"},
		},
	}
	req := makeContentPackRequest("/api/content-packs/import", otherTeam.Id, tc.admin.Id, pack)
	resp := httptest.NewRecorder()
	tc.apiServer.ContentPackImport(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("unexpected import status: %d, body: %s", resp.Code, resp.Body.String())
	}

	untouched, err := models.GetTemplate(original.Id, tc.admin.TeamID)
	if err != nil {
		t.Fatalf("error fetching original template: %v", err)
	}
	if untouched.Name != "Team One Template" || untouched.HTML != "<html>original</html>" {
		t.Fatalf("original template was overwritten by import: %+v", untouched)
	}

	imported, err := models.GetTemplateByName("Malicious Template", otherTeam.Id)
	if err != nil {
		t.Fatalf("error fetching imported template: %v", err)
	}
	if imported.Id == original.Id {
		t.Fatalf("imported template kept the client-supplied ID instead of getting a new one")
	}
}

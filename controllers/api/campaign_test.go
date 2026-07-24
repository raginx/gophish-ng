package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gophish/gophish/models"
)

func getFirstCampaign(t *testing.T) models.Campaign {
	campaigns, err := models.GetCampaigns(1)
	if err != nil {
		t.Fatalf("error getting first campaign from database: %v", err)
	}
	return campaigns[0]
}

// TestCampaignCompleteRequiresPOST ensures the complete endpoint only
// accepts POST (as documented in gophish.js's api.campaignId.complete),
// and no longer accidentally accepts GET, which is unsafe for a
// state-mutating action.
func TestCampaignCompleteRequiresPOST(t *testing.T) {
	testCtx := setupTest(t)
	createTestData(t)
	campaign := getFirstCampaign(t)
	url := fmt.Sprintf("/api/campaigns/%d/complete", campaign.Id)

	// A GET request should no longer complete the campaign.
	r := httptest.NewRequest(http.MethodGet, url, nil)
	r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testCtx.apiKey))
	w := httptest.NewRecorder()
	testCtx.apiServer.ServeHTTP(w, r)

	got, err := models.GetCampaign(campaign.Id, 1)
	if err != nil {
		t.Fatalf("error getting campaign: %v", err)
	}
	if got.Status == models.CampaignComplete {
		t.Fatalf("GET request should not have completed the campaign")
	}

	// A POST request should complete the campaign.
	r = httptest.NewRequest(http.MethodPost, url, nil)
	r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testCtx.apiKey))
	w = httptest.NewRecorder()
	testCtx.apiServer.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status code received. expected %d got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	got, err = models.GetCampaign(campaign.Id, 1)
	if err != nil {
		t.Fatalf("error getting campaign: %v", err)
	}
	if got.Status != models.CampaignComplete {
		t.Fatalf("expected campaign status %q, got %q", models.CampaignComplete, got.Status)
	}
}

func TestCampaignResultReport(t *testing.T) {
	testCtx := setupTest(t)
	createTestData(t)
	campaign := getFirstCampaign(t)
	result := campaign.Results[0]
	reportedDate := time.Now().UTC().Add(-24 * time.Hour).Truncate(time.Second)

	payload, err := json.Marshal(map[string]interface{}{
		"reported_date": reportedDate,
	})
	if err != nil {
		t.Fatalf("error marshaling payload: %v", err)
	}

	url := fmt.Sprintf("/api/campaigns/%d/results/%s/report", campaign.Id, result.RId)
	r := httptest.NewRequest(http.MethodPut, url, bytes.NewBuffer(payload))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testCtx.apiKey))
	w := httptest.NewRecorder()

	testCtx.apiServer.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status code received. expected %d got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	got, err := models.GetResult(result.RId)
	if err != nil {
		t.Fatalf("error getting result: %v", err)
	}
	if !got.Reported {
		t.Fatalf("expected result to be marked as reported")
	}
	if !got.ModifiedDate.Equal(reportedDate) {
		t.Fatalf("unexpected modified date received. expected %s got %s", reportedDate, got.ModifiedDate)
	}
}

func TestCampaignResultReportDefaultsToNow(t *testing.T) {
	testCtx := setupTest(t)
	createTestData(t)
	campaign := getFirstCampaign(t)
	result := campaign.Results[0]
	before := time.Now().UTC()

	url := fmt.Sprintf("/api/campaigns/%d/results/%s/report", campaign.Id, result.RId)
	r := httptest.NewRequest(http.MethodPut, url, bytes.NewBuffer([]byte{}))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testCtx.apiKey))
	w := httptest.NewRecorder()

	testCtx.apiServer.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status code received. expected %d got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	got, err := models.GetResult(result.RId)
	if err != nil {
		t.Fatalf("error getting result: %v", err)
	}
	if !got.Reported {
		t.Fatalf("expected result to be marked as reported")
	}
	if got.ModifiedDate.Before(before) {
		t.Fatalf("expected modified date to default to now, got %s (before %s)", got.ModifiedDate, before)
	}
}

func TestCampaignResultReportFutureDateRejected(t *testing.T) {
	testCtx := setupTest(t)
	createTestData(t)
	campaign := getFirstCampaign(t)
	result := campaign.Results[0]

	payload, err := json.Marshal(map[string]interface{}{
		"reported_date": time.Now().UTC().Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("error marshaling payload: %v", err)
	}

	url := fmt.Sprintf("/api/campaigns/%d/results/%s/report", campaign.Id, result.RId)
	r := httptest.NewRequest(http.MethodPut, url, bytes.NewBuffer(payload))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testCtx.apiKey))
	w := httptest.NewRecorder()

	testCtx.apiServer.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status code received. expected %d got %d: %s", http.StatusBadRequest, w.Code, w.Body.String())
	}

	got, err := models.GetResult(result.RId)
	if err != nil {
		t.Fatalf("error getting result: %v", err)
	}
	if got.Reported {
		t.Fatalf("result should not have been marked as reported")
	}
}

// TestCampaignResultReportWrongOwner ensures that a user can't mark a
// result as reported on a campaign they don't own.
func TestCampaignResultReportWrongOwner(t *testing.T) {
	testCtx := setupTest(t)
	createTestData(t)
	campaign := getFirstCampaign(t)
	result := campaign.Results[0]
	unauthorizedUser := createUnpriviledgedUser(t, models.RoleUser)

	url := fmt.Sprintf("/api/campaigns/%d/results/%s/report", campaign.Id, result.RId)
	r := httptest.NewRequest(http.MethodPut, url, bytes.NewBuffer([]byte{}))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", unauthorizedUser.ApiKey))
	w := httptest.NewRecorder()

	testCtx.apiServer.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unexpected status code received. expected %d got %d: %s", http.StatusNotFound, w.Code, w.Body.String())
	}

	got, err := models.GetResult(result.RId)
	if err != nil {
		t.Fatalf("error getting result: %v", err)
	}
	if got.Reported {
		t.Fatalf("result should not have been marked as reported")
	}
}

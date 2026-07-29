package api

import (
	"bytes"
	"encoding/json"
	"errors"
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

// markResultErrored is a test helper that puts a result into the Error
// status via the same path a real send failure would, so resend tests can
// start from there without actually running a campaign through a failure.
func markResultErrored(t *testing.T, rid string) models.Result {
	r, err := models.GetResult(rid)
	if err != nil {
		t.Fatalf("error getting result: %v", err)
	}
	if err := r.HandleEmailError(errors.New("simulated send failure")); err != nil {
		t.Fatalf("error marking result as errored: %v", err)
	}
	r, err = models.GetResult(rid)
	if err != nil {
		t.Fatalf("error getting result: %v", err)
	}
	return r
}

// TestCampaignResultResend covers resending a single failed result
func TestCampaignResultResend(t *testing.T) {
	testCtx := setupTest(t)
	createTestData(t)
	campaign := getFirstCampaign(t)
	result := markResultErrored(t, campaign.Results[0].RId)

	url := fmt.Sprintf("/api/campaigns/%d/results/%s/resend", campaign.Id, result.RId)
	r := httptest.NewRequest(http.MethodPut, url, nil)
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
	if got.Status != models.StatusScheduled {
		t.Fatalf("expected result status %q, got %q", models.StatusScheduled, got.Status)
	}

	m, err := models.GetMailLogByRId(result.RId)
	if err != nil {
		t.Fatalf("error getting mail log: %v", err)
	}
	if m.SendAttempt != 0 || m.Processing {
		t.Fatalf("expected a fresh, unlocked mail log, got SendAttempt=%d Processing=%v", m.SendAttempt, m.Processing)
	}
}

// TestCampaignResultResendNotEligible ensures results that aren't in an
// Error status can't be resent.
func TestCampaignResultResendNotEligible(t *testing.T) {
	testCtx := setupTest(t)
	createTestData(t)
	campaign := getFirstCampaign(t)
	result := campaign.Results[0]

	url := fmt.Sprintf("/api/campaigns/%d/results/%s/resend", campaign.Id, result.RId)
	r := httptest.NewRequest(http.MethodPut, url, nil)
	r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testCtx.apiKey))
	w := httptest.NewRecorder()

	testCtx.apiServer.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status code received. expected %d got %d: %s", http.StatusBadRequest, w.Code, w.Body.String())
	}
}

// TestCampaignResultResendWrongOwner ensures that a user can't resend a
// result on a campaign they don't own.
func TestCampaignResultResendWrongOwner(t *testing.T) {
	testCtx := setupTest(t)
	createTestData(t)
	campaign := getFirstCampaign(t)
	result := markResultErrored(t, campaign.Results[0].RId)
	unauthorizedUser := createUnpriviledgedUser(t, models.RoleUser)

	url := fmt.Sprintf("/api/campaigns/%d/results/%s/resend", campaign.Id, result.RId)
	r := httptest.NewRequest(http.MethodPut, url, nil)
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
	if got.Status != models.Error {
		t.Fatalf("result should not have been resent")
	}
}

// TestCampaignResendFailed covers resending every failed result in a
// campaign at once
func TestCampaignResendFailed(t *testing.T) {
	testCtx := setupTest(t)
	createTestData(t)
	campaign := getFirstCampaign(t)
	if len(campaign.Results) < 2 {
		t.Fatalf("expected at least 2 results in the test campaign, got %d", len(campaign.Results))
	}
	failed := markResultErrored(t, campaign.Results[0].RId)
	untouched := campaign.Results[1]

	url := fmt.Sprintf("/api/campaigns/%d/resend", campaign.Id)
	r := httptest.NewRequest(http.MethodPut, url, nil)
	r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", testCtx.apiKey))
	w := httptest.NewRecorder()

	testCtx.apiServer.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status code received. expected %d got %d: %s", http.StatusOK, w.Code, w.Body.String())
	}

	got, err := models.GetResult(failed.RId)
	if err != nil {
		t.Fatalf("error getting result: %v", err)
	}
	if got.Status != models.StatusScheduled {
		t.Fatalf("expected the failed result to be rescheduled, got status %q", got.Status)
	}

	gotUntouched, err := models.GetResult(untouched.RId)
	if err != nil {
		t.Fatalf("error getting result: %v", err)
	}
	if gotUntouched.Status != untouched.Status {
		t.Fatalf("expected non-failed result to be untouched: got status %q, want %q", gotUntouched.Status, untouched.Status)
	}
}

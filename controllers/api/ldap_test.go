package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/raginx/gophish-ng/ldap"
	"github.com/raginx/gophish-ng/models"
)

// enableSecretKey sets a valid secret_key on the package-level config so the
// LDAP bind password can be encrypted at rest during tests.
func enableSecretKey(ctx *testContext) {
	ctx.config.SecretKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
}

func doRequest(t *testing.T, ctx *testContext, method, path, apiKey string, payload interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var body *bytes.Buffer
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		body = bytes.NewBuffer(b)
	} else {
		body = bytes.NewBuffer(nil)
	}
	r := httptest.NewRequest(method, path, body)
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	w := httptest.NewRecorder()
	ctx.apiServer.ServeHTTP(w, r)
	return w
}

func TestLDAPConnectionCreateAndSecretOmitted(t *testing.T) {
	ctx := setupTest(t)
	enableSecretKey(ctx)

	payload := map[string]interface{}{
		"name":          "Corp AD",
		"server_url":    "ldaps://ad.example.com:636",
		"bind_dn":       "cn=svc,dc=example,dc=com",
		"bind_password": "s3cret",
		"base_dn":       "dc=example,dc=com",
	}
	w := doRequest(t, ctx, http.MethodPost, "/api/ldap/connections/", ctx.apiKey, payload)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	// The response must not leak the password or ciphertext.
	var created map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if v, ok := created["bind_password"]; ok && v != "" {
		t.Fatalf("bind_password leaked in response: %v", v)
	}
	if _, ok := created["bind_password_enc"]; ok {
		t.Fatal("bind_password_enc must never be serialized")
	}

	// GET list must also omit secrets.
	w = doRequest(t, ctx, http.MethodGet, "/api/ldap/connections/", ctx.apiKey, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if bytes.Contains(w.Body.Bytes(), []byte("s3cret")) {
		t.Fatal("password leaked in list response")
	}
}

func TestLDAPConnectionRequiresSecretKey(t *testing.T) {
	ctx := setupTest(t)
	// Intentionally do NOT enable secret_key.
	payload := map[string]interface{}{
		"name":          "Corp AD",
		"server_url":    "ldaps://ad.example.com:636",
		"bind_dn":       "cn=svc,dc=example,dc=com",
		"bind_password": "s3cret",
		"base_dn":       "dc=example,dc=com",
	}
	w := doRequest(t, ctx, http.MethodPost, "/api/ldap/connections/", ctx.apiKey, payload)
	if w.Code == http.StatusCreated {
		t.Fatal("expected failure when secret_key is not configured")
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("requires secret_key")) {
		t.Fatalf("expected a 'secret_key not configured' message, got: %s", w.Body.String())
	}
}

func TestLDAPConnectionInvalidSecretKey(t *testing.T) {
	ctx := setupTest(t)
	// Configured but not a base64-encoded 32-byte key.
	ctx.config.SecretKey = "not-a-valid-key"
	payload := map[string]interface{}{
		"name":          "Corp AD",
		"server_url":    "ldaps://ad.example.com:636",
		"bind_dn":       "cn=svc,dc=example,dc=com",
		"bind_password": "s3cret",
		"base_dn":       "dc=example,dc=com",
	}
	w := doRequest(t, ctx, http.MethodPost, "/api/ldap/connections/", ctx.apiKey, payload)
	if w.Code == http.StatusCreated {
		t.Fatal("expected failure when secret_key is invalid")
	}
	body := w.Body.String()
	// The error must be the distinct invalid-key message, stating the format,
	// and must never echo the configured value.
	if !bytes.Contains(w.Body.Bytes(), []byte("base64-encoded 32-byte key")) {
		t.Fatalf("expected an invalid-key format message, got: %s", body)
	}
	if bytes.Contains(w.Body.Bytes(), []byte("not-a-valid-key")) {
		t.Fatalf("configured secret_key value leaked in response: %s", body)
	}
}

func TestLDAPConnectionAuditorForbidden(t *testing.T) {
	ctx := setupTest(t)
	enableSecretKey(ctx)

	// Create a read-only auditor and verify they cannot create a connection.
	role, err := models.GetRoleBySlug(models.RoleAuditor)
	if err != nil {
		t.Fatalf("get auditor role: %v", err)
	}
	auditor := models.User{
		Username: "ldap-auditor",
		Hash:     "hash",
		ApiKey:   "ldap-auditor-key",
		Role:     role,
		RoleID:   role.ID,
		TeamID:   ctx.admin.TeamID,
	}
	if err := models.PutUser(&auditor); err != nil {
		t.Fatalf("create auditor: %v", err)
	}

	payload := map[string]interface{}{
		"name": "Corp AD", "server_url": "ldaps://ad.example.com:636",
		"bind_dn": "cn=svc", "bind_password": "s3cret", "base_dn": "dc=example,dc=com",
	}
	w := doRequest(t, ctx, http.MethodPost, "/api/ldap/connections/", auditor.ApiKey, payload)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for auditor, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLDAPGroupSyncCreateReferencesConnectionAndGroup(t *testing.T) {
	ctx := setupTest(t)
	enableSecretKey(ctx)
	stubGroupResolver(t)

	connID := createTestLDAPConnection(t, ctx)

	// Create a group to be managed.
	group := models.Group{Name: "LDAP Managed", UserId: 1}
	group.Targets = []models.Target{{BaseRecipient: models.BaseRecipient{Email: "seed@example.com"}}}
	if err := models.PostGroup(&group); err != nil {
		t.Fatalf("post group: %v", err)
	}

	// Create the mapping: the client identifies the group only by GUID (any
	// textual form); DN/name come from server-side resolution, and spoofed
	// client values are discarded.
	payload := adSyncPayload(connID)
	payload["group_id"] = group.Id
	payload["group_directory_id"] = "{A1B2C3D4-E5F6-4788-99AA-BBCCDDEEFF00}"
	payload["group_dn"] = "CN=Spoof,DC=other,DC=com"
	payload["group_name"] = "Spoof"
	w := doRequest(t, ctx, http.MethodPost, "/api/ldap/syncs/", ctx.apiKey, payload)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var created map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if created["group_directory_id"] != apiTestGroupGUID || created["group_dn"] != apiTestGroupDN || created["group_name"] != apiTestGroupName {
		t.Fatalf("selected group not persisted/normalized: %s", w.Body.String())
	}
	for _, legacy := range []string{"search_base", "search_filter", "search_scope"} {
		if _, ok := created[legacy]; ok {
			t.Fatalf("legacy field %q must not be serialized: %s", legacy, w.Body.String())
		}
	}

	// Mapping referencing a non-existent connection must be rejected.
	bad := adSyncPayload(999999)
	bad["group_id"] = group.Id
	w = doRequest(t, ctx, http.MethodPost, "/api/ldap/syncs/", ctx.apiKey, bad)
	if w.Code == http.StatusCreated {
		t.Fatal("expected failure for mapping with unknown connection")
	}

	// Mapping without a selected AD group must be rejected clearly.
	w = doRequest(t, ctx, http.MethodPost, "/api/ldap/syncs/", ctx.apiKey, map[string]interface{}{
		"group_id": group.Id, "connection_id": connID, "enabled": true,
	})
	if w.Code != http.StatusBadRequest || !bytes.Contains(w.Body.Bytes(), []byte("no Active Directory group selected")) {
		t.Fatalf("expected 400 'no Active Directory group selected', got %d: %s", w.Code, w.Body.String())
	}

	// A GUID that does not resolve on this connection is rejected (400) and
	// nothing is created.
	unknown := adSyncPayload(connID)
	unknown["group_id"] = group.Id
	unknown["group_directory_id"] = "09090909-0909-0909-0909-090909090909"
	w = doRequest(t, ctx, http.MethodPost, "/api/ldap/syncs/", ctx.apiKey, unknown)
	if w.Code != http.StatusBadRequest || !bytes.Contains(w.Body.Bytes(), []byte("was not found")) {
		t.Fatalf("expected 400 not-found error, got %d: %s", w.Code, w.Body.String())
	}

	// Mapping on a Generic LDAP connection must be rejected clearly.
	genericID := createTestGenericLDAPConnection(t, ctx)
	gen := adSyncPayload(genericID)
	gen["group_id"] = group.Id
	w = doRequest(t, ctx, http.MethodPost, "/api/ldap/syncs/", ctx.apiKey, gen)
	if w.Code != http.StatusBadRequest || !bytes.Contains(w.Body.Bytes(), []byte("require an Active Directory connection")) {
		t.Fatalf("expected 400 AD-only error, got %d: %s", w.Code, w.Body.String())
	}
}

// TestLDAPGroupSyncCreateBackendFailureSanitized verifies a directory failure
// during resolution yields a sanitized 500 and creates nothing - using the
// REAL resolver against an unroutable AD server.
func TestLDAPGroupSyncCreateBackendFailureSanitized(t *testing.T) {
	ctx := setupTest(t)
	enableSecretKey(ctx)
	connID := createTestLDAPConnectionWith(t, ctx, map[string]interface{}{
		"name": "Dead AD", "server_url": "ldap://127.0.0.1:1",
		"bind_dn": "cn=svc,dc=example,dc=com", "bind_password": "s3cret", "base_dn": "dc=example,dc=com",
		"directory_id_attr": "objectGUID",
	})
	group := models.Group{Name: "Backend Fail", UserId: 1}
	group.Targets = []models.Target{{BaseRecipient: models.BaseRecipient{Email: "seed@example.com"}}}
	if err := models.PostGroup(&group); err != nil {
		t.Fatalf("post group: %v", err)
	}
	payload := adSyncPayload(connID)
	payload["group_id"] = group.Id
	w := doRequest(t, ctx, http.MethodPost, "/api/ldap/syncs/", ctx.apiKey, payload)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for unreachable directory, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, leak := range []string{"s3cret", "127.0.0.1:1", "cn=svc", "bind_password"} {
		if bytes.Contains([]byte(body), []byte(leak)) {
			t.Fatalf("resolution error leaked %q: %s", leak, body)
		}
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("Unable to resolve the selected Active Directory group")) {
		t.Fatalf("expected sanitized message, got: %s", body)
	}
	if _, err := models.GetLDAPGroupSyncByGroup(group.Id, ctx.admin.TeamID); err == nil {
		t.Fatal("no mapping may be created when resolution fails")
	}
}

// TestLDAPGroupSyncPutImmutableFields verifies the v1 update contract over the
// API: connection and selected group are fixed, DN/name are server-owned,
// settings are editable.
func TestLDAPGroupSyncPutImmutableFields(t *testing.T) {
	ctx := setupTest(t)
	enableSecretKey(ctx)
	stubGroupResolver(t)
	connID := createTestLDAPConnection(t, ctx)
	otherConnID := createTestLDAPConnectionWith(t, ctx, map[string]interface{}{
		"name": "Other AD", "server_url": "ldaps://ad2.example.com:636",
		"bind_dn": "cn=svc", "bind_password": "s3cret", "base_dn": "dc=example,dc=com",
		"directory_id_attr": "objectGUID",
	})
	create := doRequest(t, ctx, http.MethodPost, "/api/groups/", ctx.apiKey, map[string]interface{}{
		"name": "Immutable Managed", "ldap_sync": adSyncPayload(connID),
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("create managed group: %d %s", create.Code, create.Body.String())
	}
	var g map[string]interface{}
	_ = json.Unmarshal(create.Body.Bytes(), &g)
	gid := int64(g["id"].(float64))
	m, err := models.GetLDAPGroupSyncByGroup(gid, ctx.admin.TeamID)
	if err != nil {
		t.Fatalf("mapping: %v", err)
	}
	base := map[string]interface{}{"id": m.Id, "group_id": gid, "connection_id": connID, "group_directory_id": apiTestGroupGUID, "enabled": true}

	// Changing the connection → 400.
	p := map[string]interface{}{}
	for k, v := range base {
		p[k] = v
	}
	p["connection_id"] = otherConnID
	w := doRequest(t, ctx, http.MethodPut, fmt.Sprintf("/api/ldap/syncs/%d", m.Id), ctx.apiKey, p)
	if w.Code != http.StatusBadRequest || !bytes.Contains(w.Body.Bytes(), []byte("connection of an existing synchronization mapping cannot be changed")) {
		t.Fatalf("expected 400 immutable connection, got %d: %s", w.Code, w.Body.String())
	}
	// Changing the selected group → 400.
	p = map[string]interface{}{}
	for k, v := range base {
		p[k] = v
	}
	p["group_directory_id"] = "09090909-0909-0909-0909-090909090909"
	w = doRequest(t, ctx, http.MethodPut, fmt.Sprintf("/api/ldap/syncs/%d", m.Id), ctx.apiKey, p)
	if w.Code != http.StatusBadRequest || !bytes.Contains(w.Body.Bytes(), []byte("selected Active Directory group of an existing synchronization mapping cannot be changed")) {
		t.Fatalf("expected 400 immutable group, got %d: %s", w.Code, w.Body.String())
	}
	// Editable settings apply; spoofed DN/name are discarded.
	p = map[string]interface{}{}
	for k, v := range base {
		p[k] = v
	}
	p["group_dn"] = "CN=Spoof,DC=other,DC=com"
	p["group_name"] = "Spoof"
	p["attr_email"] = "userPrincipalName"
	p["exclude_disabled"] = false
	p["sync_freq"] = 7200
	w = doRequest(t, ctx, http.MethodPut, fmt.Sprintf("/api/ldap/syncs/%d", m.Id), ctx.apiKey, p)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 updating settings, got %d: %s", w.Code, w.Body.String())
	}
	after, _ := models.GetLDAPGroupSync(m.Id, ctx.admin.TeamID)
	if after.GroupDN != apiTestGroupDN || after.GroupName != apiTestGroupName || after.ConnectionId != connID || after.GroupDirectoryId != apiTestGroupGUID {
		t.Fatalf("immutable/server-owned fields changed: %+v", after)
	}
	if after.AttrEmail != "userPrincipalName" || after.ExcludeDisabled || after.SyncFreq != 7200 {
		t.Fatalf("editable settings not applied: %+v", after)
	}
	// The Gophish group name was not touched by the mapping update.
	grp, _ := models.GetGroup(gid, ctx.admin.TeamID)
	if grp.Name != "Immutable Managed" {
		t.Fatalf("group renamed by mapping update: %q", grp.Name)
	}
}

// TestLDAPGroupSyncExcludeDisabledDefault pins omitted → true, explicit true
// → true, explicit false → false, on both create paths.
func TestLDAPGroupSyncExcludeDisabledDefault(t *testing.T) {
	ctx := setupTest(t)
	enableSecretKey(ctx)
	stubGroupResolver(t)
	connID := createTestLDAPConnection(t, ctx)

	cases := []struct {
		name string
		set  interface{} // nil = omitted
		want bool
	}{{"omitted", nil, true}, {"true", true, true}, {"false", false, false}}
	for _, tc := range cases {
		payload := adSyncPayload(connID)
		delete(payload, "enabled")
		if tc.set != nil {
			payload["exclude_disabled"] = tc.set
		}
		w := doRequest(t, ctx, http.MethodPost, "/api/groups/", ctx.apiKey, map[string]interface{}{
			"name": "Excl " + tc.name, "ldap_sync": payload,
		})
		if w.Code != http.StatusCreated {
			t.Fatalf("%s: create: %d %s", tc.name, w.Code, w.Body.String())
		}
		var g map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &g)
		m, err := models.GetLDAPGroupSyncByGroup(int64(g["id"].(float64)), ctx.admin.TeamID)
		if err != nil {
			t.Fatalf("%s: mapping: %v", tc.name, err)
		}
		if m.ExcludeDisabled != tc.want {
			t.Fatalf("%s: exclude_disabled = %v, want %v", tc.name, m.ExcludeDisabled, tc.want)
		}
	}

	// Standalone mapping endpoint, omitted → true.
	group := models.Group{Name: "Excl Standalone", UserId: 1}
	group.Targets = []models.Target{{BaseRecipient: models.BaseRecipient{Email: "seed@example.com"}}}
	if err := models.PostGroup(&group); err != nil {
		t.Fatalf("post group: %v", err)
	}
	payload := adSyncPayload(connID)
	payload["group_id"] = group.Id
	w := doRequest(t, ctx, http.MethodPost, "/api/ldap/syncs/", ctx.apiKey, payload)
	if w.Code != http.StatusCreated {
		t.Fatalf("standalone create: %d %s", w.Code, w.Body.String())
	}
	m, _ := models.GetLDAPGroupSyncByGroup(group.Id, ctx.admin.TeamID)
	if !m.ExcludeDisabled {
		t.Fatal("standalone mapping: omitted exclude_disabled must default to true")
	}
}

// TestGroupPostLDAPManagedTrimsName verifies the managed-group name is trimmed
// before the uniqueness check and persistence.
func TestGroupPostLDAPManagedTrimsName(t *testing.T) {
	ctx := setupTest(t)
	enableSecretKey(ctx)
	stubGroupResolver(t)
	connID := createTestLDAPConnection(t, ctx)
	w := doRequest(t, ctx, http.MethodPost, "/api/groups/", ctx.apiKey, map[string]interface{}{
		"name": "   Marketing   ", "ldap_sync": adSyncPayload(connID),
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	if _, err := models.GetGroupByName("Marketing", ctx.admin.TeamID); err != nil {
		t.Fatalf("expected trimmed name to be persisted: %v", err)
	}
	// A second managed group with the same (untrimmed) name collides.
	w = doRequest(t, ctx, http.MethodPost, "/api/groups/", ctx.apiKey, map[string]interface{}{
		"name": " Marketing ", "ldap_sync": adSyncPayload(connID),
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for duplicate trimmed name, got %d: %s", w.Code, w.Body.String())
	}
}

// TestGroupDeleteRemovesMapping verifies DELETE /api/groups/:id on a managed
// group removes its mapping so the scheduler no longer sees it.
func TestGroupDeleteRemovesMapping(t *testing.T) {
	ctx := setupTest(t)
	enableSecretKey(ctx)
	stubGroupResolver(t)
	connID := createTestLDAPConnection(t, ctx)
	create := doRequest(t, ctx, http.MethodPost, "/api/groups/", ctx.apiKey, map[string]interface{}{
		"name": "To Delete", "ldap_sync": adSyncPayload(connID),
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", create.Code, create.Body.String())
	}
	var g map[string]interface{}
	_ = json.Unmarshal(create.Body.Bytes(), &g)
	gid := int64(g["id"].(float64))
	w := doRequest(t, ctx, http.MethodDelete, fmt.Sprintf("/api/groups/%d", gid), ctx.apiKey, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", w.Code, w.Body.String())
	}
	if _, err := models.GetLDAPGroupSyncByGroup(gid, ctx.admin.TeamID); err == nil {
		t.Fatal("mapping must be removed with its group")
	}
	enabled, _ := models.GetAllEnabledLDAPGroupSyncs()
	for _, m := range enabled {
		if m.GroupId == gid {
			t.Fatal("scheduler discovery still sees the deleted group's mapping")
		}
	}
	// The connection is now deletable (no references).
	w = doRequest(t, ctx, http.MethodDelete, fmt.Sprintf("/api/ldap/connections/%d", connID), ctx.apiKey, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("connection delete after group delete: %d %s", w.Code, w.Body.String())
	}
}

// TestLDAPManagedCreateFinalPayload pins the UI's final create contract: the
// group is sent with an empty targets list and an ldap_sync that identifies
// the AD group only by connection_id + group_directory_id. Legacy OU keys are
// ignored (not rejected, not stored), the response carries the
// server-resolved group metadata, and the mapping exposes last_sync_skipped.
func TestLDAPManagedCreateFinalPayload(t *testing.T) {
	ctx := setupTest(t)
	enableSecretKey(ctx)
	stubGroupResolver(t)
	connID := createTestLDAPConnection(t, ctx)

	w := doRequest(t, ctx, http.MethodPost, "/api/groups/", ctx.apiKey, map[string]interface{}{
		"name":    "Marketing",
		"targets": []interface{}{},
		"ldap_sync": map[string]interface{}{
			"connection_id":      connID,
			"group_directory_id": apiTestGroupGUID,
			"exclude_disabled":   true,
			"enabled":            true,
			"sync_freq":          3600,
			"attr_email":         "userPrincipalName",
			// Legacy OU-era keys that a stale client might still send:
			"search_base":   "OU=Old,DC=example,DC=com",
			"search_filter": "(objectClass=person)",
			"search_scope":  "sub",
		},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var g map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &g)
	gid := int64(g["id"].(float64))

	// The mapping as the edit UI loads it (GET /ldap/syncs/?group_id=).
	get := doRequest(t, ctx, http.MethodGet, fmt.Sprintf("/api/ldap/syncs/?group_id=%d", gid), ctx.apiKey, nil)
	if get.Code != http.StatusOK {
		t.Fatalf("get mapping: %d %s", get.Code, get.Body.String())
	}
	var syncs []map[string]interface{}
	_ = json.Unmarshal(get.Body.Bytes(), &syncs)
	if len(syncs) != 1 {
		t.Fatalf("expected one mapping, got %d", len(syncs))
	}
	m := syncs[0]
	if m["group_directory_id"] != apiTestGroupGUID || m["group_dn"] != apiTestGroupDN || m["group_name"] != apiTestGroupName {
		t.Fatalf("server-observed group metadata missing: %v", m)
	}
	// (last_sync_status/last_sync_error are omitempty: absent until a run.)
	for _, key := range []string{"last_sync_skipped", "last_sync_added", "last_sync_updated", "last_sync_removed", "attr_email", "exclude_disabled", "enabled", "sync_freq"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("mapping JSON missing %q: %v", key, m)
		}
	}
	for _, legacy := range []string{"search_base", "search_filter", "search_scope"} {
		if _, ok := m[legacy]; ok {
			t.Fatalf("legacy key %q must not be exposed: %v", legacy, m)
		}
	}
	if m["attr_email"] != "userPrincipalName" || m["exclude_disabled"] != true {
		t.Fatalf("settings not persisted: %v", m)
	}

	// Sync Now against the (unreachable) test directory fails safely and
	// reports the failure in the standard envelope; the group stays empty.
	run := doRequest(t, ctx, http.MethodPost, fmt.Sprintf("/api/ldap/syncs/%v/sync", m["id"]), ctx.apiKey, nil)
	if run.Code != http.StatusOK {
		t.Fatalf("sync now: %d %s", run.Code, run.Body.String())
	}
	var res map[string]interface{}
	_ = json.Unmarshal(run.Body.Bytes(), &res)
	if res["success"] != false {
		t.Fatalf("expected failed sync against unreachable directory, got %v", res)
	}
	if ts, _ := models.GetTargets(gid); len(ts) != 0 {
		t.Fatalf("failed sync must not change membership, got %d members", len(ts))
	}
	after := doRequest(t, ctx, http.MethodGet, fmt.Sprintf("/api/ldap/syncs/%v", m["id"]), ctx.apiKey, nil)
	var reloaded map[string]interface{}
	_ = json.Unmarshal(after.Body.Bytes(), &reloaded)
	if reloaded["last_sync_status"] != "error" || reloaded["group_name"] != apiTestGroupName {
		t.Fatalf("failed sync must record an error and keep the last-known group name: %v", reloaded)
	}
}

// TestLDAPBrowseRouteRemoved verifies the OU/container browser endpoint is
// gone while the group search endpoint remains routed.
func TestLDAPBrowseRouteRemoved(t *testing.T) {
	ctx := setupTest(t)
	enableSecretKey(ctx)
	connID := createTestLDAPConnection(t, ctx)
	w := doRequest(t, ctx, http.MethodGet, fmt.Sprintf("/api/ldap/connections/%d/browse", connID), ctx.apiKey, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for removed browse route, got %d: %s", w.Code, w.Body.String())
	}
	w = doRequest(t, ctx, http.MethodGet, fmt.Sprintf("/api/ldap/connections/%d/groups?q=%s", connID, strings.Repeat("a", 65)), ctx.apiKey, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("group search route must remain functional (expected 400 for over-long query), got %d: %s", w.Code, w.Body.String())
	}
}

func TestLDAPConnectionDeleteBlockedWhenReferenced(t *testing.T) {
	ctx := setupTest(t)
	enableSecretKey(ctx)
	stubGroupResolver(t)

	connID := createTestLDAPConnection(t, ctx)

	group := models.Group{Name: "LDAP Managed", UserId: 1}
	group.Targets = []models.Target{{BaseRecipient: models.BaseRecipient{Email: "seed@example.com"}}}
	if err := models.PostGroup(&group); err != nil {
		t.Fatalf("post group: %v", err)
	}
	syncPayload := adSyncPayload(connID)
	syncPayload["group_id"] = group.Id
	syncResp := doRequest(t, ctx, http.MethodPost, "/api/ldap/syncs/", ctx.apiKey, syncPayload)
	if syncResp.Code != http.StatusCreated {
		t.Fatalf("create mapping: %d %s", syncResp.Code, syncResp.Body.String())
	}

	// Delete must be blocked with 409 and a clear message, and the
	// connection must still exist afterwards.
	w := doRequest(t, ctx, http.MethodDelete, fmt.Sprintf("/api/ldap/connections/%d", connID), ctx.apiKey, nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 Conflict, got %d: %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("group synchronization")) {
		t.Fatalf("expected a clear in-use message, got: %s", w.Body.String())
	}
	get := doRequest(t, ctx, http.MethodGet, fmt.Sprintf("/api/ldap/connections/%d", connID), ctx.apiKey, nil)
	if get.Code != http.StatusOK {
		t.Fatalf("connection should still exist, got %d", get.Code)
	}
}

// helper: create a connection and return its id
// createTestLDAPConnection creates an Active Directory connection (stable
// identity objectGUID), which group-based managed synchronization requires.
func createTestLDAPConnection(t *testing.T, ctx *testContext) int64 {
	t.Helper()
	return createTestLDAPConnectionWith(t, ctx, map[string]interface{}{
		"name": "Corp AD", "server_url": "ldaps://ad.example.com:636",
		"bind_dn": "cn=svc", "bind_password": "s3cret", "base_dn": "dc=example,dc=com",
		"directory_id_attr": "objectGUID",
	})
}

// createTestGenericLDAPConnection creates a Generic LDAP connection (no
// objectGUID stable identity).
func createTestGenericLDAPConnection(t *testing.T, ctx *testContext) int64 {
	t.Helper()
	return createTestLDAPConnectionWith(t, ctx, map[string]interface{}{
		"name": "Generic LDAP", "server_url": "ldaps://ldap.example.com:636",
		"bind_dn": "cn=svc", "bind_password": "s3cret", "base_dn": "dc=example,dc=com",
	})
}

func createTestLDAPConnectionWith(t *testing.T, ctx *testContext, payload map[string]interface{}) int64 {
	t.Helper()
	resp := doRequest(t, ctx, http.MethodPost, "/api/ldap/connections/", ctx.apiKey, payload)
	if resp.Code != http.StatusCreated {
		t.Fatalf("create connection: %d %s", resp.Code, resp.Body.String())
	}
	var conn map[string]interface{}
	_ = json.Unmarshal(resp.Body.Bytes(), &conn)
	return int64(conn["id"].(float64))
}

// Selected test group (under the test connection BaseDN dc=example,dc=com).
const (
	apiTestGroupGUID = "a1b2c3d4-e5f6-4788-99aa-bbccddeeff00"
	apiTestGroupDN   = "CN=Marketing,OU=Groups,DC=example,DC=com"
	apiTestGroupName = "Marketing"
)

// adSyncPayload is a valid ldap_sync body selecting the test AD group. Only
// connection_id and group_directory_id identify the group; group_dn/group_name
// are server-resolved and need not be sent.
func adSyncPayload(connID int64) map[string]interface{} {
	return map[string]interface{}{
		"connection_id":      connID,
		"enabled":            true,
		"group_directory_id": apiTestGroupGUID,
	}
}

// resolverCalls counts how often the (stubbed) directory resolver ran.
type resolverCalls struct{ n int }

// stubGroupResolver replaces the directory resolver with an in-memory one
// that knows exactly the test group (by GUID) and reports every other GUID as
// not found, so mapping-creation tests need no live directory.
func stubGroupResolver(t *testing.T) *resolverCalls {
	t.Helper()
	calls := &resolverCalls{}
	old := resolveDirectoryGroup
	resolveDirectoryGroup = func(conn *models.LDAPConnection, id string) (ldap.DirectoryGroup, error) {
		calls.n++
		if !conn.IsActiveDirectory() {
			return ldap.DirectoryGroup{}, ldap.ErrNotActiveDirectory
		}
		norm, err := models.NormalizeDirectoryIDText("objectGUID", id)
		if err != nil {
			return ldap.DirectoryGroup{}, ldap.ErrGroupIdentityInvalid
		}
		if norm == apiTestGroupGUID {
			return ldap.DirectoryGroup{Name: apiTestGroupName, DN: apiTestGroupDN, DirectoryId: norm}, nil
		}
		return ldap.DirectoryGroup{}, ldap.ErrGroupNotFound
	}
	t.Cleanup(func() { resolveDirectoryGroup = old })
	return calls
}

// stubGroupResolverError makes every resolution fail with err.
func stubGroupResolverError(t *testing.T, err error) {
	t.Helper()
	old := resolveDirectoryGroup
	resolveDirectoryGroup = func(*models.LDAPConnection, string) (ldap.DirectoryGroup, error) { return ldap.DirectoryGroup{}, err }
	t.Cleanup(func() { resolveDirectoryGroup = old })
}

// TestGroupPostLDAPManagedCreatesEmptyGroup verifies a single POST /api/groups
// with an ldap_sync object creates an LDAP-managed group with no members.
func TestGroupPostLDAPManagedCreatesEmptyGroup(t *testing.T) {
	ctx := setupTest(t)
	enableSecretKey(ctx)
	stubGroupResolver(t)
	connID := createTestLDAPConnection(t, ctx)

	w := doRequest(t, ctx, http.MethodPost, "/api/groups/", ctx.apiKey, map[string]interface{}{
		"name":      "AD Employees",
		"ldap_sync": adSyncPayload(connID),
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating LDAP-managed group, got %d: %s", w.Code, w.Body.String())
	}
	var g map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &g)
	gid := int64(g["id"].(float64))

	managed, err := models.IsGroupLDAPManaged(gid, ctx.admin.TeamID)
	if err != nil || !managed {
		t.Fatalf("expected group %d to be LDAP-managed (managed=%v err=%v)", gid, managed, err)
	}
	ts, _ := models.GetTargets(gid)
	if len(ts) != 0 {
		t.Fatalf("expected 0 members on a new LDAP-managed group, got %d", len(ts))
	}
	// The mapping carries the server-resolved group metadata.
	m, err := models.GetLDAPGroupSyncByGroup(gid, ctx.admin.TeamID)
	if err != nil || m.GroupDN != apiTestGroupDN || m.GroupName != apiTestGroupName || m.GroupDirectoryId != apiTestGroupGUID {
		t.Fatalf("server-resolved group metadata missing on mapping: %+v (%v)", m, err)
	}
}

// TestGroupPostLDAPManagedRollsBackOnBadMapping verifies no orphan group is
// left when the mapping is invalid.
func TestGroupPostLDAPManagedRollsBackOnBadMapping(t *testing.T) {
	ctx := setupTest(t)
	enableSecretKey(ctx)
	stubGroupResolver(t)

	w := doRequest(t, ctx, http.MethodPost, "/api/groups/", ctx.apiKey, map[string]interface{}{
		"name":      "Orphan Candidate",
		"ldap_sync": adSyncPayload(999999), // non-existent connection
	})
	if w.Code == http.StatusCreated {
		t.Fatal("expected failure for LDAP-managed group with unknown connection")
	}
	if _, err := models.GetGroupByName("Orphan Candidate", ctx.admin.TeamID); err == nil {
		t.Fatal("orphan group was left behind after mapping failure")
	}

	// A Generic LDAP connection is rejected clearly and leaves no orphan.
	genericID := createTestGenericLDAPConnection(t, ctx)
	w = doRequest(t, ctx, http.MethodPost, "/api/groups/", ctx.apiKey, map[string]interface{}{
		"name":      "Generic Orphan Candidate",
		"ldap_sync": adSyncPayload(genericID),
	})
	if w.Code != http.StatusBadRequest || !bytes.Contains(w.Body.Bytes(), []byte("require an Active Directory connection")) {
		t.Fatalf("expected 400 AD-only error, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := models.GetGroupByName("Generic Orphan Candidate", ctx.admin.TeamID); err == nil {
		t.Fatal("orphan group was left behind after AD-only rejection")
	}

	// A missing group selection is rejected and leaves no orphan.
	connID := createTestLDAPConnection(t, ctx)
	w = doRequest(t, ctx, http.MethodPost, "/api/groups/", ctx.apiKey, map[string]interface{}{
		"name":      "No Selection Candidate",
		"ldap_sync": map[string]interface{}{"connection_id": connID, "enabled": true},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing group selection, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := models.GetGroupByName("No Selection Candidate", ctx.admin.TeamID); err == nil {
		t.Fatal("orphan group was left behind after missing-selection rejection")
	}

	// A GUID that does not resolve in the directory is rejected and leaves no
	// orphan.
	unknown := adSyncPayload(connID)
	unknown["group_directory_id"] = "09090909-0909-0909-0909-090909090909"
	w = doRequest(t, ctx, http.MethodPost, "/api/groups/", ctx.apiKey, map[string]interface{}{
		"name": "Unresolved Candidate", "ldap_sync": unknown,
	})
	if w.Code != http.StatusBadRequest || !bytes.Contains(w.Body.Bytes(), []byte("was not found")) {
		t.Fatalf("expected 400 not-found, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := models.GetGroupByName("Unresolved Candidate", ctx.admin.TeamID); err == nil {
		t.Fatal("orphan group was left behind after resolution failure")
	}

	// A directory/backend failure during resolution is a sanitized 500 and
	// leaves no orphan.
	stubGroupResolverError(t, fmt.Errorf("%w: dial tcp 10.0.0.1:636: timeout", ldap.ErrLDAPConnect))
	w = doRequest(t, ctx, http.MethodPost, "/api/groups/", ctx.apiKey, map[string]interface{}{
		"name": "Backend Candidate", "ldap_sync": adSyncPayload(connID),
	})
	if w.Code != http.StatusInternalServerError || bytes.Contains(w.Body.Bytes(), []byte("10.0.0.1")) {
		t.Fatalf("expected sanitized 500, got %d: %s", w.Code, w.Body.String())
	}
	if _, err := models.GetGroupByName("Backend Candidate", ctx.admin.TeamID); err == nil {
		t.Fatal("orphan group was left behind after backend failure")
	}
}

// TestGroupPutLDAPManagedMembershipLocked verifies manual membership edits via
// PUT /api/groups/:id are rejected for an LDAP-managed group.
func TestGroupPutLDAPManagedMembershipLocked(t *testing.T) {
	ctx := setupTest(t)
	enableSecretKey(ctx)
	stubGroupResolver(t)
	connID := createTestLDAPConnection(t, ctx)

	create := doRequest(t, ctx, http.MethodPost, "/api/groups/", ctx.apiKey, map[string]interface{}{
		"name":      "Managed Locked",
		"ldap_sync": adSyncPayload(connID),
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("create managed group: %d %s", create.Code, create.Body.String())
	}
	var g map[string]interface{}
	_ = json.Unmarshal(create.Body.Bytes(), &g)
	gid := int64(g["id"].(float64))

	// Attempt to manually add a member via PUT - must be rejected with 409.
	w := doRequest(t, ctx, http.MethodPut, fmt.Sprintf("/api/groups/%d", gid), ctx.apiKey, map[string]interface{}{
		"id":   gid,
		"name": "Managed Locked",
		"targets": []map[string]interface{}{
			{"email": "intruder@example.com"},
		},
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409 for manual edit of LDAP-managed group, got %d: %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("managed by LDAP synchronization")) {
		t.Fatalf("expected a clear lock message, got: %s", w.Body.String())
	}
	// Membership must remain empty (the manual edit was rejected).
	ts, _ := models.GetTargets(gid)
	if len(ts) != 0 {
		t.Fatalf("LDAP-managed group membership changed despite lock: %d members", len(ts))
	}
}

// TestGroupPutManualStillWorks confirms manual groups are unaffected by the lock.
func TestGroupPutManualStillWorks(t *testing.T) {
	ctx := setupTest(t)

	group := models.Group{Name: "Manual Group", UserId: 1}
	group.Targets = []models.Target{{BaseRecipient: models.BaseRecipient{Email: "a@example.com"}}}
	group.TeamId = ctx.admin.TeamID
	if err := models.PostGroup(&group); err != nil {
		t.Fatalf("post manual group: %v", err)
	}

	w := doRequest(t, ctx, http.MethodPut, fmt.Sprintf("/api/groups/%d", group.Id), ctx.apiKey, map[string]interface{}{
		"id":   group.Id,
		"name": "Manual Group",
		"targets": []map[string]interface{}{
			{"email": "a@example.com"},
			{"email": "b@example.com"},
		},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 updating manual group, got %d: %s", w.Code, w.Body.String())
	}
	ts, _ := models.GetTargets(group.Id)
	if len(ts) != 2 {
		t.Fatalf("expected manual group to have 2 members after update, got %d", len(ts))
	}
}

// --- Active Directory group discovery endpoint ------------------------------

func TestLDAPGroupsSearchModifyObjectsUserAllowed(t *testing.T) {
	ctx := setupTest(t)
	enableSecretKey(ctx)
	// Unroutable AD server: authorization and validation pass, the dial fails.
	connID := createTestLDAPConnectionWith(t, ctx, map[string]interface{}{
		"name": "Dead AD", "server_url": "ldap://127.0.0.1:1",
		"bind_dn": "cn=svc,dc=example,dc=com", "bind_password": "s3cret", "base_dn": "dc=example,dc=com",
		"directory_id_attr": "objectGUID",
	})

	// A standard "user" role has modify_objects but NOT modify_system.
	role, err := models.GetRoleBySlug(models.RoleUser)
	if err != nil {
		t.Fatalf("get user role: %v", err)
	}
	user := models.User{
		Username: "ldap-groups-objuser", Hash: "hash", ApiKey: "ldap-groups-objuser-key",
		Role: role, RoleID: role.ID, TeamID: ctx.admin.TeamID,
	}
	if err := models.PutUser(&user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	w := doRequest(t, ctx, http.MethodGet,
		fmt.Sprintf("/api/ldap/connections/%d/groups?q=mark", connID), user.ApiKey, nil)
	if w.Code == http.StatusForbidden {
		t.Fatalf("modify_objects user must be allowed to search groups, got 403: %s", w.Body.String())
	}
	// It reached the (failing) backend: standard sanitized 500.
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for unreachable AD, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, leak := range []string{"s3cret", "127.0.0.1:1", "cn=svc", "bind_password", "ldap://127.0.0.1"} {
		if bytes.Contains([]byte(body), []byte(leak)) {
			t.Fatalf("group search error response leaked %q: %s", leak, body)
		}
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("Unable to search Active Directory groups")) {
		t.Fatalf("expected generic sanitized message, got: %s", body)
	}
}

func TestLDAPGroupsSearchAuditorForbidden(t *testing.T) {
	ctx := setupTest(t)
	enableSecretKey(ctx)
	connID := createTestLDAPConnection(t, ctx)

	role, err := models.GetRoleBySlug(models.RoleAuditor)
	if err != nil {
		t.Fatalf("get auditor role: %v", err)
	}
	auditor := models.User{
		Username: "ldap-groups-auditor", Hash: "hash", ApiKey: "ldap-groups-auditor-key",
		Role: role, RoleID: role.ID, TeamID: ctx.admin.TeamID,
	}
	if err := models.PutUser(&auditor); err != nil {
		t.Fatalf("create auditor: %v", err)
	}
	w := doRequest(t, ctx, http.MethodGet,
		fmt.Sprintf("/api/ldap/connections/%d/groups?q=a", connID), auditor.ApiKey, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for auditor group search, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLDAPGroupsSearchGenericLDAPRejected(t *testing.T) {
	ctx := setupTest(t)
	enableSecretKey(ctx)
	// Unroutable server proves the rejection happens before any network access.
	connID := createTestLDAPConnectionWith(t, ctx, map[string]interface{}{
		"name": "Generic", "server_url": "ldap://127.0.0.1:1",
		"bind_dn": "cn=svc,dc=example,dc=com", "bind_password": "s3cret", "base_dn": "dc=example,dc=com",
	})
	w := doRequest(t, ctx, http.MethodGet,
		fmt.Sprintf("/api/ldap/connections/%d/groups?q=a", connID), ctx.apiKey, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for Generic LDAP connection, got %d: %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("only supported for Active Directory connections")) {
		t.Fatalf("expected clear AD-only message, got: %s", w.Body.String())
	}
}

func TestLDAPGroupsSearchQueryTooLongRejected(t *testing.T) {
	ctx := setupTest(t)
	enableSecretKey(ctx)
	connID := createTestLDAPConnectionWith(t, ctx, map[string]interface{}{
		"name": "Dead AD", "server_url": "ldap://127.0.0.1:1",
		"bind_dn": "cn=svc,dc=example,dc=com", "bind_password": "s3cret", "base_dn": "dc=example,dc=com",
		"directory_id_attr": "objectGUID",
	})
	long := strings.Repeat("a", 65)
	w := doRequest(t, ctx, http.MethodGet,
		fmt.Sprintf("/api/ldap/connections/%d/groups?q=%s", connID, long), ctx.apiKey, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for over-long query, got %d: %s", w.Code, w.Body.String())
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("too long")) {
		t.Fatalf("expected too-long message, got: %s", w.Body.String())
	}
}

func TestLDAPGroupsSearchWrongTeamNotFound(t *testing.T) {
	ctx := setupTest(t)
	enableSecretKey(ctx)
	connID := createTestLDAPConnection(t, ctx)

	role, _ := models.GetRoleBySlug(models.RoleUser)
	other := models.User{
		Username: "groups-other-team", Hash: "hash", ApiKey: "groups-other-team-key",
		Role: role, RoleID: role.ID, TeamID: ctx.admin.TeamID + 999,
	}
	if err := models.PutUser(&other); err != nil {
		t.Fatalf("create other-team user: %v", err)
	}
	w := doRequest(t, ctx, http.MethodGet,
		fmt.Sprintf("/api/ldap/connections/%d/groups?q=a", connID), other.ApiKey, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for cross-team connection, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLDAPGroupsSearchMethodNotAllowed(t *testing.T) {
	ctx := setupTest(t)
	enableSecretKey(ctx)
	connID := createTestLDAPConnection(t, ctx)
	w := doRequest(t, ctx, http.MethodPost,
		fmt.Sprintf("/api/ldap/connections/%d/groups", connID), ctx.apiKey, map[string]interface{}{"filter": "(objectClass=*)"})
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for POST on group search, got %d: %s", w.Code, w.Body.String())
	}
}

// --- Connection summary endpoint (object-tier) -----------------------------

func TestLDAPConnectionSummaryModifyObjectsAllowed(t *testing.T) {
	ctx := setupTest(t)
	enableSecretKey(ctx)
	createTestLDAPConnection(t, ctx)

	// A standard "user" (modify_objects, not modify_system) may use summary.
	role, _ := models.GetRoleBySlug(models.RoleUser)
	user := models.User{Username: "sum-user", Hash: "h", ApiKey: "sum-user-key",
		Role: role, RoleID: role.ID, TeamID: ctx.admin.TeamID}
	if err := models.PutUser(&user); err != nil {
		t.Fatalf("create user: %v", err)
	}

	w := doRequest(t, ctx, http.MethodGet, "/api/ldap/connections/summary", user.ApiKey, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for modify_objects user, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	// Must contain id/name/base_dn and NOTHING sensitive.
	for _, want := range []string{"\"id\"", "\"name\"", "\"base_dn\""} {
		if !bytes.Contains(w.Body.Bytes(), []byte(want)) {
			t.Fatalf("summary missing field %s: %s", want, body)
		}
	}
	for _, leak := range []string{"server_url", "bind_dn", "bind_password", "directory_id_attr", "start_tls", "ignore_cert_errors", "s3cret", "ldaps://ad.example.com"} {
		if bytes.Contains(w.Body.Bytes(), []byte(leak)) {
			t.Fatalf("summary leaked %q: %s", leak, body)
		}
	}

	// The SAME user must still be denied the administrative list endpoint.
	adminList := doRequest(t, ctx, http.MethodGet, "/api/ldap/connections/", user.ApiKey, nil)
	if adminList.Code != http.StatusForbidden {
		t.Fatalf("modify_objects user must NOT access admin connection list, got %d", adminList.Code)
	}
}

func TestLDAPConnectionSummaryAuditorForbidden(t *testing.T) {
	ctx := setupTest(t)
	enableSecretKey(ctx)
	createTestLDAPConnection(t, ctx)

	role, _ := models.GetRoleBySlug(models.RoleAuditor)
	auditor := models.User{Username: "sum-auditor", Hash: "h", ApiKey: "sum-auditor-key",
		Role: role, RoleID: role.ID, TeamID: ctx.admin.TeamID}
	if err := models.PutUser(&auditor); err != nil {
		t.Fatalf("create auditor: %v", err)
	}
	w := doRequest(t, ctx, http.MethodGet, "/api/ldap/connections/summary", auditor.ApiKey, nil)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for auditor on summary, got %d: %s", w.Code, w.Body.String())
	}
}

func TestLDAPConnectionSummaryCrossTeamIsolated(t *testing.T) {
	ctx := setupTest(t)
	enableSecretKey(ctx)
	createTestLDAPConnection(t, ctx)

	role, _ := models.GetRoleBySlug(models.RoleUser)
	other := models.User{Username: "sum-other", Hash: "h", ApiKey: "sum-other-key",
		Role: role, RoleID: role.ID, TeamID: ctx.admin.TeamID + 999}
	if err := models.PutUser(&other); err != nil {
		t.Fatalf("create other-team user: %v", err)
	}
	w := doRequest(t, ctx, http.MethodGet, "/api/ldap/connections/summary", other.ApiKey, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (empty) for other team, got %d", w.Code)
	}
	var sums []map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &sums)
	if len(sums) != 0 {
		t.Fatalf("other team must see no connections, got %d", len(sums))
	}
}

// --- /api/ldap/syncs/?group_id= filter -------------------------------------

func TestLDAPSyncsGroupIDFilter(t *testing.T) {
	ctx := setupTest(t)
	enableSecretKey(ctx)
	stubGroupResolver(t)
	connID := createTestLDAPConnection(t, ctx)

	// Managed group via the atomic create path.
	create := doRequest(t, ctx, http.MethodPost, "/api/groups/", ctx.apiKey, map[string]interface{}{
		"name":      "Filter Managed",
		"ldap_sync": adSyncPayload(connID),
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("create managed group: %d %s", create.Code, create.Body.String())
	}
	var g map[string]interface{}
	_ = json.Unmarshal(create.Body.Bytes(), &g)
	gid := int64(g["id"].(float64))

	// Filter by the managed group's id → exactly one mapping.
	w := doRequest(t, ctx, http.MethodGet, fmt.Sprintf("/api/ldap/syncs/?group_id=%d", gid), ctx.apiKey, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var got []map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 mapping for managed group, got %d", len(got))
	}
	if int64(got[0]["group_id"].(float64)) != gid {
		t.Fatalf("mapping group_id mismatch")
	}

	// A manual (unmanaged) group → empty list.
	manual := models.Group{Name: "Filter Manual", UserId: 1, TeamId: ctx.admin.TeamID}
	manual.Targets = []models.Target{{BaseRecipient: models.BaseRecipient{Email: "a@example.com"}}}
	if err := models.PostGroup(&manual); err != nil {
		t.Fatalf("post manual group: %v", err)
	}
	w = doRequest(t, ctx, http.MethodGet, fmt.Sprintf("/api/ldap/syncs/?group_id=%d", manual.Id), ctx.apiKey, nil)
	var empty []map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &empty)
	if w.Code != http.StatusOK || len(empty) != 0 {
		t.Fatalf("unmanaged group must yield empty list, got code=%d len=%d", w.Code, len(empty))
	}

	// Invalid group_id → empty list, not an error.
	w = doRequest(t, ctx, http.MethodGet, "/api/ldap/syncs/?group_id=not-a-number", ctx.apiKey, nil)
	var bad []map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &bad)
	if w.Code != http.StatusOK || len(bad) != 0 {
		t.Fatalf("invalid group_id must yield empty list, got code=%d len=%d", w.Code, len(bad))
	}

	// No filter → unchanged behavior (returns the list including our mapping).
	w = doRequest(t, ctx, http.MethodGet, "/api/ldap/syncs/", ctx.apiKey, nil)
	var all []map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &all)
	if w.Code != http.StatusOK || len(all) < 1 {
		t.Fatalf("unfiltered list regressed, got code=%d len=%d", w.Code, len(all))
	}
}

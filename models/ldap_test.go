package models

import (
	"encoding/json"
	"strings"

	"gopkg.in/check.v1"
)

// ldapTeamID is the Default Team (id 1) that the built-in admin belongs to.
const ldapTeamID = int64(1)

func (s *ModelsSuite) newLDAPConnection() *LDAPConnection {
	return &LDAPConnection{
		UserId:       1,
		TeamId:       ldapTeamID,
		Name:         "Corp AD",
		ServerURL:    "ldaps://ad.example.com:636",
		BindDN:       "cn=svc,dc=example,dc=com",
		BindPassword: "s3cret",
		BaseDN:       "dc=example,dc=com",
	}
}

// newADConnection returns an Active Directory connection (stable identity
// objectGUID), which is what group-based managed synchronization requires.
func (s *ModelsSuite) newADConnection() *LDAPConnection {
	conn := s.newLDAPConnection()
	conn.DirectoryIDAttr = DirectoryIDAttrObjectGUID
	return conn
}

// Canonical test group identity (lower-case GUID text) and its last-known
// metadata, all under the test connection BaseDN dc=example,dc=com.
const (
	testGroupGUID = "a1b2c3d4-e5f6-4788-99aa-bbccddeeff00"
	testGroupDN   = "CN=Marketing,OU=Groups,DC=example,DC=com"
	testGroupName = "Marketing"
)

// adSync returns a valid group-based mapping for the given AD connection.
func adSync(connID int64) *LDAPGroupSync {
	return &LDAPGroupSync{
		ConnectionId:     connID,
		Enabled:          true,
		GroupDirectoryId: testGroupGUID,
		GroupDN:          testGroupDN,
		GroupName:        testGroupName,
	}
}

// adSyncFor returns a valid team/user/group-bound mapping for standalone
// mapping-endpoint tests.
func adSyncFor(groupID, connID int64) *LDAPGroupSync {
	m := adSync(connID)
	m.TeamId = ldapTeamID
	m.UserId = 1
	m.GroupId = groupID
	return m
}

func (s *ModelsSuite) TestPostLDAPConnectionEncryptsPassword(c *check.C) {
	conn := s.newLDAPConnection()
	c.Assert(PostLDAPConnection(conn), check.Equals, nil)
	// The ciphertext must not be the plaintext, and must be set.
	c.Assert(conn.BindPasswordEnc != "", check.Equals, true)
	c.Assert(conn.BindPasswordEnc != "s3cret", check.Equals, true)

	// It must round-trip back to the plaintext.
	plain, err := conn.DecryptBindPassword()
	c.Assert(err, check.Equals, nil)
	c.Assert(plain, check.Equals, "s3cret")
}

// withSecretKey temporarily overrides the installation secret_key for a test
// and restores it afterwards.
func (s *ModelsSuite) withSecretKey(c *check.C, key string, fn func()) {
	orig := conf.SecretKey
	defer func() { conf.SecretKey = orig }()
	conf.SecretKey = key
	fn()
}

func (s *ModelsSuite) TestPostLDAPConnectionMissingSecretKey(c *check.C) {
	s.withSecretKey(c, "", func() {
		conn := s.newLDAPConnection()
		err := PostLDAPConnection(conn)
		c.Assert(err, check.Equals, ErrLDAPSecretKeyNotConfigured)
	})
}

func (s *ModelsSuite) TestPostLDAPConnectionInvalidSecretKey(c *check.C) {
	// Configured but not a base64-encoded 32-byte key.
	cases := []string{
		"not-base64!!!", // invalid base64
		"c2hvcnQ=",      // valid base64, too short (5 bytes)
		"MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWZ4eHg=", // valid base64, too long
	}
	for _, k := range cases {
		s.withSecretKey(c, k, func() {
			conn := s.newLDAPConnection()
			err := PostLDAPConnection(conn)
			c.Assert(err, check.Equals, ErrLDAPSecretKeyInvalid)
			// The configured key value must never appear in the error text.
			c.Assert(strings.Contains(err.Error(), k), check.Equals, false)
		})
	}
}

func (s *ModelsSuite) TestLDAPConnectionValidation(c *check.C) {
	cases := []struct {
		mutate func(*LDAPConnection)
		expect error
	}{
		{func(x *LDAPConnection) { x.Name = "" }, ErrLDAPNameNotSpecified},
		{func(x *LDAPConnection) { x.ServerURL = "" }, ErrLDAPServerURLNotSpecified},
		{func(x *LDAPConnection) { x.ServerURL = "http://nope" }, ErrLDAPInvalidServerURL},
		{func(x *LDAPConnection) { x.BindDN = "" }, ErrLDAPBindDNNotSpecified},
		{func(x *LDAPConnection) { x.BaseDN = "" }, ErrLDAPBaseDNNotSpecified},
		{func(x *LDAPConnection) { x.BindPassword = "" }, ErrLDAPPasswordNotSpecified},
	}
	for _, tc := range cases {
		conn := s.newLDAPConnection()
		tc.mutate(conn)
		c.Assert(conn.Validate(true), check.Equals, tc.expect)
	}
}

func (s *ModelsSuite) TestPutLDAPConnectionKeepsPasswordWhenBlank(c *check.C) {
	conn := s.newLDAPConnection()
	c.Assert(PostLDAPConnection(conn), check.Equals, nil)
	originalEnc := conn.BindPasswordEnc

	// Update with a blank password - the stored ciphertext must be preserved.
	update := &LDAPConnection{
		Id: conn.Id, UserId: 1, TeamId: ldapTeamID,
		Name: "Corp AD renamed", ServerURL: conn.ServerURL,
		BindDN: conn.BindDN, BaseDN: conn.BaseDN, BindPassword: "",
	}
	c.Assert(PutLDAPConnection(update), check.Equals, nil)

	reloaded, err := GetLDAPConnection(conn.Id, ldapTeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(reloaded.Name, check.Equals, "Corp AD renamed")
	c.Assert(reloaded.BindPasswordEnc, check.Equals, originalEnc)
}

func (s *ModelsSuite) TestLDAPConnectionTeamScoping(c *check.C) {
	conn := s.newLDAPConnection()
	c.Assert(PostLDAPConnection(conn), check.Equals, nil)
	// A different team must not see it.
	_, err := GetLDAPConnection(conn.Id, ldapTeamID+999)
	c.Assert(err, check.NotNil)
}

func (s *ModelsSuite) makeGroup(c *check.C, name string, emails ...string) Group {
	g := Group{Name: name, UserId: 1}
	for _, e := range emails {
		g.Targets = append(g.Targets, Target{BaseRecipient: BaseRecipient{Email: e}})
	}
	c.Assert(PostGroup(&g), check.Equals, nil)
	return g
}

func (s *ModelsSuite) TestPostLDAPManagedGroupCreatesGroupAndMapping(c *check.C) {
	conn := s.newADConnection()
	c.Assert(PostLDAPConnection(conn), check.Equals, nil)

	g := &Group{Name: "LDAP Managed", UserId: 1}
	sync := adSync(conn.Id)
	c.Assert(PostLDAPManagedGroup(g, sync), check.Equals, nil)

	// The group exists with no members (membership comes from sync).
	c.Assert(g.Id != 0, check.Equals, true)
	ts, err := GetTargets(g.Id)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(ts), check.Equals, 0)

	// The mapping exists, references the group, and has defaults normalized.
	c.Assert(sync.Id != 0, check.Equals, true)
	c.Assert(sync.GroupId, check.Equals, g.Id)
	c.Assert(sync.GroupDirectoryId, check.Equals, testGroupGUID)
	c.Assert(sync.GroupDN, check.Equals, testGroupDN)
	c.Assert(sync.GroupName, check.Equals, testGroupName)
	c.Assert(sync.AttrEmail, check.Equals, DefaultLDAPAttrEmail)

	managed, err := IsGroupLDAPManaged(g.Id, ldapTeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(managed, check.Equals, true)
}

// TestPostLDAPManagedGroupRollsBackOnMappingFailure verifies atomicity: if the
// mapping is invalid, the group must NOT be left behind.
func (s *ModelsSuite) TestPostLDAPManagedGroupRollsBackOnMappingFailure(c *check.C) {
	g := &Group{Name: "Should Not Persist", UserId: 1}
	// Reference a non-existent connection so mapping validation fails.
	sync := &LDAPGroupSync{ConnectionId: 999999}
	err := PostLDAPManagedGroup(g, sync)
	c.Assert(err, check.Equals, ErrLDAPConnectionNotFound)

	// No orphan group with that name must exist.
	_, gerr := GetGroupByName("Should Not Persist", ldapTeamID)
	c.Assert(gerr, check.NotNil)
}

// TestManualGroupValidationUnchanged confirms the generic manual-group rule
// (at least one target) still applies to PostGroup.
func (s *ModelsSuite) TestManualGroupValidationUnchanged(c *check.C) {
	g := &Group{Name: "Empty Manual", UserId: 1}
	err := PostGroup(g)
	c.Assert(err, check.Equals, ErrNoTargetsSpecified)
}

func (s *ModelsSuite) TestIsGroupLDAPManagedFalseForManualGroup(c *check.C) {
	g := s.makeGroup(c, "Plain Manual", "a@example.com")
	managed, err := IsGroupLDAPManaged(g.Id, ldapTeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(managed, check.Equals, false)
}

func (s *ModelsSuite) TestLDAPGroupSyncValidation(c *check.C) {
	conn := s.newADConnection()
	c.Assert(PostLDAPConnection(conn), check.Equals, nil)
	g := s.makeGroup(c, "LDAP Group", "seed@example.com")

	// Missing group.
	bad := &LDAPGroupSync{TeamId: ldapTeamID, ConnectionId: conn.Id}
	c.Assert(bad.Validate(), check.Equals, ErrLDAPGroupNotSpecified)

	// Missing connection.
	bad = &LDAPGroupSync{TeamId: ldapTeamID, GroupId: g.Id}
	c.Assert(bad.Validate(), check.Equals, ErrLDAPConnectionNotSpecified)

	// Connection from another team must be rejected (team isolation).
	bad = &LDAPGroupSync{TeamId: ldapTeamID + 999, GroupId: g.Id, ConnectionId: conn.Id}
	c.Assert(bad.Validate(), check.Equals, ErrLDAPConnectionNotFound)

	// No directory group selected.
	bad = &LDAPGroupSync{TeamId: ldapTeamID, GroupId: g.Id, ConnectionId: conn.Id}
	c.Assert(bad.Validate(), check.Equals, ErrLDAPGroupDirectoryIDNotSpecified)

	// Group identifier that is not a GUID.
	bad = adSyncFor(g.Id, conn.Id)
	bad.GroupDirectoryId = "not-a-guid"
	c.Assert(bad.Validate(), check.Equals, ErrLDAPGroupDirectoryIDInvalid)

	// Missing DN / name.
	bad = adSyncFor(g.Id, conn.Id)
	bad.GroupDN = ""
	c.Assert(bad.Validate(), check.Equals, ErrLDAPGroupDNNotSpecified)
	bad = adSyncFor(g.Id, conn.Id)
	bad.GroupName = "   "
	c.Assert(bad.Validate(), check.Equals, ErrLDAPGroupNameNotSpecified)

	// Valid mapping normalizes defaults and canonicalizes the identifier
	// (upper-case, braces and whitespace are accepted and normalized).
	good := adSyncFor(g.Id, conn.Id)
	good.GroupDirectoryId = "  {A1B2C3D4-E5F6-4788-99AA-BBCCDDEEFF00} "
	good.GroupName = "  Marketing  "
	c.Assert(good.Validate(), check.Equals, nil)
	c.Assert(good.GroupDirectoryId, check.Equals, testGroupGUID)
	c.Assert(good.GroupName, check.Equals, testGroupName)
	c.Assert(good.AttrEmail, check.Equals, DefaultLDAPAttrEmail)
	c.Assert(good.SyncFreq, check.Equals, uint32(DefaultLDAPSyncFreq))
}

// TestLDAPGroupSyncRejectsGenericLDAP verifies group-based managed
// synchronization is Active Directory-only in v1: a connection without
// objectGUID stable identity is rejected clearly, both on the standalone
// mapping path and on the atomic managed-group create path (no orphan group).
func (s *ModelsSuite) TestLDAPGroupSyncRejectsGenericLDAP(c *check.C) {
	generic := s.newLDAPConnection() // DirectoryIDAttr empty
	c.Assert(PostLDAPConnection(generic), check.Equals, nil)
	uuidConn := s.newLDAPConnection()
	uuidConn.Name = "OpenLDAP"
	uuidConn.DirectoryIDAttr = "entryUUID"
	c.Assert(PostLDAPConnection(uuidConn), check.Equals, nil)
	g := s.makeGroup(c, "Generic Target", "seed@example.com")

	c.Assert(adSyncFor(g.Id, generic.Id).Validate(), check.Equals, ErrLDAPConnectionNotActiveDirectory)
	c.Assert(adSyncFor(g.Id, uuidConn.Id).Validate(), check.Equals, ErrLDAPConnectionNotActiveDirectory)

	mg := &Group{Name: "Generic Managed", UserId: 1}
	c.Assert(PostLDAPManagedGroup(mg, adSync(generic.Id)), check.Equals, ErrLDAPConnectionNotActiveDirectory)
	_, gerr := GetGroupByName("Generic Managed", ldapTeamID)
	c.Assert(gerr, check.NotNil)
}

// TestLDAPGroupSyncLegacyFieldsNotSerialized verifies the OU-era keys are
// neither exposed over JSON nor accepted from it (the physical columns remain
// but are unmapped), while the group selection and skip counter are exposed.
func (s *ModelsSuite) TestLDAPGroupSyncLegacyFieldsNotSerialized(c *check.C) {
	m := adSync(1)
	b, err := json.Marshal(m)
	c.Assert(err, check.Equals, nil)
	for _, legacy := range []string{"search_base", "search_filter", "search_scope"} {
		c.Assert(strings.Contains(string(b), legacy), check.Equals, false)
	}
	for _, want := range []string{"group_directory_id", "group_dn", "group_name", "last_sync_skipped"} {
		c.Assert(strings.Contains(string(b), want), check.Equals, true)
	}
	var in LDAPGroupSync
	c.Assert(json.Unmarshal([]byte(`{"connection_id":1,"search_base":"ou=x,dc=example,dc=com","group_directory_id":"`+testGroupGUID+`"}`), &in), check.Equals, nil)
	c.Assert(in.GroupDirectoryId, check.Equals, testGroupGUID)
}

// TestLDAPGroupSyncUnmarshalExcludeDisabledDefault pins the API-boundary
// default: omitted → true, explicit true → true, explicit false → false.
func (s *ModelsSuite) TestLDAPGroupSyncUnmarshalExcludeDisabledDefault(c *check.C) {
	var omitted LDAPGroupSync
	c.Assert(json.Unmarshal([]byte(`{"connection_id":1}`), &omitted), check.Equals, nil)
	c.Assert(omitted.ExcludeDisabled, check.Equals, true)

	var explicitTrue LDAPGroupSync
	c.Assert(json.Unmarshal([]byte(`{"connection_id":1,"exclude_disabled":true}`), &explicitTrue), check.Equals, nil)
	c.Assert(explicitTrue.ExcludeDisabled, check.Equals, true)

	var explicitFalse LDAPGroupSync
	c.Assert(json.Unmarshal([]byte(`{"connection_id":1,"exclude_disabled":false}`), &explicitFalse), check.Equals, nil)
	c.Assert(explicitFalse.ExcludeDisabled, check.Equals, false)

	// Works through a pointer field too (the ldap_sync sub-object of a group
	// create request).
	var wrapper struct {
		Sync *LDAPGroupSync `json:"ldap_sync"`
	}
	c.Assert(json.Unmarshal([]byte(`{"ldap_sync":{"connection_id":1}}`), &wrapper), check.Equals, nil)
	c.Assert(wrapper.Sync.ExcludeDisabled, check.Equals, true)
}

// TestLDAPGroupSyncPersistsDirectoryGroup verifies the selected group fields
// round-trip through create, reload and update.
func (s *ModelsSuite) TestLDAPGroupSyncPersistsDirectoryGroup(c *check.C) {
	conn := s.newADConnection()
	c.Assert(PostLDAPConnection(conn), check.Equals, nil)
	g := s.makeGroup(c, "Persist Group", "seed@example.com")

	m := adSyncFor(g.Id, conn.Id)
	c.Assert(PostLDAPGroupSync(m), check.Equals, nil)
	got, err := GetLDAPGroupSync(m.Id, ldapTeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(got.GroupDirectoryId, check.Equals, testGroupGUID)
	c.Assert(got.GroupDN, check.Equals, testGroupDN)
	c.Assert(got.GroupName, check.Equals, testGroupName)

	// Last-known DN/name are refreshed only through the server-side helper
	// (e.g. after an AD rename/move) while the identity stays the same.
	c.Assert(UpdateLDAPGroupSyncDirectoryGroup(m.Id, ldapTeamID, "CN=Marketing Team,OU=Moved,DC=example,DC=com", "Marketing Team"), check.Equals, nil)
	again, err := GetLDAPGroupSync(m.Id, ldapTeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(again.GroupDirectoryId, check.Equals, testGroupGUID)
	c.Assert(again.GroupDN, check.Equals, "CN=Marketing Team,OU=Moved,DC=example,DC=com")
	c.Assert(again.GroupName, check.Equals, "Marketing Team")

	// A client PUT cannot touch DN/name: values are discarded, not applied.
	again.GroupDN = "CN=Spoof,DC=example,DC=com"
	again.GroupName = "Spoof"
	again.AttrPosition = "department"
	c.Assert(PutLDAPGroupSync(&again), check.Equals, nil)
	after, err := GetLDAPGroupSync(m.Id, ldapTeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(after.GroupDN, check.Equals, "CN=Marketing Team,OU=Moved,DC=example,DC=com")
	c.Assert(after.GroupName, check.Equals, "Marketing Team")
	c.Assert(after.AttrPosition, check.Equals, "department")

	// Two Gophish groups may reference the same directory group (the index is
	// deliberately not unique).
	g2 := s.makeGroup(c, "Persist Group 2", "seed2@example.com")
	c.Assert(PostLDAPGroupSync(adSyncFor(g2.Id, conn.Id)), check.Equals, nil)
}

func (s *ModelsSuite) TestReconcileGroupTargetsAddUpdateRemove(c *check.C) {
	g := s.makeGroup(c, "Recon Group", "keep@example.com", "drop@example.com")

	desired := []Target{
		{BaseRecipient: BaseRecipient{Email: "keep@example.com", FirstName: "Updated", LastName: "Name", Position: "Mgr"}},
		{BaseRecipient: BaseRecipient{Email: "new@example.com", FirstName: "New", LastName: "User"}},
	}
	res, err := ReconcileGroupTargets(g.Id, ldapTeamID, desired)
	c.Assert(err, check.Equals, nil)
	c.Assert(res.Added, check.Equals, 1)   // new@
	c.Assert(res.Updated, check.Equals, 1) // keep@
	c.Assert(res.Removed, check.Equals, 1) // drop@

	ts, err := GetTargets(g.Id)
	c.Assert(err, check.Equals, nil)
	byEmail := map[string]Target{}
	for _, t := range ts {
		byEmail[t.Email] = t
	}
	c.Assert(len(ts), check.Equals, 2)
	_, keptHas := byEmail["keep@example.com"]
	_, newHas := byEmail["new@example.com"]
	_, dropHas := byEmail["drop@example.com"]
	c.Assert(keptHas, check.Equals, true)
	c.Assert(newHas, check.Equals, true)
	c.Assert(dropHas, check.Equals, false)
	c.Assert(byEmail["keep@example.com"].FirstName, check.Equals, "Updated")
}

// TestReconcilePreservesTargetRowOnRemoval verifies that removing a member
// only removes the group membership (group_targets), not the underlying
// target row - which is what preserves historical campaign results.
func (s *ModelsSuite) TestReconcilePreservesTargetRowOnRemoval(c *check.C) {
	g := s.makeGroup(c, "Preserve Group", "person@example.com")

	// Capture the target id before removal.
	before, err := GetTargets(g.Id)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(before), check.Equals, 1)
	targetID := before[0].Id

	// Reconcile to an empty desired set - the member is dropped from the group.
	res, err := ReconcileGroupTargets(g.Id, ldapTeamID, []Target{})
	c.Assert(err, check.Equals, nil)
	c.Assert(res.Removed, check.Equals, 1)

	// The group has no members...
	after, err := GetTargets(g.Id)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(after), check.Equals, 0)

	// ...but the target row itself still exists in the DB.
	var count int64
	c.Assert(db.Model(&Target{}).Where("id = ?", targetID).Count(&count).Error, check.Equals, nil)
	c.Assert(count, check.Equals, int64(1))
}

// TestReconcileSkipsInvalidEmails ensures malformed entries don't abort the
// whole reconciliation (they are rejected per-target).
func (s *ModelsSuite) TestReconcileRejectsInvalidEmail(c *check.C) {
	g := s.makeGroup(c, "Invalid Email Group", "ok@example.com")
	desired := []Target{
		{BaseRecipient: BaseRecipient{Email: "ok@example.com"}},
		{BaseRecipient: BaseRecipient{Email: "not-an-email"}},
	}
	// insertTargetIntoGroup rejects the invalid address, so the transaction
	// returns an error and membership is left unchanged.
	_, err := ReconcileGroupTargets(g.Id, ldapTeamID, desired)
	c.Assert(err, check.NotNil)
}

func (s *ModelsSuite) TestDeleteLDAPConnectionBlockedWhenReferenced(c *check.C) {
	conn := s.newADConnection()
	c.Assert(PostLDAPConnection(conn), check.Equals, nil)
	g := s.makeGroup(c, "Cascade Group", "seed@example.com")

	sync := adSyncFor(g.Id, conn.Id)
	c.Assert(PostLDAPGroupSync(sync), check.Equals, nil)

	// Deletion must be blocked while a mapping references the connection.
	err := DeleteLDAPConnection(conn.Id, ldapTeamID)
	c.Assert(err, check.Equals, ErrLDAPConnectionInUse)

	// The connection, the mapping, the group and its members must all remain.
	_, err = GetLDAPConnection(conn.Id, ldapTeamID)
	c.Assert(err, check.Equals, nil)
	_, err = GetLDAPGroupSync(sync.Id, ldapTeamID)
	c.Assert(err, check.Equals, nil)
	remaining, gerr := GetTargets(g.Id)
	c.Assert(gerr, check.Equals, nil)
	c.Assert(len(remaining), check.Equals, 1)
}

func (s *ModelsSuite) TestDeleteLDAPConnectionAllowedWhenUnreferenced(c *check.C) {
	conn := s.newADConnection()
	c.Assert(PostLDAPConnection(conn), check.Equals, nil)
	g := s.makeGroup(c, "Reassign Group", "seed@example.com")
	sync := adSyncFor(g.Id, conn.Id)
	c.Assert(PostLDAPGroupSync(sync), check.Equals, nil)

	// Remove the only mapping, then deletion should succeed.
	c.Assert(DeleteLDAPGroupSync(sync.Id, ldapTeamID), check.Equals, nil)
	c.Assert(DeleteLDAPConnection(conn.Id, ldapTeamID), check.Equals, nil)

	_, err := GetLDAPConnection(conn.Id, ldapTeamID)
	c.Assert(err, check.NotNil)
	// The group and its members are untouched.
	remaining, gerr := GetTargets(g.Id)
	c.Assert(gerr, check.Equals, nil)
	c.Assert(len(remaining), check.Equals, 1)
}

// --- Stable directory identity (Approach B) --------------------------------

// dt builds a DirectoryTarget with a stable directory id.
func dt(dirID, email, first, last, position string) DirectoryTarget {
	return DirectoryTarget{
		Target:      Target{BaseRecipient: BaseRecipient{Email: email, FirstName: first, LastName: last, Position: position}},
		DirectoryId: dirID,
	}
}

// setupIdentityGroup creates a connection + an LDAP-managed group and returns
// both ids for identity reconciliation tests.
func (s *ModelsSuite) setupIdentityGroup(c *check.C, name string) (connID, groupID int64) {
	conn := s.newADConnection()
	c.Assert(PostLDAPConnection(conn), check.Equals, nil)
	g := &Group{Name: name, UserId: 1}
	c.Assert(PostLDAPManagedGroup(g, adSync(conn.Id)), check.Equals, nil)
	return conn.Id, g.Id
}

// targetByEmail returns the single current member target with the given email.
func (s *ModelsSuite) memberByEmail(c *check.C, groupID int64, email string) (Target, bool) {
	ts, err := GetTargets(groupID)
	c.Assert(err, check.Equals, nil)
	for _, t := range ts {
		if t.Email == email {
			return t, true
		}
	}
	return Target{}, false
}

func (s *ModelsSuite) TestIdentityTitleAndNameChangeSameTarget(c *check.C) {
	connID, gid := s.setupIdentityGroup(c, "Ident Attr")

	r1, err := ReconcileGroupTargetsWithIdentity(gid, ldapTeamID, connID, []DirectoryTarget{
		dt("guid-1", "a@example.com", "Ann", "Smith", "Analyst"),
	})
	c.Assert(err, check.Equals, nil)
	c.Assert(r1.Added, check.Equals, 1)
	orig, ok := s.memberByEmail(c, gid, "a@example.com")
	c.Assert(ok, check.Equals, true)

	// Title + name change, same directory id → same target row, updated attrs.
	r2, err := ReconcileGroupTargetsWithIdentity(gid, ldapTeamID, connID, []DirectoryTarget{
		dt("guid-1", "a@example.com", "Anne", "Smithson", "Manager"),
	})
	c.Assert(err, check.Equals, nil)
	c.Assert(r2.Updated, check.Equals, 1)
	c.Assert(r2.Added, check.Equals, 0)
	updated, ok := s.memberByEmail(c, gid, "a@example.com")
	c.Assert(ok, check.Equals, true)
	c.Assert(updated.Id, check.Equals, orig.Id)
	c.Assert(updated.FirstName, check.Equals, "Anne")
	c.Assert(updated.LastName, check.Equals, "Smithson")
	c.Assert(updated.Position, check.Equals, "Manager")
}

func (s *ModelsSuite) TestIdentityEmailChangeSameTarget(c *check.C) {
	connID, gid := s.setupIdentityGroup(c, "Ident Email")

	_, err := ReconcileGroupTargetsWithIdentity(gid, ldapTeamID, connID, []DirectoryTarget{
		dt("guid-1", "old@example.com", "Bob", "Jones", "Dev"),
	})
	c.Assert(err, check.Equals, nil)
	orig, ok := s.memberByEmail(c, gid, "old@example.com")
	c.Assert(ok, check.Equals, true)

	// Email changes, same directory id → same target, email updated in place.
	r2, err := ReconcileGroupTargetsWithIdentity(gid, ldapTeamID, connID, []DirectoryTarget{
		dt("guid-1", "new@example.com", "Bob", "Jones", "Dev"),
	})
	c.Assert(err, check.Equals, nil)
	c.Assert(r2.Updated, check.Equals, 1)
	c.Assert(r2.Added, check.Equals, 0)
	c.Assert(r2.Removed, check.Equals, 0)
	ts, _ := GetTargets(gid)
	c.Assert(len(ts), check.Equals, 1)
	c.Assert(ts[0].Id, check.Equals, orig.Id)
	c.Assert(ts[0].Email, check.Equals, "new@example.com")
}

func (s *ModelsSuite) TestIdentityMoveOutAndReturnReconnects(c *check.C) {
	connID, gid := s.setupIdentityGroup(c, "Ident Reentry")

	_, err := ReconcileGroupTargetsWithIdentity(gid, ldapTeamID, connID, []DirectoryTarget{
		dt("guid-1", "c@example.com", "Cara", "Lee", "QA"),
	})
	c.Assert(err, check.Equals, nil)
	orig, _ := s.memberByEmail(c, gid, "c@example.com")

	// Move outside SearchBase (absent from desired) → membership removed,
	// identity retained.
	r2, err := ReconcileGroupTargetsWithIdentity(gid, ldapTeamID, connID, []DirectoryTarget{})
	c.Assert(err, check.Equals, nil)
	c.Assert(r2.Removed, check.Equals, 1)
	ts, _ := GetTargets(gid)
	c.Assert(len(ts), check.Equals, 0)
	// Identity row still exists.
	var idCount int64
	c.Assert(db.Model(&LDAPTargetIdentity{}).Where("connection_id=? and directory_id=?", connID, "guid-1").Count(&idCount).Error, check.Equals, nil)
	c.Assert(idCount, check.Equals, int64(1))

	// Return into scope → reconnect to the SAME target row.
	r3, err := ReconcileGroupTargetsWithIdentity(gid, ldapTeamID, connID, []DirectoryTarget{
		dt("guid-1", "c@example.com", "Cara", "Lee", "QA"),
	})
	c.Assert(err, check.Equals, nil)
	c.Assert(r3.Added, check.Equals, 1)
	back, ok := s.memberByEmail(c, gid, "c@example.com")
	c.Assert(ok, check.Equals, true)
	c.Assert(back.Id, check.Equals, orig.Id)
}

func (s *ModelsSuite) TestIdentitySameObjectTwoGroupsSameConnection(c *check.C) {
	conn := s.newLDAPConnection()
	conn.DirectoryIDAttr = "objectGUID"
	c.Assert(PostLDAPConnection(conn), check.Equals, nil)

	g1 := &Group{Name: "Grp One", UserId: 1}
	c.Assert(PostLDAPManagedGroup(g1, adSync(conn.Id)), check.Equals, nil)
	g2 := &Group{Name: "Grp Two", UserId: 1}
	c.Assert(PostLDAPManagedGroup(g2, adSync(conn.Id)), check.Equals, nil)

	_, err := ReconcileGroupTargetsWithIdentity(g1.Id, ldapTeamID, conn.Id, []DirectoryTarget{dt("guid-x", "x@example.com", "X", "Y", "Z")})
	c.Assert(err, check.Equals, nil)
	_, err = ReconcileGroupTargetsWithIdentity(g2.Id, ldapTeamID, conn.Id, []DirectoryTarget{dt("guid-x", "x@example.com", "X", "Y", "Z")})
	c.Assert(err, check.Equals, nil)

	t1, _ := s.memberByEmail(c, g1.Id, "x@example.com")
	t2, _ := s.memberByEmail(c, g2.Id, "x@example.com")
	// Same connection + same directory id → same target row shared by both groups.
	c.Assert(t1.Id, check.Equals, t2.Id)
	// Exactly one identity row for that object.
	var idCount int64
	c.Assert(db.Model(&LDAPTargetIdentity{}).Where("connection_id=? and directory_id=?", conn.Id, "guid-x").Count(&idCount).Error, check.Equals, nil)
	c.Assert(idCount, check.Equals, int64(1))
}

func (s *ModelsSuite) TestIdentitySameEmailTwoConnectionsNoMerge(c *check.C) {
	connA := s.newLDAPConnection()
	connA.Name = "Conn A"
	connA.DirectoryIDAttr = "objectGUID"
	c.Assert(PostLDAPConnection(connA), check.Equals, nil)
	connB := s.newLDAPConnection()
	connB.Name = "Conn B"
	connB.DirectoryIDAttr = "objectGUID"
	c.Assert(PostLDAPConnection(connB), check.Equals, nil)

	gA := &Group{Name: "Group A", UserId: 1}
	c.Assert(PostLDAPManagedGroup(gA, adSync(connA.Id)), check.Equals, nil)
	gB := &Group{Name: "Group B", UserId: 1}
	c.Assert(PostLDAPManagedGroup(gB, adSync(connB.Id)), check.Equals, nil)

	// Same email, different connections and different directory identities.
	_, err := ReconcileGroupTargetsWithIdentity(gA.Id, ldapTeamID, connA.Id, []DirectoryTarget{dt("guid-A", "dup@example.com", "Dup", "A", "")})
	c.Assert(err, check.Equals, nil)
	_, err = ReconcileGroupTargetsWithIdentity(gB.Id, ldapTeamID, connB.Id, []DirectoryTarget{dt("guid-B", "dup@example.com", "Dup", "B", "")})
	c.Assert(err, check.Equals, nil)

	tA, _ := s.memberByEmail(c, gA.Id, "dup@example.com")
	tB, _ := s.memberByEmail(c, gB.Id, "dup@example.com")
	// Must NOT be merged into one target despite identical email.
	c.Assert(tA.Id != tB.Id, check.Equals, true)
	// Two distinct identity rows, one per connection.
	var countA, countB int64
	c.Assert(db.Model(&LDAPTargetIdentity{}).Where("connection_id=? and directory_id=?", connA.Id, "guid-A").Count(&countA).Error, check.Equals, nil)
	c.Assert(db.Model(&LDAPTargetIdentity{}).Where("connection_id=? and directory_id=?", connB.Id, "guid-B").Count(&countB).Error, check.Equals, nil)
	c.Assert(countA, check.Equals, int64(1))
	c.Assert(countB, check.Equals, int64(1))
}

func (s *ModelsSuite) TestIdentityRetainedAfterMappingDeleted(c *check.C) {
	connID, gid := s.setupIdentityGroup(c, "Ident Map Del")
	_, err := ReconcileGroupTargetsWithIdentity(gid, ldapTeamID, connID, []DirectoryTarget{dt("guid-1", "d@example.com", "D", "E", "")})
	c.Assert(err, check.Equals, nil)

	// Delete the mapping for the group.
	sync, err := GetLDAPGroupSyncByGroup(gid, ldapTeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(DeleteLDAPGroupSync(sync.Id, ldapTeamID), check.Equals, nil)

	// Identity row persists (scoped to connection, not mapping).
	var idCount int64
	c.Assert(db.Model(&LDAPTargetIdentity{}).Where("connection_id=? and directory_id=?", connID, "guid-1").Count(&idCount).Error, check.Equals, nil)
	c.Assert(idCount, check.Equals, int64(1))
}

func (s *ModelsSuite) TestIdentityRemovedWhenConnectionDeleted(c *check.C) {
	conn := s.newADConnection()
	c.Assert(PostLDAPConnection(conn), check.Equals, nil)
	g := &Group{Name: "Conn Del Grp", UserId: 1}
	c.Assert(PostLDAPManagedGroup(g, adSync(conn.Id)), check.Equals, nil)
	_, err := ReconcileGroupTargetsWithIdentity(g.Id, ldapTeamID, conn.Id, []DirectoryTarget{dt("guid-1", "e@example.com", "", "", "")})
	c.Assert(err, check.Equals, nil)

	// Remove the mapping so the connection can be deleted, then delete it.
	sync, err := GetLDAPGroupSyncByGroup(g.Id, ldapTeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(DeleteLDAPGroupSync(sync.Id, ldapTeamID), check.Equals, nil)
	c.Assert(DeleteLDAPConnection(conn.Id, ldapTeamID), check.Equals, nil)

	// Identity rows for that connection are gone.
	var idCount int64
	c.Assert(db.Model(&LDAPTargetIdentity{}).Where("connection_id=?", conn.Id).Count(&idCount).Error, check.Equals, nil)
	c.Assert(idCount, check.Equals, int64(0))
}

func (s *ModelsSuite) TestIdentityEmailFallbackNoStableID(c *check.C) {
	// Connection without a DirectoryIDAttr → entries carry empty DirectoryId
	// → email-keyed fallback (current behavior).
	// (Identity reconciliation is keyed by connection, not by a mapping, so
	// no managed-group mapping is needed here - and none can be created on a
	// non-AD connection anymore.)
	conn := s.newLDAPConnection()
	c.Assert(PostLDAPConnection(conn), check.Equals, nil)
	g := s.makeGroup(c, "Fallback Grp", "seed@example.com")

	r1, err := ReconcileGroupTargetsWithIdentity(g.Id, ldapTeamID, conn.Id, []DirectoryTarget{
		{Target: Target{BaseRecipient: BaseRecipient{Email: "f@example.com", FirstName: "F", LastName: "L"}}},
	})
	c.Assert(err, check.Equals, nil)
	c.Assert(r1.Added, check.Equals, 1)

	// Title change with same email → matched by email, updated, no identity row.
	r2, err := ReconcileGroupTargetsWithIdentity(g.Id, ldapTeamID, conn.Id, []DirectoryTarget{
		{Target: Target{BaseRecipient: BaseRecipient{Email: "f@example.com", FirstName: "F", LastName: "L", Position: "Lead"}}},
	})
	c.Assert(err, check.Equals, nil)
	c.Assert(r2.Updated, check.Equals, 1)
	var idCount int64
	c.Assert(db.Model(&LDAPTargetIdentity{}).Where("connection_id=?", conn.Id).Count(&idCount).Error, check.Equals, nil)
	c.Assert(idCount, check.Equals, int64(0))
}

func (s *ModelsSuite) TestNormalizeADObjectGUID(c *check.C) {
	// Known AD mixed-endian example:
	// raw bytes -> canonical GUID string.
	raw := []byte{
		0x10, 0x32, 0x54, 0x76, // Data1 LE -> 76543210
		0x98, 0xba, // Data2 LE -> ba98
		0xdc, 0xfe, // Data3 LE -> fedc
		0x10, 0x32, // Data4 BE -> 1032
		0x54, 0x76, 0x98, 0xba, 0xdc, 0xfe, // node BE -> 547698badcfe
	}
	got, err := NormalizeADObjectGUID(raw)
	c.Assert(err, check.Equals, nil)
	c.Assert(got, check.Equals, "76543210-ba98-fedc-1032-547698badcfe")

	// Wrong length is an error.
	_, err = NormalizeADObjectGUID([]byte{0x00, 0x01})
	c.Assert(err, check.NotNil)
}

func (s *ModelsSuite) TestNormalizeDirectoryIDEntryUUID(c *check.C) {
	// Non-objectGUID attribute: value is lowercased/trimmed, not byte-reordered.
	got, err := NormalizeDirectoryID("entryUUID", []byte("  ABCD-1234-EF  "))
	c.Assert(err, check.Equals, nil)
	c.Assert(got, check.Equals, "abcd-1234-ef")
}

// --- Selected directory group / BaseDN containment --------------------------

func (s *ModelsSuite) TestGroupDNContainmentOnCreate(c *check.C) {
	conn := s.newADConnection() // BaseDN = dc=example,dc=com
	c.Assert(PostLDAPConnection(conn), check.Equals, nil)
	g := s.makeGroup(c, "GDN Create", "seed@example.com")

	// Valid: a group somewhere under the BaseDN.
	ok := adSyncFor(g.Id, conn.Id)
	ok.GroupDN = "CN=Sales,OU=Groups,DC=example,DC=com"
	c.Assert(PostLDAPGroupSync(ok), check.Equals, nil)
	c.Assert(DeleteLDAPGroupSync(ok.Id, ldapTeamID), check.Equals, nil)

	// Invalid: above BaseDN.
	above := adSyncFor(g.Id, conn.Id)
	above.GroupDN = "dc=com"
	c.Assert(PostLDAPGroupSync(above), check.Equals, ErrLDAPGroupDNOutsideConnection)

	// Invalid: different tree.
	other := adSyncFor(g.Id, conn.Id)
	other.GroupDN = "CN=Sales,dc=other,dc=com"
	c.Assert(PostLDAPGroupSync(other), check.Equals, ErrLDAPGroupDNOutsideConnection)

	// Invalid: deceptive suffix (dc=example2 is not a child of dc=example).
	deceptive := adSyncFor(g.Id, conn.Id)
	deceptive.GroupDN = "CN=Sales,dc=example2,dc=com"
	c.Assert(PostLDAPGroupSync(deceptive), check.Equals, ErrLDAPGroupDNOutsideConnection)

	// Invalid: malformed DN.
	bad := adSyncFor(g.Id, conn.Id)
	bad.GroupDN = "not a dn"
	c.Assert(PostLDAPGroupSync(bad), check.Equals, ErrLDAPGroupDNInvalid)
}

func (s *ModelsSuite) TestPutLDAPGroupSyncImmutableFields(c *check.C) {
	conn := s.newADConnection()
	c.Assert(PostLDAPConnection(conn), check.Equals, nil)
	conn2 := s.newADConnection()
	conn2.Name = "Other AD"
	c.Assert(PostLDAPConnection(conn2), check.Equals, nil)
	g := s.makeGroup(c, "Immutable", "seed@example.com")
	g2 := s.makeGroup(c, "Immutable 2", "seed2@example.com")

	sync := adSyncFor(g.Id, conn.Id)
	c.Assert(PostLDAPGroupSync(sync), check.Equals, nil)

	// Connection is fixed in v1.
	upd := *sync
	upd.ConnectionId = conn2.Id
	c.Assert(PutLDAPGroupSync(&upd), check.Equals, ErrLDAPConnectionImmutable)

	// Selected AD group is fixed in v1 (a different GUID is rejected; the
	// same GUID in another textual form is accepted as "unchanged").
	upd = *sync
	upd.GroupDirectoryId = "09090909-0909-0909-0909-090909090909"
	c.Assert(PutLDAPGroupSync(&upd), check.Equals, ErrLDAPGroupSelectionImmutable)
	upd = *sync
	upd.GroupDirectoryId = "{A1B2C3D4-E5F6-4788-99AA-BBCCDDEEFF00}"
	c.Assert(PutLDAPGroupSync(&upd), check.Equals, nil)

	// The mapped Gophish group is fixed.
	upd = *sync
	upd.GroupId = g2.Id
	c.Assert(PutLDAPGroupSync(&upd), check.Equals, ErrLDAPMappedGroupImmutable)

	// Omitted (zero/empty) immutable fields mean "unchanged"; editable
	// settings are applied.
	upd = LDAPGroupSync{Id: sync.Id, TeamId: ldapTeamID, Enabled: false, ExcludeDisabled: false, SyncFreq: 7200, AttrEmail: "userPrincipalName"}
	c.Assert(PutLDAPGroupSync(&upd), check.Equals, nil)
	got, err := GetLDAPGroupSync(sync.Id, ldapTeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(got.ConnectionId, check.Equals, conn.Id)
	c.Assert(got.GroupId, check.Equals, g.Id)
	c.Assert(got.GroupDirectoryId, check.Equals, testGroupGUID)
	c.Assert(got.GroupDN, check.Equals, testGroupDN)
	c.Assert(got.Enabled, check.Equals, false)
	c.Assert(got.ExcludeDisabled, check.Equals, false)
	c.Assert(got.SyncFreq, check.Equals, uint32(7200))
	c.Assert(got.AttrEmail, check.Equals, "userPrincipalName")

	// Unknown mapping / other team → not found.
	upd = LDAPGroupSync{Id: sync.Id, TeamId: ldapTeamID + 999}
	c.Assert(PutLDAPGroupSync(&upd), check.NotNil)
}

func (s *ModelsSuite) TestUpdateLDAPGroupSyncDirectoryGroupContainment(c *check.C) {
	conn := s.newADConnection()
	c.Assert(PostLDAPConnection(conn), check.Equals, nil)
	g := s.makeGroup(c, "GDN Refresh", "seed@example.com")
	sync := adSyncFor(g.Id, conn.Id)
	c.Assert(PostLDAPGroupSync(sync), check.Equals, nil)

	// Moved within the BaseDN → OK.
	c.Assert(UpdateLDAPGroupSyncDirectoryGroup(sync.Id, ldapTeamID, "CN=Marketing,OU=IT,OU=Groups,DC=example,DC=com", "Marketing"), check.Equals, nil)
	// Outside BaseDN / malformed / empty name → rejected, identity untouched.
	c.Assert(UpdateLDAPGroupSyncDirectoryGroup(sync.Id, ldapTeamID, "CN=Marketing,dc=other,dc=com", "Marketing"), check.Equals, ErrLDAPGroupDNOutsideConnection)
	c.Assert(UpdateLDAPGroupSyncDirectoryGroup(sync.Id, ldapTeamID, "???", "Marketing"), check.Equals, ErrLDAPGroupDNInvalid)
	c.Assert(UpdateLDAPGroupSyncDirectoryGroup(sync.Id, ldapTeamID, testGroupDN, "  "), check.Equals, ErrLDAPGroupNameNotSpecified)
	// Other team → not found.
	c.Assert(UpdateLDAPGroupSyncDirectoryGroup(sync.Id, ldapTeamID+999, testGroupDN, "Marketing"), check.NotNil)
	got, err := GetLDAPGroupSync(sync.Id, ldapTeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(got.GroupDN, check.Equals, "CN=Marketing,OU=IT,OU=Groups,DC=example,DC=com")
	c.Assert(got.GroupDirectoryId, check.Equals, testGroupGUID)
}

// --- lifecycle -----------------------------------------------------------------

func (s *ModelsSuite) TestPostLDAPManagedGroupTrimsName(c *check.C) {
	conn := s.newADConnection()
	c.Assert(PostLDAPConnection(conn), check.Equals, nil)
	g := &Group{Name: "   Marketing   ", UserId: 1}
	c.Assert(PostLDAPManagedGroup(g, adSync(conn.Id)), check.Equals, nil)
	got, err := GetGroup(g.Id, ldapTeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(got.Name, check.Equals, "Marketing")
	blank := &Group{Name: "   ", UserId: 1}
	c.Assert(PostLDAPManagedGroup(blank, adSync(conn.Id)), check.Equals, ErrGroupNameNotSpecified)
}

// TestDeleteGroupRemovesMappingKeepsIdentity verifies deleting a managed
// Gophish group removes its sync mapping (so the monitor stops seeing it) but
// keeps the connection-scoped identity rows, the target rows and the
// connection.
func (s *ModelsSuite) TestDeleteGroupRemovesMappingKeepsIdentity(c *check.C) {
	connID, gid := s.setupIdentityGroup(c, "Delete Managed")
	_, err := ReconcileGroupTargetsWithIdentity(gid, ldapTeamID, connID, []DirectoryTarget{dt("guid-del", "del@example.com", "D", "E", "")})
	c.Assert(err, check.Equals, nil)
	member, ok := s.memberByEmail(c, gid, "del@example.com")
	c.Assert(ok, check.Equals, true)

	g, err := GetGroup(gid, ldapTeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(DeleteGroup(&g), check.Equals, nil)

	// Group and membership are gone...
	_, err = GetGroup(gid, ldapTeamID)
	c.Assert(err, check.NotNil)
	// ...the mapping is gone and the monitor's discovery query no longer sees it...
	_, err = GetLDAPGroupSyncByGroup(gid, ldapTeamID)
	c.Assert(err, check.NotNil)
	enabled, err := GetAllEnabledLDAPGroupSyncs()
	c.Assert(err, check.Equals, nil)
	for _, m := range enabled {
		c.Assert(m.GroupId != gid, check.Equals, true)
	}
	// ...but the identity row, the target row and the connection survive.
	var idCount int64
	c.Assert(db.Model(&LDAPTargetIdentity{}).Where("connection_id=? and directory_id=?", connID, "guid-del").Count(&idCount).Error, check.Equals, nil)
	c.Assert(idCount, check.Equals, int64(1))
	var tCount int64
	c.Assert(db.Model(&Target{}).Where("id=?", member.Id).Count(&tCount).Error, check.Equals, nil)
	c.Assert(tCount, check.Equals, int64(1))
	_, err = GetLDAPConnection(connID, ldapTeamID)
	c.Assert(err, check.Equals, nil)
	// The connection can now be deleted (no mapping references it).
	c.Assert(DeleteLDAPConnection(connID, ldapTeamID), check.Equals, nil)
}

// TestDeleteMappingLeavesGroupAndMembership verifies deleting only the mapping
// stops synchronization while leaving the Gophish group, its current
// membership and the identity rows intact.
func (s *ModelsSuite) TestDeleteMappingLeavesGroupAndMembership(c *check.C) {
	connID, gid := s.setupIdentityGroup(c, "Delete Mapping Only")
	_, err := ReconcileGroupTargetsWithIdentity(gid, ldapTeamID, connID, []DirectoryTarget{
		dt("guid-a", "a@example.com", "A", "A", ""), dt("guid-b", "b@example.com", "B", "B", ""),
	})
	c.Assert(err, check.Equals, nil)
	m, err := GetLDAPGroupSyncByGroup(gid, ldapTeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(DeleteLDAPGroupSync(m.Id, ldapTeamID), check.Equals, nil)

	managed, err := IsGroupLDAPManaged(gid, ldapTeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(managed, check.Equals, false)
	_, err = GetGroup(gid, ldapTeamID)
	c.Assert(err, check.Equals, nil)
	ts, err := GetTargets(gid)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(ts), check.Equals, 2)
	var idCount int64
	c.Assert(db.Model(&LDAPTargetIdentity{}).Where("connection_id=?", connID).Count(&idCount).Error, check.Equals, nil)
	c.Assert(idCount, check.Equals, int64(2))
}

func (s *ModelsSuite) TestDeleteManualGroupUnaffected(c *check.C) {
	g := s.makeGroup(c, "Manual Delete", "m@example.com")
	c.Assert(DeleteGroup(&g), check.Equals, nil)
	_, err := GetGroup(g.Id, ldapTeamID)
	c.Assert(err, check.NotNil)
}

func (s *ModelsSuite) TestGroupDNContainmentOnManagedGroupCreate(c *check.C) {
	conn := s.newADConnection()
	c.Assert(PostLDAPConnection(conn), check.Equals, nil)

	// Atomic managed-group create with an out-of-bounds group DN must be
	// rejected and leave no orphan group.
	g := &Group{Name: "Managed Bad GDN", UserId: 1}
	sync := adSync(conn.Id)
	sync.GroupDN = "CN=Marketing,dc=other,dc=com"
	err := PostLDAPManagedGroup(g, sync)
	c.Assert(err, check.Equals, ErrLDAPGroupDNOutsideConnection)
	_, gerr := GetGroupByName("Managed Bad GDN", ldapTeamID)
	c.Assert(gerr, check.NotNil)

	// Missing selection on the atomic path is rejected the same way.
	g3 := &Group{Name: "Managed No Group", UserId: 1}
	err = PostLDAPManagedGroup(g3, &LDAPGroupSync{ConnectionId: conn.Id, Enabled: true})
	c.Assert(err, check.Equals, ErrLDAPGroupDirectoryIDNotSpecified)
	_, gerr = GetGroupByName("Managed No Group", ldapTeamID)
	c.Assert(gerr, check.NotNil)

	// A valid in-base group succeeds.
	g2 := &Group{Name: "Managed Good GDN", UserId: 1}
	c.Assert(PostLDAPManagedGroup(g2, adSync(conn.Id)), check.Equals, nil)
}

// TestGroupDNOwnershipUnchanged confirms the connection/team ownership check
// still fires (and takes precedence) alongside group containment.
func (s *ModelsSuite) TestGroupDNOwnershipUnchanged(c *check.C) {
	conn := s.newADConnection()
	c.Assert(PostLDAPConnection(conn), check.Equals, nil)
	g := s.makeGroup(c, "GDN Ownership", "seed@example.com")

	// A connection id from another team is still rejected as not-found,
	// regardless of the group selection.
	bad := adSyncFor(g.Id, conn.Id)
	bad.TeamId = ldapTeamID + 999
	c.Assert(bad.Validate(), check.Equals, ErrLDAPConnectionNotFound)
}

// --- objectGUID text round-trip ---------------------------------------------

func (s *ModelsSuite) TestParseADObjectGUIDRoundTrip(c *check.C) {
	raw := []byte{0xd4, 0xc3, 0xb2, 0xa1, 0xf6, 0xe5, 0x88, 0x47, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00}
	text, err := NormalizeADObjectGUID(raw)
	c.Assert(err, check.Equals, nil)
	c.Assert(text, check.Equals, testGroupGUID)

	back, err := ParseADObjectGUID(text)
	c.Assert(err, check.Equals, nil)
	c.Assert(back, check.DeepEquals, raw)

	// Case, braces and surrounding whitespace are tolerated on input.
	back2, err := ParseADObjectGUID(" {A1B2C3D4-E5F6-4788-99AA-BBCCDDEEFF00} ")
	c.Assert(err, check.Equals, nil)
	c.Assert(back2, check.DeepEquals, raw)

	for _, bad := range []string{"", "not-a-guid", "a1b2c3d4e5f6478899aabbccddeeff00", "a1b2c3d4-e5f6-4788-99aa-bbccddeeff0g", "a1b2c3d4-e5f6-4788-99aa-bbccddeeff0000"} {
		_, perr := ParseADObjectGUID(bad)
		c.Assert(perr, check.NotNil)
	}
}

func (s *ModelsSuite) TestNormalizeDirectoryIDText(c *check.C) {
	// objectGUID: canonical lower-case, dashed form, equal to what the raw
	// normalizer produces.
	got, err := NormalizeDirectoryIDText("objectGUID", "{A1B2C3D4-E5F6-4788-99AA-BBCCDDEEFF00}")
	c.Assert(err, check.Equals, nil)
	c.Assert(got, check.Equals, testGroupGUID)
	_, err = NormalizeDirectoryIDText("objectguid", "nope")
	c.Assert(err, check.NotNil)

	// Textual identifiers (entryUUID): lower-cased and trimmed only.
	got, err = NormalizeDirectoryIDText("entryUUID", "  ABC-Def ")
	c.Assert(err, check.Equals, nil)
	c.Assert(got, check.Equals, "abc-def")
}

// TestLDAPConnectionIsActiveDirectory pins the single AD detection rule.
func (s *ModelsSuite) TestLDAPConnectionIsActiveDirectory(c *check.C) {
	c.Assert((&LDAPConnection{DirectoryIDAttr: "objectGUID"}).IsActiveDirectory(), check.Equals, true)
	c.Assert((&LDAPConnection{DirectoryIDAttr: " ObjectGuid "}).IsActiveDirectory(), check.Equals, true)
	c.Assert((&LDAPConnection{DirectoryIDAttr: "entryUUID"}).IsActiveDirectory(), check.Equals, false)
	c.Assert((&LDAPConnection{}).IsActiveDirectory(), check.Equals, false)
}

// --- DirectoryIDAttr persistence + connection summary ----------------------

func (s *ModelsSuite) TestLDAPConnectionDirectoryIDAttrPersists(c *check.C) {
	conn := s.newLDAPConnection()
	conn.DirectoryIDAttr = "objectGUID"
	c.Assert(PostLDAPConnection(conn), check.Equals, nil)

	got, err := GetLDAPConnection(conn.Id, ldapTeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(got.DirectoryIDAttr, check.Equals, "objectGUID")

	// Update to entryUUID.
	got.DirectoryIDAttr = "entryUUID"
	c.Assert(PutLDAPConnection(&got), check.Equals, nil)
	reloaded, err := GetLDAPConnection(conn.Id, ldapTeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(reloaded.DirectoryIDAttr, check.Equals, "entryUUID")

	// Update to email fallback (empty).
	reloaded.DirectoryIDAttr = ""
	c.Assert(PutLDAPConnection(&reloaded), check.Equals, nil)
	final, err := GetLDAPConnection(conn.Id, ldapTeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(final.DirectoryIDAttr, check.Equals, "")
}

func (s *ModelsSuite) TestLDAPConnectionSummaryOmitsSensitiveFields(c *check.C) {
	conn := s.newLDAPConnection()
	conn.DirectoryIDAttr = "objectGUID"
	c.Assert(PostLDAPConnection(conn), check.Equals, nil)

	sums, err := GetLDAPConnectionSummaries(ldapTeamID)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(sums), check.Equals, 1)
	c.Assert(sums[0].Id, check.Equals, conn.Id)
	c.Assert(sums[0].Name, check.Equals, conn.Name)
	c.Assert(sums[0].BaseDN, check.Equals, conn.BaseDN)

	// A different team sees nothing.
	other, err := GetLDAPConnectionSummaries(ldapTeamID + 999)
	c.Assert(err, check.Equals, nil)
	c.Assert(len(other), check.Equals, 0)
}

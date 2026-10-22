package ldap

import (
	"errors"
	"strings"
	"testing"

	ldapv3 "github.com/go-ldap/ldap/v3"

	"github.com/raginx/gophish-ng/config"
	"github.com/raginx/gophish-ng/models"
)

// End-to-end synchronization tests: an in-memory fake Active Directory
// (resolve-by-GUID + memberOf member search) in front of the real models
// layer on an in-memory SQLite database, so resolve → read → reconcile runs
// exactly as in production minus the network.

const (
	syncGroupDN   = "CN=Marketing,OU=Groups," + testBaseDN
	syncGroupName = "Marketing"
)

// fakeMember is one directory object that may show up in a member search.
type fakeMember struct {
	dn       string
	guid     []byte
	memberOf []string
	disabled bool
	group    bool // a nested group that is itself a direct member
	attrs    map[string]string
}

// fakeDir answers exactly the two queries the sync issues: the GUID lookup
// (Search) and the memberOf member search (SearchWithPaging). It evaluates
// the memberOf / objectCategory=person / disabled clauses the way AD would,
// and only returns the attributes the request asked for.
type fakeDir struct {
	groups    map[string]*ldapv3.Entry // raw GUID -> group entry
	members   []fakeMember
	pagedErr  error
	pagedReqs []*ldapv3.SearchRequest
}

func (f *fakeDir) Search(req *ldapv3.SearchRequest) (*ldapv3.SearchResult, error) {
	res := &ldapv3.SearchResult{}
	raw, ok := guidFromFilter(req.Filter)
	if !ok {
		return res, nil
	}
	if e, found := f.groups[string(raw)]; found {
		res.Entries = append(res.Entries, e)
	}
	return res, nil
}

func (f *fakeDir) SearchWithPaging(req *ldapv3.SearchRequest, _ uint32) (*ldapv3.SearchResult, error) {
	f.pagedReqs = append(f.pagedReqs, req)
	if f.pagedErr != nil {
		return nil, f.pagedErr
	}
	const marker = "(memberOf="
	i := strings.Index(req.Filter, marker)
	if i < 0 {
		return &ldapv3.SearchResult{}, nil
	}
	rest := req.Filter[i+len(marker):]
	groupDN := rest[:strings.Index(rest, ")")]
	excludeDisabled := strings.Contains(req.Filter, adDisabledClause)
	personOnly := strings.Contains(req.Filter, "(objectCategory=person)")
	res := &ldapv3.SearchResult{}
	for _, m := range f.members {
		direct := false
		for _, g := range m.memberOf {
			if strings.EqualFold(g, groupDN) {
				direct = true
			}
		}
		if !direct || (m.group && personOnly) || (excludeDisabled && m.disabled) {
			continue
		}
		e := &ldapv3.Entry{DN: m.dn}
		for _, a := range req.Attributes {
			if strings.EqualFold(a, "objectGUID") {
				if m.guid != nil {
					e.Attributes = append(e.Attributes, &ldapv3.EntryAttribute{Name: a, Values: []string{string(m.guid)}, ByteValues: [][]byte{m.guid}})
				}
				continue
			}
			if v, ok := m.attrs[a]; ok {
				e.Attributes = append(e.Attributes, &ldapv3.EntryAttribute{Name: a, Values: []string{v}, ByteValues: [][]byte{[]byte(v)}})
			}
		}
		res.Entries = append(res.Entries, e)
	}
	return res, nil
}

func guid(n byte) []byte {
	g := make([]byte, 16)
	g[0] = n
	g[15] = n
	return g
}

func user(n byte, dn, email, first, last string) fakeMember {
	return fakeMember{dn: dn, guid: guid(n), memberOf: []string{syncGroupDN}, attrs: map[string]string{"mail": email, "givenName": first, "sn": last, "title": "Staff", "userPrincipalName": strings.ToLower(first) + "@corp.example"}}
}

func newFakeDir(members ...fakeMember) *fakeDir {
	return &fakeDir{
		groups:  map[string]*ldapv3.Entry{string(testGUIDRaw): groupEntry(syncGroupDN, syncGroupName, testGUIDRaw, "")},
		members: members,
	}
}

// setupSync creates an in-memory database, an AD connection and a managed
// group mapped to the test group, and returns them.
func setupSync(t *testing.T, name string) (*models.LDAPConnection, *models.Group, *models.LDAPGroupSync) {
	t.Helper()
	conf := &config.Config{
		DBName: "sqlite3", DBPath: ":memory:", MigrationsPath: "../db/db_sqlite3/migrations/",
		SecretKey: "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY=",
	}
	if err := models.Setup(conf); err != nil {
		t.Fatalf("models setup: %v", err)
	}
	conn := &models.LDAPConnection{UserId: 1, TeamId: 1, Name: "Corp AD", ServerURL: "ldaps://ad.example.com:636",
		BindDN: "cn=svc", BindPassword: "s3cret", BaseDN: testBaseDN, DirectoryIDAttr: "objectGUID"}
	if err := models.PostLDAPConnection(conn); err != nil {
		t.Fatalf("post connection: %v", err)
	}
	g := &models.Group{Name: name, UserId: 1}
	s := &models.LDAPGroupSync{ConnectionId: conn.Id, Enabled: true, ExcludeDisabled: true,
		GroupDirectoryId: testGUIDText, GroupDN: syncGroupDN, GroupName: syncGroupName}
	if err := models.PostLDAPManagedGroup(g, s); err != nil {
		t.Fatalf("post managed group: %v", err)
	}
	return conn, g, s
}

func membersByEmail(t *testing.T, groupID int64) map[string]models.Target {
	t.Helper()
	ts, err := models.GetTargets(groupID)
	if err != nil {
		t.Fatalf("get targets: %v", err)
	}
	out := map[string]models.Target{}
	for _, x := range ts {
		out[x.Email] = x
	}
	return out
}

func reloadSync(t *testing.T, id int64) models.LDAPGroupSync {
	t.Helper()
	s, err := models.GetLDAPGroupSyncByID(id)
	if err != nil {
		t.Fatalf("reload mapping: %v", err)
	}
	return s
}

// --- membership semantics ---------------------------------------------------

func TestSyncDirectUsersOnlyNoNestedExpansion(t *testing.T) {
	conn, g, s := setupSync(t, "Direct Only")
	nestedDN := "CN=Nested,OU=Groups," + testBaseDN
	dir := newFakeDir(
		user(1, "CN=Ann,OU=Staff,"+testBaseDN, "ann@example.com", "Ann", "A"),
		user(2, "CN=Bob,OU=Staff,"+testBaseDN, "bob@example.com", "Bob", "B"),
		// A nested group that is a DIRECT member of the selected group...
		fakeMember{dn: nestedDN, guid: guid(9), memberOf: []string{syncGroupDN}, group: true},
		// ...and its own user, who is NOT a direct member of the selected group.
		fakeMember{dn: "CN=Nested User,OU=Staff," + testBaseDN, guid: guid(3), memberOf: []string{nestedDN},
			attrs: map[string]string{"mail": "nested@example.com", "givenName": "Nes", "sn": "Ted"}},
	)
	res, err := syncWith(dir, conn, s)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Discovered != 2 || res.Added != 2 || res.Skipped != 0 {
		t.Fatalf("unexpected result: %+v", res)
	}
	got := membersByEmail(t, g.Id)
	if len(got) != 2 {
		t.Fatalf("expected 2 direct members, got %d: %v", len(got), got)
	}
	if _, nested := got["nested@example.com"]; nested {
		t.Fatal("nested group member was expanded into the Gophish group")
	}
	if len(dir.pagedReqs) != 1 {
		t.Fatalf("expected exactly one member search, got %d", len(dir.pagedReqs))
	}
	if f := dir.pagedReqs[0].Filter; strings.Contains(f, "1.2.840.113556.1.4.1941") || !strings.Contains(f, "(memberOf="+syncGroupDN+")") {
		t.Fatalf("member search must use plain memberOf without matching-rule-in-chain: %s", f)
	}
	st := reloadSync(t, s.Id)
	if st.LastSyncStatus != "ok" || st.LastSyncAdded != 2 {
		t.Fatalf("sync status not recorded: %+v", st)
	}
}

func TestSyncExcludeDisabledTrueThenFalse(t *testing.T) {
	conn, g, s := setupSync(t, "Disabled")
	off := user(2, "CN=Dis,OU=Staff,"+testBaseDN, "dis@example.com", "Dis", "Abled")
	off.disabled = true
	dir := newFakeDir(user(1, "CN=On,OU=Staff,"+testBaseDN, "on@example.com", "On", "Line"), off)

	s.ExcludeDisabled = true
	if _, err := syncWith(dir, conn, s); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := membersByEmail(t, g.Id); len(got) != 1 || got["on@example.com"].Id == 0 {
		t.Fatalf("exclude_disabled=true: expected only the enabled user, got %v", got)
	}
	if !strings.Contains(dir.pagedReqs[0].Filter, adDisabledClause) {
		t.Fatal("exclude_disabled=true must add the AD disabled clause")
	}

	s.ExcludeDisabled = false
	res, err := syncWith(dir, conn, s)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Added != 1 || res.Updated != 1 {
		t.Fatalf("exclude_disabled=false: expected +1 ~1, got %+v", res)
	}
	if got := membersByEmail(t, g.Id); len(got) != 2 {
		t.Fatalf("exclude_disabled=false: expected both users, got %v", got)
	}
	if strings.Contains(dir.pagedReqs[1].Filter, adDisabledClause) {
		t.Fatal("exclude_disabled=false must not add the AD disabled clause")
	}
}

func TestSyncUserPrincipalNameAsEmailAttribute(t *testing.T) {
	conn, g, s := setupSync(t, "UPN")
	s.AttrEmail = "userPrincipalName"
	dir := newFakeDir(user(1, "CN=Ann,OU=Staff,"+testBaseDN, "", "Ann", "A")) // mail empty, UPN set
	res, err := syncWith(dir, conn, s)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if res.Added != 1 || res.Skipped != 0 {
		t.Fatalf("UPN mapping should work through the single-valued path: %+v", res)
	}
	if got := membersByEmail(t, g.Id); got["ann@corp.example"].Id == 0 {
		t.Fatalf("expected UPN-derived target, got %v", got)
	}
}

// --- group rename / move / deletion -----------------------------------------

func TestSyncGroupRenamedAndMovedSameGUIDContinues(t *testing.T) {
	conn, g, s := setupSync(t, "Renamed")
	dir := newFakeDir(user(1, "CN=Ann,OU=Staff,"+testBaseDN, "ann@example.com", "Ann", "A"))
	if _, err := syncWith(dir, conn, s); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// The AD group is renamed AND moved; same objectGUID. Members now carry the
	// new DN in memberOf.
	newDN := "CN=Marketing Team,OU=Moved," + testBaseDN
	dir.groups[string(testGUIDRaw)] = groupEntry(newDN, "Marketing Team", testGUIDRaw, "")
	dir.members[0].memberOf = []string{newDN}
	dir.members = append(dir.members, fakeMember{dn: "CN=Bob,OU=Staff," + testBaseDN, guid: guid(2), memberOf: []string{newDN},
		attrs: map[string]string{"mail": "bob@example.com", "givenName": "Bob", "sn": "B"}})

	res, err := syncWith(dir, conn, s)
	if err != nil {
		t.Fatalf("sync after rename/move: %v", err)
	}
	if res.Added != 1 || res.Updated != 1 || res.Removed != 0 {
		t.Fatalf("unexpected result after rename/move: %+v", res)
	}
	st := reloadSync(t, s.Id)
	if st.GroupDirectoryId != testGUIDText {
		t.Fatalf("group identity must not change: %q", st.GroupDirectoryId)
	}
	if st.GroupDN != newDN || st.GroupName != "Marketing Team" {
		t.Fatalf("cached DN/name not refreshed from the server: dn=%q name=%q", st.GroupDN, st.GroupName)
	}
	// The member search targeted the NEW server-observed DN, not the cache.
	if !strings.Contains(dir.pagedReqs[1].Filter, "(memberOf="+newDN+")") {
		t.Fatalf("member search must use the resolved DN: %s", dir.pagedReqs[1].Filter)
	}
	// The Gophish group name is independent and untouched.
	gg, _ := models.GetGroup(g.Id, 1)
	if gg.Name != "Renamed" {
		t.Fatalf("Gophish group must not be renamed, got %q", gg.Name)
	}
}

func TestSyncDeletedGroupIsErrorNotEmpty(t *testing.T) {
	conn, g, s := setupSync(t, "Deleted")
	dir := newFakeDir(
		user(1, "CN=Ann,OU=Staff,"+testBaseDN, "ann@example.com", "Ann", "A"),
		user(2, "CN=Bob,OU=Staff,"+testBaseDN, "bob@example.com", "Bob", "B"),
	)
	if _, err := syncWith(dir, conn, s); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	// The group is deleted in AD (and a same-name group recreated with another
	// GUID, which must not be picked up either).
	delete(dir.groups, string(testGUIDRaw))
	other := guid(7)
	dir.groups[string(other)] = groupEntry(syncGroupDN, syncGroupName, other, "")

	_, err := syncWith(dir, conn, s)
	if !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("expected ErrGroupNotFound, got %v", err)
	}
	if got := membersByEmail(t, g.Id); len(got) != 2 {
		t.Fatalf("membership must be unchanged after a failed resolution, got %d members", len(got))
	}
	if len(dir.pagedReqs) != 1 {
		t.Fatalf("no member search may run when the group does not resolve (got %d)", len(dir.pagedReqs))
	}
	st := reloadSync(t, s.Id)
	if st.LastSyncStatus != "error" || !strings.Contains(st.LastSyncError, "not found") {
		t.Fatalf("failure not recorded: %+v", st)
	}
}

func TestSyncEmptyGroupReconcilesToZero(t *testing.T) {
	conn, g, s := setupSync(t, "Empty")
	dir := newFakeDir(user(1, "CN=Ann,OU=Staff,"+testBaseDN, "ann@example.com", "Ann", "A"))
	if _, err := syncWith(dir, conn, s); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	// The group still exists but genuinely has no direct user members.
	dir.members = nil
	res, err := syncWith(dir, conn, s)
	if err != nil {
		t.Fatalf("sync of empty group must succeed: %v", err)
	}
	if res.Discovered != 0 || res.Removed != 1 {
		t.Fatalf("expected reconciliation to zero, got %+v", res)
	}
	if got := membersByEmail(t, g.Id); len(got) != 0 {
		t.Fatalf("expected empty membership, got %v", got)
	}
	st := reloadSync(t, s.Id)
	if st.LastSyncStatus != "ok" || st.LastSyncRemoved != 1 {
		t.Fatalf("empty-group sync status: %+v", st)
	}
}

// --- failure safety ---------------------------------------------------------

func TestSyncMemberSearchErrorLeavesMembershipUnchanged(t *testing.T) {
	conn, g, s := setupSync(t, "SearchErr")
	dir := newFakeDir(user(1, "CN=Ann,OU=Staff,"+testBaseDN, "ann@example.com", "Ann", "A"))
	if _, err := syncWith(dir, conn, s); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	dir.pagedErr = ldapv3.NewError(ldapv3.LDAPResultOperationsError, errors.New("paging failed"))
	if _, err := syncWith(dir, conn, s); err == nil {
		t.Fatal("expected member search error")
	}
	if got := membersByEmail(t, g.Id); len(got) != 1 {
		t.Fatalf("membership must be unchanged after a failed member search, got %d", len(got))
	}
	if st := reloadSync(t, s.Id); st.LastSyncStatus != "error" {
		t.Fatalf("failure not recorded: %+v", st)
	}
}

func TestSyncLegacyMappingWithoutSelectedGroupFails(t *testing.T) {
	conn, g, s := setupSync(t, "Legacy")
	dir := newFakeDir(user(1, "CN=Ann,OU=Staff,"+testBaseDN, "ann@example.com", "Ann", "A"))
	// A pre-group-selection (OU-era) mapping row has no stable group id. It
	// must never fall back to any subtree search.
	s.GroupDirectoryId = ""
	_, err := syncWith(dir, conn, s)
	if !errors.Is(err, ErrGroupIdentityInvalid) {
		t.Fatalf("expected ErrGroupIdentityInvalid, got %v", err)
	}
	if len(dir.pagedReqs) != 0 {
		t.Fatal("no member search may run for a mapping without a selected group")
	}
	if got := membersByEmail(t, g.Id); len(got) != 0 {
		t.Fatalf("membership must be untouched, got %v", got)
	}
}

func TestSyncRejectsGenericConnection(t *testing.T) {
	_, _, s := setupSync(t, "Generic")
	generic := &models.LDAPConnection{UserId: 1, TeamId: 1, Name: "Generic", ServerURL: "ldap://127.0.0.1:1",
		BindDN: "cn=svc", BindPassword: "s3cret", BaseDN: testBaseDN}
	if err := models.PostLDAPConnection(generic); err != nil {
		t.Fatalf("post generic: %v", err)
	}
	// Force the mapping onto the generic connection at the row level (the
	// model forbids this via the API) and prove Sync refuses it before
	// dialing.
	s.ConnectionId = generic.Id
	if _, err := Sync(s); !errors.Is(err, ErrNotActiveDirectory) {
		t.Fatalf("expected ErrNotActiveDirectory, got %v", err)
	}
}

// --- missing / invalid email ------------------------------------------------

func TestSyncSkipsMissingAndInvalidEmail(t *testing.T) {
	conn, g, s := setupSync(t, "Skips")
	noMail := user(2, "CN=NoMail,OU=Staff,"+testBaseDN, "", "No", "Mail")
	badMail := user(3, "CN=BadMail,OU=Staff,"+testBaseDN, "not an address", "Bad", "Mail")
	dir := newFakeDir(user(1, "CN=Ann,OU=Staff,"+testBaseDN, "ann@example.com", "Ann", "A"), noMail, badMail)
	res, err := syncWith(dir, conn, s)
	if err != nil {
		t.Fatalf("sync must not fail on unmappable members: %v", err)
	}
	if res.Discovered != 3 || res.Added != 1 || res.Skipped != 2 {
		t.Fatalf("expected discovered=3 added=1 skipped=2, got %+v", res)
	}
	if got := membersByEmail(t, g.Id); len(got) != 1 {
		t.Fatalf("expected 1 mapped member, got %v", got)
	}
	st := reloadSync(t, s.Id)
	if st.LastSyncSkipped != 2 || st.LastSyncStatus != "ok" {
		t.Fatalf("skipped count not persisted: %+v", st)
	}
	// No member value leaks into the recorded status.
	if strings.Contains(st.LastSyncError, "NoMail") || strings.Contains(st.LastSyncError, "not an address") {
		t.Fatalf("status must not carry member values: %q", st.LastSyncError)
	}
}

// --- stable user identity ---------------------------------------------------

func TestSyncIdentityReuseAcrossMoveRemoveReadd(t *testing.T) {
	conn, g, s := setupSync(t, "Identity")
	dir := newFakeDir(user(1, "CN=Ann,OU=Staff,"+testBaseDN, "ann@example.com", "Ann", "A"))
	if _, err := syncWith(dir, conn, s); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	first := membersByEmail(t, g.Id)["ann@example.com"]
	if first.Id == 0 {
		t.Fatal("member not created")
	}

	// Moved to another OU (DN changes), renamed and email changed; same
	// objectGUID and still a direct member → same Target updated in place.
	dir.members[0].dn = "CN=Ann Smith,OU=Elsewhere," + testBaseDN
	dir.members[0].attrs["mail"] = "ann.smith@example.com"
	dir.members[0].attrs["sn"] = "Smith"
	res, err := syncWith(dir, conn, s)
	if err != nil {
		t.Fatalf("sync after move: %v", err)
	}
	if res.Added != 0 || res.Updated != 1 || res.Removed != 0 {
		t.Fatalf("move must be an in-place update: %+v", res)
	}
	moved := membersByEmail(t, g.Id)["ann.smith@example.com"]
	if moved.Id != first.Id || moved.LastName != "Smith" {
		t.Fatalf("expected same target %d updated, got %+v", first.Id, moved)
	}

	// Removed from the AD group → only the membership goes; target row and
	// identity row survive.
	dir.members[0].memberOf = nil
	res, err = syncWith(dir, conn, s)
	if err != nil || res.Removed != 1 {
		t.Fatalf("removal: res=%+v err=%v", res, err)
	}
	if got := membersByEmail(t, g.Id); len(got) != 0 {
		t.Fatalf("expected no members, got %v", got)
	}

	// Re-added to the AD group → the same identity/Target returns (which also
	// proves the target row and identity binding survived the removal).
	dir.members[0].memberOf = []string{syncGroupDN}
	res, err = syncWith(dir, conn, s)
	if err != nil || res.Added != 1 {
		t.Fatalf("re-add: res=%+v err=%v", res, err)
	}
	back := membersByEmail(t, g.Id)["ann.smith@example.com"]
	if back.Id != first.Id {
		t.Fatalf("re-added member must reuse target %d, got %d", first.Id, back.Id)
	}
}

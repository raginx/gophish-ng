package ldap

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	ldapv3 "github.com/go-ldap/ldap/v3"

	"github.com/raginx/gophish-ng/models"
)

// Canonical test GUID: textual form and the raw mixed-endian AD bytes.
const testGUIDText = "a1b2c3d4-e5f6-4788-99aa-bbccddeeff00"

var testGUIDRaw = []byte{0xd4, 0xc3, 0xb2, 0xa1, 0xf6, 0xe5, 0x88, 0x47, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00}

const testBaseDN = "DC=example,DC=com"

// recordingSearcher captures every Search request and replies with canned
// results, so group discovery/resolution can be proven to issue exactly one
// bounded search with the right shape, without a live directory.
type recordingSearcher struct {
	reqs    []*ldapv3.SearchRequest
	entries []*ldapv3.Entry
	err     error
}

func (r *recordingSearcher) Search(req *ldapv3.SearchRequest) (*ldapv3.SearchResult, error) {
	r.reqs = append(r.reqs, req)
	return &ldapv3.SearchResult{Entries: r.entries}, r.err
}

// groupEntry builds a group entry with cn, objectGUID and (optionally) a
// groupType value, mirroring what AD returns for the requested attributes.
func groupEntry(dn, cn string, guid []byte, groupType string) *ldapv3.Entry {
	e := &ldapv3.Entry{DN: dn}
	e.Attributes = append(e.Attributes, &ldapv3.EntryAttribute{Name: "cn", Values: []string{cn}, ByteValues: [][]byte{[]byte(cn)}})
	if guid != nil {
		e.Attributes = append(e.Attributes, &ldapv3.EntryAttribute{Name: "objectGUID", Values: []string{string(guid)}, ByteValues: [][]byte{guid}})
	}
	if groupType != "" {
		e.Attributes = append(e.Attributes, &ldapv3.EntryAttribute{Name: "groupType", Values: []string{groupType}, ByteValues: [][]byte{[]byte(groupType)}})
	}
	return e
}

func adConn(url string) *models.LDAPConnection {
	return &models.LDAPConnection{ServerURL: url, BaseDN: testBaseDN, DirectoryIDAttr: "objectGUID"}
}

// --- discovery filter -------------------------------------------------------

func TestGroupSearchFilterShape(t *testing.T) {
	f := groupSearchFilter("Mark")
	for _, want := range []string{"(objectCategory=group)", "(!(isCriticalSystemObject=TRUE))", "(cn=*Mark*)", "(sAMAccountName=*Mark*)", "(displayName=*Mark*)"} {
		if !strings.Contains(f, want) {
			t.Errorf("filter missing %q: %s", want, f)
		}
	}
	// No groupType/scope/type restriction: Global, Domain Local, Universal,
	// security and distribution groups must all remain discoverable. No
	// member/user clauses either.
	for _, bad := range []string{"groupType", "member", "objectClass=user", "objectClass=person", "1.2.840.113556.1.4.1941"} {
		if strings.Contains(f, bad) {
			t.Errorf("filter must not contain %q: %s", bad, f)
		}
	}
	// Empty query: base filter only, still well-formed.
	if got := groupSearchFilter(""); got != "(&(objectCategory=group)(!(isCriticalSystemObject=TRUE)))" {
		t.Errorf("empty-query filter = %q", got)
	}
}

func TestGroupSearchFilterEscapesQuery(t *testing.T) {
	f := groupSearchFilter(`a*)(objectClass=*)\`)
	if strings.Contains(f, "(objectClass=*)") {
		t.Fatalf("query was not escaped (filter injection): %s", f)
	}
	for _, want := range []string{`\2a`, `\29`, `\28`, `\5c`} {
		if !strings.Contains(f, want) {
			t.Errorf("expected escape %q in %s", want, f)
		}
	}
}

func TestNormalizeGroupQueryBounds(t *testing.T) {
	q, err := normalizeGroupQuery("  Marketing  ")
	if err != nil || q != "Marketing" {
		t.Fatalf("trim: q=%q err=%v", q, err)
	}
	if _, err := normalizeGroupQuery(strings.Repeat("a", GroupSearchMaxQueryLen)); err != nil {
		t.Fatalf("max length must be accepted: %v", err)
	}
	if _, err := normalizeGroupQuery(strings.Repeat("a", GroupSearchMaxQueryLen+1)); !errors.Is(err, ErrGroupQueryTooLong) {
		t.Fatalf("expected ErrGroupQueryTooLong, got %v", err)
	}
}

func TestGroupSearchRequestIsBoundedUnderBase(t *testing.T) {
	req := groupSearchRequest(testBaseDN, "x")
	if req.BaseDN != testBaseDN {
		t.Errorf("base = %q, want connection BaseDN", req.BaseDN)
	}
	if req.Scope != ldapv3.ScopeWholeSubtree {
		t.Errorf("scope = %d, want subtree", req.Scope)
	}
	if req.SizeLimit != GroupSearchLimit {
		t.Errorf("SizeLimit = %d, want %d", req.SizeLimit, GroupSearchLimit)
	}
	for _, a := range req.Attributes {
		la := strings.ToLower(a)
		if la == "member" || la == "memberof" || strings.HasPrefix(la, "member;") || la == "mail" || la == "userprincipalname" || la == "proxyaddresses" {
			t.Errorf("group search must not request member/user attribute %q", a)
		}
	}
}

// --- discovery results ------------------------------------------------------

func TestSearchGroupEntriesSingleQueryAndMapping(t *testing.T) {
	s := &recordingSearcher{entries: []*ldapv3.Entry{
		groupEntry("CN=Marketing,OU=Groups,"+testBaseDN, "Marketing", testGUIDRaw, "-2147483646"),                                         // global security
		groupEntry("CN=All Staff,OU=Groups,"+testBaseDN, "All Staff", []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}, "8"), // universal distribution
	}}
	items, err := searchGroupEntries(s, testBaseDN, "a")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.reqs) != 1 {
		t.Fatalf("expected exactly 1 LDAP search, got %d", len(s.reqs))
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(items))
	}
	if items[0].Name != "Marketing" || items[0].DN != "CN=Marketing,OU=Groups,"+testBaseDN || items[0].DirectoryId != testGUIDText {
		t.Fatalf("unexpected first item: %+v", items[0])
	}
	// Raw objectGUID bytes are converted to the normalized textual identity.
	if items[1].DirectoryId != "04030201-0605-0807-090a-0b0c0d0e0f10" {
		t.Fatalf("objectGUID not normalized: %q", items[1].DirectoryId)
	}
}

// TestSearchGroupEntriesKeepsAllScopesAndTypes proves no client-side
// groupType filtering: global/domain-local/universal × security/distribution
// all come back; only the server-side isCriticalSystemObject clause hides
// system groups.
func TestSearchGroupEntriesKeepsAllScopesAndTypes(t *testing.T) {
	kinds := map[string]string{
		"GlobalSec":      "-2147483646", // 0x80000002
		"DomainLocalSec": "-2147483644", // 0x80000004
		"UniversalSec":   "-2147483640", // 0x80000008
		"GlobalDist":     "2",
		"DomainLocalDis": "4",
		"UniversalDist":  "8",
	}
	var entries []*ldapv3.Entry
	i := byte(0)
	for name, gt := range kinds {
		i++
		guid := make([]byte, 16)
		guid[0] = i
		entries = append(entries, groupEntry("CN="+name+",OU=Groups,"+testBaseDN, name, guid, gt))
	}
	s := &recordingSearcher{entries: entries}
	items, err := searchGroupEntries(s, testBaseDN, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != len(kinds) {
		t.Fatalf("expected all %d group kinds discoverable, got %d", len(kinds), len(items))
	}
	seen := map[string]bool{}
	for _, it := range items {
		seen[it.Name] = true
	}
	for name := range kinds {
		if !seen[name] {
			t.Errorf("group kind %s was filtered out", name)
		}
	}
}

func TestSearchGroupEntriesResultLimitEnforced(t *testing.T) {
	var entries []*ldapv3.Entry
	for i := 0; i < GroupSearchLimit+25; i++ {
		guid := make([]byte, 16)
		guid[0], guid[1] = byte(i), byte(i>>8)
		entries = append(entries, groupEntry("CN=G"+string(rune('a'+i%26))+",OU=Groups,"+testBaseDN, "G", guid, ""))
	}
	// Server reports sizeLimitExceeded alongside the (bounded) partial result:
	// that is a full page, not an error.
	s := &recordingSearcher{entries: entries, err: ldapv3.NewError(ldapv3.LDAPResultSizeLimitExceeded, errors.New("size limit"))}
	items, err := searchGroupEntries(s, testBaseDN, "")
	if err != nil {
		t.Fatalf("sizeLimitExceeded must not be an error: %v", err)
	}
	if len(items) != GroupSearchLimit {
		t.Fatalf("expected %d items (limit), got %d", GroupSearchLimit, len(items))
	}
	// Any other LDAP error is surfaced.
	s = &recordingSearcher{err: ldapv3.NewError(ldapv3.LDAPResultOperationsError, errors.New("boom"))}
	if _, err := searchGroupEntries(s, testBaseDN, ""); err == nil {
		t.Fatal("expected backend error to be returned")
	}
}

func TestSearchGroupEntriesSkipsEntriesWithoutGUID(t *testing.T) {
	s := &recordingSearcher{entries: []*ldapv3.Entry{
		groupEntry("CN=NoGuid,OU=Groups,"+testBaseDN, "NoGuid", nil, ""),
		groupEntry("CN=Ok,OU=Groups,"+testBaseDN, "", testGUIDRaw, ""), // name falls back to RDN
	}}
	items, err := searchGroupEntries(s, testBaseDN, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 || items[0].Name != "Ok" {
		t.Fatalf("unexpected items: %+v", items)
	}
}

func TestDirectoryGroupJSONIsMinimal(t *testing.T) {
	b, err := json.Marshal(DirectoryGroup{Name: "Marketing", DN: "CN=Marketing," + testBaseDN, DirectoryId: testGUIDText})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]interface{}
	_ = json.Unmarshal(b, &m)
	if len(m) != 3 || m["name"] == nil || m["dn"] == nil || m["directory_id"] == nil {
		t.Fatalf("DirectoryGroup JSON must be exactly name/dn/directory_id: %s", string(b))
	}
}

// --- AD-only / pre-network validation ----------------------------------------

func TestSearchGroupsRejectsNonADBeforeNetwork(t *testing.T) {
	// An unroutable server: if the AD check did not run first, this would
	// surface a connect error instead.
	generic := &models.LDAPConnection{ServerURL: "ldap://127.0.0.1:1", BaseDN: testBaseDN}
	if _, err := SearchGroups(generic, "x"); !errors.Is(err, ErrNotActiveDirectory) {
		t.Fatalf("expected ErrNotActiveDirectory for generic connection, got %v", err)
	}
	entryUUID := &models.LDAPConnection{ServerURL: "ldap://127.0.0.1:1", BaseDN: testBaseDN, DirectoryIDAttr: "entryUUID"}
	if _, err := SearchGroups(entryUUID, "x"); !errors.Is(err, ErrNotActiveDirectory) {
		t.Fatalf("expected ErrNotActiveDirectory for entryUUID connection, got %v", err)
	}
	if _, err := ResolveGroupByID(generic, testGUIDText); !errors.Is(err, ErrNotActiveDirectory) {
		t.Fatalf("expected ErrNotActiveDirectory from ResolveGroupByID, got %v", err)
	}
}

func TestSearchGroupsADConnectionReachesConnect(t *testing.T) {
	// An AD connection passes validation and proceeds to the (failing) dial:
	// the error must be the connect error, not a validation error.
	c := adConn("ldap://127.0.0.1:1")
	_, err := SearchGroups(c, "x")
	if err == nil || errors.Is(err, ErrNotActiveDirectory) || errors.Is(err, ErrGroupQueryTooLong) {
		t.Fatalf("expected a connect error for AD connection, got %v", err)
	}
	if !errors.Is(err, ErrLDAPConnect) {
		t.Fatalf("expected ErrLDAPConnect, got %v", err)
	}
}

func TestSearchGroupsQueryTooLongBeforeNetwork(t *testing.T) {
	c := adConn("ldap://127.0.0.1:1")
	if _, err := SearchGroups(c, strings.Repeat("x", GroupSearchMaxQueryLen+1)); !errors.Is(err, ErrGroupQueryTooLong) {
		t.Fatalf("expected ErrGroupQueryTooLong before network, got %v", err)
	}
}

// --- resolve by stable identity ---------------------------------------------

func TestGUIDFilterValueEncoding(t *testing.T) {
	got := guidFilterValue([]byte{0xd4, 0x00, 0x2a, 0x28, 0x29, 0x5c, 0xff})
	if got != `\d4\00\2a\28\29\5c\ff` {
		t.Fatalf("guidFilterValue = %q", got)
	}
	f := groupByIDFilter(testGUIDRaw)
	want := `(&(objectCategory=group)(objectGUID=\d4\c3\b2\a1\f6\e5\88\47\99\aa\bb\cc\dd\ee\ff\00))`
	if f != want {
		t.Fatalf("groupByIDFilter = %q, want %q", f, want)
	}
	// The filter must compile as a valid LDAP filter with the raw bytes.
	if _, err := ldapv3.CompileFilter(f); err != nil {
		t.Fatalf("GUID filter does not compile: %v", err)
	}
	// Identity lookups never match on name or DN.
	if strings.Contains(f, "cn=") || strings.Contains(f, "name=") || strings.Contains(f, "distinguishedName") {
		t.Fatalf("identity filter must not include name/DN clauses: %s", f)
	}
}

func TestResolveGroupEntriesUsesStableGUID(t *testing.T) {
	// The group was renamed AND moved since it was selected; it still resolves
	// by GUID and the server-observed DN/name are returned.
	s := &recordingSearcher{entries: []*ldapv3.Entry{
		groupEntry("CN=Marketing Team,OU=Moved,"+testBaseDN, "Marketing Team", testGUIDRaw, ""),
	}}
	g, err := resolveGroupEntries(s, testBaseDN, "{A1B2C3D4-E5F6-4788-99AA-BBCCDDEEFF00}")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.reqs) != 1 {
		t.Fatalf("expected exactly 1 search, got %d", len(s.reqs))
	}
	req := s.reqs[0]
	if req.BaseDN != testBaseDN || req.Scope != ldapv3.ScopeWholeSubtree {
		t.Fatalf("lookup must be a subtree search under BaseDN: base=%q scope=%d", req.BaseDN, req.Scope)
	}
	if !strings.Contains(req.Filter, `objectGUID=\d4\c3\b2\a1`) || strings.Contains(req.Filter, "Marketing") {
		t.Fatalf("lookup filter must be GUID-only: %s", req.Filter)
	}
	if g.DirectoryId != testGUIDText || g.DN != "CN=Marketing Team,OU=Moved,"+testBaseDN || g.Name != "Marketing Team" {
		t.Fatalf("unexpected resolved group: %+v", g)
	}
}

func TestResolveGroupEntriesNotFoundNeverFallsBackToName(t *testing.T) {
	// Directory where the original group was deleted and a same-name group
	// was recreated (different GUID). Lookup by the ORIGINAL GUID must fail,
	// never silently bind to the recreated group.
	recreatedGUID := []byte{9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9}
	dir := map[string]*ldapv3.Entry{
		string(recreatedGUID): groupEntry("CN=Marketing,OU=Groups,"+testBaseDN, "Marketing", recreatedGUID, ""),
	}
	s := &guidLookupSearcher{dir: dir}
	_, err := resolveGroupEntries(s, testBaseDN, testGUIDText)
	if !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("expected ErrGroupNotFound for deleted/recreated group, got %v", err)
	}
	// The recreated group resolves only by its own GUID.
	g, err := resolveGroupEntries(s, testBaseDN, "09090909-0909-0909-0909-090909090909")
	if err != nil || g.Name != "Marketing" {
		t.Fatalf("recreated group must resolve by its own GUID: %+v %v", g, err)
	}
}

// guidLookupSearcher answers an objectGUID filter from an in-memory table
// keyed by raw GUID, ignoring everything else - a tiny AD stand-in.
type guidLookupSearcher struct {
	dir map[string]*ldapv3.Entry
}

func (g *guidLookupSearcher) Search(req *ldapv3.SearchRequest) (*ldapv3.SearchResult, error) {
	res := &ldapv3.SearchResult{}
	raw, ok := guidFromFilter(req.Filter)
	if !ok {
		return res, nil
	}
	if e, found := g.dir[string(raw)]; found {
		res.Entries = append(res.Entries, e)
	}
	return res, nil
}

// guidFromFilter decodes the \xx-escaped objectGUID assertion value out of an
// identity-lookup filter.
func guidFromFilter(filter string) ([]byte, bool) {
	const marker = "(objectGUID="
	i := strings.Index(filter, marker)
	if i < 0 {
		return nil, false
	}
	enc := filter[i+len(marker):]
	enc = enc[:strings.Index(enc, ")")]
	raw := make([]byte, 0, 16)
	for _, part := range strings.Split(enc, `\`) {
		if part == "" {
			continue
		}
		var b byte
		for _, c := range part {
			b <<= 4
			switch {
			case c >= '0' && c <= '9':
				b |= byte(c - '0')
			case c >= 'a' && c <= 'f':
				b |= byte(c-'a') + 10
			}
		}
		raw = append(raw, b)
	}
	return raw, true
}

func TestResolveGroupEntriesRejectsDNOutsideBase(t *testing.T) {
	s := &recordingSearcher{entries: []*ldapv3.Entry{
		groupEntry("CN=Marketing,DC=other,DC=com", "Marketing", testGUIDRaw, ""),
	}}
	if _, err := resolveGroupEntries(s, testBaseDN, testGUIDText); !errors.Is(err, ErrGroupOutsideBase) {
		t.Fatalf("expected ErrGroupOutsideBase, got %v", err)
	}
	// Deceptive suffix is not "within" either.
	s = &recordingSearcher{entries: []*ldapv3.Entry{
		groupEntry("CN=Marketing,DC=example2,DC=com", "Marketing", testGUIDRaw, ""),
	}}
	if _, err := resolveGroupEntries(s, testBaseDN, testGUIDText); !errors.Is(err, ErrGroupOutsideBase) {
		t.Fatalf("expected ErrGroupOutsideBase for deceptive suffix, got %v", err)
	}
}

func TestResolveGroupEntriesRejectsMismatchedOrAmbiguous(t *testing.T) {
	// Returned object does not carry the requested identity → not found.
	other := []byte{1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
	s := &recordingSearcher{entries: []*ldapv3.Entry{groupEntry("CN=X,"+testBaseDN, "X", other, "")}}
	if _, err := resolveGroupEntries(s, testBaseDN, testGUIDText); !errors.Is(err, ErrGroupNotFound) {
		t.Fatalf("expected ErrGroupNotFound on identity mismatch, got %v", err)
	}
	// Two matches → ambiguous.
	s = &recordingSearcher{entries: []*ldapv3.Entry{
		groupEntry("CN=A,"+testBaseDN, "A", testGUIDRaw, ""),
		groupEntry("CN=B,"+testBaseDN, "B", testGUIDRaw, ""),
	}}
	if _, err := resolveGroupEntries(s, testBaseDN, testGUIDText); !errors.Is(err, ErrGroupAmbiguous) {
		t.Fatalf("expected ErrGroupAmbiguous, got %v", err)
	}
	// Invalid identifier → rejected before any search.
	s = &recordingSearcher{}
	if _, err := resolveGroupEntries(s, testBaseDN, "not-a-guid"); !errors.Is(err, ErrGroupIdentityInvalid) {
		t.Fatalf("expected ErrGroupIdentityInvalid, got %v", err)
	}
	if len(s.reqs) != 0 {
		t.Fatalf("invalid identifier must not reach the directory, got %d searches", len(s.reqs))
	}
}

func TestResolveGroupByIDInvalidIdentityBeforeNetwork(t *testing.T) {
	c := adConn("ldap://127.0.0.1:1")
	if _, err := ResolveGroupByID(c, "nope"); !errors.Is(err, ErrGroupIdentityInvalid) {
		t.Fatalf("expected ErrGroupIdentityInvalid before network, got %v", err)
	}
}

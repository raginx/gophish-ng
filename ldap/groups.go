package ldap

import (
	"errors"
	"fmt"
	"strings"

	ldapv3 "github.com/go-ldap/ldap/v3"

	"github.com/raginx/gophish-ng/dnutil"
	"github.com/raginx/gophish-ng/models"
)

// Active Directory group discovery and stable-identity resolution for
// group-based target synchronization (Issue #78).
//
// Both operations consume only a stored, team-scoped models.LDAPConnection
// (server, bind identity/credential, BaseDN, TLS settings) and dial through the
// SSRF-restricted dialer via connect. The caller can never supply a server,
// base DN or raw filter; the only caller-controlled inputs are a short search
// text (escaped) and a group identifier (parsed and re-encoded, never
// interpolated). No member or user attributes are ever requested.

const (
	// GroupSearchMaxQueryLen bounds the caller-supplied search text.
	GroupSearchMaxQueryLen = 64
	// GroupSearchLimit bounds the number of groups returned by one search. It
	// is sent as the LDAP SizeLimit (so the server stops early) and enforced
	// client-side as well.
	GroupSearchLimit = 50
)

var (
	// ErrNotActiveDirectory is returned before any network access when the
	// connection is not Active Directory. Group operations are AD-only in v1.
	ErrNotActiveDirectory = errors.New("directory group operations are only supported for Active Directory connections")
	// ErrGroupQueryTooLong is returned before any network access when the
	// search text exceeds GroupSearchMaxQueryLen.
	ErrGroupQueryTooLong = fmt.Errorf("the group search text is too long (maximum %d characters)", GroupSearchMaxQueryLen)
	// ErrGroupIdentityInvalid is returned before any network access when the
	// supplied group identifier is not a valid objectGUID.
	ErrGroupIdentityInvalid = errors.New("the Active Directory group identifier is not a valid objectGUID")
	// ErrGroupNotFound is returned when no group with the requested stable
	// identifier exists under the connection's BaseDN. Callers must treat this
	// as fatal for synchronization (never reconcile, never fall back to a DN
	// or a name).
	ErrGroupNotFound = errors.New("the selected Active Directory group was not found in the directory")
	// ErrGroupAmbiguous is returned if more than one object matched the
	// identifier. objectGUID is unique per forest, so this indicates a
	// directory anomaly and is treated as a failure.
	ErrGroupAmbiguous = errors.New("more than one directory object matched the selected Active Directory group identifier")
	// ErrGroupOutsideBase is returned when the resolved group's DN is not
	// within the connection's BaseDN.
	ErrGroupOutsideBase = errors.New("the Active Directory group is outside the connection's Base DN")
)

// DirectoryGroup is the minimal, non-sensitive description of a directory
// group returned to callers: a display name, the server-observed DN and the
// normalized stable identifier. It never carries members or member counts.
type DirectoryGroup struct {
	Name        string `json:"name"`
	DN          string `json:"dn"`
	DirectoryId string `json:"directory_id"`
}

// searcher is the minimal search surface the group operations need.
// *ldapv3.Conn satisfies it; tests substitute a recording fake.
type searcher interface {
	Search(req *ldapv3.SearchRequest) (*ldapv3.SearchResult, error)
}

// entryName returns a display label for a directory entry: the value of its
// first RDN (e.g. "Marketing" from "CN=Marketing,OU=Groups,DC=example,DC=com"),
// falling back to the full DN if it can't be parsed.
func entryName(dn string) string {
	parsed, err := ldapv3.ParseDN(dn)
	if err != nil || len(parsed.RDNs) == 0 || len(parsed.RDNs[0].Attributes) == 0 {
		return dn
	}
	return parsed.RDNs[0].Attributes[0].Value
}

// groupAttrs are the only attributes ever requested for a group entry. The
// DN comes with the entry itself. member/memberOf/user attributes are
// deliberately absent.
var groupAttrs = []string{"cn", models.DirectoryIDAttrObjectGUID}

// groupBaseFilter selects Active Directory groups while suppressing the
// system/built-in ones that AD itself marks as critical.
//
// Why only isCriticalSystemObject: AD sets isCriticalSystemObject=TRUE on
// every default system group (Builtin container groups such as
// Administrators/Users/Guests, and the domain defaults such as Domain
// Admins/Domain Users/Enterprise Admins/Cert Publishers...). Administrator-
// created groups never carry it. The groupType flags were considered and
// rejected as a filter: bit 0x1 (BUILTIN_LOCAL_GROUP) is only ever set on the
// Builtin container groups, which are already critical, so it adds nothing;
// and every other bit (0x2 global, 0x4 domain-local, 0x8 universal, 0x80000000
// security-enabled) describes a scope/type that legitimate administrator
// groups use. A few harmless non-critical defaults (e.g. DnsAdmins) stay
// visible, which is the conservative outcome we want over hiding real groups.
const groupBaseFilter = "(&(objectCategory=group)(!(isCriticalSystemObject=TRUE))"

// groupSearchFilter returns the discovery filter for an (already trimmed and
// length-checked) search text. The text is escaped with ldapv3.EscapeFilter
// so it can never alter the filter structure; it is matched as a substring of
// the common name, account name or display name. An empty text lists groups
// without a name clause (still bounded by GroupSearchLimit).
func groupSearchFilter(q string) string {
	if q == "" {
		return groupBaseFilter + ")"
	}
	e := ldapv3.EscapeFilter(q)
	return groupBaseFilter + "(|(cn=*" + e + "*)(sAMAccountName=*" + e + "*)(displayName=*" + e + "*)))"
}

// groupSearchRequest builds the single, bounded, subtree search under baseDN.
func groupSearchRequest(baseDN, q string) *ldapv3.SearchRequest {
	return ldapv3.NewSearchRequest(
		baseDN,
		ldapv3.ScopeWholeSubtree,
		ldapv3.NeverDerefAliases,
		GroupSearchLimit, // SizeLimit: the server stops after this many entries
		0,                // TimeLimit: connection timeout applies
		false,
		groupSearchFilter(q),
		groupAttrs,
		nil,
	)
}

// normalizeGroupQuery trims and bounds the caller-supplied search text.
func normalizeGroupQuery(q string) (string, error) {
	t := strings.TrimSpace(q)
	if len(t) > GroupSearchMaxQueryLen {
		return "", ErrGroupQueryTooLong
	}
	return t, nil
}

// entryToGroup maps a group entry to a DirectoryGroup. The stable identifier
// is read as raw bytes and normalized through the shared identity helper. An
// entry without a usable objectGUID (impossible in AD, but defensive) is
// skipped rather than surfaced with an empty identity.
func entryToGroup(e *ldapv3.Entry) (DirectoryGroup, bool) {
	raw := e.GetRawAttributeValue(models.DirectoryIDAttrObjectGUID)
	if len(raw) == 0 {
		return DirectoryGroup{}, false
	}
	id, err := models.NormalizeDirectoryID(models.DirectoryIDAttrObjectGUID, raw)
	if err != nil {
		return DirectoryGroup{}, false
	}
	name := strings.TrimSpace(e.GetAttributeValue("cn"))
	if name == "" {
		name = entryName(e.DN)
	}
	return DirectoryGroup{Name: name, DN: e.DN, DirectoryId: id}, true
}

// searchGroupEntries performs exactly one search and maps the results. A
// server-side sizeLimitExceeded result (code 4) is not an error here: the
// entries received up to the limit are returned, and the caller simply sees a
// full page. Everything else is returned as an error.
func searchGroupEntries(s searcher, baseDN, q string) ([]DirectoryGroup, error) {
	res, err := s.Search(groupSearchRequest(baseDN, q))
	if err != nil && !ldapv3.IsErrorWithCode(err, ldapv3.LDAPResultSizeLimitExceeded) {
		return nil, err
	}
	if res == nil {
		return []DirectoryGroup{}, nil
	}
	items := make([]DirectoryGroup, 0, len(res.Entries))
	for _, e := range res.Entries {
		if len(items) >= GroupSearchLimit {
			break
		}
		if g, ok := entryToGroup(e); ok {
			items = append(items, g)
		}
	}
	return items, nil
}

// SearchGroups searches the connection's directory, under its stored BaseDN,
// for Active Directory groups whose name contains query. It issues a single
// bounded LDAP search, returns at most GroupSearchLimit minimal
// DirectoryGroup items, and never requests member or user data. The
// connection must be Active Directory; the query is trimmed, bounded and
// escaped server-side. Validation failures are returned before any network
// access.
func SearchGroups(conn *models.LDAPConnection, query string) ([]DirectoryGroup, error) {
	if !conn.IsActiveDirectory() {
		return nil, ErrNotActiveDirectory
	}
	q, err := normalizeGroupQuery(query)
	if err != nil {
		return nil, err
	}
	lc, err := connect(conn)
	if err != nil {
		return nil, err
	}
	defer lc.Close()
	return searchGroupEntries(lc, conn.BaseDN, q)
}

// guidFilterValue encodes raw objectGUID bytes for use as an LDAP filter
// assertion value: every byte is written as a \xx hex escape (RFC 4515), the
// form Active Directory expects for binary attribute matching. Encoding every
// byte (not just the specials) keeps the filter free of raw non-UTF-8 bytes.
func guidFilterValue(raw []byte) string {
	var b strings.Builder
	for _, c := range raw {
		fmt.Fprintf(&b, "\\%02x", c)
	}
	return b.String()
}

// groupByIDFilter matches exactly the group object with the given raw
// objectGUID. It never includes a name or DN clause: a recreated group with
// the same name has a different objectGUID and therefore does not match.
func groupByIDFilter(raw []byte) string {
	return "(&(objectCategory=group)(objectGUID=" + guidFilterValue(raw) + "))"
}

// groupByIDRequest builds the subtree lookup under baseDN. SizeLimit 2 is
// enough to detect the (anomalous) multi-match case without reading more.
func groupByIDRequest(baseDN string, raw []byte) *ldapv3.SearchRequest {
	return ldapv3.NewSearchRequest(
		baseDN,
		ldapv3.ScopeWholeSubtree,
		ldapv3.NeverDerefAliases,
		2, 0, false,
		groupByIDFilter(raw),
		groupAttrs,
		nil,
	)
}

// parseGroupIdentity canonicalizes a textual group identifier and returns
// both the normalized text and the raw bytes used in the filter.
func parseGroupIdentity(directoryID string) (string, []byte, error) {
	norm, err := models.NormalizeDirectoryIDText(models.DirectoryIDAttrObjectGUID, directoryID)
	if err != nil {
		return "", nil, ErrGroupIdentityInvalid
	}
	raw, err := models.ParseADObjectGUID(norm)
	if err != nil {
		return "", nil, ErrGroupIdentityInvalid
	}
	return norm, raw, nil
}

// resolveGroupEntries performs the single identity lookup and applies the
// resolution rules: exactly one match, carrying the requested identifier,
// located within baseDN. The returned DN and name are the server-observed
// values (the authoritative refresh of any last-known metadata).
func resolveGroupEntries(s searcher, baseDN, directoryID string) (DirectoryGroup, error) {
	norm, raw, err := parseGroupIdentity(directoryID)
	if err != nil {
		return DirectoryGroup{}, err
	}
	res, err := s.Search(groupByIDRequest(baseDN, raw))
	if err != nil {
		if ldapv3.IsErrorWithCode(err, ldapv3.LDAPResultSizeLimitExceeded) {
			return DirectoryGroup{}, ErrGroupAmbiguous
		}
		return DirectoryGroup{}, err
	}
	if res == nil || len(res.Entries) == 0 {
		return DirectoryGroup{}, ErrGroupNotFound
	}
	if len(res.Entries) > 1 {
		return DirectoryGroup{}, ErrGroupAmbiguous
	}
	g, ok := entryToGroup(res.Entries[0])
	if !ok || g.DirectoryId != norm {
		// The server returned an object that does not carry the identity we
		// asked for; never accept it as the selected group.
		return DirectoryGroup{}, ErrGroupNotFound
	}
	within, werr := dnutil.Within(g.DN, baseDN)
	if werr != nil || !within {
		return DirectoryGroup{}, ErrGroupOutsideBase
	}
	return g, nil
}

// ResolveGroupByID locates the Active Directory group with the given stable
// identifier (normalized objectGUID text) under the connection's stored
// BaseDN and returns its server-observed name and DN together with the
// normalized identifier.
//
// It is the authoritative way to (re)validate a selected group: a renamed or
// moved group still resolves (identity is the GUID, not the DN/name), while a
// deleted group yields ErrGroupNotFound and a recreated same-name group is
// NOT matched. Callers must never fall back to a client-supplied DN or to a
// name match. Invalid identifiers and non-AD connections fail before any
// network access.
func ResolveGroupByID(conn *models.LDAPConnection, directoryID string) (DirectoryGroup, error) {
	if !conn.IsActiveDirectory() {
		return DirectoryGroup{}, ErrNotActiveDirectory
	}
	if _, _, err := parseGroupIdentity(directoryID); err != nil {
		return DirectoryGroup{}, err
	}
	lc, err := connect(conn)
	if err != nil {
		return DirectoryGroup{}, err
	}
	defer lc.Close()
	return resolveGroupEntries(lc, conn.BaseDN, directoryID)
}

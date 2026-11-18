package ldap

import (
	"strings"
	"testing"

	ldapv3 "github.com/go-ldap/ldap/v3"

	"github.com/raginx/gophish-ng/models"
)

// --- direct-member filter / request shape -----------------------------------

func TestMemberFilterDirectMembersOnly(t *testing.T) {
	dn := "CN=Marketing,OU=Groups,DC=example,DC=com"
	f := memberFilter(dn, false)
	want := "(&(objectCategory=person)(objectClass=user)(memberOf=" + dn + "))"
	if f != want {
		t.Fatalf("memberFilter = %q, want %q", f, want)
	}
	// Nested expansion (matching-rule-in-chain) and ranged member reads are
	// never used.
	for _, bad := range []string{"1.2.840.113556.1.4.1941", "member;range", "(member="} {
		if strings.Contains(f, bad) {
			t.Fatalf("member filter must not contain %q: %s", bad, f)
		}
	}
	if _, err := ldapv3.CompileFilter(f); err != nil {
		t.Fatalf("member filter does not compile: %v", err)
	}
}

func TestMemberFilterExcludeDisabled(t *testing.T) {
	f := memberFilter("CN=G,DC=example,DC=com", true)
	if !strings.Contains(f, "(!(userAccountControl:1.2.840.113556.1.4.803:=2))") {
		t.Fatalf("exclude-disabled clause missing: %s", f)
	}
	if !strings.HasPrefix(f, "(&(objectCategory=person)(objectClass=user)(memberOf=") || !strings.HasSuffix(f, "))") {
		t.Fatalf("exclude-disabled filter malformed: %s", f)
	}
	if _, err := ldapv3.CompileFilter(f); err != nil {
		t.Fatalf("filter does not compile: %v", err)
	}
}

func TestMemberFilterEscapesGroupDN(t *testing.T) {
	// A DN with filter metacharacters (legal in AD names) must be escaped so
	// it cannot alter the filter structure.
	dn := `CN=Sales (EMEA) *,OU=Groups\, Inc,DC=example,DC=com`
	f := memberFilter(dn, false)
	if strings.Contains(f, "(EMEA)") || strings.Contains(f, "*,") {
		t.Fatalf("group DN was not escaped in filter: %s", f)
	}
	if !strings.Contains(f, `\28EMEA\29`) || !strings.Contains(f, `\2a`) || !strings.Contains(f, `\5c`) {
		t.Fatalf("expected RFC 4515 escapes in filter: %s", f)
	}
	if _, err := ldapv3.CompileFilter(f); err != nil {
		t.Fatalf("escaped filter does not compile: %v", err)
	}
}

func TestMemberAttributesAreMinimal(t *testing.T) {
	s := &models.LDAPGroupSync{AttrEmail: "userPrincipalName", AttrFirstName: "givenName", AttrLastName: "sn", AttrPosition: "title"}
	attrs := memberAttributes(s)
	joined := strings.ToLower(strings.Join(attrs, ","))
	for _, want := range []string{"userprincipalname", "givenname", "sn", "title", "objectguid"} {
		if !strings.Contains(joined, want) {
			t.Errorf("member attributes missing %q: %v", want, attrs)
		}
	}
	for _, bad := range []string{"member", "memberof", "proxyaddresses", "mail"} {
		for _, a := range attrs {
			if strings.ToLower(a) == bad {
				t.Errorf("member attributes must not include %q: %v", bad, attrs)
			}
		}
	}
	// Duplicates (same attribute configured twice) are requested once.
	dup := memberAttributes(&models.LDAPGroupSync{AttrEmail: "mail", AttrFirstName: "mail", AttrLastName: "MAIL", AttrPosition: "objectGUID"})
	if len(dup) != 2 {
		t.Errorf("expected de-duplicated attributes, got %v", dup)
	}
}

func TestMemberSearchRequestUnderBaseDN(t *testing.T) {
	s := &models.LDAPGroupSync{AttrEmail: "mail", AttrFirstName: "givenName", AttrLastName: "sn", AttrPosition: "title", ExcludeDisabled: true}
	req := memberSearchRequest("DC=example,DC=com", s, "CN=G,OU=Groups,DC=example,DC=com")
	if req.BaseDN != "DC=example,DC=com" || req.Scope != ldapv3.ScopeWholeSubtree {
		t.Fatalf("member search must be a subtree search under the connection BaseDN: %+v", req)
	}
	if req.SizeLimit != 0 {
		t.Fatalf("member search must not be size-limited (paging provides bounded memory): %d", req.SizeLimit)
	}
	if !strings.Contains(req.Filter, "(memberOf=CN=G,OU=Groups,DC=example,DC=com)") {
		t.Fatalf("filter does not target the resolved group DN: %s", req.Filter)
	}
}

// --- connect ----------------------------------------------------------------

// TestConnectInvalidURL verifies a malformed server URL fails fast at connect
// (before any directory or DB interaction) and never panics.
func TestConnectInvalidURL(t *testing.T) {
	conn := &models.LDAPConnection{ServerURL: "http://not-ldap"}
	if err := TestConnection(conn); err == nil {
		t.Fatal("expected error for non-ldap URL, got nil")
	}
}

// TestConnectUnreachable verifies that an unreachable server surfaces a
// connect error rather than succeeding or hanging indefinitely.
func TestConnectUnreachable(t *testing.T) {
	// 127.0.0.1:1 is reserved/unused; dialing it should fail quickly.
	conn := &models.LDAPConnection{ServerURL: "ldap://127.0.0.1:1"}
	err := TestConnection(conn)
	if err == nil {
		t.Fatal("expected connect error for unreachable server, got nil")
	}
}

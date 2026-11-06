package dnutil

import "testing"

func TestWithin(t *testing.T) {
	const base = "DC=example,DC=com"
	cases := []struct {
		name      string
		candidate string
		base      string
		want      bool
		wantErr   bool
	}{
		{"equal to base", "DC=example,DC=com", base, true, false},
		{"direct child OU", "OU=Users,DC=example,DC=com", base, true, false},
		{"multi-level descendant", "OU=IT,OU=Users,DC=example,DC=com", base, true, false},
		{"above base", "DC=com", base, false, false},
		{"different tree", "OU=Users,DC=other,DC=com", base, false, false},
		{"different tld", "DC=example,DC=org", base, false, false},
		// Case-insensitive attribute names and values (AD/OpenLDAP semantics).
		{"case-insensitive", "ou=users,dc=Example,dc=Com", base, true, false},
		// A deceptive textual suffix: "example2" ends with "...com" but
		// is NOT a descendant of DC=example,DC=com. Naive suffix
		// matching on reversed strings could be fooled; DN parsing is not.
		{"deceptive suffix domain", "OU=x,DC=example2,DC=com", base, false, false},
		{"deceptive prefix value", "DC=notexample,DC=com", base, false, false},
		// Escaped comma inside an RDN value: the OU literally contains a comma.
		// Must still be recognized as a descendant.
		{"escaped comma in RDN", "OU=Sales\\, East,DC=example,DC=com", base, true, false},
		// Base with an escaped comma; a descendant below it.
		{"descendant under escaped-comma base", "OU=team,OU=Sales\\, East,DC=example,DC=com", "OU=Sales\\, East,DC=example,DC=com", true, false},
		// Malformed candidate / base.
		{"malformed candidate", "this is not a dn", base, false, true},
		{"malformed base", "OU=x,DC=example,DC=com", "not a dn", false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Within(tc.candidate, tc.base)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (within=%v)", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("Within(%q, %q) = %v, want %v", tc.candidate, tc.base, got, tc.want)
			}
		})
	}
}

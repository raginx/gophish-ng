// Package dnutil provides LDAP distinguished-name (DN) containment checks used
// to keep a selected directory group (an LDAP group-sync mapping's group DN,
// whether supplied at creation or observed during resolution/synchronization)
// within its connection's BaseDN.
//
// It uses the go-ldap DN parser/comparator so comparisons respect RFC 4514 DN
// structure and escaping - never naive string suffix/substring matching, which
// is unsafe for DNs (escaping, attribute case, and deceptive suffixes).
//
// It lives in its own small package so both the models layer (mapping
// validation) and the ldap layer (group resolution) can share one
// implementation without an import cycle: dnutil imports only go-ldap; models
// imports dnutil; ldap already imports models and may import dnutil too.
package dnutil

import (
	"errors"

	ldapv3 "github.com/go-ldap/ldap/v3"
)

// ErrInvalidDN indicates a DN string that could not be parsed per RFC 4514.
var ErrInvalidDN = errors.New("invalid distinguished name (DN)")

// Within reports whether candidate is equal to base or a descendant of base,
// i.e. candidate is contained within the subtree rooted at base.
//
// Both DNs are parsed with the go-ldap parser; comparison is case-insensitive
// on attribute names and values (AncestorOfFold/EqualFold), matching the
// case-insensitive matching rules used by directories such as Active Directory
// and OpenLDAP for the attribute types used in DNs. A malformed base or
// candidate returns (false, ErrInvalidDN).
func Within(candidate, base string) (bool, error) {
	baseDN, err := ldapv3.ParseDN(base)
	if err != nil {
		return false, ErrInvalidDN
	}
	candDN, err := ldapv3.ParseDN(candidate)
	if err != nil {
		return false, ErrInvalidDN
	}
	if baseDN.EqualFold(candDN) {
		return true, nil
	}
	// AncestorOfFold reports whether baseDN is an ancestor of candDN, i.e.
	// candDN sits below base in the tree.
	return baseDN.AncestorOfFold(candDN), nil
}

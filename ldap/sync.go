// Package ldap implements Active Directory group-based target synchronization
// for Gophish-NG (Issue #78). It connects to a directory using a reusable,
// team-scoped models.LDAPConnection, resolves the directory group selected by
// a per-group models.LDAPGroupSync mapping by its stable identity
// (objectGUID), reads that group's DIRECT user members, and reconciles them
// into the mapping's Gophish group through
// models.ReconcileGroupTargetsWithIdentity.
//
// Reconciliation is only ever applied after a complete, successful directory
// read: a failed connection/bind, an unresolvable (deleted) group, or a
// failed/partial member search must never be allowed to mass-remove group
// members. A deleted group is an error, never an empty group.
package ldap

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"strings"
	"time"

	ldapv3 "github.com/go-ldap/ldap/v3"

	"github.com/raginx/gophish-ng/dialer"
	log "github.com/raginx/gophish-ng/logger"
	"github.com/raginx/gophish-ng/models"
)

// dialTimeout bounds how long we wait to establish a directory connection.
const dialTimeout = 15 * time.Second

// memberPageSize is the paged-results page size for the member search.
const memberPageSize = 500

// ErrLDAPConnect indicates a failure to reach/connect to the directory.
var ErrLDAPConnect = errors.New("unable to connect to LDAP server")

// ErrLDAPBind indicates an authentication (bind) failure.
var ErrLDAPBind = errors.New("LDAP bind failed")

// adDisabledClause excludes disabled Active Directory accounts
// (userAccountControl bit 0x2) when ExcludeDisabled is set.
const adDisabledClause = "(!(userAccountControl:1.2.840.113556.1.4.803:=2))"

// directory is the search surface a synchronization needs. *ldapv3.Conn
// satisfies it; tests substitute an in-memory fake so the whole sync flow
// (resolve, read members, reconcile) runs without a live directory.
type directory interface {
	searcher
	SearchWithPaging(req *ldapv3.SearchRequest, pagingSize uint32) (*ldapv3.SearchResult, error)
}

// connect dials and binds to the directory described by conn, routing the
// TCP connection through the SSRF-restricted dialer (the same one used for
// SMTP/IMAP/OAuth). The caller must Close the returned connection.
func connect(conn *models.LDAPConnection) (*ldapv3.Conn, error) {
	u, err := url.Parse(conn.ServerURL)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("%w: invalid server URL", ErrLDAPConnect)
	}

	host := u.Host
	if u.Port() == "" {
		if u.Scheme == "ldaps" {
			host = net.JoinHostPort(host, "636")
		} else {
			host = net.JoinHostPort(host, "389")
		}
	}

	tlsConfig := &tls.Config{
		ServerName:         strings.Split(host, ":")[0],
		InsecureSkipVerify: conn.IgnoreCertErrors, //nolint:gosec // opt-in, lab-only, defaults to false
	}

	netDialer := dialer.Dialer()
	var lc *ldapv3.Conn
	if u.Scheme == "ldaps" {
		tcpConn, derr := netDialer.Dial("tcp", host)
		if derr != nil {
			return nil, fmt.Errorf("%w: %v", ErrLDAPConnect, derr)
		}
		tlsConn := tls.Client(tcpConn, tlsConfig)
		if herr := tlsConn.Handshake(); herr != nil {
			_ = tcpConn.Close()
			return nil, fmt.Errorf("%w: TLS handshake failed", ErrLDAPConnect)
		}
		lc = ldapv3.NewConn(tlsConn, true)
		lc.Start()
	} else {
		tcpConn, derr := netDialer.Dial("tcp", host)
		if derr != nil {
			return nil, fmt.Errorf("%w: %v", ErrLDAPConnect, derr)
		}
		lc = ldapv3.NewConn(tcpConn, false)
		lc.Start()
		if conn.StartTLS {
			if terr := lc.StartTLS(tlsConfig); terr != nil {
				lc.Close()
				return nil, fmt.Errorf("%w: StartTLS failed", ErrLDAPConnect)
			}
		}
	}

	lc.SetTimeout(dialTimeout)

	password, perr := conn.DecryptBindPassword()
	if perr != nil {
		lc.Close()
		return nil, fmt.Errorf("%w: unable to decrypt bind password", ErrLDAPBind)
	}
	if berr := lc.Bind(conn.BindDN, password); berr != nil {
		lc.Close()
		// Do not include the password or full server error detail that
		// might echo credentials.
		return nil, ErrLDAPBind
	}
	return lc, nil
}

// TestConnection dials and binds to verify the connection settings. It does
// not read or mutate any group.
func TestConnection(conn *models.LDAPConnection) error {
	lc, err := connect(conn)
	if err != nil {
		return err
	}
	lc.Close()
	return nil
}

// memberFilter returns the filter selecting the DIRECT user members of the
// group with the given (server-observed) DN:
//
//	(&(objectCategory=person)(objectClass=user)(memberOf=<group DN>)[disabled-exclusion])
//
// memberOf is the server-maintained back-link of the group's member attribute,
// so it yields exactly direct membership: a nested group that is itself a
// direct member is not a user (objectCategory=person excludes groups,
// computers and contacts) and its own members do not carry this memberOf
// value. The LDAP_MATCHING_RULE_IN_CHAIN (1.2.840.113556.1.4.1941) is
// deliberately NOT used - nested expansion is out of scope. The DN is escaped
// so it can never alter the filter structure.
func memberFilter(groupDN string, excludeDisabled bool) string {
	f := "(&(objectCategory=person)(objectClass=user)(memberOf=" + ldapv3.EscapeFilter(groupDN) + ")"
	if excludeDisabled {
		f += adDisabledClause
	}
	return f + ")"
}

// memberAttributes returns the attributes requested per member: exactly the
// configured recipient mapping plus the stable identity attribute. Nothing
// else (no member/memberOf lists, no extra personal data).
func memberAttributes(s *models.LDAPGroupSync) []string {
	seen := map[string]bool{}
	attrs := make([]string, 0, 5)
	for _, a := range []string{s.AttrEmail, s.AttrFirstName, s.AttrLastName, s.AttrPosition, models.DirectoryIDAttrObjectGUID} {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		key := strings.ToLower(a)
		if seen[key] {
			continue
		}
		seen[key] = true
		attrs = append(attrs, a)
	}
	return attrs
}

// memberSearchRequest builds the paged subtree search for direct members under
// the connection BaseDN.
func memberSearchRequest(baseDN string, s *models.LDAPGroupSync, groupDN string) *ldapv3.SearchRequest {
	return ldapv3.NewSearchRequest(
		baseDN,
		ldapv3.ScopeWholeSubtree,
		ldapv3.NeverDerefAliases,
		0, // SizeLimit: paging bounds memory; a complete set is required
		0, // TimeLimit
		false,
		memberFilter(groupDN, s.ExcludeDisabled),
		memberAttributes(s),
		nil,
	)
}

// DirectoryRead is the outcome of the directory half of a synchronization:
// the resolved group and the mapped direct members, with aggregate counts
// that explain the difference between what the directory holds and what can
// become a target. It carries no raw directory values beyond the mapped
// recipients themselves.
type DirectoryRead struct {
	// Group is the server-observed selected group (name/DN may differ from the
	// mapping's cached values after a rename/move).
	Group DirectoryGroup
	// Discovered is the number of direct user members returned by the
	// directory (after the configured disabled-account exclusion).
	Discovered int
	// Members are the discovered members that mapped to a usable recipient.
	Members []models.DirectoryTarget
	// Skipped is the number of discovered members NOT mapped because the
	// configured email attribute was empty or not a valid address.
	Skipped int
}

// mapMember converts a member entry into a DirectoryTarget. ok is false when
// the configured email attribute is empty or not a valid address - the entry
// is counted as skipped rather than failing the whole synchronization.
func mapMember(e *ldapv3.Entry, s *models.LDAPGroupSync) (models.DirectoryTarget, bool) {
	email := strings.TrimSpace(e.GetAttributeValue(s.AttrEmail))
	if email == "" {
		return models.DirectoryTarget{}, false
	}
	if _, err := mail.ParseAddress(email); err != nil {
		return models.DirectoryTarget{}, false
	}
	dirID := ""
	// objectGUID is binary; always read the raw bytes and normalize through
	// the shared identity helper.
	if raw := e.GetRawAttributeValue(models.DirectoryIDAttrObjectGUID); len(raw) > 0 {
		if norm, nerr := models.NormalizeDirectoryID(models.DirectoryIDAttrObjectGUID, raw); nerr == nil {
			dirID = norm
		} else {
			log.Errorf("LDAP sync: could not normalize objectGUID for a member of mapping %d: %s", s.Id, nerr.Error())
		}
	}
	return models.DirectoryTarget{
		Target: models.Target{
			BaseRecipient: models.BaseRecipient{
				Email:     email,
				FirstName: strings.TrimSpace(e.GetAttributeValue(s.AttrFirstName)),
				LastName:  strings.TrimSpace(e.GetAttributeValue(s.AttrLastName)),
				Position:  strings.TrimSpace(e.GetAttributeValue(s.AttrPosition)),
			},
		},
		DirectoryId: dirID,
	}, true
}

// readDirectory performs the complete directory read for a mapping:
//  1. resolve the selected group by its stable identity (objectGUID) under the
//     connection BaseDN - a deleted group is an error, never an empty set;
//  2. one paged search for the group's direct user members (as configured);
//  3. map entries to DirectoryTargets, counting unmappable ones as skipped.
//
// Any error means the desired set is unreliable and the caller must not
// reconcile.
func readDirectory(dir directory, conn *models.LDAPConnection, s *models.LDAPGroupSync) (DirectoryRead, error) {
	group, err := resolveGroupEntries(dir, conn.BaseDN, s.GroupDirectoryId)
	if err != nil {
		return DirectoryRead{}, err
	}
	// SearchWithPaging keeps memory bounded on large groups and still returns
	// a complete result set (or an error) - it does not silently truncate.
	res, err := dir.SearchWithPaging(memberSearchRequest(conn.BaseDN, s, group.DN), memberPageSize)
	if err != nil {
		return DirectoryRead{}, err
	}
	out := DirectoryRead{Group: group}
	if res == nil {
		return out, nil
	}
	out.Discovered = len(res.Entries)
	out.Members = make([]models.DirectoryTarget, 0, len(res.Entries))
	for _, e := range res.Entries {
		dt, ok := mapMember(e, s)
		if !ok {
			out.Skipped++
			continue
		}
		out.Members = append(out.Members, dt)
	}
	return out, nil
}

// ReadDirectory connects to the mapping's directory and performs the
// read-only half of a synchronization (resolve group + direct members) WITHOUT
// touching the Gophish group. It is intended for dry-run/validation tooling
// and a future preview endpoint. The connection must be Active Directory.
func ReadDirectory(conn *models.LDAPConnection, s *models.LDAPGroupSync) (DirectoryRead, error) {
	if !conn.IsActiveDirectory() {
		return DirectoryRead{}, ErrNotActiveDirectory
	}
	lc, err := connect(conn)
	if err != nil {
		return DirectoryRead{}, err
	}
	defer lc.Close()
	return readDirectory(lc, conn, s)
}

// SyncResult reports what one synchronization pass did.
type SyncResult struct {
	models.ReconcileResult
	// Discovered is the number of direct user members read from the directory.
	Discovered int
	// Skipped is the number of discovered members not mapped (empty/invalid
	// configured email).
	Skipped int
}

// recordFailure stores a failed run on the mapping without touching
// membership.
func recordFailure(s *models.LDAPGroupSync, err error) {
	_ = models.RecordLDAPSyncResult(s.Id, s.TeamId, 0, 0, 0, 0, err)
}

// Sync runs one synchronization pass for the given mapping and records the
// result. It resolves the mapping's connection (Active Directory only),
// connects, reads the selected group's direct members, and only on a fully
// successful read reconciles the group membership. On any connection, bind,
// resolution or search error, group membership is left unchanged.
func Sync(s *models.LDAPGroupSync) (SyncResult, error) {
	conn, err := models.GetLDAPConnection(s.ConnectionId, s.TeamId)
	if err != nil {
		recordErr := fmt.Errorf("%w: %v", ErrLDAPConnect, err)
		recordFailure(s, recordErr)
		return SyncResult{}, recordErr
	}
	if !conn.IsActiveDirectory() {
		recordFailure(s, ErrNotActiveDirectory)
		return SyncResult{}, ErrNotActiveDirectory
	}
	lc, err := connect(&conn)
	if err != nil {
		recordFailure(s, err)
		return SyncResult{}, err
	}
	defer lc.Close()
	return syncWith(lc, &conn, s)
}

// syncWith is Sync after the connection has been established: the complete
// directory read, the refresh of the cached group DN/name when the group was
// renamed or moved (identity unchanged), and the reconciliation.
func syncWith(dir directory, conn *models.LDAPConnection, s *models.LDAPGroupSync) (SyncResult, error) {
	read, err := readDirectory(dir, conn, s)
	if err != nil {
		// Unresolvable group / failed or partial read: do NOT reconcile.
		recordFailure(s, err)
		return SyncResult{}, err
	}

	// The group resolved by GUID: refresh the last-known DN/name if the
	// directory shows a rename or move. The Gophish group name is independent
	// and is never changed here.
	if read.Group.DN != s.GroupDN || read.Group.Name != s.GroupName {
		if uerr := models.UpdateLDAPGroupSyncDirectoryGroup(s.Id, s.TeamId, read.Group.DN, read.Group.Name); uerr != nil {
			log.Errorf("LDAP sync: could not refresh directory group metadata for mapping %d: %s", s.Id, uerr.Error())
		} else {
			s.GroupDN = read.Group.DN
			s.GroupName = read.Group.Name
		}
	}

	rr, err := models.ReconcileGroupTargetsWithIdentity(s.GroupId, s.TeamId, s.ConnectionId, read.Members)
	if err != nil {
		recordFailure(s, err)
		return SyncResult{}, err
	}

	res := SyncResult{ReconcileResult: rr, Discovered: read.Discovered, Skipped: read.Skipped}
	if rerr := models.RecordLDAPSyncResult(s.Id, s.TeamId, rr.Added, rr.Updated, rr.Removed, read.Skipped, nil); rerr != nil {
		log.Error(rerr)
	}
	if read.Skipped > 0 {
		log.Warnf("LDAP sync for group %d (team %d): %d of %d direct members skipped (empty or invalid %s)", s.GroupId, s.TeamId, read.Skipped, read.Discovered, s.AttrEmail)
	}
	log.Infof("LDAP sync for group %d (team %d): discovered %d, +%d ~%d -%d, skipped %d", s.GroupId, s.TeamId, read.Discovered, rr.Added, rr.Updated, rr.Removed, read.Skipped)
	return res, nil
}

package models

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/raginx/gophish-ng/config"
	gophishcrypto "github.com/raginx/gophish-ng/crypto"
	"github.com/raginx/gophish-ng/dnutil"
	log "github.com/raginx/gophish-ng/logger"
	"gorm.io/gorm"
)

// DirectoryIDAttrObjectGUID is the Active Directory stable per-object
// identifier attribute. A connection whose DirectoryIDAttr equals this value
// is treated as Active Directory (see LDAPConnection.IsActiveDirectory).
const DirectoryIDAttrObjectGUID = "objectGUID"

// LDAP synchronization defaults. These mirror sensible Active Directory
// values and can be overridden per group-sync mapping.
const (
	DefaultLDAPAttrEmail     = "mail"
	DefaultLDAPAttrFirstName = "givenName"
	DefaultLDAPAttrLastName  = "sn"
	DefaultLDAPAttrPosition  = "title"
	// DefaultLDAPSyncFreq is how often (in seconds) a group is re-synced
	// when no explicit interval is set.
	DefaultLDAPSyncFreq = 3600
	// MinLDAPSyncFreq is the lower bound on the sync interval, matching the
	// IMAP monitor's 30-second floor.
	MinLDAPSyncFreq = 30
)

// LDAPConnection is a reusable, team-scoped directory connection. A single
// connection can serve many LDAP-managed groups, so the server/bind
// credentials/TLS settings live here once rather than being duplicated per
// group (see Issue #78).
//
// Unlike IMAP (which is user-scoped because it holds a personal mailbox
// grant), an LDAP connection is a shared team asset like a sending profile,
// so it is team-scoped - consistent with every other shared object.
type LDAPConnection struct {
	Id        int64  `json:"id" gorm:"column:id;primaryKey"`
	UserId    int64  `json:"-" gorm:"column:user_id"`
	TeamId    int64  `json:"-" gorm:"column:team_id"`
	Name      string `json:"name"`
	ServerURL string `json:"server_url" gorm:"column:server_url"`
	BindDN    string `json:"bind_dn" gorm:"column:bind_dn"`
	// BindPassword is write-only on the wire: it is accepted on POST/PUT,
	// encrypted into BindPasswordEnc, and never serialized back out (a blank
	// value on update means "keep the existing password"), exactly like the
	// IMAP OAuth client secret.
	BindPassword    string `json:"bind_password,omitempty" gorm:"-"`
	BindPasswordEnc string `json:"-" gorm:"column:bind_password_enc"`
	BaseDN          string `json:"base_dn" gorm:"column:base_dn"`
	StartTLS        bool   `json:"start_tls" gorm:"column:start_tls"`
	// IgnoreCertErrors disables TLS certificate verification. It defaults to
	// false (verification enabled) and is intended only for lab use.
	IgnoreCertErrors bool `json:"ignore_cert_errors" gorm:"column:ignore_cert_errors"`
	// DirectoryIDAttr is the directory attribute that holds a stable,
	// immutable per-object identifier used to track the same logical directory
	// user across email/name/DN changes (e.g. "objectGUID" for Active
	// Directory, "entryUUID" for generic LDAP). Empty means no stable identity
	// is available and reconciliation falls back to email matching.
	DirectoryIDAttr string    `json:"directory_id_attr" gorm:"column:directory_id_attr"`
	LastTestStatus  string    `json:"last_test_status,omitempty" gorm:"column:last_test_status"`
	LastTestError   string    `json:"last_test_error,omitempty" gorm:"column:last_test_error"`
	ModifiedDate    time.Time `json:"modified_date"`
}

// TableName specifies the database tablename for Gorm to use.
func (LDAPConnection) TableName() string {
	return "ldap_connection"
}

// IsActiveDirectory reports whether the connection represents Active
// Directory. It is derived from the configured stable identity attribute
// (objectGUID) - no separate schema field is needed. Group-based managed
// synchronization is Active Directory-only in v1.
func (c *LDAPConnection) IsActiveDirectory() bool {
	return strings.EqualFold(strings.TrimSpace(c.DirectoryIDAttr), DirectoryIDAttrObjectGUID)
}

// LDAPGroupSync maps a single Gophish Group to an LDAPConnection and to ONE
// selected directory group whose direct user members are synchronized into
// the Gophish group. It holds only the synchronization-specific selection,
// attribute mapping and interval. The existence of a row marks its group as
// "LDAP-managed".
//
// Directory group identity: GroupDirectoryId (the normalized AD objectGUID)
// is authoritative - a renamed or moved AD group keeps the same logical
// identity. GroupDN and GroupName are server-observed, last-known metadata:
// they are set from directory resolution when the mapping is created and
// refreshed by every successful synchronization. Over the API they are
// read-only outputs; client-supplied values are ignored (create) or
// discarded (update). Synchronization always re-resolves the group by
// GroupDirectoryId and fails WITHOUT reconciling when it cannot be resolved.
//
// Immutability: GroupId, ConnectionId and GroupDirectoryId are fixed once the
// mapping exists (see PutLDAPGroupSync); only the attribute mapping,
// ExcludeDisabled, Enabled and SyncFreq are editable.
type LDAPGroupSync struct {
	Id           int64 `json:"id" gorm:"column:id;primaryKey"`
	UserId       int64 `json:"-" gorm:"column:user_id"`
	TeamId       int64 `json:"-" gorm:"column:team_id"`
	GroupId      int64 `json:"group_id" gorm:"column:group_id"`
	ConnectionId int64 `json:"connection_id" gorm:"column:connection_id"`
	Enabled      bool  `json:"enabled"`
	// GroupDirectoryId is the stable identifier of the selected directory
	// group (normalized objectGUID for Active Directory). Authoritative and
	// immutable once the mapping exists.
	GroupDirectoryId string `json:"group_directory_id" gorm:"column:group_directory_id"`
	// GroupDN is the last-known, server-observed distinguished name of the
	// selected group (read-only over the API).
	GroupDN string `json:"group_dn" gorm:"column:group_dn"`
	// GroupName is the last-known, server-observed display name of the
	// selected group (read-only over the API).
	GroupName     string `json:"group_name" gorm:"column:group_name"`
	AttrEmail     string `json:"attr_email" gorm:"column:attr_email"`
	AttrFirstName string `json:"attr_first_name" gorm:"column:attr_first_name"`
	AttrLastName  string `json:"attr_last_name" gorm:"column:attr_last_name"`
	AttrPosition  string `json:"attr_position" gorm:"column:attr_position"`
	// ExcludeDisabled, when set, appends an AD-style filter clause excluding
	// disabled accounts (userAccountControl bit 2). It defaults to true for
	// new mappings even when omitted from the JSON payload (see
	// UnmarshalJSON); an explicit false is honored.
	ExcludeDisabled bool      `json:"exclude_disabled" gorm:"column:exclude_disabled"`
	SyncFreq        uint32    `json:"sync_freq" gorm:"column:sync_freq"`
	LastSync        time.Time `json:"last_sync,omitempty"`
	LastSyncStatus  string    `json:"last_sync_status,omitempty" gorm:"column:last_sync_status"`
	LastSyncError   string    `json:"last_sync_error,omitempty" gorm:"column:last_sync_error"`
	LastSyncAdded   int64     `json:"last_sync_added" gorm:"column:last_sync_added"`
	LastSyncUpdated int64     `json:"last_sync_updated" gorm:"column:last_sync_updated"`
	LastSyncRemoved int64     `json:"last_sync_removed" gorm:"column:last_sync_removed"`
	// LastSyncSkipped counts direct group members discovered in the directory
	// during the last run that could not be mapped to a target because the
	// configured email attribute was empty or not a valid address. A large
	// value explains "N users in AD, few targets in Gophish" without exposing
	// any member value.
	LastSyncSkipped int64     `json:"last_sync_skipped" gorm:"column:last_sync_skipped"`
	ModifiedDate    time.Time `json:"modified_date"`
}

// TableName specifies the database tablename for Gorm to use.
func (LDAPGroupSync) TableName() string {
	return "ldap_group_sync"
}

// UnmarshalJSON decodes a mapping with ExcludeDisabled defaulting to true when
// the field is omitted (a plain bool would otherwise silently become false),
// while an explicit true/false is honored as sent. GORM does not use this
// method, so stored rows always round-trip their persisted value.
func (s *LDAPGroupSync) UnmarshalJSON(b []byte) error {
	type plain LDAPGroupSync // no methods: avoids recursing into UnmarshalJSON
	in := plain(*s)
	in.ExcludeDisabled = true
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}
	*s = LDAPGroupSync(in)
	return nil
}

// LDAP validation errors.
var (
	ErrLDAPNameNotSpecified      = errors.New("no LDAP connection name specified")
	ErrLDAPServerURLNotSpecified = errors.New("no LDAP server URL specified")
	ErrLDAPInvalidServerURL      = errors.New("invalid LDAP server URL (expected ldap:// or ldaps://)")
	ErrLDAPBindDNNotSpecified    = errors.New("no bind DN specified")
	ErrLDAPPasswordNotSpecified  = errors.New("no bind password specified")
	// ErrLDAPSecretKeyNotConfigured mirrors ErrOAuthSecretKeyNotConfigured:
	// the bind password is encrypted at rest, so it can't be stored without
	// secret_key. Caught at save time so the admin finds out early.
	ErrLDAPSecretKeyNotConfigured = errors.New("storing an LDAP bind password requires secret_key to be configured in config.json")
	// ErrLDAPSecretKeyInvalid is returned when secret_key is configured but
	// not a valid base64-encoded 32-byte key, so it can't be used to encrypt
	// the bind password. The configured value itself is never included.
	ErrLDAPSecretKeyInvalid       = errors.New("secret_key in config.json is invalid: it must be a base64-encoded 32-byte key (generate with: openssl rand -base64 32)")
	ErrLDAPBaseDNNotSpecified     = errors.New("no base DN specified")
	ErrLDAPGroupNotSpecified      = errors.New("no group specified for LDAP sync")
	ErrLDAPConnectionNotSpecified = errors.New("no LDAP connection specified for sync")
	ErrLDAPConnectionNotFound     = errors.New("referenced LDAP connection not found for this team")
	ErrLDAPGroupNotFound          = errors.New("referenced group not found for this team")
	// ErrLDAPConnectionInUse is returned when deleting a connection that is
	// still referenced by one or more group-sync mappings.
	ErrLDAPConnectionInUse = errors.New("this LDAP connection is used by one or more group synchronization mappings; remove or reassign those mappings before deleting it")
	// ErrLDAPConnectionNotActiveDirectory is returned when a managed-group
	// mapping references a connection that is not Active Directory. Group-based
	// synchronization is AD-only in v1; Generic LDAP connections remain usable
	// for connection management only.
	ErrLDAPConnectionNotActiveDirectory = errors.New("directory-managed groups currently require an Active Directory connection (stable identity objectGUID); Generic LDAP connections are not supported for group synchronization")
	// ErrLDAPGroupDirectoryIDNotSpecified is returned when no directory group
	// has been selected for the mapping.
	ErrLDAPGroupDirectoryIDNotSpecified = errors.New("no Active Directory group selected for synchronization")
	// ErrLDAPGroupDirectoryIDInvalid is returned when the selected group's
	// stable identifier is not a valid objectGUID.
	ErrLDAPGroupDirectoryIDInvalid = errors.New("the selected Active Directory group identifier is not a valid objectGUID")
	// ErrLDAPGroupDNNotSpecified is returned when the selected group's DN is
	// missing.
	ErrLDAPGroupDNNotSpecified = errors.New("the selected Active Directory group has no distinguished name (DN)")
	// ErrLDAPGroupDNInvalid is returned when the selected group's DN is not a
	// valid distinguished name.
	ErrLDAPGroupDNInvalid = errors.New("the selected Active Directory group DN is not a valid LDAP distinguished name (DN)")
	// ErrLDAPGroupDNOutsideConnection is returned when the selected group's DN
	// is not within the connection's configured Base DN - a mapping may never
	// reference a directory object outside the connection boundary.
	ErrLDAPGroupDNOutsideConnection = errors.New("the selected Active Directory group must be within the connection's Base DN")
	// ErrLDAPGroupNameNotSpecified is returned when the selected group's
	// display name is missing.
	ErrLDAPGroupNameNotSpecified = errors.New("the selected Active Directory group has no name")
	// ErrLDAPMappedGroupImmutable is returned when an update tries to point an
	// existing mapping at a different Gophish group.
	ErrLDAPMappedGroupImmutable = errors.New("the Gophish group of an existing LDAP synchronization mapping cannot be changed")
	// ErrLDAPConnectionImmutable is returned when an update tries to change the
	// connection of an existing mapping (fixed in v1).
	ErrLDAPConnectionImmutable = errors.New("the LDAP connection of an existing synchronization mapping cannot be changed")
	// ErrLDAPGroupSelectionImmutable is returned when an update tries to
	// change the selected Active Directory group of an existing mapping (fixed
	// in v1; create a new managed group instead).
	ErrLDAPGroupSelectionImmutable = errors.New("the selected Active Directory group of an existing synchronization mapping cannot be changed")
)

// Validate checks an LDAPConnection before persisting it. isNew indicates an
// insert (bind password required) vs. an update (blank password = unchanged).
func (c *LDAPConnection) Validate(isNew bool) error {
	c.Name = strings.TrimSpace(c.Name)
	c.ServerURL = strings.TrimSpace(c.ServerURL)
	switch {
	case c.Name == "":
		return ErrLDAPNameNotSpecified
	case c.ServerURL == "":
		return ErrLDAPServerURLNotSpecified
	case c.BindDN == "":
		return ErrLDAPBindDNNotSpecified
	case c.BaseDN == "":
		return ErrLDAPBaseDNNotSpecified
	}

	u, err := url.Parse(c.ServerURL)
	if err != nil || (u.Scheme != "ldap" && u.Scheme != "ldaps") || u.Host == "" {
		return ErrLDAPInvalidServerURL
	}

	// A bind password is required at creation. On update, a blank value
	// means "keep the existing encrypted password" (handled in PutLDAPConnection).
	if isNew && c.BindPassword == "" {
		return ErrLDAPPasswordNotSpecified
	}
	// Encrypting a (new) password needs a usable secret_key; fail fast with a
	// specific, actionable error - missing vs. configured-but-invalid.
	if c.BindPassword != "" {
		if _, err := conf.SecretKeyBytes(); err != nil {
			return classifySecretKeyError(err)
		}
	}
	return nil
}

// classifySecretKeyError maps a SecretKeyBytes() failure onto a specific LDAP
// error: a missing key keeps the "not configured" message, while a configured
// but undecodable/wrong-length key reports the expected format. The configured
// value is never included in either error.
func classifySecretKeyError(err error) error {
	if errors.Is(err, config.ErrSecretKeyNotConfigured) {
		return ErrLDAPSecretKeyNotConfigured
	}
	return ErrLDAPSecretKeyInvalid
}

// encryptBindPassword encrypts c.BindPassword into c.BindPasswordEnc using the
// configured secret_key, mirroring StoreOAuthToken's handling of IMAP tokens.
func (c *LDAPConnection) encryptBindPassword() error {
	key, err := conf.SecretKeyBytes()
	if err != nil {
		return classifySecretKeyError(err)
	}
	enc, err := gophishcrypto.Encrypt([]byte(c.BindPassword), key)
	if err != nil {
		return err
	}
	c.BindPasswordEnc = enc
	return nil
}

// DecryptBindPassword returns the plaintext bind password for use when
// dialing the directory. It never leaves the server process.
func (c *LDAPConnection) DecryptBindPassword() (string, error) {
	if c.BindPasswordEnc == "" {
		return "", nil
	}
	key, err := conf.SecretKeyBytes()
	if err != nil {
		return "", err
	}
	plain, err := gophishcrypto.Decrypt(c.BindPasswordEnc, key)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// GetLDAPConnections returns the LDAP connections visible to the given team.
func GetLDAPConnections(teamID int64) ([]LDAPConnection, error) {
	cs := []LDAPConnection{}
	err := db.Where("team_id=?", teamID).Find(&cs).Error
	if err != nil {
		log.Error(err)
	}
	return cs, err
}

// LDAPConnectionSummary is the minimal, non-sensitive view of a connection
// exposed to object-tier (modify_objects) callers so the Groups UI can offer a
// connection picker and show the administrator-defined browse root. It
// deliberately omits server URL, bind identity/credential, TLS/security
// settings, directory_id_attr, and test details.
type LDAPConnectionSummary struct {
	Id     int64  `json:"id"`
	Name   string `json:"name"`
	BaseDN string `json:"base_dn"`
}

// GetLDAPConnectionSummaries returns id/name/base_dn for every connection
// owned by the given team. No credentials or infrastructure configuration are
// included.
func GetLDAPConnectionSummaries(teamID int64) ([]LDAPConnectionSummary, error) {
	out := []LDAPConnectionSummary{}
	err := db.Model(&LDAPConnection{}).
		Where("team_id=?", teamID).
		Select("id, name, base_dn").
		Scan(&out).Error
	if err != nil {
		log.Error(err)
	}
	return out, err
}

// GetLDAPConnection returns a single connection scoped to the team.
func GetLDAPConnection(id int64, teamID int64) (LDAPConnection, error) {
	c := LDAPConnection{}
	err := db.Where("team_id=? and id=?", teamID, id).First(&c).Error
	return c, err
}

// PostLDAPConnection creates a new connection.
func PostLDAPConnection(c *LDAPConnection) error {
	if err := c.Validate(true); err != nil {
		return err
	}
	if err := c.encryptBindPassword(); err != nil {
		return err
	}
	c.ModifiedDate = time.Now().UTC()
	err := db.Create(c).Error
	if err != nil {
		log.Error(err)
	}
	return err
}

// PutLDAPConnection updates an existing connection. A blank BindPassword
// preserves the stored encrypted password.
func PutLDAPConnection(c *LDAPConnection) error {
	if err := c.Validate(false); err != nil {
		return err
	}
	existing, err := GetLDAPConnection(c.Id, c.TeamId)
	if err != nil {
		return err
	}
	if c.BindPassword == "" {
		// Keep the previously stored secret unchanged.
		c.BindPasswordEnc = existing.BindPasswordEnc
	} else {
		if err := c.encryptBindPassword(); err != nil {
			return err
		}
	}
	c.ModifiedDate = time.Now().UTC()
	err = db.Model(&LDAPConnection{}).Where("id=? and team_id=?", c.Id, c.TeamId).Updates(map[string]interface{}{
		"name":               c.Name,
		"server_url":         c.ServerURL,
		"bind_dn":            c.BindDN,
		"bind_password_enc":  c.BindPasswordEnc,
		"base_dn":            c.BaseDN,
		"start_tls":          c.StartTLS,
		"ignore_cert_errors": c.IgnoreCertErrors,
		"directory_id_attr":  c.DirectoryIDAttr,
		"modified_date":      c.ModifiedDate,
	}).Error
	if err != nil {
		log.Error(err)
	}
	return err
}

// DeleteLDAPConnection removes a connection, but only if no group-sync
// mapping still references it. A referenced connection is kept and
// ErrLDAPConnectionInUse is returned so the operator removes/reassigns the
// mappings first - this avoids silently tearing down synchronization and
// never touches the underlying Gophish groups.
func DeleteLDAPConnection(id int64, teamID int64) error {
	return db.Transaction(func(tx *gorm.DB) error {
		// Confirm the connection exists for this team before doing anything.
		var count int64
		if err := tx.Model(&LDAPConnection{}).Where("id=? and team_id=?", id, teamID).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			return gorm.ErrRecordNotFound
		}
		var refs int64
		if err := tx.Model(&LDAPGroupSync{}).Where("connection_id=? and team_id=?", id, teamID).Count(&refs).Error; err != nil {
			return err
		}
		if refs > 0 {
			return ErrLDAPConnectionInUse
		}
		// The connection has no mappings; remove its connection-scoped identity
		// records (they are meaningless without the connection), then the
		// connection itself. Targets and campaign history are untouched.
		if err := tx.Where("connection_id=? and team_id=?", id, teamID).Delete(&LDAPTargetIdentity{}).Error; err != nil {
			return err
		}
		return tx.Where("id=? and team_id=?", id, teamID).Delete(&LDAPConnection{}).Error
	})
}

// RecordLDAPTestResult stores the outcome of a connection test.
func RecordLDAPTestResult(id int64, teamID int64, testErr error) error {
	status := "ok"
	msg := ""
	if testErr != nil {
		status = "error"
		msg = testErr.Error()
	}
	return db.Model(&LDAPConnection{}).Where("id=? and team_id=?", id, teamID).Updates(map[string]interface{}{
		"last_test_status": status,
		"last_test_error":  msg,
	}).Error
}

// normalize trims the directory-group selection and fills in defaults for a
// group-sync mapping. (ExcludeDisabled defaults to true at the schema and UI
// level; a Go bool cannot distinguish "unset" from false, so it is not
// touched here.)
func (s *LDAPGroupSync) normalize() {
	s.GroupDirectoryId = strings.TrimSpace(s.GroupDirectoryId)
	s.GroupDN = strings.TrimSpace(s.GroupDN)
	s.GroupName = strings.TrimSpace(s.GroupName)
	if s.AttrEmail == "" {
		s.AttrEmail = DefaultLDAPAttrEmail
	}
	if s.AttrFirstName == "" {
		s.AttrFirstName = DefaultLDAPAttrFirstName
	}
	if s.AttrLastName == "" {
		s.AttrLastName = DefaultLDAPAttrLastName
	}
	if s.AttrPosition == "" {
		s.AttrPosition = DefaultLDAPAttrPosition
	}
	if s.SyncFreq < MinLDAPSyncFreq {
		s.SyncFreq = DefaultLDAPSyncFreq
	}
}

// Validate checks a group-sync mapping and enforces the same-team invariant:
// the referenced group and connection must belong to the mapping's team.
func (s *LDAPGroupSync) Validate() error {
	if s.GroupId == 0 {
		s.normalize()
		return ErrLDAPGroupNotSpecified
	}
	if err := s.validateConfig(); err != nil {
		return err
	}
	if _, err := GetGroup(s.GroupId, s.TeamId); err != nil {
		return ErrLDAPGroupNotFound
	}
	return nil
}

// validateDirectoryGroup validates the selected directory group against the
// (already team-verified) connection:
//   - GroupDirectoryId is required and is canonicalized with the same
//     normalization used for synchronized user identities, so a value typed or
//     relayed in any case/brace style compares equal to what sync observes;
//   - GroupDN is required, must parse as a DN and must be within the
//     connection's BaseDN (a mapping can never point outside the boundary);
//   - GroupName is required.
//
// This is shape/containment validation only. It deliberately performs no
// directory access: the client-supplied DN/name are last-known metadata, and
// authoritative resolution by GroupDirectoryId happens server-side at
// selection and on every synchronization.
func (s *LDAPGroupSync) validateDirectoryGroup(conn *LDAPConnection) error {
	if s.GroupDirectoryId == "" {
		return ErrLDAPGroupDirectoryIDNotSpecified
	}
	norm, err := NormalizeDirectoryIDText(conn.DirectoryIDAttr, s.GroupDirectoryId)
	if err != nil {
		return ErrLDAPGroupDirectoryIDInvalid
	}
	s.GroupDirectoryId = norm
	if s.GroupDN == "" {
		return ErrLDAPGroupDNNotSpecified
	}
	within, err := dnutil.Within(s.GroupDN, conn.BaseDN)
	if err != nil {
		// A malformed candidate or base DN. Neither value is echoed.
		return ErrLDAPGroupDNInvalid
	}
	if !within {
		return ErrLDAPGroupDNOutsideConnection
	}
	if s.GroupName == "" {
		return ErrLDAPGroupNameNotSpecified
	}
	return nil
}

// GetLDAPGroupSyncs returns all group-sync mappings for the team.
func GetLDAPGroupSyncs(teamID int64) ([]LDAPGroupSync, error) {
	ss := []LDAPGroupSync{}
	err := db.Where("team_id=?", teamID).Find(&ss).Error
	if err != nil {
		log.Error(err)
	}
	return ss, err
}

// GetAllEnabledLDAPGroupSyncs returns every enabled mapping across all teams.
// Used by the background monitor.
func GetAllEnabledLDAPGroupSyncs() ([]LDAPGroupSync, error) {
	ss := []LDAPGroupSync{}
	err := db.Where("enabled=?", true).Find(&ss).Error
	return ss, err
}

// GetLDAPGroupSync returns a single mapping scoped to the team.
func GetLDAPGroupSync(id int64, teamID int64) (LDAPGroupSync, error) {
	s := LDAPGroupSync{}
	err := db.Where("team_id=? and id=?", teamID, id).First(&s).Error
	return s, err
}

// GetLDAPGroupSyncByID returns a mapping by id without a team filter, for the
// background monitor which already iterates team-scoped rows.
func GetLDAPGroupSyncByID(id int64) (LDAPGroupSync, error) {
	s := LDAPGroupSync{}
	err := db.Where("id=?", id).First(&s).Error
	return s, err
}

// GetLDAPGroupSyncByGroup returns the sync mapping that manages the given
// group within the team, if any.
func GetLDAPGroupSyncByGroup(groupID int64, teamID int64) (LDAPGroupSync, error) {
	s := LDAPGroupSync{}
	err := db.Where("group_id=? and team_id=?", groupID, teamID).First(&s).Error
	return s, err
}

// PostLDAPGroupSync creates a new mapping.
func PostLDAPGroupSync(s *LDAPGroupSync) error {
	if err := s.Validate(); err != nil {
		return err
	}
	s.ModifiedDate = time.Now().UTC()
	err := db.Create(s).Error
	if err != nil {
		log.Error(err)
	}
	return err
}

// validateConfig validates the sync mapping's own configuration (connection
// ownership, directory type, selected directory group, attributes, interval)
// WITHOUT requiring the target group to already exist. This is the portion of
// Validate() safe to run inside the create-group transaction, where the group
// row is not yet committed. Group existence is validated by the full
// Validate() used on the standalone mapping endpoints.
func (s *LDAPGroupSync) validateConfig() error {
	s.normalize()
	if s.ConnectionId == 0 {
		return ErrLDAPConnectionNotSpecified
	}
	// Same-team invariant - enforced in Go, consistent with the rest of the
	// codebase, rather than relying on DB foreign keys.
	conn, err := GetLDAPConnection(s.ConnectionId, s.TeamId)
	if err != nil {
		return ErrLDAPConnectionNotFound
	}
	// Group-based synchronization is Active Directory-only in v1.
	if !conn.IsActiveDirectory() {
		return ErrLDAPConnectionNotActiveDirectory
	}
	return s.validateDirectoryGroup(&conn)
}

// PostLDAPManagedGroup atomically creates an LDAP-managed group and its
// synchronization mapping in a single transaction.
//
// This is the narrowest path that lets an LDAP-managed group be created
// without manually supplied members: it deliberately does NOT call
// Group.Validate() (which requires at least one target for manual groups).
// Manual group creation (PostGroup) is unchanged. The group starts empty and
// is populated by the first LDAP synchronization.
//
// Atomicity: the group and the mapping are inserted in one transaction. If the
// mapping is invalid or its insert fails, the whole transaction rolls back, so
// no empty/orphan group is left behind.
func PostLDAPManagedGroup(g *Group, s *LDAPGroupSync) error {
	// The group still needs a name and a team; reuse the shared helpers but
	// skip the "must have targets" rule that only applies to manual groups.
	// The name is trimmed: it is typically auto-filled from the selected AD
	// group and must never carry stray whitespace.
	g.Name = strings.TrimSpace(g.Name)
	if g.Name == "" {
		return ErrGroupNameNotSpecified
	}
	if err := setTeamIdFromUser(&g.TeamId, g.UserId); err != nil {
		return err
	}
	// The mapping lives on the same team as the group and references the group
	// we are about to create.
	s.TeamId = g.TeamId
	s.UserId = g.UserId
	if err := s.validateConfig(); err != nil {
		return err
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		// Insert the (member-less) group.
		if err := tx.Save(g).Error; err != nil {
			return err
		}
		// Link the mapping to the freshly created group and insert it. Any
		// failure here rolls back the group insert above.
		s.GroupId = g.Id
		s.ModifiedDate = time.Now().UTC()
		if err := tx.Create(s).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		log.Error(err)
		return err
	}
	// Tags are set after commit, matching PostGroup's behavior.
	return setTagsFor(TaggableGroup, g.Id, g.TeamId, g.Tags)
}

// IsGroupLDAPManaged reports whether the given group (within the team) is
// managed by an LDAP synchronization mapping. Used to enforce that LDAP-managed
// group membership is owned exclusively by synchronization and cannot be edited
// manually through the group API.
func IsGroupLDAPManaged(groupID int64, teamID int64) (bool, error) {
	var count int64
	err := db.Model(&LDAPGroupSync{}).Where("group_id=? and team_id=?", groupID, teamID).Count(&count).Error
	if err != nil {
		log.Error(err)
		return false, err
	}
	return count > 0, nil
}

// LDAPTargetIdentity maps a stable directory-object identifier, scoped to a
// specific LDAPConnection (directory), to a Gophish target. Identity is unique
// per (ConnectionId, DirectoryId): the identifier is only meaningful inside its
// own directory, never globally - two different connections may legitimately
// surface the same email or even colliding raw identifiers without being the
// same logical object. These rows persist across group-membership removal and
// mapping deletion; they are removed only when the owning connection is
// deleted.
type LDAPTargetIdentity struct {
	Id           int64     `json:"-" gorm:"column:id;primaryKey"`
	TeamId       int64     `json:"-" gorm:"column:team_id"`
	ConnectionId int64     `json:"-" gorm:"column:connection_id"`
	DirectoryId  string    `json:"-" gorm:"column:directory_id"`
	TargetId     int64     `json:"-" gorm:"column:target_id"`
	ModifiedDate time.Time `json:"-"`
}

// TableName specifies the database tablename for Gorm to use.
func (LDAPTargetIdentity) TableName() string {
	return "ldap_target_identity"
}

// getIdentityTarget returns the target id previously bound to (connectionID,
// directoryID), or (0, false) if none exists.
func getIdentityTarget(tx *gorm.DB, connectionID int64, directoryID string) (int64, bool, error) {
	var ident LDAPTargetIdentity
	err := tx.Where("connection_id=? and directory_id=?", connectionID, directoryID).First(&ident).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return ident.TargetId, true, nil
}

// bindIdentity records that (connectionID, directoryID) maps to targetID. Safe
// to call when the row already exists (keeps the existing mapping).
func bindIdentity(tx *gorm.DB, teamID, connectionID int64, directoryID string, targetID int64) error {
	ident := LDAPTargetIdentity{
		TeamId:       teamID,
		ConnectionId: connectionID,
		DirectoryId:  directoryID,
		TargetId:     targetID,
		ModifiedDate: time.Now().UTC(),
	}
	// Insert only if not present; the unique (connection_id, directory_id)
	// index guarantees one row per directory object.
	return tx.Where("connection_id=? and directory_id=?", connectionID, directoryID).
		FirstOrCreate(&ident).Error
}

// PutLDAPGroupSync updates the editable settings of an existing mapping:
// attribute mapping, ExcludeDisabled, Enabled and SyncFreq. Everything else is
// fixed in v1 and taken from the stored row:
//   - GroupId, ConnectionId and GroupDirectoryId are immutable; a request that
//     supplies a different non-empty value is rejected (a zero/empty value
//     means "unchanged");
//   - GroupDN/GroupName are server-owned (set by directory resolution and
//     refreshed by synchronization) and any client-supplied value is discarded.
//
// Sync-state columns are written by the sync routine, not here.
func PutLDAPGroupSync(s *LDAPGroupSync) error {
	existing, err := GetLDAPGroupSync(s.Id, s.TeamId)
	if err != nil {
		return err
	}
	if s.GroupId != 0 && s.GroupId != existing.GroupId {
		return ErrLDAPMappedGroupImmutable
	}
	if s.ConnectionId != 0 && s.ConnectionId != existing.ConnectionId {
		return ErrLDAPConnectionImmutable
	}
	if requested := strings.TrimSpace(s.GroupDirectoryId); requested != "" {
		norm, nerr := NormalizeDirectoryIDText(DirectoryIDAttrObjectGUID, requested)
		if nerr != nil || norm != existing.GroupDirectoryId {
			return ErrLDAPGroupSelectionImmutable
		}
	}
	// Immutable and server-owned fields always come from the stored row.
	s.GroupId = existing.GroupId
	s.ConnectionId = existing.ConnectionId
	s.GroupDirectoryId = existing.GroupDirectoryId
	s.GroupDN = existing.GroupDN
	s.GroupName = existing.GroupName
	if err := s.Validate(); err != nil {
		return err
	}
	s.ModifiedDate = time.Now().UTC()
	err = db.Model(&LDAPGroupSync{}).Where("id=? and team_id=?", s.Id, s.TeamId).Updates(map[string]interface{}{
		"enabled":          s.Enabled,
		"attr_email":       s.AttrEmail,
		"attr_first_name":  s.AttrFirstName,
		"attr_last_name":   s.AttrLastName,
		"attr_position":    s.AttrPosition,
		"exclude_disabled": s.ExcludeDisabled,
		"sync_freq":        s.SyncFreq,
		"modified_date":    s.ModifiedDate,
	}).Error
	if err != nil {
		log.Error(err)
	}
	return err
}

// UpdateLDAPGroupSyncDirectoryGroup refreshes the server-observed DN and name
// of a mapping's selected directory group after a successful resolution by
// GroupDirectoryId (the identity itself is never changed here). It is the only
// write path for group_dn/group_name after creation. The DN must still lie
// within the mapping's connection BaseDN.
func UpdateLDAPGroupSyncDirectoryGroup(id, teamID int64, dn, name string) error {
	existing, err := GetLDAPGroupSync(id, teamID)
	if err != nil {
		return err
	}
	conn, err := GetLDAPConnection(existing.ConnectionId, teamID)
	if err != nil {
		return ErrLDAPConnectionNotFound
	}
	dn = strings.TrimSpace(dn)
	name = strings.TrimSpace(name)
	within, werr := dnutil.Within(dn, conn.BaseDN)
	if werr != nil {
		return ErrLDAPGroupDNInvalid
	}
	if !within {
		return ErrLDAPGroupDNOutsideConnection
	}
	if name == "" {
		return ErrLDAPGroupNameNotSpecified
	}
	err = db.Model(&LDAPGroupSync{}).Where("id=? and team_id=?", id, teamID).Updates(map[string]interface{}{
		"group_dn":   dn,
		"group_name": name,
	}).Error
	if err != nil {
		log.Error(err)
	}
	return err
}

// DeleteLDAPGroupSync removes a mapping. The underlying Gophish group and its
// current targets are intentionally left intact - deleting the mapping only
// stops future synchronization.
func DeleteLDAPGroupSync(id int64, teamID int64) error {
	err := db.Where("id=? and team_id=?", id, teamID).Delete(&LDAPGroupSync{}).Error
	if err != nil {
		log.Error(err)
	}
	return err
}

// RecordLDAPSyncResult stores the outcome of a sync run on the mapping.
// skipped is the number of discovered direct members that could not be mapped
// to a target (empty/invalid configured email); it is recorded alongside the
// reconciliation counts so a low target count is never silently unexplained.
func RecordLDAPSyncResult(id int64, teamID int64, added, updated, removed, skipped int, syncErr error) error {
	status := "ok"
	msg := ""
	if syncErr != nil {
		status = "error"
		msg = syncErr.Error()
	}
	return db.Model(&LDAPGroupSync{}).Where("id=? and team_id=?", id, teamID).Updates(map[string]interface{}{
		"last_sync":         time.Now().UTC(),
		"last_sync_status":  status,
		"last_sync_error":   msg,
		"last_sync_added":   added,
		"last_sync_updated": updated,
		"last_sync_removed": removed,
		"last_sync_skipped": skipped,
	}).Error
}

// ReconcileResult reports what a reconciliation changed.
type ReconcileResult struct {
	Added   int
	Updated int
	Removed int
}

// ReconcileGroupTargets brings a group's membership in line with the supplied
// desired set, reusing the exact same add/update/remove semantics as
// PutGroup: new targets are inserted, matching targets have their attributes
// updated, and targets no longer present have only their group membership
// (group_targets row) removed - the target row and any historical campaign
// results are preserved.
//
// This is the single reconciliation path shared by manual edits (PutGroup)
// and LDAP synchronization. Callers MUST NOT invoke it with a partial or
// failed directory read; see ldap.Sync, which only reconciles after a
// complete, successful search.
func ReconcileGroupTargets(groupID int64, teamID int64, desired []Target) (ReconcileResult, error) {
	res := ReconcileResult{}
	// Confirm the group belongs to the team before touching membership.
	g, err := GetGroup(groupID, teamID)
	if err != nil {
		return res, err
	}

	existing, err := GetTargets(g.Id)
	if err != nil {
		return res, err
	}

	// De-duplicate the desired set by email (same as PutGroup's cacheNew),
	// keeping the last occurrence's attributes.
	desiredByEmail := make(map[string]Target, len(desired))
	for _, t := range desired {
		desiredByEmail[t.Email] = t
	}
	existingByEmail := make(map[string]Target, len(existing))
	for _, t := range existing {
		existingByEmail[t.Email] = t
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		// Remove memberships no longer present in the desired set. Only the
		// join row is deleted - never the target or its results.
		for _, t := range existing {
			if _, ok := desiredByEmail[t.Email]; ok {
				continue
			}
			if derr := tx.Where("group_id=? and target_id=?", g.Id, t.Id).Delete(&GroupTarget{}).Error; derr != nil {
				return derr
			}
			res.Removed++
		}
		// Add new targets and update changed ones.
		for _, nt := range desired {
			if cur, ok := existingByEmail[nt.Email]; ok {
				nt.Id = cur.Id
				if uerr := UpdateTarget(tx, nt); uerr != nil {
					return uerr
				}
				res.Updated++
				continue
			}
			if ierr := insertTargetIntoGroup(tx, nt, g.Id); ierr != nil {
				return ierr
			}
			res.Added++
		}
		return nil
	})
	if err != nil {
		return ReconcileResult{}, err
	}
	return res, nil
}

// DirectoryTarget is a target discovered in a directory search, carrying the
// optional stable directory identifier read from the connection's configured
// DirectoryIDAttr. DirectoryId is empty when the directory/entry exposes no
// stable identifier, in which case reconciliation falls back to email keying.
type DirectoryTarget struct {
	Target
	DirectoryId string
}

// updateTargetRecipient updates all synchronized recipient attributes on a
// target matched by stable identity, INCLUDING email. (The manual path's
// UpdateTarget intentionally never changes email because email is its match
// key; the identity path matches by directory id, so an email change is a
// legitimate in-place update of the same logical object.)
func updateTargetRecipient(tx *gorm.DB, targetID int64, r BaseRecipient) error {
	return tx.Model(&Target{}).Where("id = ?", targetID).Updates(map[string]interface{}{
		"email":      r.Email,
		"first_name": r.FirstName,
		"last_name":  r.LastName,
		"position":   r.Position,
	}).Error
}

// createIsolatedTarget always inserts a NEW target row (no value-based
// FirstOrCreate dedup) and links it to the group. This is used by the
// identity path so a directory object from one connection never silently
// binds to an existing target that merely shares an email with a manual
// target or a different connection's object - preserving connection-scoped
// identity isolation.
func createIsolatedTarget(tx *gorm.DB, r BaseRecipient, gid int64) (int64, error) {
	if _, err := mail.ParseAddress(r.Email); err != nil {
		return 0, err
	}
	t := Target{BaseRecipient: r}
	if err := tx.Create(&t).Error; err != nil {
		return 0, err
	}
	if err := tx.Create(&GroupTarget{GroupId: gid, TargetId: t.Id}).Error; err != nil {
		return 0, err
	}
	return t.Id, nil
}

// ReconcileGroupTargetsWithIdentity brings an LDAP-managed group's membership
// in line with the desired directory set, using connection-scoped stable
// identity when available and falling back to email keying otherwise.
//
// Matching rules (per the approved identity model):
//   - Entry WITH a directory id: match by (connectionID, directory_id) via the
//     identity table. If bound, reuse that exact target and update its
//     attributes (incl. email). If unbound, create a brand-new isolated target
//     and record the identity binding - never reuse a target by email, so two
//     connections sharing an email are never merged.
//   - Entry WITHOUT a directory id: fall back to the email-keyed behavior of
//     ReconcileGroupTargets (current semantics; generic LDAP stays usable).
//
// Membership removal deletes only the group_targets join row; target rows,
// identity rows, and campaign history are preserved so a later re-entry
// reconnects to the same target. Manual groups never use this path.
func ReconcileGroupTargetsWithIdentity(groupID, teamID, connectionID int64, desired []DirectoryTarget) (ReconcileResult, error) {
	res := ReconcileResult{}
	g, err := GetGroup(groupID, teamID)
	if err != nil {
		return res, err
	}
	existing, err := GetTargets(g.Id)
	if err != nil {
		return res, err
	}

	// Index existing membership by target id and by email (for the fallback).
	existingByID := make(map[int64]Target, len(existing))
	existingByEmail := make(map[string]Target, len(existing))
	for _, t := range existing {
		existingByID[t.Id] = t
		existingByEmail[t.Email] = t
	}

	// Split desired entries into identity-keyed and email-fallback sets, and
	// de-duplicate: identity entries by directory id, fallback entries by email.
	identityDesired := make(map[string]DirectoryTarget)
	emailDesired := make(map[string]Target)
	for _, d := range desired {
		if d.DirectoryId != "" {
			identityDesired[d.DirectoryId] = d
		} else {
			emailDesired[d.Email] = d.Target
		}
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		// Resolve each identity-keyed desired entry to a concrete target id,
		// and track which target ids should remain members this run.
		keep := make(map[int64]bool)

		for dirID, d := range identityDesired {
			targetID, bound, gerr := getIdentityTarget(tx, connectionID, dirID)
			if gerr != nil {
				return gerr
			}
			if bound {
				// Known directory object: update its attributes in place.
				if uerr := updateTargetRecipient(tx, targetID, d.BaseRecipient); uerr != nil {
					return uerr
				}
				// Ensure it is a member of this group (it may have been
				// removed earlier and is now re-entering scope).
				if _, isMember := existingByID[targetID]; !isMember {
					if cerr := tx.Create(&GroupTarget{GroupId: g.Id, TargetId: targetID}).Error; cerr != nil {
						return cerr
					}
					res.Added++
				} else {
					res.Updated++
				}
				keep[targetID] = true
				continue
			}
			// New directory object: create an isolated target, link it, and
			// record the identity binding.
			newID, cerr := createIsolatedTarget(tx, d.BaseRecipient, g.Id)
			if cerr != nil {
				return cerr
			}
			if berr := bindIdentity(tx, teamID, connectionID, dirID, newID); berr != nil {
				return berr
			}
			keep[newID] = true
			res.Added++
		}

		// Email-fallback entries (no stable id): reuse current email-keyed
		// behavior for membership within this group.
		for email, nt := range emailDesired {
			if cur, ok := existingByEmail[email]; ok {
				nt.Id = cur.Id
				if uerr := UpdateTarget(tx, nt); uerr != nil {
					return uerr
				}
				keep[cur.Id] = true
				res.Updated++
				continue
			}
			if ierr := insertTargetIntoGroup(tx, nt, g.Id); ierr != nil {
				return ierr
			}
			// insertTargetIntoGroup may reuse an existing target by value;
			// re-read membership isn't necessary for counting here.
			res.Added++
		}

		// Remove memberships for existing targets not kept this run. Only the
		// join row is deleted; target + identity rows persist.
		for _, t := range existing {
			if keep[t.Id] {
				continue
			}
			if derr := tx.Where("group_id=? and target_id=?", g.Id, t.Id).Delete(&GroupTarget{}).Error; derr != nil {
				return derr
			}
			res.Removed++
		}
		return nil
	})
	if err != nil {
		return ReconcileResult{}, err
	}
	return res, nil
}

// NormalizeADObjectGUID converts a raw 16-byte Active Directory objectGUID into
// its canonical textual form xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx.
//
// AD stores a GUID as a mixed-endian binary value: the first three groups
// (uint32, uint16, uint16) are little-endian, the last two are big-endian. This
// matches the standard Microsoft GUID string rendering of the raw bytes.
func NormalizeADObjectGUID(raw []byte) (string, error) {
	if len(raw) != 16 {
		return "", fmt.Errorf("objectGUID must be 16 bytes, got %d", len(raw))
	}
	return fmt.Sprintf(
		"%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		raw[3], raw[2], raw[1], raw[0], // Data1 (little-endian)
		raw[5], raw[4], // Data2 (little-endian)
		raw[7], raw[6], // Data3 (little-endian)
		raw[8], raw[9], // Data4 (big-endian)
		raw[10], raw[11], raw[12], raw[13], raw[14], raw[15],
	), nil
}

// ParseADObjectGUID is the inverse of NormalizeADObjectGUID: it parses the
// canonical textual form xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx (any case,
// optional surrounding braces/whitespace) back into the raw 16-byte
// mixed-endian value Active Directory stores and matches in filters.
func ParseADObjectGUID(text string) ([]byte, error) {
	t := strings.TrimSpace(text)
	t = strings.TrimSuffix(strings.TrimPrefix(t, "{"), "}")
	if len(t) != 36 || t[8] != '-' || t[13] != '-' || t[18] != '-' || t[23] != '-' {
		return nil, fmt.Errorf("objectGUID must be in the form xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx")
	}
	hexOnly := strings.ReplaceAll(t, "-", "")
	d, err := hex.DecodeString(hexOnly)
	if err != nil || len(d) != 16 {
		return nil, fmt.Errorf("objectGUID contains non-hexadecimal characters")
	}
	// Undo the mixed-endian rendering: Data1/Data2/Data3 little-endian,
	// Data4 big-endian.
	raw := []byte{
		d[3], d[2], d[1], d[0],
		d[5], d[4],
		d[7], d[6],
		d[8], d[9], d[10], d[11], d[12], d[13], d[14], d[15],
	}
	return raw, nil
}

// NormalizeDirectoryID canonicalizes a stable directory identifier value for
// storage/comparison. For the AD objectGUID attribute the raw 16-byte value is
// rendered as a canonical GUID string; for textual identifiers such as
// entryUUID the value is lower-cased and trimmed without otherwise altering it.
// attrName is the configured DirectoryIDAttr (used to detect objectGUID).
func NormalizeDirectoryID(attrName string, raw []byte) (string, error) {
	if strings.EqualFold(strings.TrimSpace(attrName), DirectoryIDAttrObjectGUID) {
		return NormalizeADObjectGUID(raw)
	}
	return strings.ToLower(strings.TrimSpace(string(raw))), nil
}

// NormalizeDirectoryIDText canonicalizes a TEXTUAL stable identifier (as
// relayed by a client or stored on a mapping) so it is byte-for-byte equal to
// what NormalizeDirectoryID produces from the raw directory value. For
// objectGUID the text must be a valid GUID and is re-rendered through the
// same canonical formatter (round-trip via ParseADObjectGUID); for other
// identifiers it is lower-cased and trimmed.
func NormalizeDirectoryIDText(attrName, text string) (string, error) {
	if strings.EqualFold(strings.TrimSpace(attrName), DirectoryIDAttrObjectGUID) {
		raw, err := ParseADObjectGUID(text)
		if err != nil {
			return "", err
		}
		return NormalizeADObjectGUID(raw)
	}
	return strings.ToLower(strings.TrimSpace(text)), nil
}

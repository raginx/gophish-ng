package models

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	goose "github.com/pressly/goose/v3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Issue #78 ships one migration pair that creates the final LDAP / Active
// Directory schema directly. These tests prove the SQLite migration produces
// that schema through the normal repository migration sequence, and that the
// MySQL counterpart declares the same tables, columns and indexes.

const (
	sqliteMigrationsDir = "../db/db_sqlite3/migrations"
	mysqlMigrationsDir  = "../db/db_mysql/migrations"
	ldapMigrationFile   = "20261019120000_0.19.0_ldap_ad_group_sync.sql"
)

// ldapSchema is the final schema the implementation requires.
var ldapSchema = map[string][]string{
	"ldap_connection": {"id", "user_id", "team_id", "name", "server_url", "bind_dn", "bind_password_enc", "base_dn",
		"start_tls", "ignore_cert_errors", "directory_id_attr", "last_test_status", "last_test_error", "modified_date"},
	"ldap_group_sync": {"id", "user_id", "team_id", "group_id", "connection_id", "enabled",
		"group_directory_id", "group_dn", "group_name",
		"attr_email", "attr_first_name", "attr_last_name", "attr_position", "exclude_disabled", "sync_freq",
		"last_sync", "last_sync_status", "last_sync_error", "last_sync_added", "last_sync_updated", "last_sync_removed",
		"last_sync_skipped", "modified_date"},
	"ldap_target_identity": {"id", "team_id", "connection_id", "directory_id", "target_id", "modified_date"},
}

// obsoleteLDAPColumns must not exist anywhere in the final schema.
var obsoleteLDAPColumns = []string{"search_base", "search_filter", "search_scope"}

// openGooseDB opens a temp file-backed SQLite DB through the same gorm driver
// the app uses and returns it plus a cleanup.
func openGooseDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "migtest.db")
	gdb, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("open temp db: %v", err)
	}
	cleanup := func() {
		if sqlDB, derr := gdb.DB(); derr == nil {
			_ = sqlDB.Close()
		}
	}
	return gdb, cleanup
}

// migrateFresh runs the full repository migration sequence on a fresh DB.
func migrateFresh(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	gdb, cleanup := openGooseDB(t)
	sqlDB, err := gdb.DB()
	if err != nil {
		cleanup()
		t.Fatalf("sql db: %v", err)
	}
	if err := goose.SetDialect("sqlite3"); err != nil {
		cleanup()
		t.Fatalf("set dialect: %v", err)
	}
	if err := goose.Up(sqlDB, sqliteMigrationsDir); err != nil {
		cleanup()
		t.Fatalf("fresh Up: %v", err)
	}
	return gdb, cleanup
}

func hasColumn(t *testing.T, gdb *gorm.DB, table, column string) bool {
	t.Helper()
	var count int64
	err := gdb.Raw("SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?", table, column).Scan(&count).Error
	if err != nil {
		t.Fatalf("pragma_table_info(%s): %v", table, err)
	}
	return count > 0
}

func hasTable(t *testing.T, gdb *gorm.DB, table string) bool {
	t.Helper()
	var count int64
	err := gdb.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name = ?", table).Scan(&count).Error
	if err != nil {
		t.Fatalf("sqlite_master lookup: %v", err)
	}
	return count > 0
}

// indexUnique reports whether the named index exists on table and whether it
// is unique, via pragma_index_list.
func indexUnique(t *testing.T, gdb *gorm.DB, table, index string) (exists bool, unique bool) {
	t.Helper()
	type row struct {
		Name   string
		Unique int
	}
	var rows []row
	err := gdb.Raw("SELECT name, \"unique\" AS \"unique\" FROM pragma_index_list(?) WHERE name = ?", table, index).Scan(&rows).Error
	if err != nil {
		t.Fatalf("pragma_index_list(%s): %v", table, err)
	}
	if len(rows) == 0 {
		return false, false
	}
	return true, rows[0].Unique == 1
}

// TestLDAPMigrationFreshSchema verifies the normal migration sequence creates
// the final LDAP schema: all required tables and columns, no obsolete OU-era
// columns, and every column present exactly once.
func TestLDAPMigrationFreshSchema(t *testing.T) {
	gdb, cleanup := migrateFresh(t)
	defer cleanup()

	for table, cols := range ldapSchema {
		if !hasTable(t, gdb, table) {
			t.Fatalf("table %s missing", table)
		}
		for _, col := range cols {
			var n int64
			if err := gdb.Raw("SELECT COUNT(*) FROM pragma_table_info(?) WHERE name=?", table, col).Scan(&n).Error; err != nil {
				t.Fatalf("count %s.%s: %v", table, col, err)
			}
			if n != 1 {
				t.Fatalf("expected exactly one %s.%s column, got %d", table, col, n)
			}
		}
		for _, col := range obsoleteLDAPColumns {
			if hasColumn(t, gdb, table, col) {
				t.Fatalf("obsolete column %s.%s must not exist in the final schema", table, col)
			}
		}
	}

	// Defaults the implementation relies on.
	var attrEmail, excl, skipped string
	row := gdb.Raw("SELECT dflt_value FROM pragma_table_info('ldap_group_sync') WHERE name='attr_email'").Row()
	_ = row.Scan(&attrEmail)
	row = gdb.Raw("SELECT dflt_value FROM pragma_table_info('ldap_group_sync') WHERE name='exclude_disabled'").Row()
	_ = row.Scan(&excl)
	row = gdb.Raw("SELECT dflt_value FROM pragma_table_info('ldap_group_sync') WHERE name='last_sync_skipped'").Row()
	_ = row.Scan(&skipped)
	if attrEmail != "'mail'" || excl != "1" || skipped != "0" {
		t.Fatalf("unexpected defaults: attr_email=%s exclude_disabled=%s last_sync_skipped=%s", attrEmail, excl, skipped)
	}
}

// TestLDAPMigrationIndexes verifies the index/uniqueness contract.
func TestLDAPMigrationIndexes(t *testing.T) {
	gdb, cleanup := migrateFresh(t)
	defer cleanup()

	cases := []struct {
		table, index string
		unique       bool
	}{
		{"ldap_group_sync", "idx_ldap_group_sync_group_id", true},
		{"ldap_group_sync", "idx_ldap_group_sync_conn_group", false},
		{"ldap_target_identity", "idx_ldap_target_identity_conn_dir", true},
		{"ldap_target_identity", "idx_ldap_target_identity_target", false},
	}
	for _, c := range cases {
		exists, unique := indexUnique(t, gdb, c.table, c.index)
		if !exists {
			t.Fatalf("index %s on %s missing", c.index, c.table)
		}
		if unique != c.unique {
			t.Fatalf("index %s on %s: unique=%v, want %v", c.index, c.table, unique, c.unique)
		}
	}
}

// TestLDAPMigrationConstraintsBehave exercises the constraints with data:
// two Gophish groups may reference the same directory group on the same
// connection, a group may have only one mapping, and a directory identity may
// be bound only once per connection (but the same id may exist on another
// connection).
func TestLDAPMigrationConstraintsBehave(t *testing.T) {
	gdb, cleanup := migrateFresh(t)
	defer cleanup()

	const guid = "a1b2c3d4-e5f6-4788-99aa-bbccddeeff00"
	ins := "INSERT INTO ldap_group_sync (team_id, group_id, connection_id, enabled, group_directory_id, group_dn, group_name) VALUES (?,?,?,?,?,?,?)"
	if err := gdb.Exec(ins, 1, 10, 1, true, guid, "CN=G,DC=example,DC=com", "G").Error; err != nil {
		t.Fatalf("first mapping: %v", err)
	}
	// Same AD group, different Gophish group → allowed (non-unique index).
	if err := gdb.Exec(ins, 1, 11, 1, true, guid, "CN=G,DC=example,DC=com", "G").Error; err != nil {
		t.Fatalf("second mapping on the same directory group must be allowed: %v", err)
	}
	// Same Gophish group twice → rejected.
	if err := gdb.Exec(ins, 1, 10, 1, true, guid, "CN=G,DC=example,DC=com", "G").Error; err == nil {
		t.Fatal("a Gophish group must have at most one mapping")
	}

	idIns := "INSERT INTO ldap_target_identity (team_id, connection_id, directory_id, target_id) VALUES (?,?,?,?)"
	if err := gdb.Exec(idIns, 1, 1, guid, 100).Error; err != nil {
		t.Fatalf("identity: %v", err)
	}
	if err := gdb.Exec(idIns, 1, 1, guid, 101).Error; err == nil {
		t.Fatal("duplicate (connection_id, directory_id) identity binding must be rejected")
	}
	// The same directory id on another connection is a different object.
	if err := gdb.Exec(idIns, 1, 2, guid, 102).Error; err != nil {
		t.Fatalf("identity on another connection must be allowed: %v", err)
	}
}

// TestLDAPMigrationModelRoundTrip verifies the ORM models map onto the fresh
// schema (every mapped column exists) by writing and reading each table.
func TestLDAPMigrationModelRoundTrip(t *testing.T) {
	gdb, cleanup := migrateFresh(t)
	defer cleanup()

	conn := LDAPConnection{TeamId: 1, Name: "Corp AD", ServerURL: "ldaps://ad.example.com:636", BindDN: "cn=svc,dc=example,dc=com",
		BindPasswordEnc: "ENCRYPTED-PLACEHOLDER", BaseDN: "dc=example,dc=com", DirectoryIDAttr: "objectGUID"}
	if err := gdb.Create(&conn).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}
	m := LDAPGroupSync{TeamId: 1, GroupId: 1, ConnectionId: conn.Id, Enabled: true, GroupDirectoryId: "a1b2c3d4-e5f6-4788-99aa-bbccddeeff00",
		GroupDN: "CN=G,DC=example,DC=com", GroupName: "G", AttrEmail: "mail", AttrFirstName: "givenName", AttrLastName: "sn", AttrPosition: "title",
		ExcludeDisabled: true, SyncFreq: 3600, LastSyncSkipped: 2}
	if err := gdb.Create(&m).Error; err != nil {
		t.Fatalf("create mapping: %v", err)
	}
	ident := LDAPTargetIdentity{TeamId: 1, ConnectionId: conn.Id, DirectoryId: "guid-1", TargetId: 1}
	if err := gdb.Create(&ident).Error; err != nil {
		t.Fatalf("create identity: %v", err)
	}
	var back LDAPGroupSync
	if err := gdb.First(&back, m.Id).Error; err != nil {
		t.Fatalf("read mapping: %v", err)
	}
	if back.GroupDirectoryId != m.GroupDirectoryId || back.GroupName != "G" || back.LastSyncSkipped != 2 || !back.ExcludeDisabled {
		t.Fatalf("mapping did not round-trip: %+v", back)
	}
}

// --- MySQL counterpart (structural parity; no MySQL server in tests) --------

var (
	createTableRe = regexp.MustCompile("(?is)CREATE TABLE(?: IF NOT EXISTS)?\\s+`?(\\w+)`?\\s*\\((.*?)\\n\\);")
	columnRe      = regexp.MustCompile("(?m)^\\s*`?([a-z_]+)`?\\s+[A-Za-z]")
	indexRe       = regexp.MustCompile("(?i)CREATE (UNIQUE )?INDEX (\\w+) ON `?(\\w+)`?\\s*\\(([^)]*)\\)")
)

// parseMigrationSchema extracts table -> column names and index -> definition
// from a migration file's Up section.
func parseMigrationSchema(t *testing.T, path string) (map[string][]string, map[string]string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	s := string(b)
	upStart := strings.Index(s, "-- +goose Up")
	downStart := strings.Index(s, "-- +goose Down")
	if upStart < 0 || downStart < 0 || downStart < upStart {
		t.Fatalf("%s: missing or misordered goose Up/Down markers", path)
	}
	up := s[upStart:downStart]
	tables := map[string][]string{}
	for _, m := range createTableRe.FindAllStringSubmatch(up, -1) {
		var cols []string
		for _, line := range strings.Split(m[2], "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "--") {
				continue
			}
			if cm := columnRe.FindStringSubmatch(line); cm != nil {
				cols = append(cols, strings.ToLower(cm[1]))
			}
		}
		sort.Strings(cols)
		tables[strings.ToLower(m[1])] = cols
	}
	indexes := map[string]string{}
	for _, m := range indexRe.FindAllStringSubmatch(up, -1) {
		kind := "index"
		if strings.TrimSpace(m[1]) != "" {
			kind = "unique"
		}
		cols := strings.ReplaceAll(strings.ToLower(m[4]), " ", "")
		indexes[m[2]] = kind + ":" + strings.ToLower(m[3]) + ":" + cols
	}
	return tables, indexes
}

// TestLDAPMigrationMySQLParity verifies the MySQL migration declares exactly
// the same tables, columns and indexes (with the same uniqueness) as the
// SQLite migration that the SQLite tests above validate against a real
// database, and that it contains no obsolete OU-era column.
func TestLDAPMigrationMySQLParity(t *testing.T) {
	sqliteTables, sqliteIdx := parseMigrationSchema(t, filepath.Join(sqliteMigrationsDir, ldapMigrationFile))
	mysqlTables, mysqlIdx := parseMigrationSchema(t, filepath.Join(mysqlMigrationsDir, ldapMigrationFile))

	for table, want := range ldapSchema {
		sort.Strings(want)
		if got := sqliteTables[table]; strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("SQLite %s columns = %v, want %v", table, got, want)
		}
		if got := mysqlTables[table]; strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("MySQL %s columns = %v, want %v", table, got, want)
		}
		for _, col := range obsoleteLDAPColumns {
			for _, c := range mysqlTables[table] {
				if c == col {
					t.Fatalf("MySQL %s must not declare obsolete column %s", table, col)
				}
			}
		}
	}
	if len(sqliteIdx) != 4 || len(mysqlIdx) != 4 {
		t.Fatalf("expected 4 indexes in each dialect, got sqlite=%v mysql=%v", sqliteIdx, mysqlIdx)
	}
	for name, def := range sqliteIdx {
		if mysqlIdx[name] != def {
			t.Fatalf("index %s differs: sqlite=%q mysql=%q", name, def, mysqlIdx[name])
		}
	}
	if mysqlIdx["idx_ldap_group_sync_conn_group"] != "index:ldap_group_sync:connection_id,group_directory_id" {
		t.Fatalf("conn_group index must be non-unique on (connection_id, group_directory_id): %q", mysqlIdx["idx_ldap_group_sync_conn_group"])
	}
	if mysqlIdx["idx_ldap_target_identity_conn_dir"] != "unique:ldap_target_identity:connection_id,directory_id" {
		t.Fatalf("identity index must be unique on (connection_id, directory_id): %q", mysqlIdx["idx_ldap_target_identity_conn_dir"])
	}
	// Exactly one LDAP migration per dialect.
	for _, dir := range []string{sqliteMigrationsDir, mysqlMigrationsDir} {
		matches, _ := filepath.Glob(filepath.Join(dir, "*ldap*.sql"))
		if len(matches) != 1 {
			t.Fatalf("expected exactly one LDAP migration in %s, got %v", dir, matches)
		}
	}
}

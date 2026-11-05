package models

import (
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/raginx/gophish-ng/config"
)

// PostgreSQL integration test.
//
// It only runs when GOPHISH_TEST_POSTGRES_DSN points at a PostgreSQL database
// dedicated to tests, e.g.
//
//	GOPHISH_TEST_POSTGRES_DSN="host=localhost user=gophish dbname=gophish_test sslmode=disable" go test ./models/ -run TestPostgres
//
// With GOPHISH_TEST_POSTGRES_RESET=1 the public schema of that database is
// DROPPED and recreated first, so the full migration chain is exercised from
// an empty database; the reset is refused unless the database name contains
// "test" or "dev". Without the reset flag the test runs against whatever
// state the database is in (migrations are idempotent). The default
// `go test ./...` run skips this test, so PostgreSQL is never required for
// unit tests.
func TestPostgresMigrationsAndBootstrap(t *testing.T) {
	dsn := os.Getenv("GOPHISH_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("GOPHISH_TEST_POSTGRES_DSN not set; skipping PostgreSQL integration test")
	}
	conf := &config.Config{
		DBName:         "postgres",
		DBPath:         dsn,
		MigrationsPath: filepath.Join("..", "db", "db_postgres", "migrations"),
	}
	if os.Getenv("GOPHISH_TEST_POSTGRES_RESET") == "1" {
		resetPostgresSchema(t, conf)
	}
	if err := Setup(conf); err != nil {
		t.Fatalf("Setup on PostgreSQL: %v", err)
	}
	// Leave the package back on the in-memory SQLite database other tests
	// expect, whatever happens below.
	defer func() {
		_ = Setup(&config.Config{DBName: "sqlite3", DBPath: ":memory:", MigrationsPath: filepath.Join("..", "db", "db_sqlite3", "migrations")})
	}()

	// The full migration chain applied: the newest migration file is the
	// current goose version.
	var version int64
	if err := db.Raw("SELECT MAX(version_id) FROM goose_db_version").Scan(&version).Error; err != nil {
		t.Fatalf("read goose version: %v", err)
	}
	if want := newestMigrationVersion(t, conf.MigrationsPath); version != want {
		t.Fatalf("goose version = %d, want %d", version, want)
	}

	// Bootstrap: the default team and the admin user with the admin role exist.
	admin, err := GetUserByUsername("admin")
	if err != nil {
		t.Fatalf("admin user not bootstrapped: %v", err)
	}
	if admin.Role.Slug != RoleAdmin || admin.TeamID == 0 || admin.ApiKey == "" {
		t.Fatalf("admin bootstrap incomplete: %+v", admin)
	}

	// Round-trip the main object types through GORM on PostgreSQL: group with
	// targets, template, page, sending profile. Rows are removed afterwards so
	// the test can be re-run without a reset.
	g := Group{Name: "pg-it-group", UserId: admin.Id, TeamId: admin.TeamID,
		Targets: []Target{{BaseRecipient: BaseRecipient{Email: "pg-it@example.com", FirstName: "PG", LastName: "IT"}}}}
	if err := PostGroup(&g); err != nil {
		t.Fatalf("PostGroup: %v", err)
	}
	defer func() { _ = DeleteGroup(&g) }()
	targets, err := GetTargets(g.Id)
	if err != nil || len(targets) != 1 || targets[0].Email != "pg-it@example.com" {
		t.Fatalf("GetTargets: %v %+v", err, targets)
	}
	// Duplicate names must surface as gorm.ErrDuplicatedKey via TranslateError.
	dup := Group{Name: "pg-it-group", UserId: admin.Id, TeamId: admin.TeamID,
		Targets: []Target{{BaseRecipient: BaseRecipient{Email: "pg-it2@example.com"}}}}
	if err := PostGroup(&dup); err == nil {
		_ = DeleteGroup(&dup)
		t.Fatal("expected duplicate group name to be rejected by the unique index")
	}

	tpl := Template{Name: "pg-it-template", UserId: admin.Id, TeamId: admin.TeamID, Subject: "s", Text: "text", HTML: "<p>html</p>"}
	if err := PostTemplate(&tpl); err != nil {
		t.Fatalf("PostTemplate: %v", err)
	}
	defer func() { _ = DeleteTemplate(tpl.Id, admin.TeamID) }()

	page := Page{Name: "pg-it-page", UserId: admin.Id, TeamId: admin.TeamID, HTML: "<html><body>page</body></html>"}
	if err := PostPage(&page); err != nil {
		t.Fatalf("PostPage: %v", err)
	}
	defer func() { _ = DeletePage(page.Id, admin.TeamID) }()

	smtp := SMTP{Name: "pg-it-smtp", UserId: admin.Id, TeamId: admin.TeamID, Host: "smtp.example.com:25", FromAddress: "from@example.com", Interface: "SMTP"}
	if err := PostSMTP(&smtp); err != nil {
		t.Fatalf("PostSMTP: %v", err)
	}
	defer func() { _ = DeleteSMTP(smtp.Id, admin.TeamID) }()

	got, err := GetSMTP(smtp.Id, admin.TeamID)
	if err != nil || got.Name != "pg-it-smtp" {
		t.Fatalf("GetSMTP: %v %+v", err, got)
	}
}

// resetPostgresSchema drops and recreates the public schema of the test
// database so the migration chain runs from scratch. It is destructive, so it
// only runs when the caller opted in with GOPHISH_TEST_POSTGRES_RESET=1 AND
// the database name in the DSN clearly marks a test/development database
// (it contains "test" or "dev"); anything else is refused without touching
// the database. Being on localhost is deliberately not treated as safe.
func resetPostgresSchema(t *testing.T, conf *config.Config) {
	t.Helper()
	name := postgresDatabaseName(conf.DBPath)
	if !isDisposablePostgresDatabase(name) {
		t.Fatalf("refusing destructive reset: destructive PostgreSQL tests require a dedicated test/development database whose name contains \"test\" or \"dev\" (configured database: %q)", name)
	}
	sqlDB, err := openPostgresRaw(conf)
	if err != nil {
		t.Fatalf("open PostgreSQL for reset: %v", err)
	}
	defer sqlDB.Close()
	for _, stmt := range []string{"DROP SCHEMA public CASCADE", "CREATE SCHEMA public"} {
		if _, err := sqlDB.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
}

// postgresDatabaseName extracts the database name from a PostgreSQL connection
// string in either the URL form (postgres://user:pw@host/dbname?...) or the
// keyword/value form (host=... dbname=... ). It returns "" when none is found.
// Only the name is ever surfaced; the rest of the DSN (which may carry a
// password) is never logged.
func postgresDatabaseName(dsn string) string {
	if strings.Contains(dsn, "://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return ""
		}
		return strings.Trim(u.Path, "/")
	}
	for _, field := range strings.Fields(dsn) {
		if strings.HasPrefix(field, "dbname=") {
			return strings.Trim(strings.TrimPrefix(field, "dbname="), "'\"")
		}
	}
	return ""
}

// isDisposablePostgresDatabase reports whether a database name clearly marks a
// test/development database.
func isDisposablePostgresDatabase(name string) bool {
	n := strings.ToLower(name)
	return n != "" && (strings.Contains(n, "test") || strings.Contains(n, "dev"))
}

func TestPostgresDatabaseNameGuard(t *testing.T) {
	cases := map[string]string{
		"host=localhost user=gophish dbname=gophish_test sslmode=disable":            "gophish_test",
		"host=/tmp dbname=gophish_dev":                                               "gophish_dev",
		"postgres://gophish:pw@localhost:5432/gophish_postgres_test?sslmode=disable": "gophish_postgres_test",
		"postgresql://gophish@localhost/gophish":                                     "gophish",
		"host=localhost user=gophish":                                                "",
	}
	for dsn, want := range cases {
		if got := postgresDatabaseName(dsn); got != want {
			t.Errorf("postgresDatabaseName(%q) = %q, want %q", dsn, got, want)
		}
	}
	for name, want := range map[string]bool{"gophish_dev": true, "gophish_test": true, "gophish_postgres_test": true, "DevDB": true, "gophish": false, "production": false, "": false} {
		if got := isDisposablePostgresDatabase(name); got != want {
			t.Errorf("isDisposablePostgresDatabase(%q) = %v, want %v", name, got, want)
		}
	}
}

// openPostgresRaw opens a plain database/sql connection through the pgx
// driver registered by gorm.io/driver/postgres.
func openPostgresRaw(conf *config.Config) (*sql.DB, error) {
	return sql.Open("pgx", conf.DBPath)
}

// newestMigrationVersion returns the highest version number among the .sql
// files in dir.
func newestMigrationVersion(t *testing.T, dir string) int64 {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no migrations found in %s: %v", dir, err)
	}
	var newest int64
	for _, f := range files {
		var v int64
		for _, c := range filepath.Base(f) {
			if c < '0' || c > '9' {
				break
			}
			v = v*10 + int64(c-'0')
		}
		if v > newest {
			newest = v
		}
	}
	return newest
}

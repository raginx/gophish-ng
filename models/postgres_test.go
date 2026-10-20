package models

import (
	"database/sql"
	"os"
	"path/filepath"
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
// dropped and recreated first, so the full migration chain is exercised from
// an empty database; without it the test runs against whatever state the
// database is in (migrations are idempotent). The default `go test ./...` run
// skips this test, so PostgreSQL is never required for unit tests.
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
// database so the migration chain runs from scratch.
func resetPostgresSchema(t *testing.T, conf *config.Config) {
	t.Helper()
	if err := Setup(conf); err != nil {
		// Setup migrates; a failure here may simply mean a half-migrated
		// database from an earlier run, which the reset below clears. Open a
		// raw connection instead.
		t.Logf("initial Setup before reset failed (continuing with reset): %v", err)
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

package models

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/raginx/gophish-ng/config"
)

// MySQL integration test.
//
// It only runs when GOPHISH_TEST_MYSQL_DSN points at a MySQL database
// dedicated to tests, e.g.
//
//	GOPHISH_TEST_MYSQL_DSN="gophish:gophish@tcp(127.0.0.1:3306)/gophish_test?parseTime=true&charset=utf8mb4&loc=Local" go test ./models/ -run TestMySQL
//
// With GOPHISH_TEST_MYSQL_RESET=1 all tables in that database are DROPPED
// first, so the full migration chain is exercised from an empty database;
// the reset is refused unless the database name has a delimiter-bounded "test"
// or "dev" segment.
// Without the reset flag the test runs against whatever state the database
// is in (migrations are idempotent). The default `go test ./...` run skips
// this test, so MySQL is never required for unit tests. This mirrors
// TestPostgresMigrationsAndBootstrap so both server backends get the same
// migration/dialect coverage.
func TestMySQLMigrationsAndBootstrap(t *testing.T) {
	dsn := os.Getenv("GOPHISH_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("GOPHISH_TEST_MYSQL_DSN not set; skipping MySQL integration test")
	}
	conf := &config.Config{
		DBName:         "mysql",
		DBPath:         dsn,
		MigrationsPath: filepath.Join("..", "db", "db_mysql", "migrations"),
	}
	if os.Getenv("GOPHISH_TEST_MYSQL_RESET") == "1" {
		resetMySQLSchema(t, conf)
	}
	if err := Setup(conf); err != nil {
		t.Fatalf("Setup on MySQL: %v", err)
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

	// Round-trip the main object types through GORM on MySQL: group with
	// targets, template, page, sending profile. Rows are removed afterwards so
	// the test can be re-run without a reset.
	g := Group{Name: "mysql-it-group", UserId: admin.Id, TeamId: admin.TeamID,
		Targets: []Target{{BaseRecipient: BaseRecipient{Email: "mysql-it@example.com", FirstName: "MySQL", LastName: "IT"}}}}
	if err := PostGroup(&g); err != nil {
		t.Fatalf("PostGroup: %v", err)
	}
	defer func() { _ = DeleteGroup(&g) }()
	targets, err := GetTargets(g.Id)
	if err != nil || len(targets) != 1 || targets[0].Email != "mysql-it@example.com" {
		t.Fatalf("GetTargets: %v %+v", err, targets)
	}
	// Duplicate names must surface as gorm.ErrDuplicatedKey via TranslateError.
	dup := Group{Name: "mysql-it-group", UserId: admin.Id, TeamId: admin.TeamID,
		Targets: []Target{{BaseRecipient: BaseRecipient{Email: "mysql-it2@example.com"}}}}
	if err := PostGroup(&dup); err == nil {
		_ = DeleteGroup(&dup)
		t.Fatal("expected duplicate group name to be rejected by the unique index")
	}

	tpl := Template{Name: "mysql-it-template", UserId: admin.Id, TeamId: admin.TeamID, Subject: "s", Text: "text", HTML: "<p>html</p>"}
	if err := PostTemplate(&tpl); err != nil {
		t.Fatalf("PostTemplate: %v", err)
	}
	defer func() { _ = DeleteTemplate(tpl.Id, admin.TeamID) }()

	page := Page{Name: "mysql-it-page", UserId: admin.Id, TeamId: admin.TeamID, HTML: "<html><body>page</body></html>"}
	if err := PostPage(&page); err != nil {
		t.Fatalf("PostPage: %v", err)
	}
	defer func() { _ = DeletePage(page.Id, admin.TeamID) }()

	smtp := SMTP{Name: "mysql-it-smtp", UserId: admin.Id, TeamId: admin.TeamID, Host: "smtp.example.com:25", FromAddress: "from@example.com", Interface: "SMTP"}
	if err := PostSMTP(&smtp); err != nil {
		t.Fatalf("PostSMTP: %v", err)
	}
	defer func() { _ = DeleteSMTP(smtp.Id, admin.TeamID) }()

	got, err := GetSMTP(smtp.Id, admin.TeamID)
	if err != nil || got.Name != "mysql-it-smtp" {
		t.Fatalf("GetSMTP: %v %+v", err, got)
	}
}

// resetMySQLSchema drops every table in the test database so the migration
// chain runs from scratch.
func resetMySQLSchema(t *testing.T, conf *config.Config) {
	t.Helper()
	name := mysqlDatabaseName(conf.DBPath)
	if !isDisposableTestDatabaseName(name) {
		t.Fatalf("refusing destructive reset: destructive MySQL tests require a dedicated test/development database whose name has a delimiter-bounded \"test\" or \"dev\" segment, e.g. \"gophish_test\" (configured database: %q)", name)
	}
	sqlDB, err := sql.Open("mysql", conf.DBPath)
	if err != nil {
		t.Fatalf("open MySQL for reset: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()
	rows, err := sqlDB.Query("SHOW TABLES")
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			_ = rows.Close()
			t.Fatalf("scan table name: %v", err)
		}
		tables = append(tables, table)
	}
	_ = rows.Close()
	if _, err := sqlDB.Exec("SET FOREIGN_KEY_CHECKS=0"); err != nil {
		t.Fatalf("disable foreign key checks: %v", err)
	}
	for _, table := range tables {
		if _, err := sqlDB.Exec("DROP TABLE IF EXISTS `" + table + "`"); err != nil {
			t.Fatalf("drop table %s: %v", table, err)
		}
	}
	if _, err := sqlDB.Exec("SET FOREIGN_KEY_CHECKS=1"); err != nil {
		t.Fatalf("re-enable foreign key checks: %v", err)
	}
}

// mysqlDatabaseName extracts the database name from a go-sql-driver DSN
// (user:pw@tcp(host:port)/dbname?params).
func mysqlDatabaseName(dsn string) string {
	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		return ""
	}
	return strings.Trim(cfg.DBName, "'\"")
}

func TestMySQLDatabaseNameGuard(t *testing.T) {
	cases := map[string]string{
		"gophish:gophish@tcp(127.0.0.1:3306)/gophish_test?parseTime=true": "gophish_test",
		"root@tcp(localhost:3306)/gophish_dev":                            "gophish_dev",
		"gophish:pw@tcp(db:3306)/gophish?charset=utf8mb4":                 "gophish",
	}
	for dsn, want := range cases {
		if got := mysqlDatabaseName(dsn); got != want {
			t.Errorf("mysqlDatabaseName(%q) = %q, want %q", dsn, got, want)
		}
	}
}

package models

import (
	"path/filepath"
	"testing"

	"github.com/gophish/gophish/config"
)

// TestSetupSucceedsWhenDefaultAdminRenamed verifies that Setup() doesn't
// fail to start Gophish just because the built-in "admin" account was
// renamed or deleted on a prior run - see gophish/gophish#2487. It uses a
// real, file-backed SQLite database rather than ":memory:" so that a second
// Setup() call reconnects to the same, already-populated database, matching
// what actually happens on a service restart.
func TestSetupSucceedsWhenDefaultAdminRenamed(t *testing.T) {
	origDB, origConf := db, conf
	defer func() { db, conf = origDB, origConf }()

	testConf := &config.Config{
		DBName:         "sqlite3",
		DBPath:         filepath.Join(t.TempDir(), "gophish-test.db"),
		MigrationsPath: "../db/db_sqlite3/migrations/",
	}

	if err := Setup(testConf); err != nil {
		t.Fatalf("initial Setup() failed: %v", err)
	}

	if err := db.Model(&User{}).Where("id = ?", 1).
		Update("username", "renamed_admin").Error; err != nil {
		t.Fatalf("failed renaming default admin: %v", err)
	}

	// Simulate a restart: Setup() reconnects to the same database, where
	// no user is named "admin" anymore.
	if err := Setup(testConf); err != nil {
		t.Fatalf("Setup() failed after renaming the default admin: %v", err)
	}

	u, err := GetUserByUsername("renamed_admin")
	if err != nil {
		t.Fatalf("renamed admin user is gone after Setup(): %v", err)
	}
	if u.Id != 1 {
		t.Fatalf("unexpected user id for renamed admin: got %d, want 1", u.Id)
	}

	users, err := GetUsers()
	if err != nil {
		t.Fatalf("error listing users: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("Setup() should not have created a second admin user: got %d users, want 1", len(users))
	}
}

package models

import "testing"

func TestResolveMaxOpenConns(t *testing.T) {
	tests := []struct {
		name     string
		dbName   string
		override int
		want     int
	}{
		{"sqlite default", "sqlite3", 0, 1},
		{"unset dbName default", "", 0, 1},
		{"mysql default", "mysql", 0, DefaultMySQLMaxOpenConns},
		{"postgres default", "postgres", 0, DefaultPostgresMaxOpenConns},
		{"sqlite override", "sqlite3", 5, 5},
		{"mysql override", "mysql", 5, 5},
		{"postgres override", "postgres", 5, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveMaxOpenConns(tt.dbName, tt.override)
			if got != tt.want {
				t.Errorf("resolveMaxOpenConns(%q, %d) = %d, want %d", tt.dbName, tt.override, got, tt.want)
			}
		})
	}
}

func TestChooseDBDialect(t *testing.T) {
	tests := map[string]string{
		"sqlite3":  "sqlite3",
		"mysql":    "mysql",
		"postgres": "postgres",
		"":         "sqlite3",
		"unknown":  "sqlite3",
	}
	for in, want := range tests {
		if got := chooseDBDialect(in); got != want {
			t.Errorf("chooseDBDialect(%q) = %q, want %q", in, got, want)
		}
	}
}

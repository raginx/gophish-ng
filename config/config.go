package config

import (
	"encoding/json"
	"errors"
	"os"

	gophishcrypto "github.com/raginx/gophish-ng/crypto"
	log "github.com/raginx/gophish-ng/logger"
)

// ErrSecretKeyNotConfigured is returned by SecretKeyBytes when secret_key
// isn't set in config.json
var ErrSecretKeyNotConfigured = errors.New("secret_key is not configured in config.json")

// AdminServer represents the Admin server configuration details
type AdminServer struct {
	ListenURL            string   `json:"listen_url"`
	UseTLS               bool     `json:"use_tls"`
	CertPath             string   `json:"cert_path"`
	KeyPath              string   `json:"key_path"`
	CSRFKey              string   `json:"csrf_key"`
	AllowedInternalHosts []string `json:"allowed_internal_hosts"`
	TrustedOrigins       []string `json:"trusted_origins"`
}

// PhishServer represents the Phish server configuration details
type PhishServer struct {
	ListenURL string `json:"listen_url"`
	UseTLS    bool   `json:"use_tls"`
	CertPath  string `json:"cert_path"`
	KeyPath   string `json:"key_path"`
}

// Config represents the configuration information.
type Config struct {
	AdminConf   AdminServer `json:"admin_server"`
	PhishConf   PhishServer `json:"phish_server"`
	DBName      string      `json:"db_name"`
	DBPath      string      `json:"db_path"`
	DBSSLCaPath string      `json:"db_sslca_path"`
	// DBMaxOpenConns overrides the database connection pool size. 0 (the
	// default) picks a dialect-appropriate default: 1 for SQLite (which
	// only supports a single writer), or a small pool for MySQL. Only
	// needed to tune for a restrictive MySQL max_connections limit.
	DBMaxOpenConns int         `json:"db_max_open_conns"`
	MigrationsPath string      `json:"migrations_prefix"`
	TestFlag       bool        `json:"test_flag"`
	ContactAddress string      `json:"contact_address"`
	Logging        *log.Config `json:"logging"`
	// SecretKey is a base64-encoded 32-byte key (e.g. from `openssl rand
	// -base64 32`) used to encrypt secrets that need to be stored at rest,
	// such as OAuth2 refresh tokens for IMAP email reporting
	SecretKey string `json:"secret_key"`
}

// SecretKeyBytes decodes and validates SecretKey for use with the crypto
// package's Encrypt/Decrypt. Returns ErrSecretKeyNotConfigured if
// SecretKey is empty, or a decoding/length error if it's set but invalid.
func (c *Config) SecretKeyBytes() ([gophishcrypto.KeySize]byte, error) {
	if c.SecretKey == "" {
		return [gophishcrypto.KeySize]byte{}, ErrSecretKeyNotConfigured
	}
	return gophishcrypto.ParseKey(c.SecretKey)
}

// Version contains the current gophish version
var Version = ""

// ServerName is the server type that is returned in the transparency response.
const ServerName = "gophish"

// LoadConfig loads the configuration from the specified filepath
func LoadConfig(filepath string) (*Config, error) {
	// Get the config file
	configFile, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	}
	config := &Config{}
	err = json.Unmarshal(configFile, config)
	if err != nil {
		return nil, err
	}
	if config.Logging == nil {
		config.Logging = &log.Config{}
	}
	// Choosing the migrations directory based on the database used.
	// The old liamstask/goose recursively walked this path looking for
	// migration files, so pointing it at the per-dialect directory (e.g.
	// db/db_sqlite3) happened to work even though the .sql files actually
	// live one level deeper, in its migrations/ subdirectory. pressly/goose
	// doesn't recurse, so the path needs to be exact.
	config.MigrationsPath = config.MigrationsPath + config.DBName + "/migrations"
	// Explicitly set the TestFlag to false to prevent config.json overrides
	config.TestFlag = false
	return config, nil
}

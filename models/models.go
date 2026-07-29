package models

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"time"

	goose "github.com/pressly/goose/v3"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/gophish/gophish/auth"
	"github.com/gophish/gophish/config"

	log "github.com/gophish/gophish/logger"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var db *gorm.DB
var conf *config.Config

const MaxDatabaseConnectionAttempts int = 10

// DefaultMySQLMaxOpenConns is the default connection pool size used for
// MySQL when conf.DBMaxOpenConns isn't set. Unlike SQLite, MySQL handles
// concurrent connections natively.
const DefaultMySQLMaxOpenConns int = 25

// DefaultMySQLConnMaxLifetime bounds how long a pooled MySQL connection is
// reused before being recycled, so it gets refreshed before the MySQL
// server's own wait_timeout (default 8h, but often lower on shared hosting)
// closes it out from under us.
const DefaultMySQLConnMaxLifetime = 5 * time.Minute

// DefaultAdminUsername is the default username for the administrative user
const DefaultAdminUsername = "admin"

// InitialAdminPassword is the environment variable that specifies which
// password to use for the initial root login instead of generating one
// randomly
const InitialAdminPassword = "GOPHISH_INITIAL_ADMIN_PASSWORD"

// InitialAdminApiToken is the environment variable that specifies the
// API token to seed the initial root login instead of generating one
// randomly
const InitialAdminApiToken = "GOPHISH_INITIAL_ADMIN_API_TOKEN"

const (
	CampaignInProgress string = "In progress"
	CampaignQueued     string = "Queued"
	CampaignCreated    string = "Created"
	CampaignEmailsSent string = "Emails Sent"
	CampaignComplete   string = "Completed"
	EventSent          string = "Email Sent"
	EventSendingError  string = "Error Sending Email"
	EventOpened        string = "Email Opened"
	EventClicked       string = "Clicked Link"
	EventDataSubmit    string = "Submitted Data"
	EventReported      string = "Email Reported"
	EventProxyRequest  string = "Proxied request"
	StatusSuccess      string = "Success"
	StatusQueued       string = "Queued"
	StatusSending      string = "Sending"
	StatusUnknown      string = "Unknown"
	StatusScheduled    string = "Scheduled"
	StatusRetry        string = "Retrying"
	Error              string = "Error"
)

// Flash is used to hold flash information for use in templates.
type Flash struct {
	Type    string
	Message string
}

// Response contains the attributes found in an API response
type Response struct {
	Message string      `json:"message"`
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
}

// Copy of auth.GenerateSecureKey to prevent cyclic import with auth library
func generateSecureKey() string {
	k := make([]byte, 32)
	io.ReadFull(rand.Reader, k)
	return fmt.Sprintf("%x", k)
}

func chooseDBDialect(name string) string {
	switch name {
	case "mysql":
		return "mysql"
	// Default database is sqlite3
	default:
		return "sqlite3"
	}
}

// resolveMaxOpenConns picks the database connection pool size. SQLite only
// supports a single writer at a time, so the pool is kept at 1 there to
// avoid "database is locked" errors (see #331). MySQL has no such
// restriction - capping it at 1 there as well just serializes every query
// through a single connection, and under load (e.g. large campaigns) that
// connection becoming stale/dropped (e.g. MySQL's wait_timeout) can stall
// sending until the service is restarted (see #2939). override, from
// conf.DBMaxOpenConns, lets operators pick a different value for their
// environment (e.g. a restrictive MySQL max_connections limit); 0 means
// "use the dialect default".
func resolveMaxOpenConns(dbName string, override int) int {
	if override != 0 {
		return override
	}
	if chooseDBDialect(dbName) == "mysql" {
		return DefaultMySQLMaxOpenConns
	}
	return 1
}

func createTemporaryPassword(u *User) error {
	var temporaryPassword string
	if envPassword := os.Getenv(InitialAdminPassword); envPassword != "" {
		temporaryPassword = envPassword
	} else {
		// This will result in a 16 character password which could be viewed as an
		// inconvenience, but it should be ok for now.
		temporaryPassword = auth.GenerateSecureKey(auth.MinPasswordLength)
	}
	hash, err := auth.GeneratePasswordHash(temporaryPassword)
	if err != nil {
		return err
	}
	u.Hash = hash
	// Anytime a temporary password is created, we will force the user
	// to change their password
	u.PasswordChangeRequired = true
	err = db.Save(u).Error
	if err != nil {
		return err
	}
	log.Infof("Please login with the username admin and the password %s", temporaryPassword)
	return nil
}

// Setup initializes the database and runs any needed migrations.
//
// First, it establishes a connection to the database, then runs any migrations
// newer than the version the database is on.
//
// Once the database is up-to-date, we create an admin user (if needed) that
// has a randomly generated API key and password.
func Setup(c *config.Config) error {
	// Setup the package-scoped config
	conf = c
	var err error

	// Register certificates for tls encrypted db connections
	if conf.DBSSLCaPath != "" {
		switch conf.DBName {
		case "mysql":
			rootCertPool := x509.NewCertPool()
			pem, err := ioutil.ReadFile(conf.DBSSLCaPath)
			if err != nil {
				log.Error(err)
				return err
			}
			if ok := rootCertPool.AppendCertsFromPEM(pem); !ok {
				log.Error("Failed to append PEM.")
				return err
			}
			mysql.RegisterTLSConfig("ssl_ca", &tls.Config{
				RootCAs: rootCertPool,
			})
			// Default database is sqlite3, which supports no tls, as connection
			// is file based
		default:
		}
	}

	// Open our database connection
	var dialector gorm.Dialector
	if chooseDBDialect(conf.DBName) == "mysql" {
		dialector = gormmysql.Open(conf.DBPath)
	} else {
		dialector = sqlite.Open(conf.DBPath)
	}
	gormConf := &gorm.Config{
		// Logging is handled separately via the gophish logger; keep gorm's
		// own SQL logging silent to match the previous LogMode(false).
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
		// Lets callers check for gorm.ErrDuplicatedKey instead of parsing
		// dialect-specific driver error codes
		TranslateError: true,
	}
	i := 0
	for {
		db, err = gorm.Open(dialector, gormConf)
		if err == nil {
			break
		}
		if err != nil && i >= MaxDatabaseConnectionAttempts {
			log.Error(err)
			return err
		}
		i += 1
		log.Warn("waiting for database to be up...")
		time.Sleep(5 * time.Second)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Error(err)
		return err
	}
	sqlDB.SetMaxOpenConns(resolveMaxOpenConns(conf.DBName, conf.DBMaxOpenConns))
	if chooseDBDialect(conf.DBName) == "mysql" {
		sqlDB.SetConnMaxLifetime(DefaultMySQLConnMaxLifetime)
	}
	// Migrate up to the latest version
	err = goose.SetDialect(chooseDBDialect(conf.DBName))
	if err != nil {
		log.Error(err)
		return err
	}
	err = goose.Up(sqlDB, conf.MigrationsPath)
	if err != nil {
		log.Error(err)
		return err
	}
	// Create the admin user if it doesn't exist
	var userCount int64
	var adminUser User
	db.Model(&User{}).Count(&userCount)
	adminRole, err := GetRoleBySlug(RoleAdmin)
	if err != nil {
		log.Error(err)
		return err
	}
	if userCount == 0 {
		adminUser = User{
			Username:               DefaultAdminUsername,
			Role:                   adminRole,
			RoleID:                 adminRole.ID,
			PasswordChangeRequired: true,
		}

		if envToken := os.Getenv(InitialAdminApiToken); envToken != "" {
			adminUser.ApiKey = envToken
		} else {
			adminUser.ApiKey = auth.GenerateSecureKey(auth.APIKeyLength)
		}

		err = db.Save(&adminUser).Error
		if err != nil {
			log.Error(err)
			return err
		}
	}
	// If this is the first time the user is installing Gophish, then we will
	// generate a temporary password for the admin user.
	//
	// We do this here instead of in the block above where the admin is created
	// since there's the chance the user executes Gophish and has some kind of
	// error, then tries restarting it. If they didn't grab the password out of
	// the logs, then they would have lost it.
	//
	// By doing the temporary password here, we will regenerate that temporary
	// password until the user is able to reset the admin password.
	if adminUser.Username == "" {
		adminUser, err = GetUserByUsername(DefaultAdminUsername)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// The default admin account was renamed or deleted after
				// initial setup. That's fine - there's no bootstrap
				// password left to manage, so there's nothing more to do
				// here.
				return nil
			}
			log.Error(err)
			return err
		}
	}
	if adminUser.PasswordChangeRequired {
		err = createTemporaryPassword(&adminUser)
		if err != nil {
			log.Error(err)
			return err
		}
	}
	return nil
}

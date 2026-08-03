package models

import (
	"errors"
	"net"
	"time"

	log "github.com/gophish/gophish/logger"
)

const DefaultIMAPFolder = "INBOX"
const DefaultIMAPFreq = 60 // Every 60 seconds

// IMAP authentication types - see IMAP.AuthType.
const (
	IMAPAuthTypeBasic  = "basic"
	IMAPAuthTypeOAuth2 = "oauth2"
)

// OAuth2 providers supported for IMAP authentication - see
// IMAP.OAuthProvider and oauth_imap.go.
const (
	OAuthProviderGoogle    = "google"
	OAuthProviderMicrosoft = "microsoft"
	OAuthProviderCustom    = "custom"
)

// IMAP contains the attributes needed to handle logging into an IMAP server to check
// for reported emails
type IMAP struct {
	UserId                      int64     `json:"-" gorm:"column:user_id"`
	Enabled                     bool      `json:"enabled"`
	Host                        string    `json:"host"`
	Port                        uint16    `json:"port,string,omitempty"`
	Username                    string    `json:"username"`
	Password                    string    `json:"password"`
	TLS                         bool      `json:"tls"`
	IgnoreCertErrors            bool      `json:"ignore_cert_errors"`
	Folder                      string    `json:"folder"`
	RestrictDomain              string    `json:"restrict_domain"`
	DeleteReportedCampaignEmail bool      `json:"delete_reported_campaign_email"`
	LastLogin                   time.Time `json:"last_login,omitempty"`
	ModifiedDate                time.Time `json:"modified_date"`
	IMAPFreq                    uint32    `json:"imap_freq,string,omitempty"`

	// AuthType selects between Basic Auth (the default, existing behavior)
	// and OAuth2
	AuthType          string `json:"auth_type" gorm:"column:auth_type"`
	OAuthProvider     string `json:"oauth_provider,omitempty" gorm:"column:oauth_provider"`
	OAuthTenantID     string `json:"oauth_tenant_id,omitempty" gorm:"column:oauth_tenant_id"`
	OAuthClientID     string `json:"oauth_client_id,omitempty" gorm:"column:oauth_client_id"`
	OAuthClientSecret string `json:"oauth_client_secret,omitempty" gorm:"column:oauth_client_secret"`
	// OAuthAuthURL/OAuthTokenURL are only used (and shown in the UI) when
	// OAuthProvider is "custom" - well-known providers use fixed endpoints
	// from oauthEndpoint in oauth_imap.go.
	OAuthAuthURL  string `json:"oauth_auth_url,omitempty" gorm:"column:oauth_auth_url"`
	OAuthTokenURL string `json:"oauth_token_url,omitempty" gorm:"column:oauth_token_url"`
	// OAuthScopes is a space-separated OAuth2 scope list. Required for the
	// "custom" provider (there's no sane default for an arbitrary IMAP
	// OAuth2 server); optional for google/microsoft, where it overrides
	// the built-in default scopes from oauthScopes in oauth_imap.go.
	OAuthScopes string `json:"oauth_scopes,omitempty" gorm:"column:oauth_scopes"`
	// OAuthRefreshTokenEnc/OAuthAccessTokenEnc are AES-256-GCM encrypted
	// (see the crypto package) and never exposed over JSON - they're only
	// ever written by the OAuth callback handler and read by
	// GetValidAccessToken.
	OAuthRefreshTokenEnc string    `json:"-" gorm:"column:oauth_refresh_token_enc"`
	OAuthAccessTokenEnc  string    `json:"-" gorm:"column:oauth_access_token_enc"`
	OAuthTokenExpiry     time.Time `json:"-" gorm:"column:oauth_token_expiry"`
}

// ErrIMAPHostNotSpecified is thrown when there is no Host specified
// in the IMAP configuration
var ErrIMAPHostNotSpecified = errors.New("No IMAP Host specified")

// ErrIMAPPortNotSpecified is thrown when there is no Port specified
// in the IMAP configuration
var ErrIMAPPortNotSpecified = errors.New("No IMAP Port specified")

// ErrInvalidIMAPHost indicates that the IMAP server string is invalid
var ErrInvalidIMAPHost = errors.New("Invalid IMAP server address")

// ErrInvalidIMAPPort indicates that the IMAP Port is invalid
var ErrInvalidIMAPPort = errors.New("Invalid IMAP Port")

// ErrIMAPUsernameNotSpecified is thrown when there is no Username specified
// in the IMAP configuration
var ErrIMAPUsernameNotSpecified = errors.New("No Username specified")

// ErrIMAPPasswordNotSpecified is thrown when there is no Password specified
// in the IMAP configuration
var ErrIMAPPasswordNotSpecified = errors.New("No Password specified")

// ErrInvalidIMAPFreq is thrown when the frequency for polling the
// IMAP server is invalid
var ErrInvalidIMAPFreq = errors.New("Invalid polling frequency")

// ErrIMAPInvalidAuthType is thrown when AuthType is neither "basic" nor
// "oauth2"
var ErrIMAPInvalidAuthType = errors.New("Invalid IMAP authentication type")

// ErrOAuthProviderNotSpecified is thrown when OAuth2 auth is selected
// without specifying a provider
var ErrOAuthProviderNotSpecified = errors.New("No OAuth provider specified")

// ErrInvalidOAuthProvider is thrown when OAuthProvider isn't one of the
// supported providers
var ErrInvalidOAuthProvider = errors.New("Invalid OAuth provider")

// ErrOAuthClientIDNotSpecified is thrown when OAuth2 auth is selected
// without specifying a client ID
var ErrOAuthClientIDNotSpecified = errors.New("No OAuth client ID specified")

// ErrOAuthClientSecretNotSpecified is thrown when OAuth2 auth is selected
// without specifying a client secret
var ErrOAuthClientSecretNotSpecified = errors.New("No OAuth client secret specified")

// ErrOAuthTenantIDNotSpecified is thrown when the Microsoft OAuth provider
// is selected without specifying a tenant ID
var ErrOAuthTenantIDNotSpecified = errors.New("No OAuth tenant ID specified")

// ErrOAuthEndpointNotSpecified is thrown when the custom OAuth provider is
// selected without specifying both the authorization and token URLs
var ErrOAuthEndpointNotSpecified = errors.New("No OAuth authorization/token URL specified")

// ErrOAuthScopesNotSpecified is thrown when the custom OAuth provider is
// selected without specifying the OAuth scopes to request
var ErrOAuthScopesNotSpecified = errors.New("No OAuth scopes specified")

// ErrOAuthSecretKeyNotConfigured is thrown when OAuth2 auth is selected but
// the server has no secret_key configured in config.json - OAuth2 tokens
// are encrypted at rest and can't be stored without it. Caught here,
// at save time, so the admin finds out before going through the provider's
// consent screen rather than after
var ErrOAuthSecretKeyNotConfigured = errors.New("OAuth2 requires secret_key to be configured in config.json")

// TableName specifies the database tablename for Gorm to use
func (im IMAP) TableName() string {
	return "imap"
}

// Validate ensures that IMAP configs/connections are valid
func (im *IMAP) Validate() error {
	if im.AuthType == "" {
		im.AuthType = IMAPAuthTypeBasic
	}

	switch {
	case im.Host == "":
		return ErrIMAPHostNotSpecified
	case im.Port == 0:
		return ErrIMAPPortNotSpecified
	case im.Username == "":
		return ErrIMAPUsernameNotSpecified
	}

	switch im.AuthType {
	case IMAPAuthTypeBasic:
		if im.Password == "" {
			return ErrIMAPPasswordNotSpecified
		}
	case IMAPAuthTypeOAuth2:
		switch im.OAuthProvider {
		case OAuthProviderGoogle, OAuthProviderMicrosoft, OAuthProviderCustom:
		case "":
			return ErrOAuthProviderNotSpecified
		default:
			return ErrInvalidOAuthProvider
		}
		if im.OAuthClientID == "" {
			return ErrOAuthClientIDNotSpecified
		}
		if im.OAuthClientSecret == "" {
			return ErrOAuthClientSecretNotSpecified
		}
		if im.OAuthProvider == OAuthProviderMicrosoft && im.OAuthTenantID == "" {
			return ErrOAuthTenantIDNotSpecified
		}
		if im.OAuthProvider == OAuthProviderCustom {
			if im.OAuthAuthURL == "" || im.OAuthTokenURL == "" {
				return ErrOAuthEndpointNotSpecified
			}
			if im.OAuthScopes == "" {
				return ErrOAuthScopesNotSpecified
			}
		}
		if _, err := conf.SecretKeyBytes(); err != nil {
			return ErrOAuthSecretKeyNotConfigured
		}
	default:
		return ErrIMAPInvalidAuthType
	}

	// Set the default value for Folder
	if im.Folder == "" {
		im.Folder = DefaultIMAPFolder
	}

	// Make sure im.Host is an IP or hostname. NB will fail if unable to resolve the hostname.
	ip := net.ParseIP(im.Host)
	_, err := net.LookupHost(im.Host)
	if ip == nil && err != nil {
		return ErrInvalidIMAPHost
	}

	// Make sure port >= 1. Port is a uint16, so it can never exceed 65535.
	if im.Port < 1 {
		return ErrInvalidIMAPPort
	}

	// Make sure the polling frequency is between every 30 seconds and every year
	// If not set it to the default
	if im.IMAPFreq < 30 || im.IMAPFreq > 31540000 {
		im.IMAPFreq = DefaultIMAPFreq
	}

	return nil
}

// GetIMAP returns the IMAP server owned by the given user.
func GetIMAP(uid int64) ([]IMAP, error) {
	im := []IMAP{}
	var count int64
	err := db.Where("user_id=?", uid).Find(&im).Count(&count).Error

	if err != nil {
		log.Error(err)
		return im, err
	}
	return im, nil
}

// PostIMAP updates IMAP settings for a user in the database.
func PostIMAP(im *IMAP, uid int64) error {
	err := im.Validate()
	if err != nil {
		log.Error(err)
		return err
	}

	// Delete old entry. TODO: Save settings and if fails to Save below replace with original
	err = DeleteIMAP(uid)
	if err != nil {
		log.Error(err)
		return err
	}

	// Insert new settings into the DB
	err = db.Create(im).Error
	if err != nil {
		log.Error("Unable to save to database: ", err.Error())
	}
	return err
}

// DeleteIMAP deletes the existing IMAP in the database.
func DeleteIMAP(uid int64) error {
	err := db.Where("user_id=?", uid).Delete(&IMAP{}).Error
	if err != nil {
		log.Error(err)
	}
	return err
}

func SuccessfulLogin(im *IMAP) error {
	err := db.Model(&im).Where("user_id = ?", im.UserId).Update("last_login", time.Now().UTC()).Error
	if err != nil {
		log.Error("Unable to update database: ", err.Error())
	}
	return err
}

package audit

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	log "github.com/raginx/gophish-ng/logger"
)

// Config controls the audit log sink.
type Config struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"`
}

// Event type identifiers. Kept as stable strings so downstream tooling (SIEM
// queries, dashboards) can rely on them and call sites don't scatter magic
// strings.
const (
	EventLoginSuccess      = "login_success"
	EventLoginFailure      = "login_failure"
	EventLogout            = "logout"
	EventImpersonate       = "impersonate"
	EventUserCreated       = "user_created"
	EventUserModified      = "user_modified"
	EventUserDeleted       = "user_deleted"
	EventPasswordChanged   = "password_changed"
	EventAPIKeyReset       = "api_key_reset"
	EventCampaignCreated   = "campaign_created"
	EventCampaignLaunched  = "campaign_launched"
	EventCampaignCompleted = "campaign_completed"
	EventCampaignDeleted   = "campaign_deleted"
	EventCampaignScrubbed  = "campaign_scrubbed"
)

// Outcome values.
const (
	OutcomeSuccess = "success"
	OutcomeFailure = "failure"
)

// Failure reasons (optional, for login_failure and similar).
const (
	ReasonUnknownUser     = "unknown_user"
	ReasonInvalidPassword = "invalid_password"
	ReasonAccountLocked   = "account_locked"
	ReasonForcedReset     = "forced_reset"
)

// Actor is who performed the action. ID is omitted when unknown (e.g. a login
// attempt for a username that doesn't exist).
type Actor struct {
	ID       int64  `json:"id,omitempty"`
	Username string `json:"username,omitempty"`
}

// Target is the object an action was performed on, when it differs from the
// actor (e.g. an admin modifying another user, or a campaign).
type Target struct {
	Type string `json:"type,omitempty"`
	ID   int64  `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// Event is a single audit record. It is serialized as one JSON object per line
// (JSON Lines). Because every field is structured JSON, untrusted values such
// as a submitted username cannot break out of their field or inject extra log
// lines
type Event struct {
	Time     time.Time      `json:"time"`
	Event    string         `json:"event"`
	Outcome  string         `json:"outcome"`
	Actor    *Actor         `json:"actor,omitempty"`
	Target   *Target        `json:"target,omitempty"`
	SourceIP string         `json:"source_ip,omitempty"`
	Reason   string         `json:"reason,omitempty"`
	Details  map[string]any `json:"details,omitempty"`
}

var (
	mu      sync.Mutex
	file    *os.File
	enabled bool
)

// Setup opens (or re-opens) the audit log sink from the given config. It is a
// no-op when the config is nil or disabled, in which case Record does noting.
// Calling Setup again closes any previously opened file, so it is safe to call
// repeatedly
func Setup(c *Config) error {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		_ = file.Close()
		file = nil
	}
	enabled = false
	if c == nil || !c.Enabled {
		return nil
	}
	if c.Path == "" {
		return fmt.Errorf("audit log is enabled but no path is configured")
	}
	// 0600: the audit log can contain usernames and client IPs, so it is
	// deliberately stricter than the general app logs
	f, err := os.OpenFile(c.Path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		return err
	}
	file = f
	enabled = true
	return nil
}

// Close closes the audit log sink. Safe to call when disabled.
func Close() error {
	mu.Lock()
	defer mu.Unlock()
	enabled = false
	if file == nil {
		return nil
	}
	err := file.Close()
	file = nil
	return err
}

// Record writes one event as a single JSON line.
//
// Fail-open by design: a disabled sink is a silent no-op, and marshal/write
// errors are logged via the app logger but never returned and never abort the
// audited action
func Record(e Event) {
	mu.Lock()
	defer mu.Unlock()
	if !enabled || file == nil {
		return
	}
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	line, err := json.Marshal(e)
	if err != nil {
		log.Errorf("audit: failed to marshal event %q: %v", e.Event, err)
		return
	}
	line = append(line, '\n')
	if _, err := file.Write(line); err != nil {
		log.Errorf("audit: failed to write event %q: %v", e.Event, err)
	}
}

// SourceIP extracts the client IP from an HTTP request. RemoteAddr has already
// been normalized to the real client address by gorilla/handlers.ProxyHeaders
// in the middleware chain, so this just strips the port.
func SourceIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

# Audit Log

Gophish can record security-relevant events (authentication, account
management, and campaign lifecycle actions ) to a dedicated, append-only
**audit log** in JSON Lines format (one JSON object
per line). The file is separate from the application log and is designed to be
shipped to a SIEM or log aggregator.

The audit log is **disabled by default**.

## Enabling

Add an `audit_log` block to `config.json`:

```json
"audit_log": {
    "enabled": true,
    "path": "audit.log"
}
```

- `enabled` - turns the audit log on. When `false` (the default), nothing is
  written and there is no runtime cost.
- `path` - file the events are appended to. It is created on startup if it
  does not exist, with permissions `0600` (owner read/write only), because the
  log contains usernames and client IP addresses.

Restart Gophish after changing the configuration.

## Format

Each line is a self-contained JSON object. Fields that don't apply to an event
are omitted.

| Field | Description |
|---|---|
| `time` | Event time, UTC, RFC 3339. |
| `event` | Event type (see table below). |
| `outcome` | `success` or `failure`. |
| `actor` | Who performed the action: `{ "id", "username" }`. `id` is omitted when unknown (e.g. a login attempt for a non-existent username). |
| `target` | The object acted upon when it differs from the actor: `{ "type", "id", "name" }`. |
| `source_ip` | Client IP of the request. |
| `reason` | Optional failure/qualifier, e.g. `invalid_password`. |
| `details` | Optional structured context, e.g. which fields changed on a user. |

Example lines:

```json
{"time":"2026-11-23T10:00:00Z","event":"login_failure","outcome":"failure","reason":"invalid_password","actor":{"id":1,"username":"admin"},"source_ip":"203.0.113.4"}
{"time":"2026-11-23T10:00:07Z","event":"login_success","outcome":"success","actor":{"id":1,"username":"admin"},"source_ip":"203.0.113.4"}
{"time":"2026-11-23T10:02:11Z","event":"user_modified","outcome":"success","actor":{"id":1,"username":"admin"},"target":{"type":"user","id":5,"name":"bob"},"source_ip":"203.0.113.4","details":{"role":{"from":"user","to":"admin"}}}
```

Because every value, including the submitted username, is a structured JSON
field, untrusted input cannot forge extra log lines or break the format (log
injection is structurally prevented).

## Recorded events

| `event` | When |
|---|---|
| `login_success` | A user logs in successfully. |
| `login_failure` | A login attempt fails. `reason` is `unknown_user`, `invalid_password`, or `account_locked`. |
| `logout` | A user logs out. |
| `impersonate` | An admin logs in as another user (`target`). |
| `user_created` | A user account is created (`target`, `details.role`, `details.account_locked`). |
| `user_modified` | A user account is changed. `details` lists the changed fields (role, team, account_locked, password_change_required, password_reset). |
| `user_deleted` | A user account is deleted (`target`). |
| `password_changed` | A user changes their own password. `reason` is `forced_reset` when it was a required reset. |
| `api_key_reset` | A user resets their own API key. |
| `campaign_created` | A campaign is created (`target`). |
| `campaign_launched` | A created campaign is launched immediately. |
| `campaign_completed` | A campaign is marked complete. |
| `campaign_deleted` | A campaign is deleted. |
| `campaign_scrubbed` | A campaign's recipient data is irreversibly anonymized. |

## Client IP behind a proxy

`source_ip` is taken from the request's remote address after
`X-Forwarded-For` / `X-Real-IP` processing, so when Gophish runs behind a
reverse proxy it reflects the real client address. Make sure your proxy sets
these headers.

## Privacy

The audit log contains personal data (usernames and client IP addresses).
Restrict read access to the file (it is created `0600`), and include it in
your data-retention and protection considerations alongside campaign results.

## Rotation

Gophish keeps the file open and appends to it; it does not rotate the file
itself. Use `logrotate` with `copytruncate` so rotation works without
restarting Gophish:

```
/path/to/audit.log {
    weekly
    rotate 12
    compress
    missingok
    notifempty
    copytruncate
}
```

## Reliability

Audit writing is **fail-open**: if a write fails (e.g. a full disk), the error
is reported in the application log and the audited action still proceed, a
logging problem never blocks login or other operations. Monitor the
application log for `audit:` errors if the audit trail is a compliance
requirement.


# Settings

## Reset API Key

`POST /api/reset`

This endpoint allows you to reset your API key to a new, randomly generated key.

This method requires you to authenticate using your existing API key.

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | The existing API key. |

**Response `200`**

API key successfully reset. The new API key is provided in the `data` response parameter.

```javascript
{
    "success": true,
    "message": "API Key successfully reset!",
    "data": "0123456789abcdef"
}
```

!!! note
    The endpoints below (IMAP email reporting, including OAuth2) are specific to this fork and aren't part of the upstream Gophish API.

## Get IMAP Settings

`GET /api/imap/`

Returns the current user's IMAP email-reporting configuration as a list (empty if none has been saved yet). Secrets (`password`, `oauth_client_secret`) are never included in the response; an `oauth_connected` field indicates whether an OAuth2 account has completed the authorization flow.

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Response `200`**

```javascript
[
  {
    "enabled": true,
    "host": "imap.gmail.com",
    "port": 993,
    "username": "user@example.com",
    "tls": true,
    "auth_type": "oauth2",
    "oauth_provider": "google",
    "oauth_client_id": "...",
    "oauth_connected": true,
    "folder": "INBOX",
    "imap_freq": 60
  }
]
```

## Save IMAP Settings

`POST /api/imap/`

Creates or updates the current user's IMAP configuration. A blank `password` or `oauth_client_secret` in the request is treated as "unchanged" and preserves the previously saved value, since those secrets are never returned by `GET /api/imap/` for the client to round-trip.

For `auth_type: "oauth2"`, `host`/`port`/`username` are still required - they identify the IMAP server and mailbox to connect to; OAuth2 only replaces the password. A `secret_key` must also be configured in `config.json` (OAuth2 tokens are encrypted at rest with it), or this call fails validation.

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Body Parameters**

| Name | Required | Description |
|---|---|---|
| `Payload` (object) | Yes | The JSON representation of the IMAP configuration to save. See `GET /api/imap/` for the field format. |

**Response `201`**

```javascript
{
  "success": true,
  "message": "Successfully saved IMAP settings."
}
```

## Validate IMAP Settings

`POST /api/imap/validate`

Tests a connection to the configured IMAP server. For `auth_type: "basic"`, the request body's credentials are used directly. For `auth_type: "oauth2"`, request-body secrets are ignored (they never round-trip to the browser) and the already-saved, already-connected configuration is tested instead - save and connect the account first.

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Body Parameters**

| Name | Required | Description |
|---|---|---|
| `Payload` (object) | Yes | The IMAP configuration to test. |

**Response `200`**

```javascript
{
  "success": true,
  "message": "Successful login."
}
```

## Connect OAuth2 Account

`GET /oauth/imap/authorize`

Starts the OAuth2 authorization-code flow for IMAP email reporting. This is a browser redirect endpoint (not a JSON API call) - it requires an authenticated admin session (not an API key) and redirects the browser to the configured provider's consent screen.

Requires an OAuth2 IMAP configuration to already be saved via `POST /api/imap/` (provider, client ID/secret, and - for the "custom" provider - the authorization/token URLs and scopes). If none is saved, redirects back to `/settings` with an error flash instead.

## OAuth2 Callback

`GET /oauth/imap/callback`

Completes the OAuth2 flow started by `/oauth/imap/authorize`. The identity provider redirects the browser here after the user grants (or denies) consent. On success, the encrypted access/refresh tokens are persisted and the browser is redirected back to `/settings#reportingSettings`; on failure (state mismatch, denied consent, exchange error), redirects back with an error flash instead.

This endpoint is only ever invoked by the identity provider's redirect - it isn't meant to be called directly.


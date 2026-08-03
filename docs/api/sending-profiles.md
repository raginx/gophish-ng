# Sending Profiles

A "Sending Profile" is the SMTP configuration that tells Gophish how to send emails.

Sending profiles support authentication and ignoring invalid SSL certificates.

Sending Profiles have the following structure:

```text
{
  id                 : int64
  name               : string
  username           : string (optional)
  password           : string (optional)
  host               : string
  interface_type     : string
  from_address       : string
  ignore_cert_errors : boolean (default:false)
  modified_date      : string(datetime)
  headers            : array({key: string, value: string}) (optional)
}
```

## Get Sending Profiles

`GET /api/smtp/`

Gets a list of the sending profiles created by the authenticated user.

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Response `200`**

A list of the sending profiles created by the authenticated user.

```javascript
[
  {
    "id" : 1,
    "name":"Example Profile",
    "interface_type":"SMTP",
    "from_address":"John Doe <john@example.com>",
    "host":"smtp.example.com:25",
    "username":"",
    "password":"",
    "ignore_cert_errors":true,
    "modified_date": "2016-11-20T14:47:51.4131367-06:00",
    "headers": [
      {
        "key": "X-Header",
        "value": "Foo Bar"
      }
    ]
  }
]
```

## Get Sending Profile

`GET /api/smtp/:id`

Returns a sending profile given an ID, returning a 404 error if no sending profile with the provided ID is found.

**Path Parameters**

| Name | Required | Description |
|---|---|---|
| `id` (integer) | Yes | The sending profile ID to return |

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Response `200`**

```javascript
{
  "id" : 1,
  "name":"Example Profile",
  "interface_type":"SMTP",
  "from_address":"John Doe <john@example.com>",
  "host":"smtp.example.com:25",
  "username":"",
  "password":"",
  "ignore_cert_errors":true,
  "modified_date": "2016-11-20T14:47:51.4131367-06:00",
  "headers": [
    {
      "key": "X-Header",
      "value": "Foo Bar"
    }
  ]
}
```

**Response `404`**

```javascript
{
  "message": "SMTP not found",
  "success": false,
  "data": null
}
```

## Create Sending Profile

`POST /api/smtp`

Creates a sending profile.

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Body Parameters**

| Name | Required | Description |
|---|---|---|
| `Payload` (object) | Yes | The body of the request is a JSON representation of a sending profile. Refer to the introduction for the valid format of a sending profile. |

**Response `201`**

```javascript
{
  "id" : 1,
  "name":"Example Profile",
  "interface_type":"SMTP",
  "from_address":"John Doe <john@example.com>",
  "host":"smtp.example.com:25",
  "username":"",
  "password":"",
  "ignore_cert_errors":true,
  "modified_date": "2016-11-20T14:47:51.4131367-06:00",
  "headers": [
    {
      "key": "X-Header",
      "value": "Foo Bar"
    }
  ]
}
```

**Response `400`**

If required fields aren't provided, or if a sending profile already exists with the provided name, a 400: Bad Request error will be returned.

```javascript
{
  "message": "Error message indicating the issue",
  "success": false,
  "data": null
}
```

This method expects the sending profile to be provided in JSON format. You must provide a sending profile `name`, the `from_address` which emails are sent from, and the SMTP relay `host`.

Sending Profiles support authentication by setting the `username` and `password`.

Additionally, many SMTP server deployments leverage self-signed certificates. To tell Gophish to ignore these invalid certificates, set the `ignore_cert_errors` field to `true`.

This method returns the JSON representation of the sending profile that was created.

## Modify Sending Profile

`PUT /api/smtp/:id`

Modifies an existing sending profile.

**Path Parameters**

| Name | Required | Description |
|---|---|---|
| `id` (integer) | Yes | The sending profile ID to modify |

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Body Parameters**

| Name | Required | Description |
|---|---|---|
| `Payload` (object) | Yes | The body of the request is a JSON representation of a sending profile. Refer to the introduction for the valid format of a sending profile. |

**Response `200`**

```javascript
{
  "id" : 1,
  "name":"Example Profile",
  "interface_type":"SMTP",
  "from_address":"John Doe <john@example.com>",
  "host":"smtp.example.com:25",
  "username":"",
  "password":"",
  "ignore_cert_errors":true,
  "modified_date": "2016-11-20T14:47:51.4131367-06:00",
  "headers": [
    {
      "key": "X-Header",
      "value": "Foo Bar"
    }
  ]
}
```

**Response `404`**

If no sending profile exists with the provided ID, a 404: Not Found error is returned.

```javascript
{
  "message": "SMTP not found",
  "success": false,
  "data": null
}
```

This method expects the sending profile to be provided in JSON format. You must provide a full sending profile, not just the fields you want to update.

This method returns the JSON representation of the sending profile that was modified.

## Delete Sending Profile

`DELETE /api/smtp/:id`

Deletes a sending profile by ID.

**Path Parameters**

| Name | Required | Description |
|---|---|---|
| `id` (integer) | Yes | The ID of the sending profile to delete |

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Response `200`**

```javascript
{
  "message": "SMTP deleted successfully!",
  "success": true,
  "data": null
}
```

**Response `404`**

```javascript
{
  "message": "SMTP not found",
  "success": false,
  "data": null
}
```

Returns a 404 error if the specified sending profile isn't found.

This method returns a status message indicating the sending profile was deleted successfully.


# Landing Pages

A "Landing Page" is the HTML content returned when targets click on the links in Gophish emails.

Landing pages have the following structure:

```text
{
  id                  : int64
  name                : string
  html                : string
  capture_credentials : bool
  capture_passwords   : bool
  modified_date       : string(datetime)
  redirect_url        : string
}
```

## Get Landing Pages

`GET /api/pages/`

Returns a list of landing pages.

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Response `200`**

```javascript
[  
 {
    "id": 1,
    "name": "Example Page",
    "html": "<html><head></head><body>This is a test page</body></html>",
    "capture_credentials": true,
    "capture_passwords": true,
    "redirect_url": "http://example.com",
    "modified_date": "2016-11-26T14:04:40.4130048-06:00"
  }
]
```

## Get Landing Page

`GET /api/pages/:id`

Returns a landing page given an ID.

**Path Parameters**

| Name | Required | Description |
|---|---|---|
| `id` (integer) | Yes | The landing page ID |

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Response `200`**

```javascript
{
   "id": 1,
   "name": "Example Page",
   "html": "<html><head></head><body>This is a test page</body></html>",
   "capture_credentials": true,
   "capture_passwords": true,
   "redirect_url": "http://example.com",
   "modified_date": "2016-11-26T14:04:40.4130048-06:00"
}
```

**Response `404`**

```javascript
{
  "message": "Page not found",
  "success": false,
  "data": null
}
```

Returns a 404 error if the specified landing page isn't found.

## Create Landing Page

`POST /api/pages/`

Creates a landing page.

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Body Parameters**

| Name | Required | Description |
|---|---|---|
| `Payload` (object) | Yes | The JSON representation of the landing page to be created |

**Response `201`**

```javascript
{
   "id": 1,
   "name": "Example Page",
   "html": "<html><head></head><body>This is a test page</body></html>",
   "capture_credentials": true,
   "capture_passwords": true,
   "redirect_url": "http://example.com",
   "modified_date": "2016-11-26T14:04:40.4130048-06:00"
}
```

This method expects the landing page to be provided in JSON format. You must provide a landing page `name` and the `html` for the landing page.

!!! info
    **Importing a Site**

    Let Gophish do the hard work for you by importing a site. By using the [Import Site](landing-pages.md#import-site) endpoint, you can simply give Gophish a URL and have the site fetched for you and returned in a format that can be used with this method.

#### Capturing Credentials

Capturing credentials is a powerful feature of Gophish. By setting certain flags, you have the ability to capture all user input, or just non-password input.

To capture credentials, set the `capture_credentials` attribute. If you want to capture passwords as well, set the `capture_passwords` attribute.

By default, Gophish will not capture passwords, as they are stored in plaintext.

Gophish also provides the ability to redirect users to a URL after they submit credentials. This is controlled by setting the `redirect_url` attribute.

## Modify Landing Page

`PUT /api/pages/:id`

Modifies an existing landing page.

**Path Parameters**

| Name | Required | Description |
|---|---|---|
| `id` (integer) | Yes | The ID of the landing page to modify |

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Body Parameters**

| Name | Required | Description |
|---|---|---|
| `Payload` (object) | Yes | The JSON representation of the landing page to be modified |

**Response `200`**

```javascript
{
   "id": 1,
   "name": "Example Page",
   "html": "<html><head></head><body>This is a test page</body></html>",
   "capture_credentials": true,
   "capture_passwords": true,
   "redirect_url": "http://example.com",
   "modified_date": "2016-11-26T14:04:40.4130048-06:00"
}
```

**Response `404`**

```javascript
{
  "message": "Page not found",
  "success": false,
  "data": null
}
```

Returns a 404 error if the specified landing page isn't found.

This method expects the landing page to be provided in JSON format. You must provide a full landing page, not just the fields you want to update.

This method returns the JSON representation of the landing page that was modified.

## Delete Landing Page

`DELETE /api/pages/:id`

Deletes a landing page.

**Path Parameters**

| Name | Required | Description |
|---|---|---|
| `id` (integer) | Yes | The ID of the landing page to delete |

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Response `200`**

```javascript
{
  "message": "Page Deleted Successfully",
  "success": true,
  "data": null
}
```

**Response `404`**

```javascript
{
  "message": "Page not found",
  "success": false,
  "data": null
}
```

Returns a 404 error if the specified landing page isn't found.

This method returns a status message indicating the landing page was deleted successfully.

## Import Site

`POST /api/import/site`

Fetches a URL to be later imported as a landing page

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Body Parameters**

| Name | Required | Description |
|---|---|---|
| `include_resources` (boolean) | No | Whether or not to create a `<base>` tag in the resulting HTML to resolve static references (recommended: `false`) |
| `url` (string) | Yes | The URL to fetch |

**Response `200`**

```javascript
{
    "html": "<html><head>..."
}
```

This endpoint simply fetches and returns the HTML from a provided URL. If `include_resources` is `false` (recommended), a `<base>` tag is added so that relative links in the HTML resolve from the original URL.

Additionally, if the HTML contains form elements, this endpoint adds another input, `__original_url`, that points to the original URL. This makes it possible to replay captured credentials later.

!!! info
    **Note:** This API endpoint doesn't actually create a new landing page. Instead, you can use the HTML returned from this endpoint as an input to the [Create Landing Page](landing-pages.md#create-landing-page) method.


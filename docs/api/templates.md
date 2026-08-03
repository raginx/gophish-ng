# Templates

A "Template" is the content of the emails that are sent to targets. They can be imported from an existing email, or created from scratch.

Additionally, templates can contain tracking images so that gophish knows when the user opens the email.

Templates have the following structure:

```text
{
  id            : int64
  name          : string
  subject       : string
  text          : string
  html          : string
  modified_date : string(datetime)
  attachments   : list(attachment)
}
```

Templates support sending attachments. Attachments have the following structure:

```text
  content: string
  type   : string
  name   : string
```

> Note: The `content` field in an attachment is expected to be base64 encoded.

## Get Templates

`GET /api/templates`

Returns a list of templates.

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Response `200`**

```javascript
[
  {
    "id" : 1,
    "name" : "Password Reset Template",
    "subject" : "{{.FirstName}}, please reset your password.",
    "text" : "Please reset your password here: {{.URL}}",
    "html" : "<html><head></head><body>Please reset your password <a href\"{{.URL}}\">here</a></body></html>",
    "modified_date" : "2016-11-21T18:30:11.1477736-06:00",
    "attachments" : [],
  }
]
```

## Get Template

`GET /api/templates/:id`

Returns a template with the provided ID.

Returns a 404: Not Found error if the specified template doesn't exist.

**Path Parameters**

| Name | Required | Description |
|---|---|---|
| `id` (integer) | Yes | The template ID |

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Response `200`**

```javascript
{
    "id" : 1,
    "name" : "Password Reset Template",
    "subject" : "{{.FirstName}}, please reset your password.",
    "text" : "Please reset your password here: {{.URL}}",
    "html" : "<html><head></head><body>Please reset your password <a href\"{{.URL}}\">here</a></body></html>",
    "modified_date" : "2016-11-21T18:30:11.1477736-06:00",
    "attachments" : [],
}
```

**Response `404`**

```javascript
{
  "message": "Template not found",
  "success": false,
  "data": null
}
```

## Create Template

`POST /api/templates/`

Creates a new template from the provided JSON request body.

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Body Parameters**

| Name | Required | Description |
|---|---|---|
| `Payload` (object) | Yes | The request body should be a JSON representation of a template. See the schema at the top of this page for the template format. |

**Response `201`**

```javascript
{
    "id" : 1,
    "name" : "Password Reset Template",
    "subject" : "{{.FirstName}}, please reset your password.",
    "text" : "Please reset your password here: {{.URL}}",
    "html" : "<html><head></head><body>Please reset your password <a href\"{{.URL}}\">here</a></body></html>",
    "modified_date" : "2016-11-21T18:30:11.1477736-06:00",
    "attachments" : [],
}
```

**Response `400`**

At least one text or HTML field must be specified, otherwise a 400: Bad Request error is returned

```javascript
{
  "message": "Need to specify at least plaintext or HTML content",
  "success": false,
  "data": null
}
```

This method expects the template to be provided in JSON format. You must provide a template `name` and the `text` and/or `html` for the template.

!!! info
    **Importing an Existing Email** 

    What better way to make pixel-perfect emails than by importing an existing email you already have in your inbox?

    Using the [Import Email](templates.md#import-template) endpoint, you can take a raw email and parse it as a valid Gophish template.

To add tracking, make sure you specify a `{{.Tracker}}` in the `html` field. The UI adds this automatically, but it needs to be specified if you're using the API.

This method returns the JSON representation of the template that was created.

## Modify Template

`PUT /api/templates/:id`

Modifies an existing template.

**Path Parameters**

| Name | Required | Description |
|---|---|---|
| `id` (integer) | Yes | The template ID to modify |

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Body Parameters**

| Name | Required | Description |
|---|---|---|
| `Payload` (object) | Yes | The JSON representation of the template you wish to modify. The entire template must be provided, not just the fields you wish to update. |

**Response `200`**

This method expects the template to be provided in JSON format. You must provide a full template, not just the fields you want to update.

This method returns the JSON representation of the template that was modified.

## Delete Template

`DELETE /api/templates/:id`

Deletes a template by ID.

**Path Parameters**

| Name | Required | Description |
|---|---|---|
| `id` (integer) | Yes | The template ID to delete |

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Response `200`**

```javascript
{
  "message": "Template deleted successfully!",
  "success": true,
  "data": null
}
```

**Response `404`**

If no template is found with the provided ID, a 404: Not Found error is returned

```javascript
{
  "message": "Template not found",
  "success": false,
  "data": null
}
```

Returns a 404 error if the specified template isn't found.

This method returns a status message indicating the template was deleted successfully.

## Import Template

`POST /api/import/email`

Imports an email as a template.

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Body Parameters**

| Name | Required | Description |
|---|---|---|
| `convert_links` (boolean) | Yes | Whether or not to convert the links within the email to {{.URL}} automatically. |
| `content` (string) | Yes | The original email content in RFC 2045 format, including the original headers. |

**Response `200`**

```javascript
{
  "text": "Email text",
  "html": "Email HTML",
  "subject": "Email subject"
}
```

 Gophish provides the ability to parse an existing email to be used as a template. This makes it easy to repurpose legitimate emails for your phishing assessments.

This endpoint expects the raw email content in [RFC 2045 format](https://www.ietf.org/rfc/rfc2045.txt), including the original headers. Usually, this is found using the "Show Original" feature of email clients.

The request body for this endpoint is a JSON request in the form of:

```javascript
{
    content:       string
    convert_links: boolean
}
```

By setting the `convert_links` attribute to `true`, Gophish will automatically change all the links in the email to `{{.URL}}`.

!!! info
    **Note:** This method doesn't fully import the email as a template. Instead, it parses the email, returning a response that can be used with the "[Create Template](templates.md#create-template)" endpoint.


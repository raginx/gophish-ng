---
description: "API reference for Gophish-NG Tags, used to categorize and filter Templates, Landing Pages, Sending Profiles, Groups, and Campaigns."
---

# Tags

A "Tag" is a team-scoped label used to categorize Templates, Landing Pages,
Sending Profiles, Groups, and Campaigns. See the
[Tags guide](../guide/tags.md) for the concept and UI. Tags have the
following structure:

```text
{
  id            : int64
  name          : string
  color         : string
  modified_date : string(datetime)
}
```

## Tags on other objects

Every taggable object (Template, Landing Page, Sending Profile, Group,
Campaign) includes a `tags` field, which is a list of tag names:

```javascript
{
  "id" : 1,
  "name" : "Password Reset Template",
  "tags" : ["de", "password-reset"],
  ...
}
```

To set an object's tags, include the `tags` field when creating or updating
it (`POST`/`PUT`). Names that don't exist yet are created automatically in
your team; names are matched case-insensitively. Passing an empty list
clears the object's tags.

## Filtering a list by tag

The list endpoints for taggable objects — `GET /api/templates`,
`/api/pages`, `/api/smtp`, `/api/groups`, and `/api/campaigns` — accept a
repeatable `tag` query parameter. Results include objects carrying **any**
of the given tags:

`GET /api/templates/?tag=de&tag=invoice`

## Get Tags

`GET /api/tags`

Returns a list of the tags defined for your team.

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Response `200`**

```javascript
[
  {
    "id" : 1,
    "name" : "de",
    "color" : "",
    "modified_date" : "2026-09-26T11:54:00.000000-06:00"
  }
]
```

## Create Tag

`POST /api/tags`

Creates a new tag from the provided JSON request body. Tags are usually
created implicitly by tagging an object; this endpoint is useful for
pre-creating a tag (for example, to assign a color).

Returns a 409: Conflict error if a tag with the same name already exists in
your team.

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Body Parameters**

| Name | Required | Description |
|---|---|---|
| `Payload` (object) | Yes | A JSON representation of a tag. Only `name` (required) and `color` are used. |

**Response `201`**

```javascript
{
  "id" : 2,
  "name" : "invoice",
  "color" : "",
  "modified_date" : "2026-09-26T12:52:00.000000-06:00"
}
```

## Modify Tag

`PUT /api/tags/:id`

Renames and/or recolors the tag with the provided ID. Renaming applies
everywhere the tag is used.

**Path Parameters**

| Name | Required | Description |
|---|---|---|
| `id` (integer) | Yes | The tag ID |

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Body Parameters**

| Name | Required | Description |
|---|---|---|
| `Payload` (object) | Yes | A JSON representation of the tag with the updated `name` and/or `color`. |

## Delete Tag

`DELETE /api/tags/:id`

Deletes the tag with the provided ID and removes it from every object it
was applied to. The objects themselves are left unchanged.

**Path Parameters**

| Name | Required | Description |
|---|---|---|
| `id` (integer) | Yes | The tag ID |

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Response `200`**

```javascript
{
  "message": "Tag deleted successfully!",
  "success": true,
  "data": null
}
```

# Users & Groups

Gophish manages recipients for campaigns in groups. Each group can contain one or more recipients. Groups have the following format:

```text
{
    id              : int64
    name            : string
    targets         : array(Target)
    modified_date   : string(datetime)
}
```

Each recipient in the `targets` field has the following format:

```text
{
    email           : string
    first_name      : string
    last_name       : string
    position        : string
}
```

## Get Groups

`GET /api/groups/`

Returns a list of groups.

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Response `200`**

```javascript
[
  {
    "id": 1,
    "name": "Example Group",
    "modified_date": "2018-10-08T15:56:13.790016Z",
    "targets": [
      {
        "email": "user@example.com",
        "first_name": "Example",
        "last_name": "User",
        "position": ""
      },
      {
        "email": "foo@bar.com",
        "first_name": "Foo",
        "last_name": "Bar",
        "position": ""
      }
    ]
  }
]
```

## Get Group

`GET /api/groups/:id`

Returns a group with the given ID.

**Path Parameters**

| Name | Required | Description |
|---|---|---|
| `id` (integer) | Yes | The group ID |

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Response `200`**

```javascript
{
  "id": 1,
  "name": "Example Group",
  "modified_date": "2018-10-08T15:56:13.790016Z",
  "targets": [
    {
      "email": "user@example.com",
      "first_name": "Example",
      "last_name": "User",
      "position": ""
    },
    {
      "email": "foo@bar.com",
      "first_name": "Foo",
      "last_name": "Bar",
      "position": ""
    }
  ]
}
```

**Response `404`**

```javascript
{
  "message": "Group not found",
  "success": false,
  "data": null
}
```

Returns a 404 error if no group is found with the provided ID.

## Get Groups Summary

`GET /api/groups/summary`

Returns a summary of each group.

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Response `200`**

```javascript
[
  {
    "id": 1,
    "name": "Example Group",
    "modified_date": "2018-10-08T15:56:13.790016Z",
    "num_targets": 2
  }
]
```

## Get Group Summary

`GET /api/groups/:id/summary`

Returns a summary for a group.

**Path Parameters**

| Name | Required | Description |
|---|---|---|
| `id` (integer) | Yes | The group ID |

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Response `200`**

```javascript
{
  "id": 1,
  "name": "Example Group",
  "modified_date": "2018-10-08T15:56:13.790016Z",
  "num_targets": 2
}
```

**Response `404`**

```javascript
{
  "message": "Group not found",
  "success": false,
  "data": null
}
```

It may be the case that you just want the number of members in a group, not necessarily the full member details. This API endpoint returns a summary for a group.

Returns a 404 error if no group is found with the provided ID.

## Create Group

`POST /api/groups/`

Creates a new group.

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Body Parameters**

| Name | Required | Description |
|---|---|---|
| `Payload` (object) | Yes | The group to create in JSON format. |

**Response `201`**

```javascript
{
  "id": 1,
  "name": "Example Group",
  "modified_date": "2018-10-08T15:56:13.790016Z",
  "targets": [
    {
      "email": "user@example.com",
      "first_name": "Example",
      "last_name": "User",
      "position": ""
    },
    {
      "email": "foo@bar.com",
      "first_name": "Foo",
      "last_name": "Bar",
      "position": ""
    }
  ]
}
```

**Response `400`**

If an invalid request is provided, an error message will be returned

```
{
  "message": "Group name not specified",
  "success": false,
  "data": null
}
```

When creating a new group, you must specify a unique `name`, as well as a list of `targets`. Here's an example request body:

```javascript
{
    "name": "Example Group",
    "targets": [
    {
        "email": "user@example.com",
        "first_name": "Example",
        "last_name": "User",
        "position": ""
    },
    {
        "email": "foo@bar.com",
        "first_name": "Foo",
        "last_name": "Bar",
        "position": ""
    }
    ]
}
```

## Modify Group

`PUT /api/groups/:id`

Modifies a group.

**Path Parameters**

| Name | Required | Description |
|---|---|---|
| `id` (integer) | Yes | The group ID |

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Body Parameters**

| Name | Required | Description |
|---|---|---|
| `Payload` (object) | Yes | The updated group content. The full group must be provided in JSON format. |

**Response `200`**

```javascript
{
  "id": 1,
  "name": "Example Modified Group",
  "modified_date": "2018-10-08T15:56:13.790016Z",
  "targets": [
    {
      "email": "foo@bar.com",
      "first_name": "Foo",
      "last_name": "Bar",
      "position": ""
    }
  ]
}
```

**Response `404`**

```javascript
{
  "message": "Group not found",
  "success": false,
  "data": null
}
```

This API endpoint allows you to modify an existing group. The request must include the complete group JSON, not just the fields you're wanting to update. This means that you need to include the matching `id` field. Here's an example request:

```javascript
{
    "id": 1,
    "name": "Example Modified Go",
    "targets": [
    {
        "email": "foo@bar.com",
        "first_name": "Foo",
        "last_name": "Bar",
        "position": ""
    }
    ]
}
```

Returns a 404 if no group is found with the provided ID.

## Delete Group

`DELETE /api/groups/:id`

Deletes a group

**Path Parameters**

| Name | Required | Description |
|---|---|---|
| `id` (number) | Yes | The group ID |

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Response `200`**

```javascript
{
  "message": "Group deleted successfully!",
  "success": true,
  "data": null
}
```

**Response `404`**

```javascript
{
  "message": "Group not found",
  "success": false,
  "data": null
}
```

Returns a 404 error if no group is found with the provided ID.

## Import Group

`POST /api/import/group`

Reads and parses a CSV, returning data that can be used to create a group.

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Body Parameters**

| Name | Required | Description |
|---|---|---|
| `file` (object) | Yes | A file upload containing the CSV content to parse. |

**Response `200`**

```javascript
[
  {
    "email": "foobar@example.com",
    "first_name": "Example",
    "last_name": "User",
    "position": "Systems Administrator"
  },
  {
    "email": "johndoe@example.com",
    "first_name": "John",
    "last_name": "Doe",
    "position": "CEO"
  }
]
```

This API endpoint allows you to upload a CSV, returning a list of group targets. For example, you can use the following `curl` command to upload the CSV:

```text
curl -k https://localhost:3333/api/import/group -XPOST \
    -F "file=@group_template.csv" \
    -H "Authorization: Bearer 0123456789abcdef"
```

The results of this API endpoint can be used as the `targets` parameter in a call to the [Create Group](users-and-groups.md#create-group) endpoint.


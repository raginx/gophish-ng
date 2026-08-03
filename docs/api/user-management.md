# User Management

Gophish supports having multiple user accounts. Each of these accounts are separate, with their own campaigns, landing pages, templates, etc.

Each user account in Gophish is assigned a **role**. These are global roles that describe the user's permissions within Gophish.

At the time of this writing, there are two roles:

| Role | Slug | **Description** |
| :--- | :--- | :--- |
| User | `user` | A non-administrative user role. Users with this role can create objects and launch campaigns. |
| Admin | `admin` | An administrative user. Users with this role can manage system-wide settings as well as other user accounts within Gophish. |

Users have the following format:

```text
{
    id              : int64
    username        : string
    role            : Role
    modified_date   : string(datetime)
}
```

Each Role has the following format:

```text
{
    name            : string
    slug            : string
    description     : string
}
```

## Get Users

`GET /api/users/`

Returns a list of all user accounts in Gophish.

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Response `200`**

```javascript
[
  {
    "id": 1,
    "username": "admin",
    "role": {
      "slug": "admin",
      "name": "Admin",
      "description": "System administrator with full permissions"
    }
  }
]
```

## Get User

`GET /api/users/:id`

Returns a user with the given ID.

**Path Parameters**

| Name | Required | Description |
|---|---|---|
| `id` (integer) | Yes | The user ID |

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Response `200`**

```javascript
[
  {
    "id": 1,
    "username": "admin",
    "role": {
      "slug": "admin",
      "name": "Admin",
      "description": "System administrator with full permissions"
    }
  }
]
```

**Response `404`**

```javascript
{
  "message": "User not found",
  "success": false,
  "data": null
}
```

## Create User

`POST /api/users/`

Creates a new user.

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes |  |

**Body Parameters**

| Name | Required | Description |
|---|---|---|
| `role` (string) | Yes | The role slug to use for the account |
| `password` (string) | Yes | The password to set for the account |
| `username` (string) | Yes | The username for the account |

**Response `200`**

```javascript
{
  "id": 2,
  "username": "exampleuser",
  "role": {
    "slug": "user",
    "name": "User",
    "description": "User role with edit access to objects and campaigns"
}
```

**Response `400`**

If an invalid request is provided, an error will be returned with the following format

```javascript
{
  "message": "Username already taken",
  "success": false,
  "data": null
}
```

## Modify User

`PUT /api/users/:id`

Modifies a user account. This can be used to change the role, reset the password, or change the username.

**Path Parameters**

| Name | Required | Description |
|---|---|---|
| `id` (string) | Yes | The user ID |

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Body Parameters**

| Name | Required | Description |
|---|---|---|
| `role` (string) | No | The role slug to use for the account |
| `password` (string) | No | The password to set for the account |
| `username` (string) | Yes | The username for the account |

**Response `200`**

```javascript
{
  "id": 2,
  "username": "exampleuser",
  "role": {
    "slug": "user",
    "name": "User",
    "description": "User role with edit access to objects and campaigns"
}
```

**Response `400`**

If an invalid request is provided, an error will be returned in the following format:

```javascript
{
  "message": "Username already taken",
  "success": false,
  "data": null
}
```

**Response `404`**

```javascript
{
  "message": "User not found",
  "success": false,
  "data": null
}
```

## Delete User

`DELETE /api/users/:id`

Deletes a user, as well as every object (landing page, template, etc.) and campaign they've created.

**Path Parameters**

| Name | Required | Description |
|---|---|---|
| `id` (string) | Yes | The user ID |

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key |

**Response `200`**

```javascript
{
  "message": "User deleted Successfully!",
  "success": true,
  "data": null
}
```

**Response `404`**

```javascript
{
  "message": "User not found",
  "success": false,
  "data": null
}
```

Returns a 404 error if no user is found with the provided ID.


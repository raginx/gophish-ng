# User Management

Gophish supports having multiple user accounts. Each of these accounts are separate, with their own campaigns, landing pages, templates, etc.

Each user account in Gophish is assigned a **role**. These are global roles that describe the user's permissions within Gophish.

At the time of this writing, there are three roles:

| Role | Slug | **Description** |
| :--- | :--- | :--- |
| User | `user` | A non-administrative user role. Users with this role can create objects and launch campaigns. |
| Admin | `admin` | An administrative user. Users with this role can manage system-wide settings as well as other user accounts within Gophish. |
| Auditor | `auditor` | A read-only role. Users with this role can view their team's objects and campaign results, but every state-changing API request is rejected with a `403`. Intended for reviewers, compliance staff, or the client an engagement is run for. |

!!! note
    The `auditor` role is a fork-specific addition and is not part of upstream Gophish. The only write an auditor may still perform is `POST /api/reset`, which rotates their own API key - that key is their own credential rather than an object belonging to their team.

!!! note
    This is a fork-specific addition (not part of the upstream Gophish API): every user also belongs to a **team**. Campaigns, templates, landing pages, groups, and sending profiles created by one user are visible to and editable by every other member of the same team, rather than being visible only to their creator. Teams are otherwise independent of each other - use separate teams to isolate unrelated engagements. See [Teams](#teams) below.

Users have the following format:

```text
{
    id              : int64
    username        : string
    role            : Role
    team            : Team
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

Each Team has the following format:

```text
{
    id              : int64
    name            : string
    modified_date   : string(datetime)
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
| `team` (string) | Yes | The team name for the account. A name that doesn't exist yet creates a new team. |
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
  },
  "team": {
    "id": 1,
    "name": "Default Team"
  }
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
| `role` (string) | No | The role slug to use for the account. Only an account with `modify_system` permission can change this - see below. |
| `team` (string) | Yes | The team name for the account. A name that doesn't exist yet creates a new team. Only an account with `modify_system` permission can change this. |
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
  },
  "team": {
    "id": 1,
    "name": "Default Team"
  }
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

Deletes a user account. Campaigns, landing pages, templates, groups, and sending profiles they created are team-owned and are left in place for the rest of the team - only the account itself is removed.

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

## Teams

!!! note
    Team management is fork-specific and not part of the upstream Gophish API.

`GET /api/teams/` and `POST /api/teams/` are admin-only (`modify_system` permission). There's no dedicated update/delete endpoint - a team is created implicitly the first time a not-yet-seen name is assigned to a user via [Create User](#create-user) or [Modify User](#modify-user).

`GET /api/teams/`

Returns a list of all teams.

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key, belonging to an account with `modify_system` permission |

**Response `200`**

```javascript
[
  {
    "id": 1,
    "name": "Default Team",
    "modified_date": "2026-08-05T12:00:00Z"
  }
]
```

`POST /api/teams/`

Creates a new team.

**Headers**

| Name | Required | Description |
|---|---|---|
| `Authorization` (string) | Yes | A valid API key, belonging to an account with `modify_system` permission |

**Body Parameters**

| Name | Required | Description |
|---|---|---|
| `name` (string) | Yes | The team name |

**Response `201`**

```javascript
{
  "id": 2,
  "name": "Engagement Beta",
  "modified_date": "2026-08-05T12:00:00Z"
}
```


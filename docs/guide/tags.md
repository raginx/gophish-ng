---
description: "Use tags to categorize and filter Email Templates, Landing Pages, Sending Profiles, Groups, and Campaigns in Gophish-NG."
---

# Tags

Tags are free-form labels you can attach to your **Email Templates**,
**Landing Pages**, **Sending Profiles**, **Groups**, and **Campaigns** to
keep them organized. Once you have more than a handful of any of these,
tags make it easy to group and find them — for example by language
(`de`, `en`), by scenario (`invoice`, `password-reset`), by client, or by
engagement.

## Adding tags to an item

Tags are edited right where you create or edit the item. In the New/Edit
dialog for a Template, Landing Page, Sending Profile, Group, or Campaign,
use the **Tags** field:

- Type a tag name and press Enter to add it. You can add as many as you
  like.
- As you type, previously used tags are suggested for autocomplete, so the
  same tag stays consistent instead of drifting into `invoice`,
  `Invoice`, and `invoices`.
- Tag names are case-insensitive: `DE` and `de` are treated as the same
  tag.
- Remove a tag by clicking the `×` on its pill.

Tags are saved together with the item when you save it.

## Filtering a list by tag

Each list page (Templates, Landing Pages, Sending Profiles, Groups, and
Campaigns) shows a **Filter by tag** control in the table toolbar, next to
the "entries per page" selector. Pick one or more tags to narrow the table
to items carrying **any** of the selected tags. Clear the selection to show
everything again.

The tag each item carries is also shown as a chip in its own **Tags**
column, so you can see at a glance how things are categorized.

## How tags are scoped

Tags are shared **within a team** and **across object types**. A tag named
`de` is a single label for your whole team: apply it to a Template and to a
Landing Page and it's the same tag, not two separate ones. Different
[teams](user-management.md) have their own, independent sets of tags.

Deleting an item removes its tag associations, but leaves the tag itself in
place for the other items still using it.

## Tags and Content Packs

When you export a [Content Pack](content-packs.md), each Template and
Landing Page carries its tags with it. On import, those tags are recreated
in the destination team (matching any tag that already exists there by
name), so your categorization travels between instances along with the
content.

## Managing tags via the API

Tags are also available through the API — every taggable object includes a
`tags` field, list endpoints accept a `?tag=` filter, and there's a
dedicated `/api/tags` endpoint for listing, renaming, and deleting tags.
See the [Tags API reference](../api/tags.md) for details.

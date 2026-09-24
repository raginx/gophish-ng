---
description: "Export and import Content Packs to share Email Templates and Landing Pages between Gophish-NG instances."
---

# Content Packs

A "Content Pack" is a single `.json` file that bundles Email Templates and
Landing Pages together, so they can be shared with another Gophish-NG
instance (or another team on the same instance) instead of recreating them
by hand.

To get started, click the "Content Packs" entry in the sidebar.

## Exporting a Content Pack

The Content Packs page lists all of your team's Email Templates and Landing
Pages in two separate tables. Tick the checkbox next to each item you want
to include, then click "Export Selected" to download a `content-pack.json`
file containing them.

> Note: Only Templates and Pages belonging to your current team can be
> exported. Attachments are included as part of their Template, but
> Sending Profiles and Groups are not part of a Content Pack.

## Importing a Content Pack

Click "Import Content Pack", choose a `.json` file exported from another
Gophish-NG instance, and click "Import". Every Template and Page in the
file is added to your current team as a new item.

If an imported Template or Page has the same name as one you already have,
it's imported anyway under a numbered name (e.g. "Login Page (2)") rather
than being skipped or overwriting the existing one. The import summary
lists anything that was renamed this way, along with any items that failed
to import.

> Note: Content Packs are only compatible between instances running a
> matching format version. If you see an "Unsupported content pack
> format_version" error, the pack was likely exported from a much older or
> newer Gophish-NG version.

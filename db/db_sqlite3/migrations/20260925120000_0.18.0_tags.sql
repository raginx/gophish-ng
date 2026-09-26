-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied
--
-- Adds team-scoped Tags that can be attached to any taggable object
-- (templates, pages, sending profiles, campaigns, groups) so users can
-- categorize and filter their content. Tags are shared across object types
-- within a team: a single "de" tag can label both a template and a page.
--
-- The link is stored polymorphically in the taggables table
-- (taggable_type + taggable_id) rather than a per-object foreign key, so a
-- new taggable object type can be added later without a schema change.
CREATE TABLE tags (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    team_id       INTEGER NOT NULL,
    name          VARCHAR(255) NOT NULL,
    color         VARCHAR(7),
    modified_date DATETIME
);
CREATE UNIQUE INDEX idx_tags_team_name ON tags(team_id, name);

CREATE TABLE taggables (
    tag_id        INTEGER NOT NULL,
    taggable_type VARCHAR(32) NOT NULL,
    taggable_id   INTEGER NOT NULL
);
CREATE UNIQUE INDEX idx_taggables_unique ON taggables(tag_id, taggable_type, taggable_id);
CREATE INDEX idx_taggables_lookup ON taggables(taggable_type, taggable_id);

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back
DROP TABLE taggables;
DROP TABLE tags;

-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied
--
-- The API layer only ever checked for an existing name-then-inserted
-- (two separate queries, no transaction), so concurrent requests could
-- race past the check and create multiple pages/templates/smtp
-- profiles/groups with the same name for the same user
-- (gophish/gophish#3278). This adds real DB-level uniqueness so that can
-- no longer happen, regardless of timing.
UPDATE pages SET name = CONCAT(name, ' (', id, ')')
WHERE id NOT IN (SELECT * FROM (SELECT MIN(id) FROM pages GROUP BY user_id, name) AS t);

UPDATE templates SET name = CONCAT(name, ' (', id, ')')
WHERE id NOT IN (SELECT * FROM (SELECT MIN(id) FROM templates GROUP BY user_id, name) AS t);

UPDATE smtp SET name = CONCAT(name, ' (', id, ')')
WHERE id NOT IN (SELECT * FROM (SELECT MIN(id) FROM smtp GROUP BY user_id, name) AS t);

UPDATE `groups` SET name = CONCAT(name, ' (', id, ')')
WHERE id NOT IN (SELECT * FROM (SELECT MIN(id) FROM `groups` GROUP BY user_id, name) AS t);

-- InnoDB row formats. 191 chars * 4 bytes = 764 bytes, safely under it.
CREATE UNIQUE INDEX idx_pages_user_id_name ON pages (user_id, name(191));
CREATE UNIQUE INDEX idx_templates_user_id_name ON templates (user_id, name(191));
CREATE UNIQUE INDEX idx_smtp_user_id_name ON smtp (user_id, name(191));
CREATE UNIQUE INDEX idx_groups_user_id_name ON `groups` (user_id, name(191));

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back
DROP INDEX idx_pages_user_id_name ON pages;
DROP INDEX idx_templates_user_id_name ON templates;
DROP INDEX idx_smtp_user_id_name ON smtp;
DROP INDEX idx_groups_user_id_name ON `groups`;

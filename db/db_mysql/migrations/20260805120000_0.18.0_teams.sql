-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied
--
-- Adds a Team concept so objects (campaigns, templates, pages, groups,
-- sending profiles, IMAP configs, results, mail logs) are visible to every
-- member of the owning team instead of being strictly siloed to the
-- creating user. See gophish/gophish#2026's sibling issue on team-based
-- RBAC and the design note in models/rbac.go.
--
-- Every existing user and every existing object is backfilled into a
-- single "Default Team" so nobody loses access on upgrade - everyone who
-- could already see each other's objects via admin impersonation now sees
-- them directly, and separate teams can be created afterwards for
-- isolating unrelated engagements.
CREATE TABLE IF NOT EXISTS `teams` (
    `id`            INTEGER PRIMARY KEY AUTO_INCREMENT,
    `name`          VARCHAR(255) NOT NULL UNIQUE,
    `modified_date` DATETIME
);

INSERT INTO `teams` (`name`, `modified_date`) VALUES ('Default Team', CURRENT_TIMESTAMP);

ALTER TABLE users ADD COLUMN team_id INTEGER;
ALTER TABLE campaigns ADD COLUMN team_id INTEGER;
ALTER TABLE templates ADD COLUMN team_id INTEGER;
ALTER TABLE pages ADD COLUMN team_id INTEGER;
ALTER TABLE `groups` ADD COLUMN team_id INTEGER;
ALTER TABLE smtp ADD COLUMN team_id INTEGER;
-- IMAP is intentionally excluded - it holds one person's connected mailbox
-- credentials/OAuth grant, not a shared campaign asset. See models/imap.go.
ALTER TABLE results ADD COLUMN team_id INTEGER;
ALTER TABLE mail_logs ADD COLUMN team_id INTEGER;
ALTER TABLE email_requests ADD COLUMN team_id INTEGER;

UPDATE users SET team_id = (SELECT id FROM `teams` WHERE name = 'Default Team');
UPDATE campaigns SET team_id = (SELECT id FROM `teams` WHERE name = 'Default Team');
UPDATE templates SET team_id = (SELECT id FROM `teams` WHERE name = 'Default Team');
UPDATE pages SET team_id = (SELECT id FROM `teams` WHERE name = 'Default Team');
UPDATE `groups` SET team_id = (SELECT id FROM `teams` WHERE name = 'Default Team');
UPDATE smtp SET team_id = (SELECT id FROM `teams` WHERE name = 'Default Team');
UPDATE results SET team_id = (SELECT id FROM `teams` WHERE name = 'Default Team');
UPDATE mail_logs SET team_id = (SELECT id FROM `teams` WHERE name = 'Default Team');
UPDATE email_requests SET team_id = (SELECT id FROM `teams` WHERE name = 'Default Team');

-- The (user_id, name) uniqueness added in 20260728190000_0.16.1_unique_names
-- no longer matches the actual visibility/namespace scope now that names
-- are shared team-wide: two different users on the same team could still
-- create same-named pages/templates/profiles/groups without the DB
-- rejecting it. Move the constraint to (team_id, name) to match.
--
-- Different users merged into the same Default Team may already have
-- reused a common name (e.g. two people both called a template "Password
-- Reset") - disambiguate those before the new unique index is created,
-- same approach as 20260728190000_0.16.1_unique_names used originally.
UPDATE pages SET name = CONCAT(name, ' (', id, ')')
WHERE id NOT IN (SELECT * FROM (SELECT MIN(id) FROM pages GROUP BY team_id, name) AS t);

UPDATE templates SET name = CONCAT(name, ' (', id, ')')
WHERE id NOT IN (SELECT * FROM (SELECT MIN(id) FROM templates GROUP BY team_id, name) AS t);

UPDATE smtp SET name = CONCAT(name, ' (', id, ')')
WHERE id NOT IN (SELECT * FROM (SELECT MIN(id) FROM smtp GROUP BY team_id, name) AS t);

UPDATE `groups` SET name = CONCAT(name, ' (', id, ')')
WHERE id NOT IN (SELECT * FROM (SELECT MIN(id) FROM `groups` GROUP BY team_id, name) AS t);

DROP INDEX idx_pages_user_id_name ON pages;
DROP INDEX idx_templates_user_id_name ON templates;
DROP INDEX idx_smtp_user_id_name ON smtp;
DROP INDEX idx_groups_user_id_name ON `groups`;
-- InnoDB row formats. 191 chars * 4 bytes = 764 bytes, safely under it.
CREATE UNIQUE INDEX idx_pages_team_id_name ON pages (team_id, name(191));
CREATE UNIQUE INDEX idx_templates_team_id_name ON templates (team_id, name(191));
CREATE UNIQUE INDEX idx_smtp_team_id_name ON smtp (team_id, name(191));
CREATE UNIQUE INDEX idx_groups_team_id_name ON `groups` (team_id, name(191));

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back
DROP TABLE `teams`;

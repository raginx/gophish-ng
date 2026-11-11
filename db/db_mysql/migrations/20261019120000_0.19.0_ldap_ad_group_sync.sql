-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied
--
-- LDAP / Active Directory target synchronization (Issue #78). See the SQLite
-- counterpart for the full description of the three tables:
-- ldap_connection (reusable team-scoped connection), ldap_group_sync (one
-- Gophish group <-> one selected directory group, identified by its stable
-- objectGUID) and ldap_target_identity (connection-scoped stable user
-- identity -> target).

CREATE TABLE IF NOT EXISTS `ldap_connection` (
    `id`                 INTEGER PRIMARY KEY AUTO_INCREMENT,
    `user_id`            BIGINT,
    `team_id`            BIGINT,
    `name`               VARCHAR(255) NOT NULL,
    `server_url`         VARCHAR(255) NOT NULL,
    `bind_dn`            VARCHAR(255),
    `bind_password_enc`  TEXT,
    `base_dn`            VARCHAR(255),
    `start_tls`          BOOLEAN NOT NULL DEFAULT 0,
    `ignore_cert_errors` BOOLEAN NOT NULL DEFAULT 0,
    `directory_id_attr`  VARCHAR(255),
    `last_test_status`   VARCHAR(255),
    `last_test_error`    TEXT,
    `modified_date`      DATETIME
);

CREATE TABLE IF NOT EXISTS `ldap_group_sync` (
    `id`                 INTEGER PRIMARY KEY AUTO_INCREMENT,
    `user_id`            BIGINT,
    `team_id`            BIGINT,
    `group_id`           BIGINT NOT NULL,
    `connection_id`      BIGINT NOT NULL,
    `enabled`            BOOLEAN NOT NULL DEFAULT 0,
    `group_directory_id` VARCHAR(255) NOT NULL DEFAULT '',
    `group_dn`           VARCHAR(1024),
    `group_name`         VARCHAR(255),
    `attr_email`         VARCHAR(255) NOT NULL DEFAULT 'mail',
    `attr_first_name`    VARCHAR(255) NOT NULL DEFAULT 'givenName',
    `attr_last_name`     VARCHAR(255) NOT NULL DEFAULT 'sn',
    `attr_position`      VARCHAR(255) NOT NULL DEFAULT 'title',
    `exclude_disabled`   BOOLEAN NOT NULL DEFAULT 1,
    `sync_freq`          BIGINT NOT NULL DEFAULT 3600,
    `last_sync`          DATETIME,
    `last_sync_status`   VARCHAR(255),
    `last_sync_error`    TEXT,
    `last_sync_added`    BIGINT NOT NULL DEFAULT 0,
    `last_sync_updated`  BIGINT NOT NULL DEFAULT 0,
    `last_sync_removed`  BIGINT NOT NULL DEFAULT 0,
    `last_sync_skipped`  BIGINT NOT NULL DEFAULT 0,
    `modified_date`      DATETIME
);

-- A group may only be managed by a single sync mapping.
CREATE UNIQUE INDEX idx_ldap_group_sync_group_id ON `ldap_group_sync` (group_id);
-- Several Gophish groups may mirror the same directory group: NOT unique.
CREATE INDEX idx_ldap_group_sync_conn_group ON `ldap_group_sync` (connection_id, group_directory_id);

CREATE TABLE IF NOT EXISTS `ldap_target_identity` (
    `id`            INTEGER PRIMARY KEY AUTO_INCREMENT,
    `team_id`       BIGINT,
    `connection_id` BIGINT NOT NULL,
    `directory_id`  VARCHAR(255) NOT NULL,
    `target_id`     BIGINT NOT NULL,
    `modified_date` DATETIME
);
-- One binding per directory object within a connection.
CREATE UNIQUE INDEX idx_ldap_target_identity_conn_dir ON `ldap_target_identity` (connection_id, directory_id);
CREATE INDEX idx_ldap_target_identity_target ON `ldap_target_identity` (target_id);

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back
DROP TABLE `ldap_target_identity`;
DROP TABLE `ldap_group_sync`;
DROP TABLE `ldap_connection`;

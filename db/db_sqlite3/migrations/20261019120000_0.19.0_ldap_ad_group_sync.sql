-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied
--
-- LDAP / Active Directory target synchronization (Issue #78).
--
-- Three tables:
--   * ldap_connection      - a reusable, team-scoped directory connection
--     (server, bind identity, encrypted credential, base DN, TLS options and
--     the stable per-object identity attribute, e.g. objectGUID). One
--     connection can serve many managed groups.
--   * ldap_group_sync      - a team-scoped mapping of one Gophish group to one
--     connection and ONE selected directory group. The selected group is
--     identified by its stable id (group_directory_id, the normalized AD
--     objectGUID); group_dn/group_name are last-known, server-observed
--     metadata. A group is "LDAP-managed" iff a mapping row references it.
--   * ldap_target_identity - connection-scoped mapping of a directory object's
--     stable identifier to a Gophish target, so the same logical user is
--     tracked across email/name/DN changes and group re-entry.
--
-- The groups/targets/group_targets tables are intentionally untouched so
-- historical campaign results are preserved.

CREATE TABLE ldap_connection (
    id integer primary key autoincrement,
    user_id bigint,
    team_id bigint,
    name varchar(255) NOT NULL,
    server_url varchar(255) NOT NULL,
    bind_dn varchar(255),
    -- Encrypted (AES-256-GCM, see the crypto package); base64 ciphertext is
    -- longer than the plaintext, hence text rather than varchar(255).
    bind_password_enc text,
    base_dn varchar(255),
    start_tls boolean NOT NULL DEFAULT 0,
    ignore_cert_errors boolean NOT NULL DEFAULT 0,
    -- Directory attribute holding a stable, immutable per-object identifier
    -- (objectGUID for Active Directory, entryUUID for generic LDAP). Empty
    -- means no stable identity (email-keyed fallback).
    directory_id_attr varchar(255),
    last_test_status varchar(255),
    last_test_error text,
    modified_date datetime
);

CREATE TABLE ldap_group_sync (
    id integer primary key autoincrement,
    user_id bigint,
    team_id bigint,
    group_id bigint NOT NULL,
    connection_id bigint NOT NULL,
    enabled boolean NOT NULL DEFAULT 0,
    -- Selected directory group: stable id (authoritative) + last-known DN/name.
    group_directory_id varchar(255) NOT NULL DEFAULT '',
    group_dn varchar(1024),
    group_name varchar(255),
    -- Recipient attribute mapping.
    attr_email varchar(255) NOT NULL DEFAULT 'mail',
    attr_first_name varchar(255) NOT NULL DEFAULT 'givenName',
    attr_last_name varchar(255) NOT NULL DEFAULT 'sn',
    attr_position varchar(255) NOT NULL DEFAULT 'title',
    exclude_disabled boolean NOT NULL DEFAULT 1,
    sync_freq bigint NOT NULL DEFAULT 3600,
    -- Last run: status and counts. last_sync_skipped counts direct members
    -- that could not be mapped to a target (empty/invalid email attribute).
    last_sync datetime,
    last_sync_status varchar(255),
    last_sync_error text,
    last_sync_added bigint NOT NULL DEFAULT 0,
    last_sync_updated bigint NOT NULL DEFAULT 0,
    last_sync_removed bigint NOT NULL DEFAULT 0,
    last_sync_skipped bigint NOT NULL DEFAULT 0,
    modified_date datetime
);

-- A group may only be managed by a single sync mapping.
CREATE UNIQUE INDEX idx_ldap_group_sync_group_id ON ldap_group_sync(group_id);
-- Several Gophish groups may mirror the same directory group: NOT unique.
CREATE INDEX idx_ldap_group_sync_conn_group ON ldap_group_sync(connection_id, group_directory_id);

CREATE TABLE ldap_target_identity (
    id integer primary key autoincrement,
    team_id bigint,
    connection_id bigint NOT NULL,
    directory_id varchar(255) NOT NULL,
    target_id bigint NOT NULL,
    modified_date datetime
);
-- One binding per directory object within a connection.
CREATE UNIQUE INDEX idx_ldap_target_identity_conn_dir ON ldap_target_identity(connection_id, directory_id);
CREATE INDEX idx_ldap_target_identity_target ON ldap_target_identity(target_id);

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back
DROP INDEX IF EXISTS idx_ldap_target_identity_target;
DROP INDEX IF EXISTS idx_ldap_target_identity_conn_dir;
DROP TABLE ldap_target_identity;
DROP INDEX IF EXISTS idx_ldap_group_sync_conn_group;
DROP INDEX IF EXISTS idx_ldap_group_sync_group_id;
DROP TABLE ldap_group_sync;
DROP TABLE ldap_connection;

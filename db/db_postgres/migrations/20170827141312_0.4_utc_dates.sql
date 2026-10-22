-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied
-- The SQLite/MySQL versions of this migration converted timestamps that
-- earlier releases had stored in local time into UTC. PostgreSQL support
-- starts at the current schema, so every timestamp is written in UTC from
-- the first run and there is no legacy data to convert. The version is kept
-- so the migration history stays aligned across database backends.
SELECT 1;

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back

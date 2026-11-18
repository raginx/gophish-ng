-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied
-- details holds JSON text (see models.Event); text rather than a binary type.
ALTER TABLE events ADD COLUMN details text;

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back
ALTER TABLE events DROP COLUMN details;

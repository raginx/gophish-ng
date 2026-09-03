-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied
ALTER TABLE smtp ADD COLUMN cc text;

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back

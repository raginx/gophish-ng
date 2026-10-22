-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied
ALTER TABLE campaigns ADD COLUMN send_by_date timestamp;

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back
ALTER TABLE campaigns DROP COLUMN send_by_date;

-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied
ALTER TABLE "campaigns" ADD COLUMN "launch_date" timestamp;
UPDATE "campaigns" SET "launch_date" = "created_date";

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back
ALTER TABLE "campaigns" DROP COLUMN "launch_date";

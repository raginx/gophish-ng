-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied
CREATE TABLE IF NOT EXISTS "webhooks" (
    "id" bigserial primary key,
    "name" varchar(255),
    "url" varchar(1000),
    "secret" varchar(255),
    "is_active" boolean default false
);

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back
DROP TABLE "webhooks";

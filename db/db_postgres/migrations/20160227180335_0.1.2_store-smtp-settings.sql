-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied
-- Move the relationship between campaigns and smtp to campaigns
ALTER TABLE campaigns ADD COLUMN "smtp_id" bigint;

-- Create a new table to store smtp records
DROP TABLE smtp;

CREATE TABLE smtp(
	id bigserial primary key,
	user_id bigint,
	interface_type varchar(255),
	name varchar(255),
	host varchar(255),
	username varchar(255),
	password varchar(255),
	from_address varchar(255),
	modified_date timestamp default CURRENT_TIMESTAMP,
	ignore_cert_errors boolean
);

-- +goose Down
-- SQL section 'Down' is executed when this migration is rolled back
DROP TABLE smtp;
ALTER TABLE campaigns DROP COLUMN "smtp_id";

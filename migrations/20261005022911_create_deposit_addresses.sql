-- +goose Up
-- Maps each deposit address to the user who owns it.
-- Addresses are stored lowercase so lookups don't depend on checksum casing.
CREATE TABLE deposit_addresses (
    address    TEXT PRIMARY KEY CHECK (address = lower(address)),
    user_id    BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX deposit_addresses_user_id_idx ON deposit_addresses (user_id);

-- +goose Down
DROP TABLE deposit_addresses;

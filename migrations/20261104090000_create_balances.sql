-- +goose Up
-- One row per user and token: how much the user has in the exchange.
CREATE TABLE balances (
    user_id    BIGINT NOT NULL,
    token      TEXT NOT NULL,
    amount     NUMERIC(78,0) NOT NULL DEFAULT 0 CHECK (amount >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),

    PRIMARY KEY (user_id, token)
);

-- +goose Down
DROP TABLE balances;

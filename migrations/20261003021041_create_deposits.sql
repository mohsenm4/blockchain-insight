-- +goose Up
CREATE TABLE deposits (
    id                BIGSERIAL PRIMARY KEY,
    user_id           BIGINT NOT NULL,
    address           TEXT NOT NULL,
    tx_hash           TEXT NOT NULL,
    log_index         INTEGER NOT NULL,
    token             TEXT NOT NULL,
    amount            NUMERIC(78,0) NOT NULL,
    block_number      BIGINT NOT NULL,
    block_hash        TEXT NOT NULL,
    status            TEXT NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending', 'confirmed', 'credited', 'reverted')),
    credited_at_block BIGINT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (tx_hash, log_index)
);

-- +goose Down
DROP TABLE deposits;

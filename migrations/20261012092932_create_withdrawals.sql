-- +goose Up
-- One row per withdrawal request. UNIQUE (user_id, idempotency_key) makes a
-- retried request find its first row instead of sending the money twice.
-- `to` is a reserved word, so the destination is to_address.
CREATE TABLE withdrawals (
    id              BIGSERIAL PRIMARY KEY,
    user_id         BIGINT NOT NULL,
    idempotency_key TEXT NOT NULL,
    to_address      TEXT NOT NULL CHECK (to_address = lower(to_address)),
    token           TEXT NOT NULL CHECK (token = lower(token)),
    amount          NUMERIC(78,0) NOT NULL CHECK (amount > 0),
    nonce           BIGINT,
    tx_hash         TEXT UNIQUE,
    status          TEXT NOT NULL DEFAULT 'queued'
                    CHECK (status IN ('queued', 'broadcast', 'confirmed', 'failed')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),

    UNIQUE (user_id, idempotency_key),
    -- once sent, a row always has its tx_hash
    CHECK (status NOT IN ('broadcast', 'confirmed') OR tx_hash IS NOT NULL)
);

-- +goose Down
DROP TABLE withdrawals;

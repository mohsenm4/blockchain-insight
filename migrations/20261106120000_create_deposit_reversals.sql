-- +goose Up
-- A credited deposit whose block was replaced before it was final. The
-- credited row is never edited: this row undoes it. UNIQUE (deposit_id)
-- makes a deposit reversed at most once.
-- status: 'reversed' — the amount was taken back from balances;
-- 'needs_review' — the balance was too low (the user already spent it), so
-- nothing was taken and a person has to handle it.
CREATE TABLE deposit_reversals (
    id         BIGSERIAL PRIMARY KEY,
    deposit_id BIGINT NOT NULL UNIQUE REFERENCES deposits (id),
    user_id    BIGINT NOT NULL,
    token      TEXT NOT NULL,
    amount     NUMERIC(78,0) NOT NULL CHECK (amount > 0),
    reason     TEXT NOT NULL,
    status     TEXT NOT NULL CHECK (status IN ('reversed', 'needs_review')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE deposit_reversals;

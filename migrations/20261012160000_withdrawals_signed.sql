-- +goose Up
-- A withdrawal is signed and saved before it is broadcast. Its hash is then
-- known even if the broadcast fails halfway, so the chain can be asked
-- whether the money left, and the same raw tx (same nonce) can be resent
-- without paying twice.
ALTER TABLE withdrawals ADD COLUMN raw_tx TEXT;

ALTER TABLE withdrawals DROP CONSTRAINT withdrawals_status_check;
ALTER TABLE withdrawals ADD CONSTRAINT withdrawals_status_check
    CHECK (status IN ('queued', 'signed', 'broadcast', 'confirmed', 'failed'));

ALTER TABLE withdrawals ADD CONSTRAINT withdrawals_signed_check
    CHECK (status <> 'signed' OR (tx_hash IS NOT NULL AND raw_tx IS NOT NULL AND nonce IS NOT NULL));

-- +goose Down
ALTER TABLE withdrawals DROP CONSTRAINT withdrawals_signed_check;
UPDATE withdrawals SET status = 'queued' WHERE status = 'signed';
ALTER TABLE withdrawals DROP CONSTRAINT withdrawals_status_check;
ALTER TABLE withdrawals ADD CONSTRAINT withdrawals_status_check
    CHECK (status IN ('queued', 'broadcast', 'confirmed', 'failed'));
ALTER TABLE withdrawals DROP COLUMN raw_tx;

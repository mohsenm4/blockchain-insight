# Ledger Schema — Design Only

## deposits

| column                 | type                           | why                                                           |
| ---------------------- | ------------------------------ | ------------------------------------------------------------- |
| id                     | BIGSERIAL PK                   | Internal identifier                                           |
| user_id                | BIGINT                         | Identifies the owner of the deposit                           |
| address                | TEXT                           | Identifies the deposit address                                |
| tx_hash                | TEXT                           | Identifies the blockchain transaction                         |
| log_index              | INTEGER                        | Identifies the specific Transfer event within the transaction |
| token                  | TEXT                           | Identifies the token                                          |
| amount                 | NUMERIC(78,0)                  | Stores large token amounts exactly                            |
| block_number           | BIGINT                         | Identifies the block containing the deposit                   |
| block_hash             | TEXT                           | Allows reorg detection                                        |
| status                 | pending / confirmed / reverted | Tracks the deposit lifecycle                                  |
| credited_at_block      | BIGINT                         | Records the block in which the deposit became credited        |
| created_at, updated_at | TIMESTAMP                      | Useful for auditing and detecting stuck records               |

**UNIQUE (tx_hash, log_index) — why:**
A transaction can contain multiple Transfer events. `tx_hash` alone is not enough, and `log_index` alone is not enough because the log index starts again for each transaction.

For reorgs, I would **keep the same deposit row and update its status**, rather than inserting a second row. This keeps the original event identity and audit history together and avoids conflicting records with the same `(tx_hash, log_index)`.

`confirmations` is **not stored**. It can be calculated as:

`current_chain_head - block_number`

The important permanent fact is `credited_at_block`: it records the block at which the deposit was actually credited.

## withdrawals

| column                 | type                                    | why                                                                  |
| ---------------------- | --------------------------------------- | -------------------------------------------------------------------- |
| id                     | BIGSERIAL PK                            | Internal identifier                                                  |
| user_id                | BIGINT                                  | Identifies the owner of the withdrawal                               |
| idempotency_key        | TEXT                                    | Prevents the same client request from being executed twice           |
| to                     | TEXT                                    | Destination address                                                  |
| token                  | TEXT                                    | Identifies the token                                                 |
| amount                 | NUMERIC(78,0)                           | Stores the withdrawal amount exactly                                 |
| nonce                  | BIGINT                                  | Identifies the transaction sequence for the sending wallet           |
| tx_hash                | TEXT                                    | Identifies the blockchain transaction after broadcast                |
| status                 | queued / broadcast / confirmed / failed | Tracks the withdrawal lifecycle                                      |
| created_at, updated_at | TIMESTAMP                               | Allows detection of stuck withdrawals and provides audit information |

**UNIQUE (user_id, idempotency_key) — why:**
The same idempotency key only needs to be unique for one user. Including `user_id` allows different users to use the same key without conflicting.

`idempotency_key` exists before `tx_hash`. It identifies the user's intent/request, while `tx_hash` identifies the actual blockchain transaction.

## Three questions

### 1. Same Transfer event delivered twice → one row? How:

Yes. Postgres enforces `UNIQUE (tx_hash, log_index)`. If two watchers process the same event, only one row can exist.

### 2. Reorg → which rows go back to pending / reverted? How do we detect it:

The watcher periodically checks deposits that are not finalized by comparing their stored `block_hash` with the current block hash for the same `block_number`.

If the hash changed, the deposit is marked `reverted`. If the transaction appears again on the new canonical chain, the existing row is updated back to `pending` and processed again.

### 3. Same withdrawal submitted twice → one on-chain tx? How:

The client sends the same `idempotency_key` when retrying. Postgres enforces `UNIQUE (user_id, idempotency_key)`, so the second request is treated as the same withdrawal and the server returns the original result instead of creating another transaction.

## State machines

### Deposit

`pending → confirmed → credited`

`pending / confirmed → reverted` when a reorg removes the deposit from the canonical chain.

If the transaction is included again:

`reverted → pending → confirmed → credited`

### Withdrawal

`queued → broadcast → confirmed`

If the transaction cannot be successfully broadcast:

`queued → failed`

If the broadcast transaction fails on-chain:

`broadcast → failed`

## Open questions

* Exact confirmation threshold for crediting deposits.
* Exact frequency and scope of reorg checks.
* Whether `credited_at_block` alone is sufficient for audit/reconciliation.

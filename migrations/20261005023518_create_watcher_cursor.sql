-- +goose Up
-- The last block each watcher has fully processed, so a restart resumes
-- where it stopped instead of skipping or rescanning blocks.
CREATE TABLE watcher_cursor (
    name       TEXT PRIMARY KEY,
    block      BIGINT NOT NULL CHECK (block >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE watcher_cursor;

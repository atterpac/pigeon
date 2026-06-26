-- +goose Up
-- Local compose drafts (autosaved client-side; independent of provider drafts).
-- +goose StatementBegin
CREATE TABLE drafts (
    id      TEXT NOT NULL,
    account TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    payload TEXT NOT NULL,        -- model.Outgoing as JSON
    updated INTEGER NOT NULL,     -- unix seconds
    PRIMARY KEY (account, id)
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_drafts_updated ON drafts(account, updated DESC);
-- +goose StatementEnd

-- Snoozed messages: hidden from inbox until `until`, then returned.
-- +goose StatementBegin
CREATE TABLE snoozes (
    account TEXT NOT NULL,
    message TEXT NOT NULL,
    until   INTEGER NOT NULL,     -- unix seconds
    created INTEGER NOT NULL,
    PRIMARY KEY (account, message)
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_snoozes_due ON snoozes(account, until);
-- +goose StatementEnd

-- Completion log for "Done today" metrics (Done == Archive).
-- +goose StatementBegin
CREATE TABLE done_log (
    account TEXT NOT NULL,
    message TEXT NOT NULL,
    at      INTEGER NOT NULL,     -- unix seconds
    PRIMARY KEY (account, message)
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_done_at ON done_log(account, at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS done_log;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS snoozes;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS drafts;
-- +goose StatementEnd

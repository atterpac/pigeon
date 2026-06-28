-- +goose Up
-- Address book harvested from message envelopes (From/To/Cc/Bcc) during sync,
-- ranked by frequency + recency to drive recipient autocomplete.
-- +goose StatementBegin
CREATE TABLE contacts (
    account   TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    addr      TEXT NOT NULL,            -- lowercased email address (key)
    name      TEXT NOT NULL DEFAULT '', -- most recent non-empty display name seen
    last_seen INTEGER NOT NULL,         -- unix seconds of the newest message it appeared in
    freq      INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (account, addr)
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_contacts_rank ON contacts(account, freq DESC, last_seen DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS contacts;
-- +goose StatementEnd

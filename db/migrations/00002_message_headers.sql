-- +goose Up
-- Stable RFC 5322 header metadata for reply/forward threading, independent of
-- the provider's own message id.
-- +goose StatementBegin
ALTER TABLE messages ADD COLUMN rfc_message_id TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE messages ADD COLUMN refs TEXT NOT NULL DEFAULT ''; -- space-separated Message-IDs (References chain)
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE messages DROP COLUMN refs;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE messages DROP COLUMN rfc_message_id;
-- +goose StatementEnd

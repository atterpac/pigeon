-- +goose Up
-- +goose StatementBegin
ALTER TABLE messages ADD COLUMN body_cached_at INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE messages ADD COLUMN last_opened_at INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE parts ADD COLUMN cached_at INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd
-- +goose StatementBegin
UPDATE messages
SET body_cached_at = CAST(strftime('%s', 'now') AS INTEGER)
WHERE body_loaded = 1 AND body_cached_at = 0;
-- +goose StatementEnd
-- +goose StatementBegin
UPDATE parts
SET cached_at = CAST(strftime('%s', 'now') AS INTEGER)
WHERE cached_at = 0;
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_messages_body_cache ON messages(account, body_loaded, body_cached_at);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_messages_last_opened ON messages(account, last_opened_at);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_parts_cached_at ON parts(account, cached_at);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_parts_cached_at;
-- +goose StatementEnd
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_messages_last_opened;
-- +goose StatementEnd
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_messages_body_cache;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE parts DROP COLUMN cached_at;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE messages DROP COLUMN last_opened_at;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE messages DROP COLUMN body_cached_at;
-- +goose StatementEnd

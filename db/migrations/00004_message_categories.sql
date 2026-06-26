-- +goose Up
-- +goose StatementBegin
ALTER TABLE messages ADD COLUMN category TEXT NOT NULL DEFAULT 'primary';
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_messages_category ON messages(account, category, date DESC);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS idx_messages_category;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE messages DROP COLUMN category;
-- +goose StatementEnd

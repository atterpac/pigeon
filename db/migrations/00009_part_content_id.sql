-- +goose Up
-- +goose StatementBegin
ALTER TABLE parts ADD COLUMN content_id TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE parts DROP COLUMN content_id;
-- +goose StatementEnd

-- +goose Up
-- +goose StatementBegin
ALTER TABLE sync_state ADD COLUMN backfill_done INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE sync_state DROP COLUMN backfill_done;
-- +goose StatementEnd

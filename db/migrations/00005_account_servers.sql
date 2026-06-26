-- +goose Up
-- +goose StatementBegin
ALTER TABLE accounts ADD COLUMN imap_host TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE accounts ADD COLUMN imap_port INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE accounts ADD COLUMN smtp_host TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE accounts ADD COLUMN smtp_port INTEGER NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE accounts DROP COLUMN imap_host;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE accounts DROP COLUMN imap_port;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE accounts DROP COLUMN smtp_host;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE accounts DROP COLUMN smtp_port;
-- +goose StatementEnd

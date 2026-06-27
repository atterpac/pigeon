-- +goose Up
-- +goose StatementBegin
ALTER TABLE mailboxes ADD COLUMN icon TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE mailboxes ADD COLUMN icon_weight TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE mailboxes ADD COLUMN icon_color TEXT NOT NULL DEFAULT '';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE mailboxes DROP COLUMN icon;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE mailboxes DROP COLUMN icon_weight;
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE mailboxes DROP COLUMN icon_color;
-- +goose StatementEnd

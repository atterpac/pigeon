-- +goose Up
-- +goose StatementBegin
CREATE TABLE accounts (
    id     TEXT PRIMARY KEY,
    kind   INTEGER NOT NULL,            -- 0 imap, 1 gmail
    email  TEXT NOT NULL,
    name   TEXT NOT NULL DEFAULT ''
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE mailboxes (
    id      TEXT NOT NULL,              -- LabelID / folder id
    account TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name    TEXT NOT NULL,
    path    TEXT NOT NULL DEFAULT '',   -- IMAP path; empty for Gmail
    role    INTEGER NOT NULL DEFAULT 0, -- model.Role
    unread  INTEGER NOT NULL DEFAULT 0,
    total   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (account, id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE threads (
    id      TEXT NOT NULL,
    account TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    subject TEXT NOT NULL DEFAULT '',
    last    INTEGER NOT NULL DEFAULT 0, -- unix seconds of newest message
    unread  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (account, id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE messages (
    id        TEXT NOT NULL,
    account   TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    thread    TEXT NOT NULL DEFAULT '',
    subject   TEXT NOT NULL DEFAULT '',
    from_json TEXT NOT NULL DEFAULT '[]', -- []model.Address as JSON
    to_json   TEXT NOT NULL DEFAULT '[]',
    cc_json   TEXT NOT NULL DEFAULT '[]',
    bcc_json  TEXT NOT NULL DEFAULT '[]',
    date      INTEGER NOT NULL DEFAULT 0, -- unix seconds
    snippet   TEXT NOT NULL DEFAULT '',
    flags     TEXT NOT NULL DEFAULT '',   -- space-separated flags
    has_attachments INTEGER NOT NULL DEFAULT 0,
    body_loaded     INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (account, id)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_messages_thread ON messages(account, thread);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_messages_date ON messages(account, date DESC);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE message_labels (
    account TEXT NOT NULL,
    message TEXT NOT NULL,
    label   TEXT NOT NULL,
    PRIMARY KEY (account, message, label)
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_message_labels_label ON message_labels(account, label);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE parts (
    account      TEXT NOT NULL,
    message      TEXT NOT NULL,
    seq          INTEGER NOT NULL,       -- ordering within the message
    content_type TEXT NOT NULL DEFAULT '',
    charset      TEXT NOT NULL DEFAULT '',
    disposition  TEXT NOT NULL DEFAULT '',
    filename     TEXT NOT NULL DEFAULT '',
    size         INTEGER NOT NULL DEFAULT 0,
    content      BLOB,                   -- inline/small bodies (may be compressed)
    blob_ref     TEXT NOT NULL DEFAULT '', -- spooled large attachments
    PRIMARY KEY (account, message, seq)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE sync_state (
    account  TEXT NOT NULL,
    mailbox  TEXT NOT NULL,
    cursor   BLOB,                  -- incremental (forward) sync position
    backfill BLOB,                  -- newest->oldest paging position
    PRIMARY KEY (account, mailbox)
);
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE op_log (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    account  TEXT NOT NULL,
    op       TEXT NOT NULL,             -- send|flags|labels|move|delete|draft
    payload  TEXT NOT NULL,            -- JSON args
    attempts INTEGER NOT NULL DEFAULT 0,
    next_at  INTEGER NOT NULL DEFAULT 0,
    created  INTEGER NOT NULL DEFAULT 0
);
-- +goose StatementEnd
-- +goose StatementBegin
CREATE INDEX idx_op_log_ready ON op_log(account, next_at);
-- +goose StatementEnd

-- +goose StatementBegin
-- Own-content FTS index keyed by the messages table's implicit rowid. The sync
-- engine keeps it in step: delete-by-rowid then insert on each upsert.
CREATE VIRTUAL TABLE messages_fts USING fts5(
    subject, sender, body,
    tokenize='unicode61'
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS messages_fts;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS op_log;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS sync_state;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS parts;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS message_labels;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS messages;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS threads;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS mailboxes;
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS accounts;
-- +goose StatementEnd

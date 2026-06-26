-- name: UpsertAccount :exec
INSERT INTO accounts (id, kind, email, name, imap_host, imap_port, smtp_host, smtp_port)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
    kind = excluded.kind,
    email = excluded.email,
    name = excluded.name,
    imap_host = excluded.imap_host,
    imap_port = excluded.imap_port,
    smtp_host = excluded.smtp_host,
    smtp_port = excluded.smtp_port;

-- name: GetAccount :one
SELECT id, kind, email, name, imap_host, imap_port, smtp_host, smtp_port FROM accounts WHERE id = ?;

-- name: ListAccounts :many
SELECT id, kind, email, name, imap_host, imap_port, smtp_host, smtp_port FROM accounts ORDER BY email;

-- name: DeleteAccount :exec
DELETE FROM accounts WHERE id = ?;

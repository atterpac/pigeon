-- name: UpsertMailbox :exec
INSERT INTO mailboxes (id, account, name, path, role, unread, total)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(account, id) DO UPDATE SET
    name = excluded.name,
    path = excluded.path,
    role = excluded.role,
    unread = excluded.unread,
    total = excluded.total;

-- name: ListMailboxes :many
SELECT id, account, name, path, role, unread, total
FROM mailboxes WHERE account = ? ORDER BY role, name;

-- name: DeleteMailbox :exec
DELETE FROM mailboxes WHERE account = ? AND id = ?;

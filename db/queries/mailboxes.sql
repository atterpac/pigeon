-- name: UpsertMailbox :exec
INSERT INTO mailboxes (id, account, name, path, role, unread, total)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(account, id) DO UPDATE SET
    name = excluded.name,
    path = excluded.path,
    role = excluded.role,
    unread = excluded.unread,
    total = excluded.total;

-- UpsertMailbox (above) deliberately omits the icon columns from its ON CONFLICT
-- update, so a user's chosen icon survives periodic topology sync.

-- name: ListMailboxes :many
SELECT id, account, name, path, role, unread, total, icon, icon_weight, icon_color
FROM mailboxes WHERE account = ? ORDER BY role, name;

-- name: SetMailboxIcon :exec
UPDATE mailboxes SET icon = ?, icon_weight = ?, icon_color = ?
WHERE account = ? AND id = ?;

-- name: DeleteMailbox :exec
DELETE FROM mailboxes WHERE account = ? AND id = ?;

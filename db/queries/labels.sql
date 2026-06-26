-- name: AddLabel :exec
INSERT INTO message_labels (account, message, label)
VALUES (?, ?, ?)
ON CONFLICT(account, message, label) DO NOTHING;

-- name: RemoveLabel :exec
DELETE FROM message_labels WHERE account = ? AND message = ? AND label = ?;

-- name: ListMessageLabels :many
SELECT label FROM message_labels WHERE account = ? AND message = ?;

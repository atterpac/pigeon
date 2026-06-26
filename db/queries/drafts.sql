-- name: UpsertDraft :exec
INSERT INTO drafts (id, account, payload, updated)
VALUES (?, ?, ?, ?)
ON CONFLICT(account, id) DO UPDATE SET
    payload = excluded.payload,
    updated = excluded.updated;

-- name: GetDraft :one
SELECT id, account, payload, updated FROM drafts WHERE account = ? AND id = ?;

-- name: ListDrafts :many
SELECT id, account, payload, updated FROM drafts WHERE account = ? ORDER BY updated DESC;

-- name: DeleteDraft :exec
DELETE FROM drafts WHERE account = ? AND id = ?;
